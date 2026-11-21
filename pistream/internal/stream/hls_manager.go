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
	IsPaused       bool
	DisableHwAccel bool
	mu             sync.Mutex
}

type HLSManager struct {
	cfg             *config.Config
	sessions        map[string]*HLSSession
	stoppedSessions map[string]time.Time
	mu              sync.RWMutex
}

func NewHLSManager(cfg *config.Config) *HLSManager {
	_ = os.MkdirAll(cfg.TranscodeTempDir, 0755)
	mgr := &HLSManager{
		cfg:             cfg,
		sessions:        make(map[string]*HLSSession),
		stoppedSessions: make(map[string]time.Time),
	}
	go mgr.throttleLoop()
	go mgr.cleanupLoop()
	return mgr
}

func (m *HLSManager) isSessionStopped(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	m.mu.RLock()
	_, stopped := m.stoppedSessions[sessionID]
	m.mu.RUnlock()
	return stopped
}

// StopSession 主動立即終止指定 Session 的 FFmpeg 轉碼/換殼行程並清除暫存切片。
// 當 tombstone=true 時會短暫標記該 sessionID 為已終止，防止關閉播放器瞬間仍在途中的切片請求重新喚起 FFmpeg。
func (m *HLSManager) StopSession(sessionID string, tombstone bool) bool {
	if sessionID == "" {
		return false
	}

	m.mu.Lock()
	if tombstone {
		m.stoppedSessions[sessionID] = time.Now()
	}
	sess, exists := m.sessions[sessionID]
	if exists {
		delete(m.sessions, sessionID)
	}
	m.mu.Unlock()

	if !exists {
		return false
	}

	sess.mu.Lock()
	fmt.Printf("[HLSManager] 主動停止並回收 Session: %s\n", sessionID)
	m.terminateSessionLocked(sess)
	sess.mu.Unlock()
	return true
}

func (m *HLSManager) terminateSessionLocked(sess *HLSSession) {
	if sess.CancelFunc != nil {
		m.resumeFFmpegLocked(sess)
		sess.CancelFunc()
		if sess.ProcessDone != nil {
			select {
			case <-sess.ProcessDone:
			case <-time.After(2 * time.Second):
			}
		}
		sess.CancelFunc = nil
	}
	if sess.OutputDir != "" {
		_ = os.RemoveAll(sess.OutputDir)
	}
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
	if m.isSessionStopped(sessionID) {
		return "", context.Canceled
	}

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
			// 若前瞻緩衝低於 3 個切片 (12秒)，提前喚醒已暫停的 FFmpeg 繼續補充切片
			if sess.IsPaused {
				lowWaterSeg := filepath.Join(sess.OutputDir, fmt.Sprintf("seg_%05d.ts", sess.CurrentHeadSeg+3))
				lowWaterNext := filepath.Join(sess.OutputDir, fmt.Sprintf("seg_%05d.ts", sess.CurrentHeadSeg+4))
				if !isSegmentComplete(lowWaterSeg, lowWaterNext, sess.ProcessDone) {
					m.resumeFFmpegLocked(sess)
				}
			}
			sess.mu.Unlock()
			return targetSegPath, nil
		}

		needRestart := sess.Cmd == nil ||
			isProcessExited(sess.ProcessDone) ||
			isSeekJump

		if needRestart {
			m.restartFFmpegLocked(sess, segIndex)
		} else if sess.IsPaused {
			m.resumeFFmpegLocked(sess)
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
				if sess.Mode == "transcode" && !sess.DisableHwAccel && m.cfg.HwAccel != "" {
					fmt.Printf("[HLSManager] Session=%s 硬體加速 (%s) 啟動失敗，自動回退至 CPU 軟解轉碼\n", sess.SessionID, m.cfg.HwAccel)
					sess.DisableHwAccel = true
					m.restartFFmpegLocked(sess, segIndex)
					procDone = sess.ProcessDone
					sess.mu.Unlock()
					continue
				}
				sess.mu.Unlock()
				return "", fmt.Errorf("FFmpeg 已結束但未能產生切片 seg_%05d.ts", segIndex)
			}
			sess.mu.Unlock()
		}
	}
}

