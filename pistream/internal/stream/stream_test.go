package stream

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	// 11. 三段式降級：1080p HEVC + AAC MP4 (<= 8 Mbps)
	//     - 預設走 direct
	//     - ForceRemux=true (例如 Firefox 原生 MP4 解封裝報 MEDIA_ERR_DECODE) -> 優先改走免重編碼 remux
	//     - ForceRemux=true 且瀏覽器不支援 HEVC HLS -> 回退 transcode
	//     - ForceTranscode=true (即使品質為 original 或 1080p，或來源為低碼率 H.264) -> 強制走 1080p transcode，不被 Smart 1080p Bypass 攔截
	mp4HEVC1080p := &db.MediaFile{
		ID:         12,
		Container:  "mp4",
		VideoCodec: "hevc",
		AudioCodec: "aac",
		Width:      1920,
		Height:     1080,
		BitRate:    5_000_000,
	}
	res11Default := DecideStreamModeWithOptions(mp4HEVC1080p, "sess1", "original", 0, StreamCaps{
		AllowHEVCDirect: true,
		AllowHEVCHLS:    true,
	})
	if res11Default.Mode != "direct" {
		t.Fatalf("預期 1080p HEVC+AAC MP4 預設走 direct，實際為 %s", res11Default.Mode)
	}
	res11ForceRemux := DecideStreamModeWithOptions(mp4HEVC1080p, "sess1", "original", 0, StreamCaps{
		AllowHEVCDirect: true,
		AllowHEVCHLS:    true,
		ForceRemux:      true,
	})
	if res11ForceRemux.Mode != "remux" || res11ForceRemux.Quality != "original" {
		t.Fatalf("預期 ForceRemux 跳過 direct 改走原畫 remux，實際為 %+v", res11ForceRemux)
	}
	res11ForceRemuxNoHlsHevc := DecideStreamModeWithOptions(mp4HEVC1080p, "sess1", "original", 0, StreamCaps{
		AllowHEVCDirect: true,
		AllowHEVCHLS:    false,
		ForceRemux:      true,
	})
	if res11ForceRemuxNoHlsHevc.Mode != "transcode" {
		t.Fatalf("預期不支援 HEVC HLS 時 ForceRemux 自動回退 transcode，實際為 %+v", res11ForceRemuxNoHlsHevc)
	}
	res11ForceTrans := DecideStreamModeWithOptions(mp4HEVC1080p, "sess1", "1080p", 0, StreamCaps{
		AllowHEVCDirect: true,
		AllowHEVCHLS:    true,
		ForceTranscode:  true,
	})
	if res11ForceTrans.Mode != "transcode" || res11ForceTrans.Quality != "1080p" {
		t.Fatalf("預期 ForceTranscode 跳過 Smart 1080p Bypass 強制走 1080p transcode，實際為 %+v", res11ForceTrans)
	}
	mp4H264Low := &db.MediaFile{
		ID:         13,
		Container:  "mp4",
		VideoCodec: "h264",
		AudioCodec: "aac",
		Width:      1920,
		Height:     1080,
		BitRate:    4_000_000,
	}
	res11H264ForceTrans := DecideStreamModeWithOptions(mp4H264Low, "sess1", "1080p", 0, StreamCaps{
		ForceTranscode: true,
	})
	if res11H264ForceTrans.Mode != "transcode" || res11H264ForceTrans.Quality != "1080p" {
		t.Fatalf("預期 H.264 1080p 在 ForceTranscode=true 時不得被 Smart 1080p Bypass 攔截，實際為 %+v", res11H264ForceTrans)
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
	if !strings.Contains(joined4K, "-hwaccel drm -threads 2") {
		t.Fatalf("預期 4K HEVC 開啟 drm 時包含 -hwaccel drm -threads 2，實際參數: %s", joined4K)
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
		ID:              42,
		FilePath:        filepath.Join(tempDir, "nonexistent.mkv"),
		DurationSeconds: 120,
	}

	sess, err := mgr.getOrCreateSession(1, "sess_close_test", media, "transcode", "1080p", 0)
	if err != nil {
		t.Fatalf("建立 Session 失敗: %v", err)
	}
	if _, err := os.Stat(sess.OutputDir); err != nil {
		t.Fatalf("預期已建立暫存目錄 %s: %v", sess.OutputDir, err)
	}

	// 其他使用者不得停止 user 1 的 Session（跨帳號隔離）
	if stopped := mgr.StopSession(2, "sess_close_test", true); stopped {
		t.Fatalf("user 2 不應能停止 user 1 的 Session")
	}

	// 主動停止 Session（帶 tombstone=true）：應立即刪除暫存目錄並標記為已停止
	if stopped := mgr.StopSession(1, "sess_close_test", true); !stopped {
		t.Fatalf("預期 StopSession 回傳 true")
	}
	if _, err := os.Stat(sess.OutputDir); !os.IsNotExist(err) {
		t.Fatalf("預期暫存目錄已被刪除，但仍存在: %v", err)
	}

	// 關閉瞬間遲到的切片請求應立即回傳 context.Canceled，不得重新啟動 FFmpeg
	_, err = mgr.EnsureSegmentReady(context.Background(), 1, "sess_close_test", media, "transcode", "1080p", 0, 1, "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("預期已停止的 Session 拒絕遲到切片請求並回傳 context.Canceled，實際: %v", err)
	}

	// 非 tombstone 停止（例如切回 direct 模式）：應清除 Session 但允許同一個 sessionID 之後再切回 HLS
	if _, err := mgr.getOrCreateSession(1, "sess_direct_switch", media, "transcode", "1080p", 0); err != nil {
		t.Fatalf("建立 sess_direct_switch 失敗: %v", err)
	}
	if stopped := mgr.StopSession(1, "sess_direct_switch", false); !stopped {
		t.Fatalf("預期非 tombstone StopSession 回傳 true")
	}
	if mgr.isSessionStopped(1, "sess_direct_switch") {
		t.Fatalf("非 tombstone 停止不應將 sessionID 列入 tombstone")
	}
}

