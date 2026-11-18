package scanner

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"pistream/internal/db"
)

type ffprobeOutput struct {
	Streams []struct {
		Index     int    `json:"index"`
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		Channels  int    `json:"channels"`
		Tags      struct {
			Language string `json:"language"`
			Title    string `json:"title"`
		} `json:"tags"`
	} `json:"streams"`
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
		Size       string `json:"size"`
		BitRate    string `json:"bit_rate"`
	} `json:"format"`
}

type AudioTrack struct {
	OrderIndex int    `json:"index"`
	StreamIdx  int    `json:"stream_idx"`
	Codec      string `json:"codec"`
	Channels   int    `json:"channels"`
	Language   string `json:"language"`
	Title      string `json:"title"`
}

type SubtitleTrack struct {
	ID         string `json:"id"`          // e.g. "emb_0" or "ext_0"
	OrderIndex int    `json:"index"`       // 內嵌字幕順序或外掛索引
	Codec      string `json:"codec"`       // subrip, ass, webvtt, srt...
	Language   string `json:"language"`    // zh, cht, en...
	Title      string `json:"title"`       // 顯示名稱
	IsExternal bool   `json:"is_external"` // 是否為同目錄外掛字幕檔
	ExtPath    string `json:"ext_path,omitempty"`
}

var ValidVideoExts = map[string]bool{
	".mp4": true, ".mkv": true, ".avi": true, ".mov": true, ".ts": true, ".webm": true, ".m4v": true,
}

var ValidSubExts = map[string]bool{
	".srt": true, ".vtt": true, ".ass": true, ".ssa": true,
}

var scanMu sync.Mutex

// ScanDirectory 掃描媒體目錄，建立索引並觸發縮圖擷取
func ScanDirectory(mediaDir, cacheDir string, store *db.Store) (int, error) {
	scanMu.Lock()
	defer scanMu.Unlock()

	absMediaRoot, err := filepath.Abs(mediaDir)
	if err != nil {
		return 0, err
	}

	validPaths := make(map[string]bool)
	count := 0

	err = filepath.Walk(absMediaRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !ValidVideoExts[ext] {
			return nil
		}

		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil
		}
		validPaths[absPath] = true

		if err := ProcessSingleMediaFile(absMediaRoot, absPath, cacheDir, store); err == nil {
			count++
		}
		return nil
	})

	_, _ = store.CleanupMissingFiles(validPaths)
	return count, err
}

// ProcessSingleMediaFile 探測單一影片檔案並配對同目錄外掛字幕
func ProcessSingleMediaFile(absMediaRoot, absFilePath, cacheDir string, store *db.Store) error {
	meta, err := ProbeFile(absMediaRoot, absFilePath)
	if err != nil {
		fmt.Printf("[Scanner] 略過無法解析的檔案 %s: %v\n", absFilePath, err)
		return err
	}

	if err := store.UpsertMediaFile(meta); err != nil {
		return err
	}

	// 背景非阻塞排程產生 16:9 影片預覽縮圖
	QueueThumbnailJob(meta.ID, meta.FilePath, meta.DurationSeconds, cacheDir, store)
	return nil
}

