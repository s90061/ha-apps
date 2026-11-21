package stream

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pistream/internal/config"
	"pistream/internal/db"
)

func TestDecideStreamMode(t *testing.T) {
	mp4H264 := &db.MediaFile{
		ID:         1,
		Container:  "mp4",
		VideoCodec: "h264",
		AudioCodec: "aac",
	}

	// 1. MP4 H.264 + AAC 原畫 -> Direct Play
	res1 := DecideStreamMode(mp4H264, "sess1", "original", 0, false)
	if res1.Mode != "direct" {
		t.Fatalf("預期為 direct，實際為 %s", res1.Mode)
	}

	// 2. MKV H.264 + AC3 原畫 -> Remux (-c:v copy)
	mkvH264 := &db.MediaFile{
		ID:         2,
		Container:  "mkv",
		VideoCodec: "h264",
		AudioCodec: "ac3",
	}
	res2 := DecideStreamMode(mkvH264, "sess1", "original", 0, false)
	if res2.Mode != "remux" {
		t.Fatalf("預期為 remux，實際為 %s", res2.Mode)
	}

	// 3. MP4 HEVC 在支援 HEVC 的瀏覽器 -> Direct Play
	mp4HEVC := &db.MediaFile{
		ID:         3,
		Container:  "mp4",
		VideoCodec: "hevc",
		AudioCodec: "aac",
	}
	res3 := DecideStreamMode(mp4HEVC, "sess1", "original", 0, true)
	if res3.Mode != "direct" {
		t.Fatalf("預期支援 HEVC 的 MP4 為 direct，實際為 %s", res3.Mode)
	}

	// 4. 手動切換 720p 省流模式 -> Transcode
	res4 := DecideStreamMode(mp4H264, "sess1", "720p", 0, true)
	if res4.Mode != "transcode" || res4.Quality != "720p" {
		t.Fatalf("預期為 transcode 720p，實際為 %+v", res4)
	}

	// 5. 1080p MKV HEVC 在支援 HEVC 的瀏覽器 -> 原畫與 1080p 模式皆走免重編碼 Remux (-c:v copy)
	mkvHEVC1080p := &db.MediaFile{
		ID:         5,
		Container:  "mkv",
		VideoCodec: "hevc",
		AudioCodec: "eac3",
		Width:      1920,
		Height:     1080,
		BitRate:    4_500_000,
	}
	res5Orig := DecideStreamMode(mkvHEVC1080p, "sess1", "original", 0, true)
	if res5Orig.Mode != "remux" {
		t.Fatalf("預期支援 HEVC 的 1080p MKV 原畫為 remux，實際為 %s", res5Orig.Mode)
	}
	res5_1080p := DecideStreamMode(mkvHEVC1080p, "sess1", "1080p", 0, true)
	if res5_1080p.Mode != "remux" {
		t.Fatalf("預期低碼率 1080p MKV HEVC 切換 1080p 時自動旁路為 remux，實際為 %s", res5_1080p.Mode)
	}

	// 6. 1080p MKV HEVC 在不支援 HEVC 的瀏覽器 (或強制 H.264 兼容模式) -> Transcode
	res6 := DecideStreamMode(mkvHEVC1080p, "sess1", "1080p", 0, false)
	if res6.Mode != "transcode" {
		t.Fatalf("預期不支援 HEVC 時 1080p MKV HEVC 為 transcode，實際為 %s", res6.Mode)
	}

	// 7. 1080p MKV H.264 (碼率 <= 8 Mbps) 切換 1080p -> 智慧旁路為 Remux (-c:v copy)
	mkvH264_1080p := &db.MediaFile{
		ID:         7,
		Container:  "mkv",
		VideoCodec: "h264",
		AudioCodec: "ac3",
		Width:      1920,
		Height:     1080,
		BitRate:    5_200_000,
	}
	res7 := DecideStreamMode(mkvH264_1080p, "sess1", "1080p", 0, false)
	if res7.Mode != "remux" {
		t.Fatalf("預期 1080p H.264 MKV (5.2 Mbps) 選 1080p 時旁路為 remux，實際為 %s", res7.Mode)
	}

	// 8. 高碼率藍光原盤 1080p MKV H.264 (25 Mbps) 切換 1080p 省流 -> 仍執行 Transcode 壓低碼率
	mkvH264HighBitrate := &db.MediaFile{
		ID:         8,
		Container:  "mkv",
		VideoCodec: "h264",
		AudioCodec: "dts",
		Width:      1920,
		Height:     1080,
		BitRate:    25_000_000,
	}
	res8 := DecideStreamMode(mkvH264HighBitrate, "sess1", "1080p", 0, true)
	if res8.Mode != "transcode" {
		t.Fatalf("預期 25 Mbps 高碼率 1080p MKV 選 1080p 省流時為 transcode，實際為 %s", res8.Mode)
	}

	// 9. Windows Firefox 情境 (allowHEVCDirect=true, allowHEVCHLS=false)：
	//    4K HEVC MP4 仍應走 Direct Play (0% Pi 5 CPU)，而 4K HEVC MKV 則安全走 Transcode
	mp4HEVC4K := &db.MediaFile{
		ID:         9,
		Container:  "mp4",
		VideoCodec: "hevc",
		AudioCodec: "aac",
		Width:      3840,
		Height:     2160,
		BitRate:    22_000_000,
	}
	res9Direct := DecideStreamModeWithCaps(mp4HEVC4K, "sess1", "original", 0, true, false)
	if res9Direct.Mode != "direct" {
		t.Fatalf("預期 Windows Firefox 上 4K HEVC MP4 原畫維持 direct，實際為 %s", res9Direct.Mode)
	}
	mkvHEVC4K := &db.MediaFile{
		ID:         10,
		Container:  "mkv",
		VideoCodec: "hevc",
		AudioCodec: "ac3",
		Width:      3840,
		Height:     2160,
		BitRate:    22_000_000,
	}
	res9Mkv := DecideStreamModeWithCaps(mkvHEVC4K, "sess1", "original", 0, true, false)
	if res9Mkv.Mode != "transcode" {
		t.Fatalf("預期不支援 MSE HEVC 時 4K HEVC MKV 為 transcode，實際為 %s", res9Mkv.Mode)
	}

	// 10. 4K HEVC + EAC3 MP4：
	//     - 瀏覽器支援 HEVC MSE 但不支援 EAC3 (如 Firefox/Chrome) -> Remux (-c:v copy + 音訊轉 AAC)
	//     - 瀏覽器同時支援 HEVC + EAC3 (如支援杜比的 Edge/Safari) -> Direct Play (0% CPU)
	mp4HEVC4KEAC3 := &db.MediaFile{
		ID:         11,
		Container:  "mp4",
		VideoCodec: "hevc",
		AudioCodec: "eac3",
		Width:      3840,
		Height:     2160,
		BitRate:    25_000_000,
	}
	res10Remux := DecideStreamModeWithAllCaps(mp4HEVC4KEAC3, "sess1", "original", 0, true, true, false)
	if res10Remux.Mode != "remux" {
		t.Fatalf("預期 4K HEVC + EAC3 MP4 在不支援 EAC3 的瀏覽器走 remux，實際為 %s", res10Remux.Mode)
	}
	res10Direct := DecideStreamModeWithAllCaps(mp4HEVC4KEAC3, "sess1", "original", 0, true, true, true)
	if res10Direct.Mode != "direct" {
		t.Fatalf("預期 4K HEVC + EAC3 MP4 在支援 EAC3 的瀏覽器走 direct，實際為 %s", res10Direct.Mode)
	}
}