func TestHLSManagerSecurityBoundaries(t *testing.T) {
	tempDir := t.TempDir()
	victimDir := filepath.Join(filepath.Dir(tempDir), "victim_config_dir")
	if err := os.MkdirAll(victimDir, 0755); err != nil {
		t.Fatalf("建立測試目錄失敗: %v", err)
	}
	defer os.RemoveAll(victimDir)

	mgr := NewHLSManager(&config.Config{
		TranscodeTempDir: tempDir,
		SegmentDuration:  4,
		MaxKeepSegments:  10,
	})
	media := &db.MediaFile{
		ID:              7,
		FilePath:        filepath.Join(tempDir, "movie.mkv"),
		DurationSeconds: 40,
	}

	// 1. 路徑遍歷 sessionID 必須一律被阻斷，且不得刪除外部目錄
	for _, bad := range []string{"/../../../../config", "../victim_config_dir", "a/b", "sess\n#EXT-X-KEY", ""} {
		if ValidateSessionID(bad) {
			t.Fatalf("惡意 sessionID %q 不應通過驗證", bad)
		}
		if _, err := mgr.EnsureSegmentReady(context.Background(), 1, bad, media, "transcode", "1080p", 0, 0, ""); !errors.Is(err, ErrInvalidSessionID) {
			t.Fatalf("預期 %q 回傳 ErrInvalidSessionID，實際: %v", bad, err)
		}
	}
	if _, err := os.Stat(victimDir); err != nil {
		t.Fatalf("外部目錄不應被影響: %v", err)
	}

	// 2. 負數與超出片長範圍的切片索引必須被拒絕
	if _, err := ParseSegmentIndex("seg_-1.ts"); err == nil {
		t.Fatalf("負數切片編號應被拒絕")
	}
	if _, err := ParseSegmentIndex("seg_00001_evil.ts"); err == nil {
		t.Fatalf("夾帶非數字字元的切片名稱應被拒絕")
	}
	if _, err := mgr.EnsureSegmentReady(context.Background(), 1, "valid_sess", media, "transcode", "1080p", 0, 9999, ""); !errors.Is(err, ErrSegmentOutOfRange) {
		t.Fatalf("預期超出片長的切片編號回傳 ErrSegmentOutOfRange，實際: %v", err)
	}

	// 3. 並發 Session 上限保護：單一帳號最多 2 個（第 3 個自動淘汰最舊），全域最多 4 個
	for _, s := range []string{"s1", "s2", "s3"} {
		if _, err := mgr.getOrCreateSession(1, s, media, "transcode", "1080p", 0); err != nil {
			t.Fatalf("user 1 建立 %s 失敗: %v", s, err)
		}
	}
	if _, err := mgr.getOrCreateSession(2, "u2_s1", media, "transcode", "1080p", 0); err != nil {
		t.Fatalf("user 2 建立 u2_s1 失敗: %v", err)
	}
	if _, err := mgr.getOrCreateSession(2, "u2_s2", media, "transcode", "1080p", 0); err != nil {
		t.Fatalf("user 2 建立 u2_s2 失敗: %v", err)
	}
	// 此時全域已達 4 個 Session，user 3 嘗試再開第 5 個應回 ErrTooManySessions
	if _, err := mgr.getOrCreateSession(3, "u3_s1", media, "transcode", "1080p", 0); !errors.Is(err, ErrTooManySessions) {
		t.Fatalf("預期超過全域上限時回傳 ErrTooManySessions，實際: %v", err)
	}
}

