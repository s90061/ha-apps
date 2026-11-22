package stream

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"pistream/internal/config"
	"pistream/internal/db"
)

const (
	maxGlobalHLSSessions  = 4
	maxPerUserHLSSessions = 2
)

var (
	ErrInvalidSessionID   = errors.New("無效的串流 Session ID")
	ErrInvalidStreamParam = errors.New("無效的串流模式或畫質參數")
	ErrSegmentOutOfRange  = errors.New("切片編號超出影片範圍")
	ErrTooManySessions    = errors.New("同時串流轉碼數量已達上限，請稍後再試")

	sessionIDRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

func ValidateSessionID(sessionID string) bool {
	return sessionIDRegex.MatchString(sessionID)
}

func ValidateMode(mode string) bool {
	return mode == "remux" || mode == "transcode"
}

func ValidateQuality(quality string) bool {
	return quality == "original" || quality == "1080p" || quality == "720p"
}

func sessionKey(ownerUserID int64, sessionID string) string {
	return fmt.Sprintf("u%d:%s", ownerUserID, sessionID)
}

func isSubpath(baseDir, targetPath string) bool {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return false
	}
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absBase, absTarget)
	if err != nil {
		return false
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return false
	}
	return true
}

func maxSegmentIndex(durationSec float64, segDur int) int {
	if segDur <= 0 {
		segDur = 4
	}
	if durationSec <= 0 {
		return 1
	}
	return int(math.Ceil(durationSec/float64(segDur))) + 2
}

// bufferWatermarks 依設定的前瞻緩衝秒數與切片長度動態計算 FFmpeg 暫停 (highWater) 與喚醒 (lowWater) 的超前切片數。
// 預設 45s / 4s -> lowWater=6 (24s)、highWater=12 (48s)，並將伺服器端單次超前切片上限鎖定在 16 片內以兼顧磁碟與 /dev/shm 容量。
func bufferWatermarks(bufferSec, segDur int) (lowWater, highWater int) {
	if segDur <= 0 {
		segDur = 4
	}
	if bufferSec <= 0 {
		bufferSec = config.DefaultBufferSeconds
	}
	if bufferSec < config.MinBufferSeconds {
		bufferSec = config.MinBufferSeconds
	}
	if bufferSec > config.MaxBufferSeconds {
		bufferSec = config.MaxBufferSeconds
	}

	bufferSegs := int(math.Ceil(float64(bufferSec) / float64(segDur)))
	lowWater = bufferSegs / 2
	if lowWater < 3 {
		lowWater = 3
	}
	if lowWater > 10 {
		lowWater = 10
	}

	highWater = bufferSegs
	if highWater < lowWater+3 {
		highWater = lowWater + 3
	}
	if highWater > 16 {
		highWater = 16
	}
	return lowWater, highWater
}

func (m *HLSManager) watermarks() (lowWater, highWater int) {
	return bufferWatermarks(m.cfg.EffectiveBufferSeconds(), m.cfg.EffectiveSegmentDuration())
}

type HLSSession struct {
	Key            string
	OwnerUserID    int64
	SessionID      string
	Media          *db.MediaFile
	Mode           string
	Quality        string
	AudioTrack     int
	LastSeekSeq    string
	OutputDir      string
	StartSeg       int
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

func (m *HLSManager) isSessionStopped(ownerUserID int64, sessionID string) bool {
	if sessionID == "" {
		return false
	}
	key := sessionKey(ownerUserID, sessionID)
	m.mu.RLock()
	_, stopped := m.stoppedSessions[key]
	m.mu.RUnlock()
	return stopped
}

// StopSession 主動立即終止指定使用者擁有的 Session 之 FFmpeg 行程並清除暫存切片。
// 當 tombstone=true 時會短暫標記該 (userID, sessionID) 為已終止，防止關閉播放器瞬間仍在途中的切片請求重新喚起 FFmpeg。
func (m *HLSManager) StopSession(ownerUserID int64, sessionID string, tombstone bool) bool {
	if !ValidateSessionID(sessionID) {
		return false
	}
	key := sessionKey(ownerUserID, sessionID)

	m.mu.Lock()
	if tombstone {
		m.stoppedSessions[key] = time.Now()
	}
	sess, exists := m.sessions[key]
	if exists {
		delete(m.sessions, key)
	}
	m.mu.Unlock()

	if !exists {
		return false
	}

	sess.mu.Lock()
	fmt.Printf("[HLSManager] 主動停止並回收 Session: %s\n", key)
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
	if sess.OutputDir != "" && isSubpath(m.cfg.TranscodeTempDir, sess.OutputDir) {
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

	escSession := url.QueryEscape(sessionID)
	escMode := url.QueryEscape(mode)
	escQuality := url.QueryEscape(quality)

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
			i, escSession, escMode, escQuality, audioTrack))
		remaining -= segDur
	}
	buf.WriteString("#EXT-X-ENDLIST\n")
	return buf.String()
}

