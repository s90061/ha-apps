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

// DecideStreamMode 根據影片容器、視訊/音訊編碼、前端硬解支援度與使用者選擇的畫質，決定最佳串流模式
func DecideStreamMode(media *db.MediaFile, sessionID string, quality string, audioTrack int, allowHEVC bool) DecisionResult {
	if quality == "" {
		quality = "original"
	}
	if audioTrack < 0 {
		audioTrack = 0
	}

	vCodec := strings.ToLower(media.VideoCodec)
	aCodec := strings.ToLower(media.AudioCodec)
	container := strings.ToLower(media.Container)

	// 若使用者主動選擇「原畫極速 (original)」
	if quality == "original" {
		// Tier 1: HTTP 206 Partial Content 原生直連 (0% CPU，中間時段毫秒級跳轉)
		// 條件：相容容器 (MP4/WebM/MOV)、使用預設第一音軌、音訊為瀏覽器原生支援，且視訊為 H.264/VP9/AV1 或瀏覽器支援之 HEVC
		videoSupportedDirect := vCodec == "h264" || vCodec == "avc1" || vCodec == "vp9" || vCodec == "av1" ||
			((vCodec == "hevc" || vCodec == "h265") && allowHEVC)

		if audioTrack == 0 && isDirectPlayContainer(container) && videoSupportedDirect && isBrowserNativeAudio(aCodec) {
			return DecisionResult{
				Mode:       "direct",
				Quality:    "original",
				AudioTrack: 0,
				StreamURL:  fmt.Sprintf("/api/stream/%d/direct", media.ID),
			}
		}

		// Tier 2: Zero-Reencode Remux (-c:v copy 免重編碼極速換殼，僅轉換音軌與容器)
		// 條件：視訊為 H.264（例如 MKV 封裝的 H.264 + AC3/DTS，或多音軌切換）
		if vCodec == "h264" || vCodec == "avc1" {
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

	// Tier 3: 樹莓派 5 輕量極速轉碼 (適用於 MKV HEVC 轉 H.264、老舊編碼，或外網手動切換 1080p / 720p)
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
