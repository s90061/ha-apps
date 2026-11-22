package stream

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"pistream/internal/db"
	"pistream/internal/scanner"
)

const maxSubtitleFileBytes = 10 * 1024 * 1024 // 10 MB 上限，防止異常字幕檔耗盡記憶體

// ServeDirectStream 處理 HTTP 206 Partial Content 零拷貝直連播放（支援毫秒級任意區段拖拉）
func ServeDirectStream(w http.ResponseWriter, r *http.Request, filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "找不到實體媒體檔案", http.StatusNotFound)
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "無法讀取檔案狀態", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, stat.Name(), stat.ModTime(), file)
}

// ServeSubtitleWebVTT 支援外掛字幕 (.srt/.vtt/.ass) 記憶體極速轉換，以及內嵌字幕本地快取提取
func ServeSubtitleWebVTT(w http.ResponseWriter, r *http.Request, media *db.MediaFile, subID string, cacheDir string) {
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")

	var tracks []scanner.SubtitleTrack
	_ = json.Unmarshal([]byte(media.SubtitlesJSON), &tracks)

	var target *scanner.SubtitleTrack
	for i := range tracks {
		if tracks[i].ID == subID || fmt.Sprintf("%d", tracks[i].OrderIndex) == subID {
			target = &tracks[i]
			break
		}
	}

	if target == nil {
		_, _ = w.Write([]byte("WEBVTT\n\n"))
		return
	}

	// 1. 外掛字幕檔：直接讀取並於記憶體內轉為 WebVTT (< 2ms，不啟動 FFmpeg)，並限制最大 10 MB
	if target.IsExternal && target.ExtPath != "" {
		f, err := os.Open(target.ExtPath)
		if err != nil {
			_, _ = w.Write([]byte("WEBVTT\n\n"))
			return
		}
		raw, err := io.ReadAll(io.LimitReader(f, maxSubtitleFileBytes+1))
		_ = f.Close()
		if err != nil || len(raw) > maxSubtitleFileBytes {
			_, _ = w.Write([]byte("WEBVTT\n\n"))
			return
		}
		ext := strings.ToLower(filepath.Ext(target.ExtPath))
		switch ext {
		case ".vtt":
			_, _ = w.Write(raw)
		case ".ass", ".ssa":
			_, _ = w.Write(convertASSToWebVTT(raw))
		default:
			_, _ = w.Write(convertSRTToWebVTT(raw))
		}
		return
	}

	// 2. 內嵌字幕：檢查本地磁碟快取，避免拖拉時間軸或切換字幕時重複全檔掃描外接硬碟
	subCacheDir := filepath.Join(cacheDir, "subs")
	_ = os.MkdirAll(subCacheDir, 0755)
	cacheFile := filepath.Join(subCacheDir, fmt.Sprintf("file_%d_sub_%d.vtt", media.ID, target.OrderIndex))

	if cachedData, err := os.ReadFile(cacheFile); err == nil && len(cachedData) > 6 {
		_, _ = w.Write(cachedData)
		return
	}

	mapTarget := fmt.Sprintf("0:s:%d", target.OrderIndex)
	cmd := exec.CommandContext(r.Context(), "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-i", media.FilePath,
		"-map", mapTarget,
		"-f", "webvtt",
		"pipe:1",
	)
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	if err := cmd.Run(); err == nil && outBuf.Len() > 0 {
		_ = os.WriteFile(cacheFile, outBuf.Bytes(), 0644)
		_, _ = w.Write(outBuf.Bytes())
		return
	}

	_, _ = w.Write([]byte("WEBVTT\n\n"))
}

var srtTimeRegex = regexp.MustCompile(`(\d{2}:\d{2}:\d{2}),(\d{3})\s*-->\s*(\d{2}:\d{2}:\d{2}),(\d{3})`)
var assTagRegex = regexp.MustCompile(`\{[^}]*\}`)

// convertSRTToWebVTT 將 SRT 字幕毫秒級轉換為標準 WebVTT
func convertSRTToWebVTT(srtBytes []byte) []byte {
	// 移除 UTF-8 BOM
	content := strings.TrimPrefix(string(srtBytes), "\xef\xbb\xbf")
	content = strings.ReplaceAll(content, "\r\n", "\n")
	converted := srtTimeRegex.ReplaceAllString(content, "$1.$2 --> $3.$4")
	return []byte("WEBVTT\n\n" + converted)
}

// convertASSToWebVTT 將 ASS/SSA 字幕 Dialogue 行解析並清理特效標籤轉為 WebVTT
func convertASSToWebVTT(assBytes []byte) []byte {
	content := strings.TrimPrefix(string(assBytes), "\xef\xbb\xbf")
	scanner := bufio.NewScanner(strings.NewReader(content))

	var buf bytes.Buffer
	buf.WriteString("WEBVTT\n\n")

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "Dialogue:") {
			continue
		}
		rest := strings.TrimPrefix(line, "Dialogue:")
		parts := strings.SplitN(rest, ",", 10)
		if len(parts) < 10 {
			continue
		}
		start := normalizeASSTime(strings.TrimSpace(parts[1]))
		end := normalizeASSTime(strings.TrimSpace(parts[2]))
		text := parts[9]
		text = assTagRegex.ReplaceAllString(text, "")
		text = strings.ReplaceAll(text, `\N`, "\n")
		text = strings.ReplaceAll(text, `\n`, "\n")
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		buf.WriteString(fmt.Sprintf("%s --> %s\n%s\n\n", start, end, text))
	}
	return buf.Bytes()
}

func normalizeASSTime(t string) string {
	// ASS 時間格式通常為 H:MM:SS.cc (例如 0:01:23.45 -> 00:01:23.450)
	parts := strings.Split(t, ".")
	hms := parts[0]
	cs := "000"
	if len(parts) == 2 {
		cs = parts[1]
		if len(cs) == 2 {
			cs += "0"
		} else if len(cs) == 1 {
			cs += "00"
		}
	}
	hmsParts := strings.Split(hms, ":")
	if len(hmsParts) == 3 && len(hmsParts[0]) == 1 {
		hms = "0" + hms
	}
	return hms + "." + cs
}