func (m *HLSManager) getOrCreateSession(ownerUserID int64, sessionID string, media *db.MediaFile, mode, quality string, audioTrack int) (*HLSSession, error) {
	if !ValidateSessionID(sessionID) {
		return nil, ErrInvalidSessionID
	}
	key := sessionKey(ownerUserID, sessionID)

	var evicted *HLSSession

	m.mu.Lock()
	if sess, exists := m.sessions[key]; exists {
		sess.LastActive = time.Now()
		m.mu.Unlock()
		return sess, nil
	}

	// 限制單一帳號同時轉碼數：若超過上限則自動回收該帳號最舊的 Session（支援分頁切換不卡死）
	var userSessions []*HLSSession
	for _, s := range m.sessions {
		if s.OwnerUserID == ownerUserID {
			userSessions = append(userSessions, s)
		}
	}
	if len(userSessions) >= maxPerUserHLSSessions {
		oldest := userSessions[0]
		for _, s := range userSessions[1:] {
			if s.LastActive.Before(oldest.LastActive) {
				oldest = s
			}
		}
		delete(m.sessions, oldest.Key)
		evicted = oldest
	}

	if len(m.sessions) >= maxGlobalHLSSessions {
		m.mu.Unlock()
		if evicted != nil {
			evicted.mu.Lock()
			m.terminateSessionLocked(evicted)
			evicted.mu.Unlock()
		}
		return nil, ErrTooManySessions
	}

	outDir := filepath.Join(m.cfg.TranscodeTempDir, fmt.Sprintf("u%d_file_%d_%s", ownerUserID, media.ID, sessionID))
	if !isSubpath(m.cfg.TranscodeTempDir, outDir) {
		m.mu.Unlock()
		return nil, ErrInvalidSessionID
	}
	_ = os.MkdirAll(outDir, 0755)

	sess := &HLSSession{
		Key:            key,
		OwnerUserID:    ownerUserID,
		SessionID:      sessionID,
		Media:          media,
		Mode:           mode,
		Quality:        quality,
		AudioTrack:     audioTrack,
		OutputDir:      outDir,
		StartSeg:       -1,
		CurrentHeadSeg: -100,
		LastActive:     time.Now(),
	}
	m.sessions[key] = sess
	m.mu.Unlock()

	if evicted != nil {
		evicted.mu.Lock()
		fmt.Printf("[HLSManager] 使用者 %d 超過並發上限，自動回收最舊 Session: %s\n", ownerUserID, evicted.Key)
		m.terminateSessionLocked(evicted)
		evicted.mu.Unlock()
	}

	return sess, nil
}

