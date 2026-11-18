package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"pistream/internal/db"
)

type LibraryWatcher struct {
	mediaDir string
	cacheDir string
	store    *db.Store
	watcher  *fsnotify.Watcher
	timers   map[string]*time.Timer
	mu       sync.Mutex
}

func NewLibraryWatcher(mediaDir, cacheDir string, store *db.Store) (*LibraryWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &LibraryWatcher{
		mediaDir: mediaDir,
		cacheDir: cacheDir,
		store:    store,
		watcher:  w,
		timers:   make(map[string]*time.Timer),
	}, nil
}

func (lw *LibraryWatcher) Start(ctx context.Context) error {
	absRoot, err := filepath.Abs(lw.mediaDir)
	if err != nil {
		return err
	}

	_ = filepath.Walk(absRoot, func(path string, info os.FileInfo, err error) error {
		if err == nil && info.IsDir() {
			_ = lw.watcher.Add(path)
		}
		return nil
	})

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-lw.watcher.Events:
				if !ok {
					return
				}
				lw.handleEvent(absRoot, event)
			case <-lw.watcher.Errors:
			}
		}
	}()
	return nil
}

func (lw *LibraryWatcher) handleEvent(absRoot string, event fsnotify.Event) {
	absPath, err := filepath.Abs(event.Name)
	if err != nil {
		return
	}

	if event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
		_ = lw.store.DeleteMediaFileByPath(absPath)
		return
	}

	if event.Op&(fsnotify.Create|fsnotify.Write) != 0 {
		info, err := os.Stat(absPath)
		if err == nil && info.IsDir() {
			_ = lw.watcher.Add(absPath)
			return
		}

		ext := strings.ToLower(filepath.Ext(absPath))
		if !ValidVideoExts[ext] && !ValidSubExts[ext] {
			return
		}

		lw.mu.Lock()
		if t, exists := lw.timers[absPath]; exists {
			t.Stop()
		}
		lw.timers[absPath] = time.AfterFunc(2*time.Second, func() {
			lw.mu.Lock()
			delete(lw.timers, absPath)
			lw.mu.Unlock()

			if ValidVideoExts[ext] {
				fmt.Printf("[Watcher] 偵測到新影片，自動索引: %s\n", filepath.Base(absPath))
				_ = ProcessSingleMediaFile(absRoot, absPath, lw.cacheDir, lw.store)
			} else if ValidSubExts[ext] {
				// 外掛字幕新增時，重新掃描該目錄下的影片以掛載字幕
				_, _ = ScanDirectory(lw.mediaDir, lw.cacheDir, lw.store)
			}
		})
		lw.mu.Unlock()
	}
}

func (lw *LibraryWatcher) Close() error {
	return lw.watcher.Close()
}
