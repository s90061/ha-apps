package scanner

import (
	"math"
	"strings"
	"testing"
)

func TestThumbnailSeekConfigAndArgs(t *testing.T) {
	// 1. 長片 (>30s)：應啟用 -noaccurate_seek 且位於 -ss 與 -i 之前
	seekLong, fastLong := thumbnailSeekConfig(3600)
	if !fastLong || seekLong != 35.0 {
		t.Fatalf("長片預期 (35.0, true)，實際 (%.2f, %v)", seekLong, fastLong)
	}
	argsLong := strings.Join(buildThumbnailFFmpegArgs("/media/movie.mkv", "/cache/thumb_1.jpg", seekLong, fastLong), " ")
	if !strings.Contains(argsLong, "-noaccurate_seek -ss 35.00 -i /media/movie.mkv") {
		t.Fatalf("長片縮圖應在 -ss 與 -i 之前加入 -noaccurate_seek，實際參數: %s", argsLong)
	}

	// 2. 中等影片 (120s)：12% = 14.40s，啟用 -noaccurate_seek
	seekMed, fastMed := thumbnailSeekConfig(120)
	if !fastMed || math.Abs(seekMed-14.4) > 0.01 {
		t.Fatalf("120s 影片預期 (14.4, true)，實際 (%.2f, %v)", seekMed, fastMed)
	}

	// 3. 短片 (<=30s)：保留精準定位，不加 -noaccurate_seek 以免退回第 0 秒黑畫面
	seekShort, fastShort := thumbnailSeekConfig(20)
	if fastShort || math.Abs(seekShort-6.0) > 0.01 {
		t.Fatalf("20s 短片預期 (6.0, false)，實際 (%.2f, %v)", seekShort, fastShort)
	}
	argsShort := strings.Join(buildThumbnailFFmpegArgs("/media/clip.mp4", "/cache/thumb_2.jpg", seekShort, fastShort), " ")
	if strings.Contains(argsShort, "-noaccurate_seek") {
		t.Fatalf("30 秒以內短片不應加入 -noaccurate_seek，實際參數: %s", argsShort)
	}
	if !strings.Contains(argsShort, "-ss 6.00 -i /media/clip.mp4") {
		t.Fatalf("20s 短片應包含 -ss 6.00 -i，實際參數: %s", argsShort)
	}

	// 4. Fallback (seekSec=0)：不帶 -ss 與 -noaccurate_seek
	argsFallback := strings.Join(buildThumbnailFFmpegArgs("/media/broken.ts", "/cache/thumb_3.jpg", 0, false), " ")
	if strings.Contains(argsFallback, "-ss") || strings.Contains(argsFallback, "-noaccurate_seek") {
		t.Fatalf("Fallback 截圖不應帶 -ss 或 -noaccurate_seek，實際參數: %s", argsFallback)
	}
}