func ProbeFile(absMediaRoot, filePath string) (*db.MediaFile, error) {
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		filePath,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe 執行失敗: %w", err)
	}

	var parsed ffprobeOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, err
	}

	duration, _ := strconv.ParseFloat(parsed.Format.Duration, 64)
	size, _ := strconv.ParseInt(parsed.Format.Size, 10, 64)
	bitrate, _ := strconv.ParseInt(parsed.Format.BitRate, 10, 64)

	videoCodec := "unknown"
	audioCodec := "none"
	width := 0
	height := 0

	var audioTracks []AudioTrack
	var subs []SubtitleTrack
	audioOrder := 0
	subOrder := 0

	for _, st := range parsed.Streams {
		switch st.CodecType {
		case "video":
			if videoCodec == "unknown" {
				videoCodec = strings.ToLower(st.CodecName)
				width = st.Width
				height = st.Height
			}
		case "audio":
			if audioCodec == "none" {
				audioCodec = strings.ToLower(st.CodecName)
			}
			audioTracks = append(audioTracks, AudioTrack{
				OrderIndex: audioOrder,
				StreamIdx:  st.Index,
				Codec:      strings.ToLower(st.CodecName),
				Channels:   st.Channels,
				Language:   st.Tags.Language,
				Title:      st.Tags.Title,
			})
			audioOrder++
		case "subtitle":
			codecLower := strings.ToLower(st.CodecName)
			// 僅收錄可轉為 WebVTT 的文字型字幕 (排除 PGS/DVD 點陣圖字幕避免 FFmpeg 轉文字失敗)
			if codecLower == "hdmv_pgs_subtitle" || codecLower == "dvd_subtitle" || codecLower == "dvb_subtitle" {
				subOrder++
				continue
			}
			subs = append(subs, SubtitleTrack{
				ID:         fmt.Sprintf("emb_%d", subOrder),
				OrderIndex: subOrder,
				Codec:      codecLower,
				Language:   st.Tags.Language,
				Title:      st.Tags.Title,
				IsExternal: false,
			})
			subOrder++
		}
	}

	// 掃描同目錄下的外掛字幕 (.srt / .vtt / .ass / .ssa)
	extSubs := discoverExternalSubtitles(filePath)
	subs = append(extSubs, subs...)

	audioBytes, _ := json.Marshal(audioTracks)
	subsBytes, _ := json.Marshal(subs)

	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filePath)), ".")
	baseName := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))

	relPath, err := filepath.Rel(absMediaRoot, filePath)
	if err != nil {
		relPath = filepath.Base(filePath)
	}
	relPath = filepath.ToSlash(relPath)
	relDir := filepath.ToSlash(filepath.Dir(relPath))
	if relDir == "." {
		relDir = ""
	}

	return &db.MediaFile{
		Title:           baseName,
		FilePath:        filePath,
		RelPath:         relPath,
		RelDir:          relDir,
		FileSize:        size,
		Container:       ext,
		VideoCodec:      videoCodec,
		AudioCodec:      audioCodec,
		Width:           width,
		Height:          height,
		DurationSeconds: duration,
		BitRate:         bitrate,
		AudioTracksJSON: string(audioBytes),
		SubtitlesJSON:   string(subsBytes),
	}, nil
}

// discoverExternalSubtitles 自動尋找與影片同檔名前綴的外掛字幕檔
func discoverExternalSubtitles(videoPath string) []SubtitleTrack {
	dir := filepath.Dir(videoPath)
	videoBase := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	videoBaseLower := strings.ToLower(videoBase)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var result []SubtitleTrack
	extIdx := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		subExt := strings.ToLower(filepath.Ext(name))
		if !ValidSubExts[subExt] {
			continue
		}

		subBase := strings.TrimSuffix(name, filepath.Ext(name))
		subBaseLower := strings.ToLower(subBase)

		// 匹配與影片同名前綴，例如 Movie.srt 或 Movie.zh-TW.srt
		if subBaseLower == videoBaseLower || strings.HasPrefix(subBaseLower, videoBaseLower+".") || strings.HasPrefix(subBaseLower, videoBaseLower+"_") {
			langTag := strings.TrimPrefix(subBase, videoBase)
			langTag = strings.TrimLeft(langTag, "._- ")
			if langTag == "" {
				langTag = "外掛字幕"
			}
			result = append(result, SubtitleTrack{
				ID:         fmt.Sprintf("ext_%d", extIdx),
				OrderIndex: extIdx,
				Codec:      strings.TrimPrefix(subExt, "."),
				Language:   langTag,
				Title:      fmt.Sprintf("外掛 (%s)", name),
				IsExternal: true,
				ExtPath:    filepath.Join(dir, name),
			})
			extIdx++
		}
	}
	return result
}
