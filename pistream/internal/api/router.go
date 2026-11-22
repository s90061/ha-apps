package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"pistream/internal/auth"
	"pistream/internal/config"
	"pistream/internal/db"
	"pistream/internal/scanner"
	"pistream/internal/stream"
)

func NewRouter(cfg *config.Config, store *db.Store, hlsMgr *stream.HLSManager) http.Handler {
	mux := http.NewServeMux()
	loginLimiter := newLoginRateLimiter(loginWindow, loginMaxFailures)
	var scanRunning atomic.Bool

	setAuthCookie := func(w http.ResponseWriter, r *http.Request, value string, maxAge int) {
		http.SetCookie(w, &http.Cookie{
			Name:     auth.CookieName,
			Value:    value,
			Path:     "/",
			HttpOnly: true,
			Secure:   isHTTPSRequest(r),
			SameSite: http.SameSiteLaxMode,
			MaxAge:   maxAge,
		})
	}

	requireAuth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			token := auth.ExtractTokenFromRequest(r)
			if token == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "請先登入系統"})
				return
			}
			payload, err := auth.VerifyToken(cfg.JWTSecret, token)
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
				return
			}
			// 每次請求皆即時核對 SQLite，確保已刪除帳號、重設密碼或登出的舊憑證立即失效
			dbUser, err := store.GetUserByID(payload.UserID)
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "帳號不存在或已被移除"})
				return
			}
			if payload.TokenVersion <= 0 || payload.TokenVersion != dbUser.TokenVersion {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "登入憑證已失效，請重新登入"})
				return
			}
			payload.Username = dbUser.Username
			payload.IsAdmin = dbUser.IsAdmin
			ctx := auth.WithUser(r.Context(), payload)
			next(w, r.WithContext(ctx))
		}
	}

	requireAdmin := func(next http.HandlerFunc) http.HandlerFunc {
		return requireAuth(func(w http.ResponseWriter, r *http.Request) {
			u := auth.GetUser(r)
			if u == nil || !u.IsAdmin {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "需要管理員權限"})
				return
			}
			next(w, r)
		})
	}

	// 1. 健康檢查 (Docker HEALTHCHECK 使用，免登入)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"service": "pistream",
		})
	})

	// 2. 身份驗證 API (純 HttpOnly Cookie 驗證，停用明文 Token 回傳)
	mux.HandleFunc("POST /api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "請求格式錯誤"})
			return
		}
		username := strings.TrimSpace(body.Username)
		ip := clientIP(r)
		if allowed, retryAfter := loginLimiter.Allow(ip, username); !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "登入嘗試次數過多，請稍後再試"})
			return
		}

		if len(body.Password) > auth.MaxPasswordBytes {
			loginLimiter.RecordFailure(ip, username)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "帳號或密碼錯誤"})
			return
		}

		user, err := store.GetUserByUsername(username)
		if err != nil || !auth.VerifyPassword(body.Password, user.PasswordHash) {
			loginLimiter.RecordFailure(ip, username)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "帳號或密碼錯誤"})
			return
		}
		loginLimiter.RecordSuccess(ip, username)

		cookieVal, err := auth.SignToken(cfg.JWTSecret, user.ID, user.Username, user.IsAdmin, user.TokenVersion, 30*24*time.Hour)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "無法簽發登入憑證"})
			return
		}

		setAuthCookie(w, r, cookieVal, 30*24*3600)

		writeJSON(w, http.StatusOK, map[string]any{
			"user": user,
		})
	})

	mux.HandleFunc("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if token := auth.ExtractTokenFromRequest(r); token != "" {
			if payload, err := auth.VerifyToken(cfg.JWTSecret, token); err == nil {
				_ = store.BumpTokenVersion(payload.UserID)
			}
		}
		setAuthCookie(w, r, "", -1)
		writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
	})

	mux.HandleFunc("GET /api/auth/me", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		u := auth.GetUser(r)
		dbUser, err := store.GetUserByID(u.UserID)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "帳號不存在"})
			return
		}
		writeJSON(w, http.StatusOK, dbUser)
	}))

	mux.HandleFunc("PUT /api/auth/password", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		u := auth.GetUser(r)
		var body struct {
			CurrentPassword string `json:"current_password"`
			NewPassword     string `json:"new_password"`
		}
		if err := decodeJSON(w, r, &body); err != nil || len(body.NewPassword) < 4 || len(body.NewPassword) > auth.MaxPasswordBytes {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "新密碼長度需介於 4 至 128 個字元"})
			return
		}
		dbUser, err := store.GetUserByID(u.UserID)
		if err != nil || !auth.VerifyPassword(body.CurrentPassword, dbUser.PasswordHash) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "目前密碼驗證錯誤"})
			return
		}
		hash, err := auth.HashPassword(body.NewPassword)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "密碼加密失敗"})
			return
		}
		if err := store.UpdateUserPassword(u.UserID, hash); err != nil {
			fmt.Printf("[API] 更新個人密碼失敗 (uid=%d): %v\n", u.UserID, err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "更新密碼失敗"})
			return
		}
		// 密碼更新會遞增 token_version 吊銷其他裝置，同時為當前裝置無縫補發新 Cookie
		if updatedUser, err := store.GetUserByID(u.UserID); err == nil {
			if newCookie, err := auth.SignToken(cfg.JWTSecret, updatedUser.ID, updatedUser.Username, updatedUser.IsAdmin, updatedUser.TokenVersion, 30*24*time.Hour); err == nil {
				setAuthCookie(w, r, newCookie, 30*24*3600)
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	}))

	// 3. 管理員專用：多使用者管理 API (關閉公開註冊，僅由管理員新增/管理)
	mux.HandleFunc("GET /api/admin/users", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		users, err := store.ListUsers()
		if err != nil {
			fmt.Printf("[API] 讀取使用者清單失敗: %v\n", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "無法讀取使用者清單"})
			return
		}
		writeJSON(w, http.StatusOK, users)
	}))

	mux.HandleFunc("POST /api/admin/users", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
			IsAdmin  bool   `json:"is_admin"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "請求格式錯誤"})
			return
		}
		username := strings.TrimSpace(body.Username)
		if username == "" || len(username) > 64 || len(body.Password) < 4 || len(body.Password) > auth.MaxPasswordBytes {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "帳號不可為空且密碼需介於 4 至 128 個字元"})
			return
		}
		hash, err := auth.HashPassword(body.Password)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "密碼雜湊失敗"})
			return
		}
		created, err := store.CreateUser(username, hash, body.IsAdmin)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "帳號名稱已存在"})
			return
		}
		writeJSON(w, http.StatusCreated, created)
	}))

	mux.HandleFunc("PUT /api/admin/users/{id}/password", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		targetID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "無效的使用者 ID"})
			return
		}
		var body struct {
			Password string `json:"password"`
		}
		if err := decodeJSON(w, r, &body); err != nil || len(body.Password) < 4 || len(body.Password) > auth.MaxPasswordBytes {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "密碼長度需介於 4 至 128 個字元"})
			return
		}
		hash, err := auth.HashPassword(body.Password)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "密碼雜湊失敗"})
			return
		}
		if err := store.UpdateUserPassword(targetID, hash); err != nil {
			fmt.Printf("[API] 管理員重設密碼失敗 (target=%d): %v\n", targetID, err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "重設密碼失敗"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "password_reset"})
	}))

	mux.HandleFunc("DELETE /api/admin/users/{id}", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		u := auth.GetUser(r)
		targetID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "無效的使用者 ID"})
			return
		}
		if targetID == u.UserID {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "不可刪除當前登入的管理員帳號"})
			return
		}
		if err := store.DeleteUser(targetID); err != nil {
			fmt.Printf("[API] 刪除使用者失敗 (target=%d): %v\n", targetID, err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "刪除使用者失敗"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}))

	// 4. 媒體庫瀏覽 API (資料夾階層模式 + 全部影片模式 + 個人繼續觀看)
	mux.HandleFunc("GET /api/library/folders", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		u := auth.GetUser(r)
		relPath := r.URL.Query().Get("path")
		sortBy := r.URL.Query().Get("sort")
		res, err := store.BrowseFolderForUser(u.UserID, relPath, sortBy)
		if err != nil {
			fmt.Printf("[API] 瀏覽資料夾失敗: %v\n", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "無法讀取資料夾清單"})
			return
		}
		writeJSON(w, http.StatusOK, res)
	}))

	mux.HandleFunc("GET /api/library/files", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		u := auth.GetUser(r)
		q := r.URL.Query().Get("q")
		sortBy := r.URL.Query().Get("sort")
		files, err := store.ListMediaFilesForUser(u.UserID, q, sortBy)
		if err != nil {
			fmt.Printf("[API] 讀取影片清單失敗: %v\n", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "無法讀取影片清單"})
			return
		}
		writeJSON(w, http.StatusOK, files)
	}))

	mux.HandleFunc("GET /api/library/continue", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		u := auth.GetUser(r)
		items, err := store.GetContinueWatchingForUser(u.UserID)
		if err != nil {
			fmt.Printf("[API] 讀取繼續觀看失敗: %v\n", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "無法讀取繼續觀看清單"})
			return
		}
		writeJSON(w, http.StatusOK, items)
	}))

	mux.HandleFunc("POST /api/library/scan", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		if !scanRunning.CompareAndSwap(false, true) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "already_scanning"})
			return
		}
		go func() {
			defer scanRunning.Store(false)
			count, err := scanner.ScanDirectory(cfg.MediaDir, cfg.CacheDir, store)
			if err == nil {
				fmt.Printf("[Scanner] 手動掃描完成，目前共索引 %d 部影片\n", count)
			}
		}()
		writeJSON(w, http.StatusOK, map[string]string{"status": "scanning_started"})
	}))

	// 5. 影片 16:9 預覽縮圖服務 (若縮圖尚未完成則回傳高質感漸層 SVG 佔位圖)
	mux.HandleFunc("GET /api/thumbs/{id}", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		fileID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		thumbPath := scanner.GetThumbPath(cfg.CacheDir, fileID)
		if info, err := os.Stat(thumbPath); err == nil && info.Size() > 500 {
			w.Header().Set("Cache-Control", "public, max-age=86400")
			http.ServeFile(w, r, thumbPath)
			return
		}

		// 若尚未產生縮圖，排入佇列並先回傳消光石墨灰 SVG 佔位圖
		media, err := store.GetMediaFile(fileID)
		if err == nil {
			scanner.QueueThumbnailJob(media.ID, media.FilePath, media.DurationSeconds, cfg.CacheDir, store)
		}
		w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 360">
			<rect width="640" height="360" fill="#19191b"/>
			<rect x="276" y="136" width="88" height="88" rx="16" fill="#232325" stroke="#ededed" stroke-opacity="0.1" stroke-width="1.5"/>
			<polygon points="310,162 310,198 340,180" fill="#85858b"/>
		</svg>`
		_, _ = w.Write([]byte(svg))
	}))

	// 6. 智慧三級串流與中間時段秒開 API
	mux.HandleFunc("GET /api/stream/{id}/info", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		u := auth.GetUser(r)
		fileID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "無效的影片 ID"})
			return
		}
		media, err := store.GetMediaFile(fileID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "找不到該部影片"})
			return
		}

		sessionID := strings.TrimSpace(r.URL.Query().Get("session"))
		if sessionID == "" {
			sessionID = fmt.Sprintf("u%d_%d", u.UserID, time.Now().UnixNano())
		} else if !stream.ValidateSessionID(sessionID) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "無效的 Session ID"})
			return
		}
		quality := r.URL.Query().Get("quality")
		if quality == "" {
			quality = "original"
		} else if !stream.ValidateQuality(quality) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "無效的畫質參數"})
			return
		}
		audioTrack, _ := strconv.Atoi(r.URL.Query().Get("audio"))
		if audioTrack < 0 {
			audioTrack = 0
		}
		allowHEVCDirect := r.URL.Query().Get("hevc") == "1" || r.URL.Query().Get("hevc") == "true"
		allowHEVCHLS := allowHEVCDirect
		if rawHLS := r.URL.Query().Get("hevc_hls"); rawHLS != "" {
			allowHEVCHLS = rawHLS == "1" || rawHLS == "true"
		}
		allowEAC3 := r.URL.Query().Get("eac3") == "1" || r.URL.Query().Get("eac3") == "true"

		decision := stream.DecideStreamModeWithAllCaps(media, sessionID, quality, audioTrack, allowHEVCDirect, allowHEVCHLS, allowEAC3)
		if decision.Mode == "direct" {
			// 若由 HLS 轉碼/換殼切回原生直連 (direct)，立刻終止背景 FFmpeg 行程釋放 CPU
			hlsMgr.StopSession(u.UserID, sessionID, false)
		}
		prog, _ := store.GetProgress(u.UserID, fileID)

		writeJSON(w, http.StatusOK, map[string]any{
			"media":          media,
			"decision":       decision,
			"progress":       prog,
			"buffer_seconds": cfg.EffectiveBufferSeconds(),
		})
	}))

	mux.HandleFunc("POST /api/stream/stop", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		u := auth.GetUser(r)
		sessionID := strings.TrimSpace(r.URL.Query().Get("session"))
		if sessionID == "" && r.Body != nil {
			var body struct {
				Session string `json:"session"`
			}
			if err := decodeJSON(w, r, &body); err == nil {
				sessionID = strings.TrimSpace(body.Session)
			}
		}
		if sessionID != "" && !stream.ValidateSessionID(sessionID) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "無效的 Session ID"})
			return
		}
		stopped := false
		if sessionID != "" {
			stopped = hlsMgr.StopSession(u.UserID, sessionID, true)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "stopped",
			"stopped": stopped,
		})
	}))

	mux.HandleFunc("GET /api/stream/{id}/direct", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		fileID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "無效的影片 ID", http.StatusBadRequest)
			return
		}
		media, err := store.GetMediaFile(fileID)
		if err != nil {
			http.Error(w, "找不到影片", http.StatusNotFound)
			return
		}
		stream.ServeDirectStream(w, r, media.FilePath)
	}))

	mux.HandleFunc("GET /api/stream/{id}/hls/index.m3u8", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		fileID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "無效的影片 ID", http.StatusBadRequest)
			return
		}
		media, err := store.GetMediaFile(fileID)
		if err != nil {
			http.Error(w, "找不到影片", http.StatusNotFound)
			return
		}
		sessionID := strings.TrimSpace(r.URL.Query().Get("session"))
		if sessionID == "" {
			sessionID = "default"
		}
		if !stream.ValidateSessionID(sessionID) {
			http.Error(w, "無效的 Session ID", http.StatusBadRequest)
			return
		}
		mode := r.URL.Query().Get("mode")
		if mode == "" {
			mode = "transcode"
		}
		if !stream.ValidateMode(mode) {
			http.Error(w, "無效的串流模式", http.StatusBadRequest)
			return
		}
		quality := r.URL.Query().Get("quality")
		if quality == "" {
			quality = "original"
		}
		if !stream.ValidateQuality(quality) {
			http.Error(w, "無效的畫質參數", http.StatusBadRequest)
			return
		}
		audioTrack, _ := strconv.Atoi(r.URL.Query().Get("audio"))
		if audioTrack < 0 {
			audioTrack = 0
		}

		playlist := hlsMgr.GenerateVirtualPlaylist(media, sessionID, mode, quality, audioTrack)
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		_, _ = w.Write([]byte(playlist))
	}))

	mux.HandleFunc("GET /api/stream/{id}/hls/{seg}", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		u := auth.GetUser(r)
		fileID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "無效的影片 ID", http.StatusBadRequest)
			return
		}
		segName := r.PathValue("seg")
		if !strings.HasSuffix(segName, ".ts") {
			http.NotFound(w, r)
			return
		}
		segIdx, err := stream.ParseSegmentIndex(segName)
		if err != nil {
			http.Error(w, "無效的切片編號", http.StatusBadRequest)
			return
		}

		media, err := store.GetMediaFile(fileID)
		if err != nil {
			http.Error(w, "找不到影片", http.StatusNotFound)
			return
		}

		sessionID := strings.TrimSpace(r.URL.Query().Get("session"))
		if sessionID == "" {
			sessionID = "default"
		}
		if !stream.ValidateSessionID(sessionID) {
			http.Error(w, "無效的 Session ID", http.StatusBadRequest)
			return
		}
		mode := r.URL.Query().Get("mode")
		if mode == "" {
			mode = "transcode"
		}
		if !stream.ValidateMode(mode) {
			http.Error(w, "無效的串流模式", http.StatusBadRequest)
			return
		}
		quality := r.URL.Query().Get("quality")
		if quality == "" {
			quality = "original"
		}
		if !stream.ValidateQuality(quality) {
			http.Error(w, "無效的畫質參數", http.StatusBadRequest)
			return
		}
		audioTrack, _ := strconv.Atoi(r.URL.Query().Get("audio"))
		if audioTrack < 0 {
			audioTrack = 0
		}
		seekSeq := r.URL.Query().Get("seek")

		segPath, err := hlsMgr.EnsureSegmentReady(r.Context(), u.UserID, sessionID, media, mode, quality, audioTrack, segIdx, seekSeq)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if errors.Is(err, stream.ErrInvalidSessionID) || errors.Is(err, stream.ErrInvalidStreamParam) || errors.Is(err, stream.ErrSegmentOutOfRange) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if errors.Is(err, stream.ErrTooManySessions) {
				http.Error(w, err.Error(), http.StatusTooManyRequests)
				return
			}
			fmt.Printf("[API] 取得轉碼切片失敗 (file=%d, seg=%d): %v\n", fileID, segIdx, err)
			http.Error(w, "轉碼切片逾時或發生錯誤", http.StatusGatewayTimeout)
			return
		}

		w.Header().Set("Content-Type", "video/mp2t")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, segPath)
	}))

	mux.HandleFunc("GET /api/stream/{id}/subtitles/{subId}", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		fileID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "無效的影片 ID", http.StatusBadRequest)
			return
		}
		subID := r.PathValue("subId")
		media, err := store.GetMediaFile(fileID)
		if err != nil {
			http.Error(w, "找不到影片", http.StatusNotFound)
			return
		}
		stream.ServeSubtitleWebVTT(w, r, media, subID, cfg.CacheDir)
	}))

	// 7. 個人化播放進度回報與清除 API
	mux.HandleFunc("POST /api/stream/{id}/progress", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		u := auth.GetUser(r)
		fileID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "無效的影片 ID"})
			return
		}
		var body struct {
			PositionSeconds float64 `json:"position_seconds"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "格式錯誤"})
			return
		}
		media, err := store.GetMediaFile(fileID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "找不到影片"})
			return
		}
		_ = store.UpdateProgress(u.UserID, fileID, body.PositionSeconds, media.DurationSeconds)
		writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
	}))

	mux.HandleFunc("DELETE /api/stream/{id}/progress", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		u := auth.GetUser(r)
		fileID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "無效的影片 ID"})
			return
		}
		_ = store.DeleteProgress(u.UserID, fileID)
		writeJSON(w, http.StatusOK, map[string]string{"status": "cleared"})
	}))

	// 8. 託管 React 前端靜態產物 (SPA)
	serveIndexNoCache := func(w http.ResponseWriter, r *http.Request, indexFile string) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		http.ServeFile(w, r, indexFile)
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		indexFile := filepath.Join(cfg.WebDir, "index.html")
		if _, err := os.Stat(indexFile); err == nil {
			reqPath := filepath.Clean(r.URL.Path)
			if reqPath == "/" || reqPath == "." || reqPath == "\\" {
				serveIndexNoCache(w, r, indexFile)
				return
			}
			fullPath := filepath.Join(cfg.WebDir, reqPath)
			if isSafeStaticPath(cfg.WebDir, fullPath) {
				if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
					http.ServeFile(w, r, fullPath)
					return
				}
			}
			serveIndexNoCache(w, r, indexFile)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("PiStream Server is running."))
	})

	return withSecurityMiddleware(mux)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
