package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"pistream/internal/auth"
	"pistream/internal/config"
	"pistream/internal/db"
	"pistream/internal/scanner"
	"pistream/internal/stream"
)

func NewRouter(cfg *config.Config, store *db.Store, hlsMgr *stream.HLSManager) http.Handler {
	mux := http.NewServeMux()

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

	// 2. 身份驗證 API
	mux.HandleFunc("POST /api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "請求格式錯誤"})
			return
		}
		username := strings.TrimSpace(body.Username)
		user, err := store.GetUserByUsername(username)
		if err != nil || !auth.VerifyPassword(body.Password, user.PasswordHash) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "帳號或密碼錯誤"})
			return
		}

		token, err := auth.SignToken(cfg.JWTSecret, user.ID, user.Username, user.IsAdmin, 30*24*time.Hour)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "無法簽發登入憑證"})
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     auth.CookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   30 * 24 * 3600,
		})

		writeJSON(w, http.StatusOK, map[string]any{
			"token": token,
			"user":  user,
		})
	})

	mux.HandleFunc("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:     auth.CookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1,
		})
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
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.NewPassword) < 4 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "新密碼長度至少需 4 個字元"})
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
		_ = store.UpdateUserPassword(u.UserID, hash)
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	}))

	// 3. 管理員專用：多使用者管理 API (關閉公開註冊，僅由管理員新增/管理)
	mux.HandleFunc("GET /api/admin/users", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		users, err := store.ListUsers()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
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
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "請求格式錯誤"})
			return
		}
		username := strings.TrimSpace(body.Username)
		if username == "" || len(body.Password) < 4 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "帳號不可為空且密碼至少需 4 個字元"})
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
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Password) < 4 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "密碼長度至少需 4 個字元"})
			return
		}
		hash, err := auth.HashPassword(body.Password)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "密碼雜湊失敗"})
			return
		}
		if err := store.UpdateUserPassword(targetID, hash); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
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
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}))

	// 4. 媒體庫瀏覽 API (資料夾階層模式 + 全部影片模式 + 個人繼續觀看)
	mux.HandleFunc("GET /api/library/folders", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		u := auth.GetUser(r)
		relPath := r.URL.Query().Get("path")
		res, err := store.BrowseFolderForUser(u.UserID, relPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	}))

	mux.HandleFunc("GET /api/library/files", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		u := auth.GetUser(r)
		q := r.URL.Query().Get("q")
		files, err := store.ListMediaFilesForUser(u.UserID, q)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, files)
	}))

	mux.HandleFunc("GET /api/library/continue", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		u := auth.GetUser(r)
		items, err := store.GetContinueWatchingForUser(u.UserID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, items)
	}))

	mux.HandleFunc("POST /api/library/scan", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		go func() {
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

		// 若尚未產生縮圖，排入佇列並先回傳精緻 SVG 預覽圖
		media, err := store.GetMediaFile(fileID)
		if err == nil {
			scanner.QueueThumbnailJob(media.ID, media.FilePath, media.DurationSeconds, cfg.CacheDir, store)
		}
		w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 360">
			<defs>
				<linearGradient id="bg" x1="0%" y1="0%" x2="100%" y2="100%">
					<stop offset="0%" stop-color="#0f172a"/>
					<stop offset="50%" stop-color="#1e293b"/>
					<stop offset="100%" stop-color="#090d16"/>
				</linearGradient>
			</defs>
			<rect width="640" height="360" fill="url(#bg)"/>
			<circle cx="320" cy="180" r="42" fill="#38bdf8" fill-opacity="0.15" stroke="#38bdf8" stroke-width="2" stroke-opacity="0.4"/>
			<polygon points="308,158 308,202 346,180" fill="#38bdf8" fill-opacity="0.85"/>
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

		sessionID := r.URL.Query().Get("session")
		if sessionID == "" {
			sessionID = fmt.Sprintf("u%d_%d", u.UserID, time.Now().UnixNano())
		}
		quality := r.URL.Query().Get("quality")
		if quality == "" {
			quality = "original"
		}
		audioTrack, _ := strconv.Atoi(r.URL.Query().Get("audio"))
		allowHEVC := r.URL.Query().Get("hevc") == "1" || r.URL.Query().Get("hevc") == "true"

		decision := stream.DecideStreamMode(media, sessionID, quality, audioTrack, allowHEVC)
		prog, _ := store.GetProgress(u.UserID, fileID)

		writeJSON(w, http.StatusOK, map[string]any{
			"media":    media,
			"decision": decision,
			"progress": prog,
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
		sessionID := r.URL.Query().Get("session")
		if sessionID == "" {
			sessionID = "default"
		}
		mode := r.URL.Query().Get("mode")
		if mode == "" {
			mode = "transcode"
		}
		quality := r.URL.Query().Get("quality")
		if quality == "" {
			quality = "original"
		}
		audioTrack, _ := strconv.Atoi(r.URL.Query().Get("audio"))

		playlist := hlsMgr.GenerateVirtualPlaylist(media, sessionID, mode, quality, audioTrack)
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		_, _ = w.Write([]byte(playlist))
	}))

	mux.HandleFunc("GET /api/stream/{id}/hls/{seg}", requireAuth(func(w http.ResponseWriter, r *http.Request) {
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

		sessionID := r.URL.Query().Get("session")
		if sessionID == "" {
			sessionID = "default"
		}
		mode := r.URL.Query().Get("mode")
		if mode == "" {
			mode = "transcode"
		}
		quality := r.URL.Query().Get("quality")
		if quality == "" {
			quality = "original"
		}
		audioTrack, _ := strconv.Atoi(r.URL.Query().Get("audio"))

		segPath, err := hlsMgr.EnsureSegmentReady(r.Context(), sessionID, media, mode, quality, audioTrack, segIdx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusGatewayTimeout)
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
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
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
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		indexFile := filepath.Join(cfg.WebDir, "index.html")
		if _, err := os.Stat(indexFile); err == nil {
			reqPath := filepath.Clean(r.URL.Path)
			if reqPath == "/" || reqPath == "." || reqPath == "\\" {
				http.ServeFile(w, r, indexFile)
				return
			}
			fullPath := filepath.Join(cfg.WebDir, reqPath)
			if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
				http.ServeFile(w, r, fullPath)
				return
			}
			http.ServeFile(w, r, indexFile)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("PiStream Server is running. Please build web frontend into " + cfg.WebDir))
	})

	return mux
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