func TestHLSManagerAdjacentSeekReuse(t *testing.T) {
	tempDir := t.TempDir()
	mgr := NewHLSManager(&config.Config{
		TranscodeTempDir: tempDir,
		SegmentDuration:  4,
		MaxKeepSegments:  10,
	})

	media := &db.MediaFile{
		ID:              99,
		FilePath:        filepath.Join(tempDir, "nonexistent_remux.mkv"),
		DurationSeconds: 4000,
	}

	sess, err := mgr.getOrCreateSession(1, "sess_adj_seek", media, "remux", "original", 0)
	if err != nil {
		t.Fatalf("建立 Session 失敗: %v", err)
	}

	// 模擬 FFmpeg 剛於 Seg=820 啟動且已完成寫出 seg_00820.ts、seg_00821.ts，正在寫 seg_00822.ts
	runningDone := make(chan struct{})
	defer close(runningDone)
	sess.StartSeg = 820
	sess.CurrentHeadSeg = 820
	sess.LastSeekSeq = "7"
	sess.ProcessDone = runningDone

	for _, name := range []string{"seg_00820.ts", "seg_00821.ts", "seg_00822.ts"} {
		if err := os.WriteFile(filepath.Join(sess.OutputDir, name), []byte("ts-payload"), 0644); err != nil {
			t.Fatalf("寫入測試切片 %s 失敗: %v", name, err)
		}
	}

	// 1. 緊鄰切片 Seg=821 且 seekSeq 變更 ("7" -> "8")：必須直接沿用現有切片，不得清除目錄或重啟 FFmpeg
	gotPath, err := mgr.EnsureSegmentReady(context.Background(), 1, "sess_adj_seek", media, "remux", "original", 0, 821, "8")
	if err != nil {
		t.Fatalf("預期直接沿用緊鄰切片 seg_00821.ts，實際錯誤: %v", err)
	}
	if filepath.Base(gotPath) != "seg_00821.ts" {
		t.Fatalf("預期回傳 seg_00821.ts，實際: %s", gotPath)
	}
	if sess.LastSeekSeq != "8" || sess.StartSeg != 820 || sess.CurrentHeadSeg != 821 {
		t.Fatalf("沿用緊鄰切片後 Session 狀態異常: StartSeg=%d Head=%d LastSeekSeq=%q", sess.StartSeg, sess.CurrentHeadSeg, sess.LastSeekSeq)
	}

	// 2. 遠距離跳轉（往前 Seg=560、往後 Seg=825，或離起點超過 2 片 Seg=823）不得誤沿用
	seg560 := filepath.Join(sess.OutputDir, "seg_00560.ts")
	seg561 := filepath.Join(sess.OutputDir, "seg_00561.ts")
	if canReuseAdjacentSeekLocked(sess, 560, seg560, seg561) {
		t.Fatalf("往回遠距跳轉 Seg=560 不應沿用 StartSeg=820 的行程")
	}
	seg825 := filepath.Join(sess.OutputDir, "seg_00825.ts")
	seg826 := filepath.Join(sess.OutputDir, "seg_00826.ts")
	if canReuseAdjacentSeekLocked(sess, 825, seg825, seg826) {
		t.Fatalf("往前跨多片跳轉 Seg=825 不應沿用 StartSeg=820 的行程")
	}
	sess.CurrentHeadSeg = 822
	seg823 := filepath.Join(sess.OutputDir, "seg_00823.ts")
	seg824 := filepath.Join(sess.OutputDir, "seg_00824.ts")
	if canReuseAdjacentSeekLocked(sess, 823, seg823, seg824) {
		t.Fatalf("距離 StartSeg 超過 2 片 (823-820=3) 時應允許重新定位 FFmpeg")
	}
}