// EnsureSegmentReady 確保請求的切片已就緒；若偵測到使用者拖拉時間軸，立即重啟 FFmpeg 跳轉至新時段
func (m *HLSManager) EnsureSegmentReady(reqCtx context.Context, ownerUserID int64, sessionID string, media *db.MediaFile, mode, quality string, audioTrack, segIndex int, seekSeq string) (string, error) {
	if !ValidateSessionID(sessionID) {
		return "", ErrInvalidSessionID
	}
	if !ValidateMode(mode) || !ValidateQuality(quality) {
		return "", ErrInvalidStreamParam
	}
	if segIndex < 0 || segIndex > maxSegmentIndex(media.DurationSeconds, m.cfg.SegmentDuration) {
		return "", ErrSegmentOutOfRange
	}
	if m.isSessionStopped(ownerUserID, sessionID) {
		return "", context.Canceled
	}

	sess, err := m.getOrCreateSession(ownerUserID, sessionID, media, mode, quality, audioTrack)
	if err != nil {
		return "", err
	}

	targetSegPath := filepath.Join(sess.OutputDir, fmt.Sprintf("seg_%05d.ts", segIndex))
	nextSegPath := filepath.Join(sess.OutputDir, fmt.Sprintf("seg_%05d.ts", segIndex+1))

	sess.mu.Lock()
	sess.LastActive = time.Now()

	// 若切換了畫質或音軌，或前端觸發了跨緩衝區跳轉 (seekSeq 變更)，強制清除舊切片並重啟 FFmpeg；
	// 但若剛跳轉啟動的 FFmpeg 仍在連續輸出且前端緊接著請求同一起點附近緊鄰的切片 (例如 Seg=820 -> Seg=821)，
	// 直接沿用現有 FFmpeg 的連續切片，避免 remux 重啟後退回同一個關鍵幀造成畫面重複閃跳與影音錯位。
	configChanged := sess.Mode != mode || sess.Quality != quality || sess.AudioTrack != audioTrack
	seekChanged := seekSeq != "" && seekSeq != sess.LastSeekSeq
	if seekChanged && !configChanged && canReuseAdjacentSeekLocked(sess, segIndex, targetSegPath, nextSegPath) {
		sess.LastSeekSeq = seekSeq
		seekChanged = false
	}
	if configChanged || seekChanged {
		sess.Mode = mode
		sess.Quality = quality
		sess.AudioTrack = audioTrack
		sess.LastSeekSeq = seekSeq
		m.clearAllSegmentsLocked(sess)
		m.restartFFmpegLocked(sess, segIndex)
	} else {
		lowWater, highWater := m.watermarks()
		maxForwardJump := highWater
		if maxForwardJump < 6 {
			maxForwardJump = 6
		}
		if mode == "remux" {
			maxForwardJump = 1
		}
		isSeekJump := sess.CurrentHeadSeg >= 0 && (segIndex < sess.CurrentHeadSeg || (segIndex-sess.CurrentHeadSeg) > maxForwardJump)

		if (!isSeekJump || mode != "remux") && isSegmentComplete(targetSegPath, nextSegPath, sess.ProcessDone) {
			if segIndex > sess.CurrentHeadSeg {
				sess.CurrentHeadSeg = segIndex
				m.pruneOldSegmentsLocked(sess, segIndex)
			}
			// 若前瞻緩衝低於低水位，提前喚醒已暫停的 FFmpeg 繼續補充切片
			if sess.IsPaused {
				lowWaterSeg := filepath.Join(sess.OutputDir, fmt.Sprintf("seg_%05d.ts", sess.CurrentHeadSeg+lowWater))
				lowWaterNext := filepath.Join(sess.OutputDir, fmt.Sprintf("seg_%05d.ts", sess.CurrentHeadSeg+lowWater+1))
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
	sess.StartSeg = startSeg
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
	stderrPipe, pipeErr := cmd.StderrPipe()
	if pipeErr != nil {
		cmd.Stderr = &stderrBuf
	}
	sess.Cmd = cmd

	hwLabel := hwAccel
	if hwLabel == "" {
		hwLabel = "none"
	}
	codecLabel := sess.Media.VideoCodec
	if codecLabel == "" {
		codecLabel = "unknown"
	}
	fmt.Printf("[HLSManager] Session=%s 啟動 FFmpeg (Mode=%s, Quality=%s, Codec=%s, HwAccel=%s, Audio=%d, 跳轉至 %.1fs / Seg=%d)\n",
		sess.SessionID, sess.Mode, sess.Quality, codecLabel, hwLabel, sess.AudioTrack, startTime, startSeg)

	if err := cmd.Start(); err != nil {
		fmt.Printf("[HLSManager] 啟動 FFmpeg 失敗: %v\n", err)
		close(sess.ProcessDone)
		return
	}

	go func(c *exec.Cmd, done chan struct{}, cctx context.Context, sessionID string, pipe io.ReadCloser, errBuf *bytes.Buffer) {
		var tailLines []string
		if pipe != nil {
			scanner := bufio.NewScanner(pipe)
			scanner.Buffer(make([]byte, 0, 16*1024), 64*1024)
			printedDiag := 0
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}
				tailLines = append(tailLines, line)
				if len(tailLines) > 20 {
					tailLines = tailLines[len(tailLines)-20:]
				}
				if printedDiag < 12 && shouldLogFFmpegDiagnostic(line) {
					fmt.Printf("[HLSManager] Session=%s [ffmpeg] %s\n", sessionID, line)
					printedDiag++
				}
			}
		}

		waitErr := c.Wait()
		if waitErr != nil && cctx.Err() == nil {
			errMsg := strings.TrimSpace(strings.Join(tailLines, " | "))
			if errMsg == "" {
				errMsg = strings.TrimSpace(errBuf.String())
			}
			if len(errMsg) > 400 {
				errMsg = errMsg[len(errMsg)-400:]
			}
			fmt.Printf("[HLSManager] FFmpeg 異常結束 (%v): %s\n", waitErr, errMsg)
		}
		close(done)
	}(cmd, sess.ProcessDone, ctx, sess.SessionID, stderrPipe, &stderrBuf)
}

func shouldLogFFmpegDiagnostic(line string) bool {
	lower := strings.ToLower(line)
	for _, kw := range []string{
		"hwaccel",
		"v4l2",
		"dma_heap",
		"dma-heap",
		"dmabuf",
		"drm_prime",
		"hwcontext_drm",
		"rpi-hevc",
		"cma_alloc",
		"permission denied",
		"operation not permitted",
		"falling back",
		"unable to open",
		"unable to find",
	} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
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

func canReuseAdjacentSeekLocked(sess *HLSSession, segIndex int, targetPath, nextPath string) bool {
	if sess.StartSeg < 0 {
		return false
	}
	if segIndex < sess.StartSeg || segIndex > sess.CurrentHeadSeg+1 || (segIndex-sess.StartSeg) > 2 {
		return false
	}
	if sess.Cmd != nil && !isProcessExited(sess.ProcessDone) {
		return true
	}
	return isSegmentComplete(targetPath, nextPath, sess.ProcessDone)
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

// throttleLoop 前瞻緩衝節流迴圈：當 FFmpeg 產生的切片超前播放頭達高水位 (依 buffer_seconds 動態計算) 時自動發送 SIGSTOP 暫停轉碼，
// 讓樹莓派 CPU 瞬間降至 0% 並防止未播切片塞滿 /dev/shm 或 /tmp
func (m *HLSManager) throttleLoop() {
	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		_, highWater := m.watermarks()
		m.mu.RLock()
		sessions := make([]*HLSSession, 0, len(m.sessions))
		for _, s := range m.sessions {
			sessions = append(sessions, s)
		}
		m.mu.RUnlock()

		for _, sess := range sessions {
			sess.mu.Lock()
			if !sess.IsPaused && sess.Cmd != nil && !isProcessExited(sess.ProcessDone) && sess.CurrentHeadSeg >= 0 {
				highWaterSeg := filepath.Join(sess.OutputDir, fmt.Sprintf("seg_%05d.ts", sess.CurrentHeadSeg+highWater))
				highWaterNext := filepath.Join(sess.OutputDir, fmt.Sprintf("seg_%05d.ts", sess.CurrentHeadSeg+highWater+1))
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
	if !strings.HasPrefix(segName, "seg_") || !strings.HasSuffix(segName, ".ts") {
		return 0, errors.New("無效的切片檔名格式")
	}
	clean := strings.TrimSuffix(strings.TrimPrefix(segName, "seg_"), ".ts")
	if len(clean) == 0 || len(clean) > 6 {
		return 0, errors.New("無效的切片編號長度")
	}
	for i := 0; i < len(clean); i++ {
		if clean[i] < '0' || clean[i] > '9' {
			return 0, errors.New("切片編號僅限數字")
		}
	}
	idx, err := strconv.Atoi(clean)
	if err != nil || idx < 0 {
		return 0, errors.New("無效的切片編號")
	}
	return idx, nil
}
