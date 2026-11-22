package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"pistream/internal/auth"
	"pistream/internal/config"
	"pistream/internal/db"
	"pistream/internal/stream"
)

func setupSecurityTestServer(t *testing.T) (http.Handler, *db.Store, *stream.HLSManager) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatalf("初始化測試資料庫失敗: %v", err)
	}
	t.Cleanup(func() { _ = store.Conn.Close() })

	if err := store.EnsureDefaultAdmin("admin", "AdminPass123!"); err != nil {
		t.Fatalf("建立測試管理員失敗: %v", err)
	}
	userHash, err := auth.HashPassword("UserPass123!")
	if err != nil {
		t.Fatalf("雜湊測試密碼失敗: %v", err)
	}
	if _, err := store.CreateUser("bob", userHash, false); err != nil {
		t.Fatalf("建立一般使用者失敗: %v", err)
	}
	if err := store.UpsertMediaFile(&db.MediaFile{
		Title:           "Sample 4K",
		FilePath:        "/secret/internal/path/sample.mkv",
		RelPath:         "Movies/sample.mkv",
		RelDir:          "Movies",
		FileSize:        1024 * 1024 * 500,
		Container:       "mkv",
		VideoCodec:      "hevc",
		AudioCodec:      "aac",
		Width:           3840,
		Height:          2160,
		DurationSeconds: 120,
	}); err != nil {
		t.Fatalf("建立測試媒體失敗: %v", err)
	}

	cfg := &config.Config{
		DBPath:           dbPath,
		MediaDir:         tmpDir,
		CacheDir:         filepath.Join(tmpDir, "cache"),
		TranscodeTempDir: filepath.Join(tmpDir, "hls"),
		WebDir:           filepath.Join(tmpDir, "web"),
		SegmentDuration:  4,
		MaxKeepSegments:  10,
		JWTSecret:        "super-secret-test-key-32-bytes-minimum",
	}
	hlsMgr := stream.NewHLSManager(cfg)
	return NewRouter(cfg, store, hlsMgr), store, hlsMgr
}

func loginAndGetCookie(t *testing.T, handler http.Handler, username, password string, https bool) (*http.Cookie, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req := httptest.NewRequest(http.MethodPost, "http://pistream.local:8080/api/auth/login", bytes.NewReader(body))
	req.Host = "pistream.local:8080"
	req.RemoteAddr = "192.168.1.50:54321"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://pistream.local:8080")
	if https {
		req.Header.Set("X-Forwarded-Proto", "https")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("登入 %s 失敗: status=%d body=%s", username, rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			return c, resp
		}
	}
	t.Fatalf("登入回應未設定 %s Cookie", auth.CookieName)
	return nil, resp
}

func TestSecurityCookieOnlyAndNoTokenLeak(t *testing.T) {
	handler, _, _ := setupSecurityTestServer(t)

	// 1. 登入不回傳明文 token，HTTPS 代理下設 Secure=true，HTTP 區網下 Secure=false
	httpCookie, resp := loginAndGetCookie(t, handler, "admin", "AdminPass123!", false)
	if _, hasToken := resp["token"]; hasToken {
		t.Fatalf("登入 JSON 不應回傳明文 token 欄位")
	}
	if !httpCookie.HttpOnly || httpCookie.Secure {
		t.Fatalf("HTTP 區網 Cookie 應為 HttpOnly=true, Secure=false，實際: %+v", httpCookie)
	}

	httpsCookie, _ := loginAndGetCookie(t, handler, "admin", "AdminPass123!", true)
	if !httpsCookie.HttpOnly || !httpsCookie.Secure {
		t.Fatalf("HTTPS Cookie 應為 HttpOnly=true, Secure=true，實際: %+v", httpsCookie)
	}

	// 2. ?token= 與 Authorization: Bearer 必須被拒絕 (401)
	req := httptest.NewRequest(http.MethodGet, "http://pistream.local:8080/api/library/files?token="+httpsCookie.Value, nil)
	req.Header.Set("Authorization", "Bearer "+httpsCookie.Value)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("Query/Bearer Token 必須被拒絕並回傳 401，實際: %d", rec.Code)
	}
}

