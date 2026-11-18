package stream

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"pistream/internal/config"
	"pistream/internal/db"
)

type HLSSession struct {
	SessionID      string
	Media          *db.MediaFile
	Mode           string
	Quality        string
	AudioTrack     int
	LastSeekSeq    string
	OutputDir      string
	CurrentHeadSeg int
	LastActive     time.Time
	CancelFunc     context.CancelFunc
	Cmd            *exec.Cmd
	ProcessDone    chan struct{}
	mu             sync.Mutex
}

type HLSManager struct {
	cfg      *config.Config
	sessions map[string]*HLSSession
	mu       sync.RWMutex
}

func NewHLSManager(cfg *config.Config) *HLSManager {
	_ = os.MkdirAll(cfg.TranscodeTempDir, 0755)
	mgr := &HLSManager{
		cfg:      cfg,
		sessions: make(map[string]*HLSSession),
	}
	go mgr.cleanupLoop()
	return mgr
}

// GenerateVirtualPlaylist 瞬間生成完整時間軸的虛擬 VOD M3U8 清單，讓前端可隨時跳轉至影片任意時段
func (m *HLSManager) GenerateVirtualPlaylist(media *db.MediaFile, sessionID, mode, quality string, audioTrack int) string {
	segDur := float64(m.cfg.SegmentDuration)
	totalSegs := int(math.Floor(media.DurationSeconds / segDur))
	rem := media.DurationSeconds - float64(totalSegs)*segDur
	if rem >= 0.5 || totalSegs == 0 {
		totalSegs++
	}
	if totalSegs <= 0 {
		totalSegs = 1
	}

	var buf bytes.Buffer
	buf.WriteString("#EXTM3U\n")
	buf.WriteString("#EXT-X-VERSION:3\n")
	buf.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n")
	buf.WriteString("#EXT-X-INDEPENDENT-SEGMENTS\n")
	buf.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", m.cfg.SegmentDuration))
	buf.WriteString("#EXT-X-MEDIA-SEQUENCE:0\n")

	remaining := media.DurationSeconds
	for i := 0; i < totalSegs; i++ {
		dur := segDur
		if i == totalSegs-1 && remaining > 0 {
			dur = remaining
		}
		buf.WriteString(fmt.Sprintf("#EXTINF:%.3f,\n", dur))
		buf.WriteString(fmt.Sprintf("seg_%05d.ts?session=%s&mode=%s&quality=%s&audio=%d\n",
			i, sessionID, mode, quality, audioTrack))
		remaining -= segDur
	}
	buf.WriteString("#EXT-X-ENDLIST\n")
	return buf.String()
}

func (m *HLSManager) getOrCreateSession(sessionID string, media *db.MediaFile, mode, quality string, audioTrack int) *HLSSession {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sess, exists := m.sessions[sessionID]; exists {
		sess.LastActive = time.Now()
		return sess
	}

	outDir := filepath.Join(m.cfg.TranscodeTempDir, fmt.Sprintf("file_%d_%s", media.ID, sessionID))
	_ = os.MkdirAll(outDir, 0755)

	sess := &HLSSession{
		SessionID:      sessionID,
		Media:          media,
		Mode:           mode,
		Quality:        quality,
		AudioTrack:     audioTrack,
		OutputDir:      outDir,
		CurrentHeadSeg: -100,
		LastActive:     time.Now(),
	}
	m.sessions[sessionID] = sess
	return sess
}

