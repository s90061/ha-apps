package scanner

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"pistream/internal/db"
)

type thumbJob struct {
	fileID   int64
	filePath string
	duration float64
	cacheDir string
	store    *db.Store
}

var (
	thumbQueue chan thumbJob
	thumbOnce  sync.Once
)

func initThumbWorker() {
	thumbQueue = make(chan thumbJob, 500)
	go func() {
		for job := range thumbQueue {
			generateThumbnail(job)
		}
	}()
}

// QueueThumbnailJob 將縮圖工作排入單執行緒佇列，避免同時大量讀取外接硬碟影響影片串流
func QueueThumbnailJob(fileID int64, filePath string, duration float64, cacheDir string, store *db.Store) {
	thumbOnce.Do(initThumbWorker)
	select {
	case thumbQueue <- thumbJob{
		fileID:   fileID,
		filePath: filePath,
		duration: duration,
		cacheDir: cacheDir,
		store:    store,
	}:
	default:
	}
}

func GetThumbPath(cacheDir string, fileID int64) string {
	return filepath.Join(cacheDir, "thumbs", fmt.Sprintf("thumb_%d.jpg", fileID))
}

func generateThumbnail(job thumbJob) {
	thumbDir := filepath.Join(job.cacheDir, "thumbs")
	_ = os.MkdirAll(thumbDir, 0755)
	outPath := GetThumbPath(job.cacheDir, job.fileID)

	if info, err := os.Stat(outPath); err == nil && info.Size() > 500 {
		_ = job.store.MarkThumbReady(job.fileID)
		return
	}

	// 選取影片 10% 處或第 25 秒的關鍵幀，避開片頭黑畫面
	seekSec := 5.0
	if job.duration > 30 {
		seekSec = math.Min(35.0, job.duration*0.12)
	} else if job.duration > 2 {
		seekSec = job.duration * 0.3
	}

	cmd := exec.Command("ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-ss", fmt.Sprintf("%.2f", seekSec),
		"-i", job.filePath,
		"-frames:v", "1",
		"-vf", "scale=640:360:force_original_aspect_ratio=increase,crop=640:360",
		"-q:v", "4",
		"-y",
		outPath,
	)

	if err := cmd.Run(); err != nil {
		// Fallback: 若 seek 位置失敗，嘗試從 0 秒擷取
		fallbackCmd := exec.Command("ffmpeg",
			"-hide_banner",
			"-loglevel", "error",
			"-i", job.filePath,
			"-frames:v", "1",
			"-vf", "scale=640:360:force_original_aspect_ratio=increase,crop=640:360",
			"-q:v", "4",
			"-y",
			outPath,
		)
		if err2 := fallbackCmd.Run(); err2 != nil {
			return
		}
	}

	_ = job.store.MarkThumbReady(job.fileID)
}