func TestSecuritySessionRevocationOnLogoutPasswordResetAndDelete(t *testing.T) {
	handler, store, _ := setupSecurityTestServer(t)

	// 1. 登出後舊 Cookie 立刻失效
	bobCookie, _ := loginAndGetCookie(t, handler, "bob", "UserPass123!", false)
	logoutReq := httptest.NewRequest(http.MethodPost, "http://pistream.local:8080/api/auth/logout", nil)
	logoutReq.Host = "pistream.local:8080"
	logoutReq.Header.Set("Origin", "http://pistream.local:8080")
	logoutReq.AddCookie(bobCookie)
	logoutRec := httptest.NewRecorder()
	handler.ServeHTTP(logoutRec, logoutReq)
	if logoutRec.Code != http.StatusOK {
		t.Fatalf("登出應成功，實際: %d", logoutRec.Code)
	}

	replayReq := httptest.NewRequest(http.MethodGet, "http://pistream.local:8080/api/library/files", nil)
	replayReq.AddCookie(bobCookie)
	replayRec := httptest.NewRecorder()
	handler.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusUnauthorized {
		t.Fatalf("登出後重放舊 Cookie 必須回傳 401，實際: %d", replayRec.Code)
	}

	// 2. 管理員重設密碼後，舊 Cookie 必須立刻失效
	bobCookie2, _ := loginAndGetCookie(t, handler, "bob", "UserPass123!", false)
	adminCookie, _ := loginAndGetCookie(t, handler, "admin", "AdminPass123!", false)
	bobUser, _ := store.GetUserByUsername("bob")

	resetBody := strings.NewReader(`{"password":"NewBobPassword!"}`)
	resetReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("http://pistream.local:8080/api/admin/users/%d/password", bobUser.ID), resetBody)
	resetReq.Host = "pistream.local:8080"
	resetReq.Header.Set("Origin", "http://pistream.local:8080")
	resetReq.AddCookie(adminCookie)
	resetRec := httptest.NewRecorder()
	handler.ServeHTTP(resetRec, resetReq)
	if resetRec.Code != http.StatusOK {
		t.Fatalf("管理員重設密碼應成功，實際: %d %s", resetRec.Code, resetRec.Body.String())
	}

	checkReq := httptest.NewRequest(http.MethodGet, "http://pistream.local:8080/api/library/files", nil)
	checkReq.AddCookie(bobCookie2)
	checkRec := httptest.NewRecorder()
	handler.ServeHTTP(checkRec, checkReq)
	if checkRec.Code != http.StatusUnauthorized {
		t.Fatalf("密碼重設後舊 Cookie 必須回傳 401，實際: %d", checkRec.Code)
	}

	// 3. 刪除帳號後，舊 Cookie 必須立刻失效
	bobCookie3, _ := loginAndGetCookie(t, handler, "bob", "NewBobPassword!", false)
	delReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("http://pistream.local:8080/api/admin/users/%d", bobUser.ID), nil)
	delReq.Host = "pistream.local:8080"
	delReq.Header.Set("Origin", "http://pistream.local:8080")
	delReq.AddCookie(adminCookie)
	delRec := httptest.NewRecorder()
	handler.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Fatalf("刪除帳號應成功，實際: %d", delRec.Code)
	}

	afterDelReq := httptest.NewRequest(http.MethodGet, "http://pistream.local:8080/api/library/files", nil)
	afterDelReq.AddCookie(bobCookie3)
	afterDelRec := httptest.NewRecorder()
	handler.ServeHTTP(afterDelRec, afterDelReq)
	if afterDelRec.Code != http.StatusUnauthorized {
		t.Fatalf("刪除帳號後舊 Cookie 必須回傳 401，實際: %d", afterDelRec.Code)
	}
}

func TestSecurityLoginRateLimitingAndBodyLimit(t *testing.T) {
	handler, _, _ := setupSecurityTestServer(t)

	// 超過 64KB 的惡意 Body 應直接拒絕
	hugeBody := fmt.Sprintf(`{"username":"admin","password":"%s"}`, strings.Repeat("x", 70*1024))
	hugeReq := httptest.NewRequest(http.MethodPost, "http://pistream.local:8080/api/auth/login", strings.NewReader(hugeBody))
	hugeReq.Host = "pistream.local:8080"
	hugeReq.RemoteAddr = "10.0.0.9:1234"
	hugeRec := httptest.NewRecorder()
	handler.ServeHTTP(hugeRec, hugeReq)
	if hugeRec.Code != http.StatusBadRequest {
		t.Fatalf("超大 JSON Body 應回傳 400，實際: %d", hugeRec.Code)
	}

	// 連續錯誤登入 10 次後，第 11 次應觸發 429
	for i := 0; i < loginMaxFailures; i++ {
		req := httptest.NewRequest(http.MethodPost, "http://pistream.local:8080/api/auth/login", strings.NewReader(`{"username":"bob","password":"wrong"}`))
		req.Host = "pistream.local:8080"
		req.RemoteAddr = "10.0.0.99:5555"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("第 %d 次錯誤登入預期 401，實際: %d", i+1, rec.Code)
		}
	}

	blockedReq := httptest.NewRequest(http.MethodPost, "http://pistream.local:8080/api/auth/login", strings.NewReader(`{"username":"bob","password":"UserPass123!"}`))
	blockedReq.Host = "pistream.local:8080"
	blockedReq.RemoteAddr = "10.0.0.99:5555"
	blockedRec := httptest.NewRecorder()
	handler.ServeHTTP(blockedRec, blockedReq)
	if blockedRec.Code != http.StatusTooManyRequests {
		t.Fatalf("超過限流門檻應回傳 429，實際: %d", blockedRec.Code)
	}
}

