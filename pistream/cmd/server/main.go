package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"pistream/internal/api"
	"pistream/internal/config"
	"pistream/internal/db"
	"pistream/internal/scanner"
	"pistream/internal/stream"
)

func main() {
	cfg := config.Load()

	_ = os.MkdirAll(cfg.MediaDir, 0755)
	_ = os.MkdirAll(filepath.Join(cfg.CacheDir, "thumbs"), 0755)
	_ = os.MkdirAll(filepath.Join(cfg.CacheDir, "subs"), 0755)
	_ = os.MkdirAll(cfg.TranscodeTempDir, 0755)
	_ = os.MkdirAll(filepath.Dir(cfg.DBPath), 0755)

	store, err := db.InitDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("資料庫啟動失敗: %v", err)
	}

	if err := store.EnsureDefaultAdmin(cfg.AdminUser, cfg.AdminPassword); err != nil {
		log.Fatalf("初始化預設管理員帳號失敗: %v", err)
	}

	if changed, prevDir, clearedProg, clearedMedia, err := store.SyncMediaDir(cfg.MediaDir); err != nil {
		log.Fatalf("同步媒體庫路徑設定失敗: %v", err)
	} else if changed {
		fmt.Printf("🔄 偵測到媒體庫路徑變更 (%s -> %s)，已立刻清空 %d 筆觀看紀錄與 %d 筆舊影片索引，並清除縮圖與字幕快取\n",
			prevDir, cfg.MediaDir, clearedProg, clearedMedia)
		_ = os.RemoveAll(filepath.Join(cfg.CacheDir, "thumbs"))
		_ = os.RemoveAll(filepath.Join(cfg.CacheDir, "subs"))
		_ = os.MkdirAll(filepath.Join(cfg.CacheDir, "thumbs"), 0755)
		_ = os.MkdirAll(filepath.Join(cfg.CacheDir, "subs"), 0755)
	}

	fmt.Printf("==================================================\n")
	fmt.Printf("🍓 PiStream — 樹莓派 5 極速遠端媒體伺服器啟動中...\n")
	fmt.Printf("📂 媒體庫路徑: %s\n", cfg.MediaDir)
	fmt.Printf("🗄️  資料庫路徑: %s\n", cfg.DBPath)
	fmt.Printf("🚀 RAM Disk 轉碼區: %s\n", cfg.TranscodeTempDir)
	fmt.Printf("👤 預設管理員帳號: %s\n", cfg.AdminUser)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scanner.SetThumbnailContext(ctx)

	// 背景執行初次掃描與縮圖排程，確保 HTTP 伺服器與 Docker 健康檢查秒開
	go func() {
		count, err := scanner.ScanDirectory(cfg.MediaDir, cfg.CacheDir, store)
		if err != nil {
			fmt.Printf("[Scanner] 初次掃描發生錯誤: %v\n", err)
			return
		}
		fmt.Printf("📚 媒體庫掃描完成，已索引 %d 部影片\n", count)
	}()

	watcher, err := scanner.NewLibraryWatcher(cfg.MediaDir, cfg.CacheDir, store)
	if err == nil {
		if err := watcher.Start(ctx); err != nil {
			fmt.Printf("[Watcher] 啟動失敗: %v\n", err)
		}
	}

	hlsMgr := stream.NewHLSManager(cfg)
	router := api.NewRouter(cfg, store, hlsMgr)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		fmt.Printf("🌐 遠端播放入口: http://0.0.0.0:%s\n", cfg.Port)
		fmt.Printf("==================================================\n")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 伺服器異常終止: %v", err)
		}
	}()

	<-stop
	fmt.Println("\n正在關閉 PiStream 伺服器...")
	cancel()
	if hlsMgr != nil {
		hlsMgr.StopAllSessions()
	}
	if watcher != nil {
		_ = watcher.Close()
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	fmt.Println("伺服器已安全停止。")
}