func TestHLSManagerBufferWatermarks(t *testing.T) {
	cases := []struct {
		name      string
		bufferSec int
		segDur    int
		wantLow   int
		wantHigh  int
	}{
		{"default_45s_4s", 45, 4, 6, 12},
		{"zero_fallback_to_default", 0, 4, 6, 12},
		{"min_16s_4s", 16, 4, 3, 6},
		{"below_min_clamped", 8, 4, 3, 6},
		{"medium_60s_4s", 60, 4, 7, 15},
		{"max_120s_4s_capped", 120, 4, 10, 16},
		{"above_max_clamped", 300, 4, 10, 16},
		{"default_45s_6s_seg", 45, 6, 4, 8},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			low, high := bufferWatermarks(tc.bufferSec, tc.segDur)
			if low != tc.wantLow || high != tc.wantHigh {
				t.Fatalf("bufferWatermarks(%d, %d) = (%d, %d)，預期 (%d, %d)",
					tc.bufferSec, tc.segDur, low, high, tc.wantLow, tc.wantHigh)
			}
		})
	}

	mgr := NewHLSManager(&config.Config{
		TranscodeTempDir: t.TempDir(),
		SegmentDuration:  4,
		MaxKeepSegments:  10,
	})
	if low, high := mgr.watermarks(); low != 6 || high != 12 {
		t.Fatalf("未顯式設定 BufferSeconds 時應套用預設 45s 水位 (6, 12)，實際 (%d, %d)", low, high)
	}
}

func TestHLSManagerStopAndRecreateDirIsolation(t *testing.T) {
	tempDir := t.TempDir()
	mgr := NewHLSManager(&config.Config{
		TranscodeTempDir: tempDir,
		SegmentDuration:  4,
		MaxKeepSegments:  10,
	})
	media := &db.MediaFile{
		ID:              7794,
		FilePath:        filepath.Join(tempDir, "movie.mp4"),
		DurationSeconds: 120,
	}

	// 1. 建立第一個 Session (例如從 HLS 切回 Direct Play 前的舊行程)
	sess1, err := mgr.getOrCreateSession(1, "sess_same_id", media, "transcode", "720p", 0)
	if err != nil {
		t.Fatalf("建立 sess1 失敗: %v", err)
	}
	dir1 := sess1.OutputDir

	// 2. 模擬 StopSession(tombstone=false) 先從 map 移除 sess1，但在刪除 dir1 之前前端就以同一個 sessionID 觸發降級建立 sess2
	mgr.mu.Lock()
	delete(mgr.sessions, sess1.Key)
	mgr.mu.Unlock()

	sess2, err := mgr.getOrCreateSession(1, "sess_same_id", media, "transcode", "1080p", 0)
	if err != nil {
		t.Fatalf("建立 sess2 失敗: %v", err)
	}
	dir2 := sess2.OutputDir
	if dir1 == dir2 {
		t.Fatalf("同 sessionID 重建時必須分配獨立暫存目錄，否則併發回收會誤刪新目錄: %s", dir1)
	}

	// 3. 舊 sess1 延遲執行 terminateSessionLocked(sess1) 刪除 dir1：不得影響 sess2 的 dir2
	sess1.mu.Lock()
	mgr.terminateSessionLocked(sess1)
	sess1.mu.Unlock()

	if _, err := os.Stat(dir1); !os.IsNotExist(err) {
		t.Fatalf("預期舊目錄 dir1 已被刪除，但仍存在: %v", err)
	}
	if _, err := os.Stat(dir2); err != nil {
		t.Fatalf("預期新目錄 dir2 不受舊 Session 清理影響而保留，實際錯誤: %v", err)
	}
}