func TestBuildFFmpegArgsSeekAndOffset(t *testing.T) {
	// 1. Remux 模式：必須包含 -noaccurate_seek、-copyts、-start_at_zero，保留關鍵幀真實 PTS，避免字幕錯位
	remuxArgs := BuildFFmpegArgs(TranscodeParams{
		InputFile:      "/media/movie.mkv",
		OutputDir:      "/dev/shm/out",
		StartTimeSec:   600.0,
		StartSegNumber: 150,
		SegmentDur:     4,
		Mode:           "remux",
		Quality:        "original",
		AudioTrack:     1,
	})

	joinedRemux := strings.Join(remuxArgs, " ")
	if !strings.Contains(joinedRemux, "-noaccurate_seek -ss 600.000 -i /media/movie.mkv") {
		t.Fatalf("預期 remux 包含 -noaccurate_seek -ss 置於 -i 之前，實際參數: %s", joinedRemux)
	}
	if !strings.Contains(joinedRemux, "-c:v copy") {
		t.Fatalf("預期 remux 包含 -c:v copy，實際參數: %s", joinedRemux)
	}
	if !strings.Contains(joinedRemux, "-copyts -start_at_zero") {
		t.Fatalf("預期 remux 包含 -copyts -start_at_zero 保留關鍵幀真實 PTS，實際參數: %s", joinedRemux)
	}
	if !strings.Contains(joinedRemux, "-map 0:a:1?") {
		t.Fatalf("預期映射第二音軌 0:a:1?，實際參數: %s", joinedRemux)
	}

	// 2. Transcode 模式：精確跳轉並透過 output_ts_offset 對齊時間戳
	transArgs := BuildFFmpegArgs(TranscodeParams{
		InputFile:      "/media/movie.mkv",
		OutputDir:      "/dev/shm/out",
		StartTimeSec:   600.0,
		StartSegNumber: 150,
		SegmentDur:     4,
		Mode:           "transcode",
		Quality:        "1080p",
		AudioTrack:     1,
	})

	joinedTrans := strings.Join(transArgs, " ")
	if !strings.Contains(joinedTrans, "-ss 600.000 -i /media/movie.mkv") {
		t.Fatalf("預期 transcode -ss 置於 -i 之前，實際參數: %s", joinedTrans)
	}
	if !strings.Contains(joinedTrans, "output_ts_offset=600.000") {
		t.Fatalf("預期 transcode 包含 output_ts_offset=600.000 對齊時間戳，實際參數: %s", joinedTrans)
	}
}

