package stream

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

type TranscodeParams struct {
	InputFile      string
	OutputDir      string
	StartTimeSec   float64
	StartSegNumber int
	SegmentDur     int
	Mode           string // "remux" 或 "transcode"
	Quality        string // "original", "1080p", "720p"
	AudioTrack     int    // 0, 1, 2...
	HwAccel        string // "", "vaapi"
	VaapiDevice    string // 例如 "/dev/dri/renderD128"
}

// BuildFFmpegArgs 針對樹莓派 5 (ARM64) 與任意時段秒開跳轉建構 FFmpeg 參數
func BuildFFmpegArgs(p TranscodeParams) []string {
	args := []string{
		"-hide_banner",
		"-loglevel", "warning",
	}

	useVAAPI := p.Mode != "remux" && strings.EqualFold(p.HwAccel, "vaapi")
	if useVAAPI {
		dev := p.VaapiDevice
		if dev == "" {
			dev = "/dev/dri/renderD128"
		}
		args = append(args,
			"-hwaccel", "vaapi",
			"-hwaccel_device", dev,
			"-hwaccel_output_format", "vaapi",
		)
	}

	// 1. 將 -ss 置於 -i 之前：啟用 Input Keyframe Seeking，在幾十毫秒內直接跳至目標關鍵幀
	// 在 remux (-c:v copy) 模式下必須搭配 -noaccurate_seek，避免音訊被精確裁切而視訊停在關鍵幀，導致影音 PTS 錯位卡住
	if p.StartTimeSec > 0 {
		if p.Mode == "remux" {
			args = append(args, "-noaccurate_seek")
		}
		args = append(args, "-ss", fmt.Sprintf("%.3f", p.StartTimeSec))
	}

	audioIdx := p.AudioTrack
	if audioIdx < 0 {
		audioIdx = 0
	}

	args = append(args,
		"-i", p.InputFile,
		"-map", "0:v:0",
		"-map", fmt.Sprintf("0:a:%d?", audioIdx),
		"-sn",
	)

	gopSize := strconv.Itoa(p.SegmentDur * 24)

	if p.Mode == "remux" {
		// Tier 2: 免重編碼換殼 (-c:v copy)，樹莓派 5 CPU 負載趨近 0%，處理速度可達 50x 以上
		args = append(args, "-c:v", "copy")
	} else if useVAAPI {
		// Tier 3 (硬體加速): Synology DS418play Intel Celeron J3355 (HD Graphics 500) h264_vaapi 硬體編解碼
		targetWidth := "1920"
		avgBitrate := "4M"
		maxRate := "6M"
		bufSize := "8M"
		if p.Quality == "720p" {
			targetWidth = "1280"
			avgBitrate = "2M"
			maxRate = "3M"
			bufSize = "4M"
		}
		scaleFilter := fmt.Sprintf("scale_vaapi=w='min(%s,iw)':h=-2:format=nv12", targetWidth)

		args = append(args,
			"-c:v", "h264_vaapi",
			"-vf", scaleFilter,
			"-g", gopSize,
			"-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", p.SegmentDur),
			"-b:v", avgBitrate,
			"-maxrate", maxRate,
			"-bufsize", bufSize,
		)
	} else {
		// Tier 3: 樹莓派 5 四核心 Cortex-A76 省流轉碼最佳化
		// 使用 veryfast 並移除 zerolatency（保留 B-frame 與碼率控制效率），搭配 VBV maxrate 確保真正省流
		scaleFilter := "scale='min(1920,iw)':-2"
		maxRate := "4M"
		bufSize := "8M"
		crf := "25"

		if p.Quality == "720p" {
			scaleFilter = "scale='min(1280,iw)':-2"
			maxRate = "2M"
			bufSize = "4M"
			crf = "26"
		}

		args = append(args,
			"-c:v", "libx264",
			"-preset", "veryfast",
			"-pix_fmt", "yuv420p",
			"-threads", "4",
			"-vf", scaleFilter,
			"-g", gopSize,
			"-sc_threshold", "0",
			"-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", p.SegmentDur),
			"-crf", crf,
			"-maxrate", maxRate,
			"-bufsize", bufSize,
		)
	}

	// 音訊統一轉為高相容性雙聲道 AAC
	args = append(args,
		"-c:a", "aac",
		"-b:a", "192k",
		"-ac", "2",
	)

	segPattern := filepath.Join(p.OutputDir, "seg_%05d.ts")

	// 關鍵設定：
	// 1. 在 remux (-c:v copy) 模式下，-noaccurate_seek -ss 會跳至目標秒數前的最近關鍵幀 K (K <= StartTimeSec)。
	//    若使用 -avoid_negative_ts make_zero 搭配 output_ts_offset=StartTimeSec，會將關鍵幀 K 的畫面強行蓋上 StartTimeSec 的假時間戳，
	//    導致影片實際播放 K 秒的畫面、但播放器時間軸與 WebVTT 字幕卻顯示 StartTimeSec，產生 1~10 秒的字幕提前錯位。
	//    因此 remux 模式改用 -copyts -start_at_zero，直接保留原始影片關鍵幀 K 的真實 PTS 時間戳，讓影音與字幕 100% 精準對齊。
	// 2. 在 transcode (重編碼) 模式下，FFmpeg 會精確由 StartTimeSec 開始輸出並每 SegmentDur 秒強制插入關鍵幀，
	//    故使用 -avoid_negative_ts make_zero 搭配 -segment_format_options output_ts_offset 對齊時間軸。
	if p.Mode == "remux" {
		args = append(args,
			"-copyts",
			"-start_at_zero",
			"-muxdelay", "0",
			"-muxpreload", "0",
			"-f", "segment",
			"-segment_time", strconv.Itoa(p.SegmentDur),
			"-segment_start_number", strconv.Itoa(p.StartSegNumber),
			"-segment_format", "mpegts",
			segPattern,
		)
	} else {
		args = append(args,
			"-avoid_negative_ts", "make_zero",
			"-muxdelay", "0",
			"-muxpreload", "0",
			"-f", "segment",
			"-segment_time", strconv.Itoa(p.SegmentDur),
			"-segment_start_number", strconv.Itoa(p.StartSegNumber),
			"-segment_format", "mpegts",
			"-segment_format_options", fmt.Sprintf("output_ts_offset=%.3f", p.StartTimeSec),
			segPattern,
		)
	}

	return args
}