func TestSecurityCSRFHeadersTraversalAndInfoLeak(t *testing.T) {
	handler, _, _ := setupSecurityTestServer(t)
	bobCookie, _ := loginAndGetCookie(t, handler, "bob", "UserPass123!", false)
	adminCookie, _ := loginAndGetCookie(t, handler, "admin", "AdminPass123!", false)

	// 1. 跨站 CSRF Origin 必須回傳 403
	csrfReq := httptest.NewRequest(http.MethodPost, "http://pistream.local:8080/api/stream/stop?session=abc", nil)
	csrfReq.Host = "pistream.local:8080"
	csrfReq.Header.Set("Origin", "https://evil.example")
	csrfReq.AddCookie(bobCookie)
	csrfRec := httptest.NewRecorder()
	handler.ServeHTTP(csrfRec, csrfReq)
	if csrfRec.Code != http.StatusForbidden {
		t.Fatalf("跨站 Origin POST 應回傳 403，實際: %d", csrfRec.Code)
	}

	// 2. HA Ingress / 反向代理透過 X-Forwarded-Host 匹配 Origin 應允許通過
	ingressReq := httptest.NewRequest(http.MethodPost, "http://172.30.32.1:8080/api/stream/stop?session=valid_123", nil)
	ingressReq.Host = "172.30.32.1:8080"
	ingressReq.Header.Set("X-Forwarded-Host", "ha.myhome.example")
	ingressReq.Header.Set("Origin", "https://ha.myhome.example")
	ingressReq.Header.Set("Sec-Fetch-Site", "same-origin")
	ingressReq.AddCookie(bobCookie)
	ingressRec := httptest.NewRecorder()
	handler.ServeHTTP(ingressRec, ingressReq)
	if ingressRec.Code != http.StatusOK {
		t.Fatalf("同源 HA Ingress POST 應允許通過，實際: %d", ingressRec.Code)
	}

	// 3. 安全標頭檢查（含相容 HA Ingress 的 frame-ancestors 'self'）
	if ingressRec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("缺少 X-Content-Type-Options: nosniff")
	}
	if ingressRec.Header().Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Fatalf("預期 X-Frame-Options: SAMEORIGIN，實際: %q", ingressRec.Header().Get("X-Frame-Options"))
	}
	if !strings.Contains(ingressRec.Header().Get("Content-Security-Policy"), "frame-ancestors 'self'") {
		t.Fatalf("CSP 應包含 frame-ancestors 'self'")
	}

	// 4. 路徑遍歷 sessionID 與 M3U8 注入必須回傳 400
	travReq := httptest.NewRequest(http.MethodGet, "http://pistream.local:8080/api/stream/1/hls/seg_00000.ts?session=/../../../../config", nil)
	travReq.AddCookie(bobCookie)
	travRec := httptest.NewRecorder()
	handler.ServeHTTP(travRec, travReq)
	if travRec.Code != http.StatusBadRequest {
		t.Fatalf("路徑遍歷 sessionID 應回傳 400，實際: %d", travRec.Code)
	}

	m3u8InjReq := httptest.NewRequest(http.MethodGet, "http://pistream.local:8080/api/stream/1/hls/index.m3u8?session=ok&mode=evil", nil)
	m3u8InjReq.AddCookie(bobCookie)
	m3u8InjRec := httptest.NewRecorder()
	handler.ServeHTTP(m3u8InjRec, m3u8InjReq)
	if m3u8InjRec.Code != http.StatusBadRequest {
		t.Fatalf("非法 mode 應回傳 400，實際: %d", m3u8InjRec.Code)
	}

	// 5. 一般使用者呼叫 POST /api/library/scan 應回傳 403；管理員才可呼叫
	scanBobReq := httptest.NewRequest(http.MethodPost, "http://pistream.local:8080/api/library/scan", nil)
	scanBobReq.Host = "pistream.local:8080"
	scanBobReq.Header.Set("Origin", "http://pistream.local:8080")
	scanBobReq.AddCookie(bobCookie)
	scanBobRec := httptest.NewRecorder()
	handler.ServeHTTP(scanBobRec, scanBobReq)
	if scanBobRec.Code != http.StatusForbidden {
		t.Fatalf("非管理員呼叫 /api/library/scan 應回傳 403，實際: %d", scanBobRec.Code)
	}

	// 6. API 回應不得外露內部伺服器 file_path
	filesReq := httptest.NewRequest(http.MethodGet, "http://pistream.local:8080/api/library/files", nil)
	filesReq.AddCookie(adminCookie)
	filesRec := httptest.NewRecorder()
	handler.ServeHTTP(filesRec, filesReq)
	if filesRec.Code != http.StatusOK {
		t.Fatalf("讀取影片清單失敗: %d", filesRec.Code)
	}
	if strings.Contains(filesRec.Body.String(), "/secret/internal/path") || strings.Contains(filesRec.Body.String(), `"file_path"`) {
		t.Fatalf("API 回應不應外露伺服器內部 file_path: %s", filesRec.Body.String())
	}

	// 7. 字幕回應不應包含 Access-Control-Allow-Origin: *
	subReq := httptest.NewRequest(http.MethodGet, "http://pistream.local:8080/api/stream/1/subtitles/emb_0", nil)
	subReq.AddCookie(adminCookie)
	subRec := httptest.NewRecorder()
	handler.ServeHTTP(subRec, subReq)
	if subRec.Header().Get("Access-Control-Allow-Origin") == "*" {
		t.Fatalf("字幕端點不應設定 Access-Control-Allow-Origin: *")
	}

	// 8. /api/stream/{id}/info 應回傳有效前瞻緩衝秒數 (預設 45s)
	infoReq := httptest.NewRequest(http.MethodGet, "http://pistream.local:8080/api/stream/1/info?session=sess_buf_check", nil)
	infoReq.AddCookie(adminCookie)
	infoRec := httptest.NewRecorder()
	handler.ServeHTTP(infoRec, infoReq)
	if infoRec.Code != http.StatusOK {
		t.Fatalf("讀取串流資訊失敗: %d %s", infoRec.Code, infoRec.Body.String())
	}
	var infoResp map[string]any
	if err := json.Unmarshal(infoRec.Body.Bytes(), &infoResp); err != nil {
		t.Fatalf("解析串流資訊 JSON 失敗: %v", err)
	}
	if bufSec, ok := infoResp["buffer_seconds"].(float64); !ok || int(bufSec) != config.DefaultBufferSeconds {
		t.Fatalf("預期 buffer_seconds=%d，實際: %v", config.DefaultBufferSeconds, infoResp["buffer_seconds"])
	}

	// 9. /api/stream/{id}/info 的 force_remux 與 force_transcode 參數應正確生效
	remuxReq := httptest.NewRequest(http.MethodGet, "http://pistream.local:8080/api/stream/1/info?session=sess_fr&hevc=1&hevc_hls=1&force_remux=1", nil)
	remuxReq.AddCookie(adminCookie)
	remuxRec := httptest.NewRecorder()
	handler.ServeHTTP(remuxRec, remuxReq)
	var remuxResp struct {
		Decision stream.DecisionResult `json:"decision"`
	}
	if err := json.Unmarshal(remuxRec.Body.Bytes(), &remuxResp); err != nil || remuxResp.Decision.Mode != "remux" {
		t.Fatalf("預期 force_remux=1 回傳 remux，實際: %+v err=%v", remuxResp.Decision, err)
	}

	transReq := httptest.NewRequest(http.MethodGet, "http://pistream.local:8080/api/stream/1/info?session=sess_ft&quality=1080p&hevc=1&hevc_hls=1&force_transcode=1", nil)
	transReq.AddCookie(adminCookie)
	transRec := httptest.NewRecorder()
	handler.ServeHTTP(transRec, transReq)
	var transResp struct {
		Decision stream.DecisionResult `json:"decision"`
	}
	if err := json.Unmarshal(transRec.Body.Bytes(), &transResp); err != nil || transResp.Decision.Mode != "transcode" || transResp.Decision.Quality != "1080p" {
		t.Fatalf("預期 force_transcode=1 回傳 1080p transcode，實際: %+v err=%v", transResp.Decision, err)
	}
}