func TestBuildFFmpegArgsVAAPI(t *testing.T) {
	transArgs := BuildFFmpegArgs(TranscodeParams{
		InputFile:      "/media/movie.mkv",
		OutputDir:      "/dev/shm/out",
		StartTimeSec:   120.0,
		StartSegNumber: 30,
		SegmentDur:     4,
		Mode:           "transcode",
		Quality:        "720p",
		AudioTrack:     0,
		HwAccel:        "vaapi",
		VaapiDevice:    "/dev/dri/renderD128",
	})
	joinedTrans := strings.Join(transArgs, " ")
	for _, want := range []string{"-hwaccel vaapi", "-hwaccel_device /dev/dri/renderD128", "-c:v h264_vaapi", "scale_vaapi=w='min(1280,iw)':h=-2:format=nv12"} {
		if !strings.Contains(joinedTrans, want) {
			t.Fatalf("預期 VAAPI 轉碼參數包含 %q，實際參數: %s", want, joinedTrans)
		}
	}

	remuxArgs := BuildFFmpegArgs(TranscodeParams{
		InputFile:      "/media/movie.mkv",
		OutputDir:      "/dev/shm/out",
		StartTimeSec:   0,
		StartSegNumber: 0,
		SegmentDur:     4,
		Mode:           "remux",
		Quality:        "original",
		HwAccel:        "vaapi",
	})
	if strings.Contains(strings.Join(remuxArgs, " "), "-hwaccel vaapi") {
		t.Fatalf("remux 模式不應啟用 VAAPI 硬體編解碼")
	}
}