func (m *HLSManager) restartFFmpegLocked(sess *HLSSession, startSeg int) {
	if sess.CancelFunc != nil {
		m.resumeFFmpegLocked(sess)
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
	sess.IsPaused = false

	hwAccel := m.cfg.HwAccel
	if sess.DisableHwAccel {
		hwAccel = ""
	}

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
		SourceWidth:    sess.Media.Width,
		SourceHeight:   sess.Media.Height,
		SourceCodec:    sess.Media.VideoCodec,
		HwAccel:        hwAccel,
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

func (m *HLSManager) pauseFFmpegLocked(sess *HLSSession) {
	if sess.IsPaused || sess.Cmd == nil || isProcessExited(sess.ProcessDone) {
		return
	}
	if err := suspendProcess(sess.Cmd); err == nil {
		sess.IsPaused = true
	}
}

func (m *HLSManager) resumeFFmpegLocked(sess *HLSSession) {
	if !sess.IsPaused || sess.Cmd == nil || isProcessExited(sess.ProcessDone) {
		sess.IsPaused = false
		return
	}
	_ = resumeProcess(sess.Cmd)
	sess.IsPaused = false
}

// throttleLoop 前瞻緩衝節流迴圈：當 FFmpeg 產生的切片超前播放頭達 6 個 (24秒) 時自動發送 SIGSTOP 暫停轉碼，
// 讓樹莓派 CPU 瞬間降至 0% 並防止未播切片塞滿 /dev/shm 或 /tmp
func (m *HLSManager) throttleLoop() {
	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		m.mu.RLock()
		sessions := make([]*HLSSession, 0, len(m.sessions))
		for _, s := range m.sessions {
			sessions = append(sessions, s)
		}
		m.mu.RUnlock()

		for _, sess := range sessions {
			sess.mu.Lock()
			if !sess.IsPaused && sess.Cmd != nil && !isProcessExited(sess.ProcessDone) && sess.CurrentHeadSeg >= 0 {
				highWaterSeg := filepath.Join(sess.OutputDir, fmt.Sprintf("seg_%05d.ts", sess.CurrentHeadSeg+6))
				highWaterNext := filepath.Join(sess.OutputDir, fmt.Sprintf("seg_%05d.ts", sess.CurrentHeadSeg+7))
				if isSegmentComplete(highWaterSeg, highWaterNext, sess.ProcessDone) {
					m.pauseFFmpegLocked(sess)
				}
			}
			sess.mu.Unlock()
		}
	}
}

func (m *HLSManager) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Second)
	for range ticker.C {
		m.mu.Lock()
		now := time.Now()
		var expired []*HLSSession
		for id, sess := range m.sessions {
			sess.mu.Lock()
			isIdle := now.Sub(sess.LastActive) > 45*time.Second
			sess.mu.Unlock()
			if isIdle {
				fmt.Printf("[HLSManager] 回收閒置 Session: %s\n", id)
				expired = append(expired, sess)
				delete(m.sessions, id)
			}
		}
		for id, stoppedAt := range m.stoppedSessions {
			if now.Sub(stoppedAt) > 2*time.Minute {
				delete(m.stoppedSessions, id)
			}
		}
		m.mu.Unlock()

		for _, sess := range expired {
			sess.mu.Lock()
			m.terminateSessionLocked(sess)
			sess.mu.Unlock()
		}
	}
}

func ParseSegmentIndex(segName string) (int, error) {
	clean := strings.TrimPrefix(segName, "seg_")
	clean = strings.TrimSuffix(clean, ".ts")
	var idx int
	_, err := fmt.Sscanf(clean, "%d", &idx)
	return idx, err
}