func TestHLSManagerStopDoesNotSpawnOrphanHwAccelFallback(t *testing.T) {
	tempDir := t.TempDir()
	mgr := NewHLSManager(&config.Config{
		TranscodeTempDir: tempDir,
		SegmentDuration:  4,
		MaxKeepSegments:  10,
		HwAccel:          "drm",
	})
	media := &db.MediaFile{
		ID:              7794,
		FilePath:        filepath.Join(tempDir, "movie_hevc.mp4"),
		DurationSeconds: 7200,
		VideoCodec:      "hevc",
	}

	// 1. 建立正在以 drm 硬解執行的 transcode Session（以可控制 ctx/done 模擬活著的 FFmpeg，無需實際啟動外部程式）
	sess, err := mgr.getOrCreateSession(2, "sess_orphan_test", media, "transcode", "1080p", 0)
	if err != nil {
		t.Fatalf("建立 Session 失敗: %v", err)
	}
	procCtx, cancel := context.WithCancel(context.Background())
	procDone := make(chan struct{})
	go func() {
		<-procCtx.Done()
		close(procDone)
	}()

	sess.mu.Lock()
	sess.StartSeg = 390
	sess.CurrentHeadSeg = 420
	sess.ProcessCtx = procCtx
	sess.CancelFunc = cancel
	sess.ProcessDone = procDone
	sess.Cmd = &exec.Cmd{}
	sess.mu.Unlock()

	// 2. 模擬前端仍有在途中的切片請求 Seg=426 正在 EnsureSegmentReady 迴圈中等待
	errCh := make(chan error, 1)
	go func() {
		_, waitErr := mgr.EnsureSegmentReady(context.Background(), 2, "sess_orphan_test", media, "transcode", "1080p", 0, 426, "")
		errCh <- waitErr
	}()

	// 稍候讓 EnsureSegmentReady 進入輪詢等待迴圈
	time.Sleep(80 * time.Millisecond)

	// 3. 模擬使用者切回原畫 (Direct -> Remux) 或退出播放器觸發 StopSession
	if stopped := mgr.StopSession(2, "sess_orphan_test", false); !stopped {
		t.Fatalf("預期 StopSession 回傳 true")
	}

	select {
	case waitErr := <-errCh:
		if !errors.Is(waitErr, context.Canceled) {
			t.Fatalf("預期 EnsureSegmentReady 因 Session 被主動終止而回傳 context.Canceled，實際: %v", waitErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("等待 EnsureSegmentReady 結束逾時")
	}

	// 4. 核心斷言：絕不可誤判為 drm 啟動失敗而回退 CPU 軟解，亦不可在 m.sessions 之外重新啟動孤兒 FFmpeg 或重建暫存目錄
	sess.mu.Lock()
	terminated := sess.Terminated
	disabledHw := sess.DisableHwAccel
	orphanCmd := sess.Cmd
	orphanDone := sess.ProcessDone
	outDir := sess.OutputDir
	sess.mu.Unlock()

	if !terminated {
		t.Fatalf("預期舊 Session 已標記 Terminated=true")
	}
	if disabledHw {
		t.Fatalf("主動停止 Session 不得誤判為硬體加速失敗而設定 DisableHwAccel=true")
	}
	if orphanCmd != nil || orphanDone != nil {
		t.Fatalf("已回收的 Session 不得重新啟動孤兒 FFmpeg 行程 (Cmd=%v, ProcessDone=%v)", orphanCmd, orphanDone)
	}
	if _, statErr := os.Stat(outDir); !os.IsNotExist(statErr) {
		t.Fatalf("已回收的 Session 暫存目錄不得被孤兒流程重建: %s", outDir)
	}

	// 5. 驗證 StopSessionIfConfigMismatch：相同設定保留、切換模式（transcode -> remux）時立刻回收
	sess2, err := mgr.getOrCreateSession(2, "sess_switch_test", media, "transcode", "1080p", 0)
	if err != nil {
		t.Fatalf("建立 sess2 失敗: %v", err)
	}
	if mgr.StopSessionIfConfigMismatch(2, "sess_switch_test", media.ID, "transcode", "1080p", 0) {
		t.Fatalf("相同影片與參數不應觸發回收")
	}
	if !mgr.StopSessionIfConfigMismatch(2, "sess_switch_test", media.ID, "remux", "original", 0) {
		t.Fatalf("從 transcode/1080p 切換為 remux/original 時應立即回收舊 Session")
	}
	sess2.mu.Lock()
	if !sess2.Terminated {
		sess2.mu.Unlock()
		t.Fatalf("預期被 StopSessionIfConfigMismatch 回收的 Session 已標記 Terminated=true")
	}
	sess2.mu.Unlock()
}