// EnsureSegmentReady 確保請求的切片已就緒；若偵測到使用者拖拉時間軸，立即重啟 FFmpeg 跳轉至新時段
func (m *HLSManager) EnsureSegmentReady(reqCtx context.Context, sessionID string, media *db.MediaFile, mode, quality string, audioTrack, segIndex int, seekSeq string) (string, error) {
	sess := m.getOrCreateSession(sessionID, media, mode, quality, audioTrack)

	targetSegPath := filepath.Join(sess.OutputDir, fmt.Sprintf("seg_%05d.ts", segIndex))
	nextSegPath := filepath.Join(sess.OutputDir, fmt.Sprintf("seg_%05d.ts", segIndex+1))

	sess.mu.Lock()
	sess.LastActive = time.Now()

	// 若切換了畫質或音軌，或前端觸發了跨緩衝區跳轉 (seekSeq 變更)，強制清除舊切片並重啟 FFmpeg
	configChanged := sess.Mode != mode || sess.Quality != quality || sess.AudioTrack != audioTrack
	seekChanged := seekSeq != "" && seekSeq != sess.LastSeekSeq
	if configChanged || seekChanged {
		sess.Mode = mode
		sess.Quality = quality
		sess.AudioTrack = audioTrack
		sess.LastSeekSeq = seekSeq
		m.clearAllSegmentsLocked(sess)
		m.restartFFmpegLocked(sess, segIndex)
	} else {
		maxForwardJump := 6
		if mode == "remux" {
			maxForwardJump = 1
		}
		isSeekJump := sess.CurrentHeadSeg >= 0 && (segIndex < sess.CurrentHeadSeg || (segIndex-sess.CurrentHeadSeg) > maxForwardJump)

		if (!isSeekJump || mode != "remux") && isSegmentComplete(targetSegPath, nextSegPath, sess.ProcessDone) {
			if segIndex > sess.CurrentHeadSeg {
				sess.CurrentHeadSeg = segIndex
				m.pruneOldSegmentsLocked(sess, segIndex)
			}
			sess.mu.Unlock()
			return targetSegPath, nil
		}

		needRestart := sess.Cmd == nil ||
			isProcessExited(sess.ProcessDone) ||
			isSeekJump

		if needRestart {
			m.restartFFmpegLocked(sess, segIndex)
		}
	}

	procDone := sess.ProcessDone
	sess.mu.Unlock()

	timeout := time.After(12 * time.Second)
	ticker := time.NewTicker(35 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-reqCtx.Done():
			return "", reqCtx.Err()
		case <-timeout:
			return "", fmt.Errorf("等待轉碼切片 seg_%05d.ts 逾時", segIndex)
		case <-ticker.C:
			sess.mu.Lock()
			sess.LastActive = time.Now()
			if sess.ProcessDone != procDone {
				sess.mu.Unlock()
				return "", context.Canceled
			}
			if isSegmentComplete(targetSegPath, nextSegPath, procDone) {
				if segIndex > sess.CurrentHeadSeg {
					sess.CurrentHeadSeg = segIndex
				}
				m.pruneOldSegmentsLocked(sess, segIndex)
				sess.mu.Unlock()
				return targetSegPath, nil
			}
			if isProcessExited(procDone) {
				sess.mu.Unlock()
				return "", fmt.Errorf("FFmpeg 已結束但未能產生切片 seg_%05d.ts", segIndex)
			}
			sess.mu.Unlock()
		}
	}
}

