package stream

import (
	"fmt"
	"strings"

	"pistream/internal/db"
)

type DecisionResult struct {
	Mode       string `json:"mode"`        // "direct", "remux", "transcode"
	Quality    string `json:"quality"`     // "original", "1080p", "720p"
	AudioTrack int    `json:"audio_track"` // 音軌順序索引 (0, 1, ...)
	StreamURL  string `json:"stream_url"`
}

func isBrowserNativeAudio(codec string) bool {
	switch strings.ToLower(codec) {
	case "aac", "mp3", "opus", "vorbis", "flac", "none", "":
		return true
	default:
		return false
	}
}

func isDirectPlayContainer(container string) bool {
	switch strings.ToLower(container) {
	case "mp4", "m4v", "webm", "mov":
		return true
	default:
		return false
	}
}

// effectiveBitRate 取得影片有效碼率 (bps)；若容器標頭未記錄 BitRate，則由檔案大小與時長推算
func effectiveBitRate(media *db.MediaFile) int64 {
	if media.BitRate > 0 {
		return media.BitRate
	}
	if media.DurationSeconds > 0 && media.FileSize > 0 {
		return int64(float64(media.FileSize*8) / media.DurationSeconds)
	}
	return 0
}

// DecideStreamMode 根據影片容器、視訊/音訊編碼、前端硬解支援度與使用者選擇的畫質，決定最佳串流模式
func DecideStreamMode(media *db.MediaFile, sessionID string, quality string, audioTrack int, allowHEVC bool) DecisionResult {
	return DecideStreamModeWithCaps(media, sessionID, quality, audioTrack, allowHEVC, allowHEVC)
}

// DecideStreamModeWithCaps 區分瀏覽器原生直連 (Direct Play) 與 HLS MSE (Remux) 的 HEVC 硬解支援度，
// 避免像 Windows Firefox 等支援 MP4 HEVC 直連但不支援 MSE HEVC 的瀏覽器誤退回樹莓派 CPU 轉碼
func DecideStreamModeWithCaps(media *db.MediaFile, sessionID string, quality string, audioTrack int, allowHEVCDirect bool, allowHEVCHLS bool) DecisionResult {
	if quality == "" {
		quality = "original"
	}
	if audioTrack < 0 {
		audioTrack = 0
	}

	vCodec := strings.ToLower(media.VideoCodec)
	aCodec := strings.ToLower(media.AudioCodec)
	container := strings.ToLower(media.Container)

	isH264 := vCodec == "h264" || vCodec == "avc1"
	isHEVC := vCodec == "hevc" || vCodec == "h265"
	videoSupportedDirect := isH264 || vCodec == "vp9" || vCodec == "av1" || (isHEVC && allowHEVCDirect)
	videoSupportedRemux := isH264 || (isHEVC && allowHEVCHLS)

	// 若使用者主動選擇「原畫極速 (original)」
	if quality == "original" {
		// Tier 1: HTTP 206 Partial Content 原生直連 (0% CPU，中間時段毫秒級跳轉)
		// 條件：相容容器 (MP4/WebM/MOV)、使用預設第一音軌、音訊為瀏覽器原生支援，且視訊為 H.264/VP9/AV1 或瀏覽器支援之 HEVC
		if audioTrack == 0 && isDirectPlayContainer(container) && videoSupportedDirect && isBrowserNativeAudio(aCodec) {
			return DecisionResult{
				Mode:       "direct",
				Quality:    "original",
				AudioTrack: 0,
				StreamURL:  fmt.Sprintf("/api/stream/%d/direct", media.ID),
			}
		}

		// Tier 2: Zero-Reencode Remux (-c:v copy 免重編碼極速換殼，僅轉換音軌與容器)
		// 條件：視訊為 H.264，或瀏覽器支援硬解之 HEVC（例如 1080p MKV H.264/HEVC + AC3/DTS/AAC，或多音軌切換）
		if videoSupportedRemux {
			return DecisionResult{
				Mode:       "remux",
				Quality:    "original",
				AudioTrack: audioTrack,
				StreamURL: fmt.Sprintf(
					"/api/stream/%d/hls/index.m3u8?session=%s&mode=remux&quality=original&audio=%d",
					media.ID, sessionID, audioTrack,
				),
			}
		}
	}

	// 智慧 1080p 免重編碼旁路 (Smart 1080p Bypass)：
	// 當使用者選擇 1080p，若原片解析度本來就 <= 1080p (Width <= 1920)、有效碼率 <= 8 Mbps，
	// 且視訊編碼為瀏覽器可直接解碼 (H.264 或支援硬解的 HEVC)，則無需啟動 libx264 消耗 90%+ CPU 二次重編碼，
	// 直接改走 direct 或 remux (-c:v copy)，讓樹莓派 5 CPU 佔用率降至 0% ~ 2%。
	if quality == "1080p" && media.Width > 0 && media.Width <= 1920 && (media.Height == 0 || media.Height <= 1088) {
		bitRate := effectiveBitRate(media)
		if bitRate >= 0 && bitRate <= 8_000_000 {
			if audioTrack == 0 && isDirectPlayContainer(container) && videoSupportedDirect && isBrowserNativeAudio(aCodec) {
				return DecisionResult{
					Mode:       "direct",
					Quality:    "1080p",
					AudioTrack: 0,
					StreamURL:  fmt.Sprintf("/api/stream/%d/direct", media.ID),
				}
			}
			if videoSupportedRemux {
				return DecisionResult{
					Mode:       "remux",
					Quality:    "1080p",
					AudioTrack: audioTrack,
					StreamURL: fmt.Sprintf(
						"/api/stream/%d/hls/index.m3u8?session=%s&mode=remux&quality=1080p&audio=%d",
						media.ID, sessionID, audioTrack,
					),
				}
			}
		}
	}

	// Tier 3: 樹莓派 5 輕量極速轉碼 (適用於不支援 HEVC 瀏覽器上的 MKV HEVC 轉 H.264、4K/高碼率縮放，或手動切換 720p)
	return DecisionResult{
		Mode:       "transcode",
		Quality:    quality,
		AudioTrack: audioTrack,
		StreamURL: fmt.Sprintf(
			"/api/stream/%d/hls/index.m3u8?session=%s&mode=transcode&quality=%s&audio=%d",
			media.ID, sessionID, quality, audioTrack,
		),
	}
}