func TestBuildFFmpegArgsPi5LowCPU1080p(t *testing.T) {
	// 1. 1080p 原片 (SourceWidth=1920) 轉 1080p：應省略 -vf scale 濾鏡，並啟用 superfast + readrate 節流
	args1080 := BuildFFmpegArgs(TranscodeParams{
		InputFile:      "/media/ep01_1080p_hevc.mkv",
		OutputDir:      "/dev/shm/out",
		StartTimeSec:   30.0,
		StartSegNumber: 7,
		SegmentDur:     4,
		Mode:           "transcode",
		Quality:        "1080p",
		SourceWidth:    1920,
		SourceHeight:   1080,
		SourceCodec:    "hevc",
	})
	joined1080 := strings.Join(args1080, " ")

	for _, want := range []string{
		"-readrate 1.2",
		"-readrate_initial_burst 12",
		"-preset superfast",
		"-x264-params bframes=2:ref=1:rc-lookahead=0:weightp=0",
		"-fpsmax 30",
	} {
		if !strings.Contains(joined1080, want) {
			t.Fatalf("預期 Pi 5 低 CPU 1080p 參數包含 %q，實際參數: %s", want, joined1080)
		}
	}
	if strings.Contains(joined1080, "-vf scale") {
		t.Fatalf("原片寬度已為 1920 時，轉 1080p 不應掛載冗餘的 -vf scale 濾鏡，實際參數: %s", joined1080)
	}

	// 2. 4K HEVC 原片 (SourceWidth=3840, HwAccel=drm) 轉 1080p：應啟用 -hwaccel drm 與 fast_bilinear 快速縮放濾鏡
	args4K := BuildFFmpegArgs(TranscodeParams{
		InputFile:    "/media/movie_4k.mkv",
		OutputDir:    "/dev/shm/out",
		SegmentDur:   4,
		Mode:         "transcode",
		Quality:      "1080p",
		SourceWidth:  3840,
		SourceHeight: 2160,
		SourceCodec:  "hevc",
		HwAccel:      "drm",
	})
	joined4K := strings.Join(args4K, " ")
	if !strings.Contains(joined4K, "-hwaccel drm") {
		t.Fatalf("預期 4K HEVC 開啟 drm 時包含 -hwaccel drm，實際參數: %s", joined4K)
	}
	if !strings.Contains(joined4K, "-loglevel info -nostats") {
		t.Fatalf("預期啟用 drm 硬解時使用 -loglevel info -nostats 以記錄硬解診斷行，實際參數: %s", joined4K)
	}
	if !strings.Contains(joined4K, "scale='min(1920,iw)':-2:flags=fast_bilinear") {
		t.Fatalf("預期 4K 轉 1080p 啟用 fast_bilinear 縮放，實際參數: %s", joined4K)
	}

	// 3. H.264 原片即使 HwAccel=drm 也不應傳入 -hwaccel drm（Pi 5 僅有 HEVC 硬解）
	argsH264 := BuildFFmpegArgs(TranscodeParams{
		InputFile:   "/media/movie_1080p.mkv",
		OutputDir:   "/dev/shm/out",
		SegmentDur:  4,
		Mode:        "transcode",
		Quality:     "720p",
		SourceWidth: 1920,
		SourceCodec: "h264",
		HwAccel:     "drm",
	})
	if strings.Contains(strings.Join(argsH264, " "), "-hwaccel drm") {
		t.Fatalf("Pi 5 無 H.264 硬解，H.264 轉碼不應啟用 -hwaccel drm")
	}

	// 4. 驗證 FFmpeg 硬解診斷過濾器只攔截關鍵訊息、不洗版一般資訊行
	if !shouldLogFFmpegDiagnostic("[hevc @ 0x1234] Hwaccel V4L2 HEVC stateless V4; devices: /dev/media2,/dev/video19; buffers: src DMABuf, dst DMABuf") {
		t.Fatalf("預期應擷取 Hwaccel V4L2 HEVC stateless 確認行")
	}
	if !shouldLogFFmpegDiagnostic("Unable to open any dma_heap device") {
		t.Fatalf("預期應擷取 dma_heap 錯誤行")
	}
	if shouldLogFFmpegDiagnostic("Stream #0:0: Video: hevc (Main 10), yuv420p10le, 3840x2160") {
		t.Fatalf("一般串流資訊行不應被當作硬解診斷訊息洗版")
	}
}

func TestHLSManagerStopSessionAndTombstone(t *testing.T) {
	tempDir := t.TempDir()
	mgr := NewHLSManager(&config.Config{
		TranscodeTempDir: tempDir,
		SegmentDuration:  4,
		MaxKeepSegments:  10,
	})

	media := &db.MediaFile{
		ID:       42,
		FilePath: filepath.Join(tempDir, "nonexistent.mkv"),
	}

	sess := mgr.getOrCreateSession("sess_close_test", media, "transcode", "1080p", 0)
	if _, err := os.Stat(sess.OutputDir); err != nil {
		t.Fatalf("預期已建立暫存目錄 %s: %v", sess.OutputDir, err)
	}

	// 主動停止 Session（帶 tombstone=true）：應立即刪除暫存目錄並標記為已停止
	if stopped := mgr.StopSession("sess_close_test", true); !stopped {
		t.Fatalf("預期 StopSession 回傳 true")
	}
	if _, err := os.Stat(sess.OutputDir); !os.IsNotExist(err) {
		t.Fatalf("預期暫存目錄已被刪除，但仍存在: %v", err)
	}

	// 關閉瞬間遲到的切片請求應立即回傳 context.Canceled，不得重新啟動 FFmpeg
	_, err := mgr.EnsureSegmentReady(context.Background(), "sess_close_test", media, "transcode", "1080p", 0, 1, "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("預期已停止的 Session 拒絕遲到切片請求並回傳 context.Canceled，實際: %v", err)
	}

	// 非 tombstone 停止（例如切回 direct 模式）：應清除 Session 但允許同一個 sessionID 之後再切回 HLS
	mgr.getOrCreateSession("sess_direct_switch", media, "transcode", "1080p", 0)
	if stopped := mgr.StopSession("sess_direct_switch", false); !stopped {
		t.Fatalf("預期非 tombstone StopSession 回傳 true")
	}
	if mgr.isSessionStopped("sess_direct_switch") {
		t.Fatalf("非 tombstone 停止不應將 sessionID 列入 tombstone")
	}
}
