package scanner

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"pistream/internal/db"
)

const thumbnailJobTimeout = 15 * time.Second

type thumbJob struct {
	fileID   int64
	filePath string
	duration float64
	cacheDir string
	store    *db.Store
}

var (
	thumbQueue         chan thumbJob
	thumbOnce          sync.Once
	thumbCtxMu         sync.RWMutex
	thumbBaseCtx       = context.Background()
	pendingThumbJobs   atomic.Int64
	generatedThumbJobs atomic.Int64
)

// SetThumbnailContext 設定背景縮圖工作者的根 Context，確保伺服器關閉時立即中止進行中的 FFmpeg 截圖。
func SetThumbnailContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	thumbCtxMu.Lock()
	thumbBaseCtx = ctx
	thumbCtxMu.Unlock()
}

func getThumbnailBaseContext() context.Context {
	thumbCtxMu.RLock()
	ctx := thumbBaseCtx
	thumbCtxMu.RUnlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func initThumbWorker() {
	thumbQueue = make(chan thumbJob, 500)
	go func() {
		for job := range thumbQueue {
			if generated := generateThumbnail(job); generated {
				generatedThumbJobs.Add(1)
			}
			if pendingThumbJobs.Add(-1) == 0 {
				if done := generatedThumbJobs.Swap(0); done > 0 {
					fmt.Printf("🖼️  背景縮圖產生完成，本次新建 %d 張封面縮圖\n", done)
				}
			}
		}
	}()
}

// QueueThumbnailJob 將縮圖工作排入單執行緒佇列，避免同時大量讀取外接硬碟影響影片串流
func QueueThumbnailJob(fileID int64, filePath string, duration float64, cacheDir string, store *db.Store) {
	thumbOnce.Do(initThumbWorker)
	pendingThumbJobs.Add(1)
	select {
	case thumbQueue <- thumbJob{
		fileID:   fileID,
		filePath: filePath,
		duration: duration,
		cacheDir: cacheDir,
		store:    store,
	}:
	default:
		pendingThumbJobs.Add(-1)
	}
}

func GetThumbPath(cacheDir string, fileID int64) string {
	return filepath.Join(cacheDir, "thumbs", fmt.Sprintf("thumb_%d.jpg", fileID))
}

// thumbnailSeekConfig 決定縮圖擷取秒數與是否啟用快速關鍵幀跳轉 (-noaccurate_seek)。
// 片長大於 30 秒的一般影片直接取目標點前最近關鍵幀（只解碼 1 張畫面、提速數倍且大幅降低 CPU 做功）；
// 30 秒以內短片保留精準定位，避免退回第 0 秒黑畫面。
func thumbnailSeekConfig(duration float64) (seekSec float64, fastKeyframeSeek bool) {
	if duration > 30 {
		return math.Min(35.0, duration*0.12), true
	}
	if duration > 2 {
		return duration * 0.3, false
	}
	if duration > 0 {
		return 0, false
	}
	return 5.0, false
}

func buildThumbnailFFmpegArgs(filePath, outPath string, seekSec float64, fastKeyframeSeek bool) []string {
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
	}
	if seekSec > 0 {
		if fastKeyframeSeek {
			args = append(args, "-noaccurate_seek")
		}
		args = append(args, "-ss", fmt.Sprintf("%.2f", seekSec))
	}
	args = append(args,
		"-i", filePath,
		"-frames:v", "1",
		"-vf", "scale=640:360:force_original_aspect_ratio=increase,crop=640:360",
		"-q:v", "4",
		"-y",
		outPath,
	)
	return args
}

func generateThumbnail(job thumbJob) bool {
	baseCtx := getThumbnailBaseContext()
	if baseCtx.Err() != nil {
		return false
	}

	thumbDir := filepath.Join(job.cacheDir, "thumbs")
	_ = os.MkdirAll(thumbDir, 0755)
	outPath := GetThumbPath(job.cacheDir, job.fileID)

	if info, err := os.Stat(outPath); err == nil && info.Size() > 500 {
		_ = job.store.MarkThumbReady(job.fileID)
		return false
	}

	seekSec, fastKeyframeSeek := thumbnailSeekConfig(job.duration)

	ctx, cancel := context.WithTimeout(baseCtx, thumbnailJobTimeout)
	cmd := exec.CommandContext(ctx, "ffmpeg", buildThumbnailFFmpegArgs(job.filePath, outPath, seekSec, fastKeyframeSeek)...)
	err := cmd.Run()
	cancel()

	if err != nil {
		if baseCtx.Err() != nil {
			return false
		}
		// Fallback: 若 seek 位置失敗或逾時，嘗試直接從第 0 秒擷取
		fallbackCtx, fallbackCancel := context.WithTimeout(baseCtx, thumbnailJobTimeout)
		fallbackCmd := exec.CommandContext(fallbackCtx, "ffmpeg", buildThumbnailFFmpegArgs(job.filePath, outPath, 0, false)...)
		err2 := fallbackCmd.Run()
		fallbackCancel()
		if err2 != nil {
			return false
		}
	}

	_ = job.store.MarkThumbReady(job.fileID)
	return true
}