func (m *HLSManager) restartFFmpegLocked(sess *HLSSession, startSeg int) {
	if sess.CancelFunc != nil {
		sess.CancelFunc()
		if sess.ProcessDone != nil {
			<-sess.ProcessDone
		}
	}

	// 清理先前跳轉產生的舊切片與內部清單，避免讀到 PTS 不連續或未寫完的殘留檔
	m.clearAllSegmentsLocked(sess)

	ctx, cancel := context.WithCancel(context.Background())
	sess.CancelFunc = cancel
	sess.CurrentHeadSeg = startSeg
	sess.ProcessDone = make(chan struct{})

	startTime := float64(startSeg * m.cfg.SegmentDuration)
	args := BuildFFmpegArgs(TranscodeParams{
		InputFile:      sess.Media.FilePath,
		OutputDir:      sess.OutputDir,
		StartTimeSec:   startTime,
		StartSegNumber: startSeg,
		SegmentDur:     m.cfg.SegmentDuration,
		Mode:           sess.Mode,
		Quality:        sess.Quality,
		AudioTrack:     sess.AudioTrack,
		HwAccel:        m.cfg.HwAccel,
		VaapiDevice:    m.cfg.VaapiDevice,
	})

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	sess.Cmd = cmd

	fmt.Printf("[HLSManager] Session=%s 啟動 FFmpeg (Mode=%s, Quality=%s, Audio=%d, 跳轉至 %.1fs / Seg=%d)\n",
		sess.SessionID, sess.Mode, sess.Quality, sess.AudioTrack, startTime, startSeg)

	if err := cmd.Start(); err != nil {
		fmt.Printf("[HLSManager] 啟動 FFmpeg 失敗: %v\n", err)
		close(sess.ProcessDone)
		return
	}

	go func(c *exec.Cmd, done chan struct{}, cctx context.Context, errBuf *bytes.Buffer) {
		waitErr := c.Wait()
		if waitErr != nil && cctx.Err() == nil {
			errMsg := strings.TrimSpace(errBuf.String())
			if len(errMsg) > 400 {
				errMsg = errMsg[len(errMsg)-400:]
			}
			fmt.Printf("[HLSManager] FFmpeg 異常結束 (%v): %s\n", waitErr, errMsg)
		}
		close(done)
	}(cmd, sess.ProcessDone, ctx, &stderrBuf)
}

// pruneOldSegmentsLocked 滑動視窗回收機制：自動刪除遠離播放頭的舊 .ts 切片，防止 4K 長片塞爆樹莓派 /dev/shm
func (m *HLSManager) pruneOldSegmentsLocked(sess *HLSSession, currentSeg int) {
	entries, err := os.ReadDir(sess.OutputDir)
	if err != nil {
		return
	}
	keep := m.cfg.MaxKeepSegments
	if keep < 4 {
		keep = 10
	}
	minKeep := currentSeg - keep
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "seg_") || !strings.HasSuffix(name, ".ts") {
			continue
		}
		if idx, err := ParseSegmentIndex(name); err == nil {
			if idx < minKeep {
				_ = os.Remove(filepath.Join(sess.OutputDir, name))
			}
		}
	}
}

func (m *HLSManager) clearAllSegmentsLocked(sess *HLSSession) {
	entries, err := os.ReadDir(sess.OutputDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		isSeg, _ := filepath.Match("seg_*.ts", name)
		if isSeg || name == "live_internal.m3u8" {
			_ = os.Remove(filepath.Join(sess.OutputDir, name))
		}
	}
}

func isSegmentComplete(targetPath, nextPath string, procDone chan struct{}) bool {
	info, err := os.Stat(targetPath)
	if err != nil || info.Size() == 0 {
		return false
	}
	if _, err := os.Stat(nextPath); err == nil {
		return true
	}
	if isProcessExited(procDone) {
		return true
	}
	return false
}

func isProcessExited(done chan struct{}) bool {
	if done == nil {
		return true
	}
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func (m *HLSManager) cleanupLoop() {
	ticker := time.NewTicker(25 * time.Second)
	for range ticker.C {
		m.mu.Lock()
		now := time.Now()
		for id, sess := range m.sessions {
			sess.mu.Lock()
			if now.Sub(sess.LastActive) > 60*time.Second {
				fmt.Printf("[HLSManager] 回收閒置 Session: %s\n", id)
				if sess.CancelFunc != nil {
					sess.CancelFunc()
				}
				_ = os.RemoveAll(sess.OutputDir)
				delete(m.sessions, id)
			}
			sess.mu.Unlock()
		}
		m.mu.Unlock()
	}
}

func ParseSegmentIndex(segName string) (int, error) {
	clean := strings.TrimPrefix(segName, "seg_")
	clean = strings.TrimSuffix(clean, ".ts")
	var idx int
	_, err := fmt.Sscanf(clean, "%d", &idx)
	return idx, err
}
