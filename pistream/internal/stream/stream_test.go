package stream

import (
	"strings"
	"testing"

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
