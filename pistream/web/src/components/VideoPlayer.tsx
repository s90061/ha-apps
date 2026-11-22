import React, { useEffect, useRef, useState } from 'react';
import Hls from 'hls.js';
import { ArrowLeft, Maximize2, Minimize2, Pause, Play, RotateCcw, RotateCw, Volume1, Volume2, VolumeX } from './Icons';
import { CinemaSelect } from './CinemaSelect';
import { AudioTrack, StreamInfo, SubtitleTrack } from '../types';
import { api, detectEAC3Support, detectHEVCDirectSupport, detectHEVCHlsSupport, resolveUrl } from '../services/api';
import { formatDuration } from './VideoCard';

interface VideoPlayerProps {
  fileId: number;
  onClose: () => void;
}

const VTT_CUE_TIMING_RE =
  /^((?:\d{1,2}:)?\d{2}:\d{2}[.,]\d{1,3})\s*-->\s*((?:\d{1,2}:)?\d{2}:\d{2}[.,]\d{1,3})(.*)$/gm;

function parseVttTimestamp(ts: string): number {
  const parts = ts.trim().replace(',', '.').split(':');
  if (parts.length === 3) {
    return Number(parts[0]) * 3600 + Number(parts[1]) * 60 + Number(parts[2]);
  }
  if (parts.length === 2) {
    return Number(parts[0]) * 60 + Number(parts[1]);
  }
  return Number(ts) || 0;
}

function formatVttTimestamp(totalSec: number): string {
  const clamped = Math.max(0, totalSec);
  const totalMs = Math.round(clamped * 1000);
  const ms = totalMs % 1000;
  const totalWholeSec = Math.floor(totalMs / 1000);
  const sec = totalWholeSec % 60;
  const totalMin = Math.floor(totalWholeSec / 60);
  const min = totalMin % 60;
  const hr = Math.floor(totalMin / 60);
  const pad2 = (n: number) => String(n).padStart(2, '0');
  const pad3 = (n: number) => String(n).padStart(3, '0');
  return `${pad2(hr)}:${pad2(min)}:${pad2(sec)}.${pad3(ms)}`;
}

// 在記憶體中直接平移 WebVTT 時間戳（支援 HH:MM:SS.mmm 與 MM:SS.mmm 兩種格式），
// 避免就地修改瀏覽器活體 VTTCue.startTime/endTime 觸發 Chromium CueTimeline 活躍字幕永久卡死 Bug
function shiftWebVTT(rawVtt: string, offsetSec: number): string {
  if (Math.abs(offsetSec) < 0.001) {
    return rawVtt;
  }
  return rawVtt.replace(VTT_CUE_TIMING_RE, (_match, rawStart: string, rawEnd: string, rest: string) => {
    const startSec = parseVttTimestamp(rawStart);
    const endSec = parseVttTimestamp(rawEnd);
    const shiftedEndRaw = endSec + offsetSec;
    if (shiftedEndRaw <= 0) {
      return `00:00:00.000 --> 00:00:00.000${rest}`;
    }
    const nextStart = Math.max(0, startSec + offsetSec);
    const nextEnd = Math.max(nextStart + 0.05, shiftedEndRaw);
    return `${formatVttTimestamp(nextStart)} --> ${formatVttTimestamp(nextEnd)}${rest}`;
  });
}

interface OriginalCueLayout {
  line: LineAndPositionSetting;
  snapToLines: boolean;
  lineAlign: LineAlignSetting;
}

const supportsVttLineAlign = typeof VTTCue !== 'undefined' && 'lineAlign' in VTTCue.prototype;

export const VideoPlayer: React.FC<VideoPlayerProps> = ({ fileId, onClose }) => {
  const videoRef = useRef<HTMLVideoElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const hlsRef = useRef<Hls | null>(null);
  const sessionRef = useRef<string>('sess_' + Date.now() + '_' + Math.random().toString(36).slice(2, 7));
  const savedTimeRef = useRef<number | null>(null);
  const lastValidTimeRef = useRef<number>(0);

  const [streamInfo, setStreamInfo] = useState<StreamInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [seeking, setSeeking] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  const [quality, setQuality] = useState<'original' | '1080p' | '720p'>('original');
  const [forceH264, setForceH264] = useState(false);
  const [audioTracks, setAudioTracks] = useState<AudioTrack[]>([]);
  const [selectedAudio, setSelectedAudio] = useState<number>(0);

  const [subtitles, setSubtitles] = useState<SubtitleTrack[]>([]);
  const [selectedSub, setSelectedSub] = useState<string>('off');
  const [subtitleOffset, setSubtitleOffset] = useState<number>(0);
  const subtitleOffsetRef = useRef<number>(0);
  const rawVttRef = useRef<string | null>(null);
  const subBlobUrlRef = useRef<string | null>(null);
  const subFetchAbortRef = useRef<AbortController | null>(null);
  const seekSeqRef = useRef<number>(0);
  const explicitSeekRef = useRef<{ target: number; at: number } | null>(null);
  const cueOriginalLayoutRef = useRef<WeakMap<TextTrackCue, OriginalCueLayout>>(new WeakMap());
  const bottomBarVisibleRef = useRef<boolean>(true);
  const [cueLiftPx, setCueLiftPx] = useState<number>(96);
  const [subtitleScale, setSubtitleScale] = useState<number>(() => {
    const saved = Number(localStorage.getItem('pistream_subtitle_scale'));
    return saved >= 75 && saved <= 220 ? saved : 100;
  });

  const handleSubtitleScaleChange = (scale: number) => {
    setSubtitleScale(scale);
    localStorage.setItem('pistream_subtitle_scale', String(scale));
  };

  const clearSubtitleTracks = (video: HTMLVideoElement | null) => {
    if (video) {
      // 先將現存 TextTrack 設為 disabled，強制瀏覽器立刻從 CueTimeline 與 Shadow DOM 卸載，徹底杜絕字幕殘留
      if (video.textTracks) {
        for (let i = 0; i < video.textTracks.length; i++) {
          video.textTracks[i].mode = 'disabled';
        }
      }
      while (video.querySelector('track')) {
        const t = video.querySelector('track');
        if (t) video.removeChild(t);
      }
    }
    if (subBlobUrlRef.current) {
      URL.revokeObjectURL(subBlobUrlRef.current);
      subBlobUrlRef.current = null;
    }
  };

  const syncCuePositionAboveControls = (video: HTMLVideoElement | null, raiseAboveControls: boolean) => {
    if (!video) return;
    const vRect = video.getBoundingClientRect();
    const seekRect = seekBarRef.current?.getBoundingClientRect();
    const clearancePx =
      raiseAboveControls && seekRect && vRect.height > 0 && seekRect.top > vRect.top
        ? Math.max(96, Math.round(vRect.bottom - seekRect.top + 18))
        : 96;

    if (!supportsVttLineAlign) {
      if (raiseAboveControls) {
        setCueLiftPx(clearancePx);
      }
      return;
    }

    if (!video.textTracks || video.textTracks.length === 0) return;
    const videoHeight = vRect.height || window.innerHeight || 720;
    const safeLinePct = Math.max(50, Math.min(90, Math.floor(((videoHeight - clearancePx) / videoHeight) * 100)));

    for (let t = 0; t < video.textTracks.length; t++) {
      const track = video.textTracks[t];
      if (track.mode !== 'showing' || !track.cues || track.cues.length === 0) continue;
      let changed = false;
      for (let i = 0; i < track.cues.length; i++) {
        const cue = track.cues[i] as VTTCue;
        if (!('line' in cue) || !('snapToLines' in cue) || !('lineAlign' in cue)) continue;
        let orig = cueOriginalLayoutRef.current.get(cue);
        if (!orig) {
          orig = {
            line: cue.line,
            snapToLines: cue.snapToLines,
            lineAlign: cue.lineAlign,
          };
          cueOriginalLayoutRef.current.set(cue, orig);
        }
        // 保留明確定位於畫面上半部的字幕（如歌詞／標題註解），只抬升底部對白字幕
        const isTopPinned =
          typeof orig.line === 'number' &&
          ((orig.snapToLines && orig.line >= 0 && orig.line < 5) || (!orig.snapToLines && orig.line < 50));
        if (isTopPinned) continue;

        try {
          if (raiseAboveControls) {
            if (cue.snapToLines !== false || cue.lineAlign !== 'end' || cue.line !== safeLinePct) {
              cue.snapToLines = false;
              cue.lineAlign = 'end';
              cue.line = safeLinePct;
              changed = true;
            }
          } else {
            if (cue.snapToLines !== orig.snapToLines || cue.lineAlign !== orig.lineAlign || cue.line !== orig.line) {
              cue.snapToLines = orig.snapToLines;
              cue.lineAlign = orig.lineAlign;
              cue.line = orig.line;
              changed = true;
            }
          }
        } catch {
          // ignore browser-specific VTTCue layout assignment errors
        }
      }
      // Firefox 變更活躍中 VTTCue.line 後需切換一次 mode 以立即重繪定位
      if (changed && track.activeCues && track.activeCues.length > 0) {
        track.mode = 'hidden';
        track.mode = 'showing';
      }
    }
  };

  const mountSubtitleFromRawVtt = (rawVtt: string, offsetSec: number) => {
    const video = videoRef.current;
    if (!video) return;

    clearSubtitleTracks(video);

    const shiftedVtt = shiftWebVTT(rawVtt, offsetSec);
    const blob = new Blob([shiftedVtt], { type: 'text/vtt;charset=utf-8' });
    const blobUrl = URL.createObjectURL(blob);
    subBlobUrlRef.current = blobUrl;

    const track = document.createElement('track');
    track.kind = 'subtitles';
    track.src = blobUrl;
    track.srclang = 'zh';
    track.label = '字幕';
    track.default = true;
    track.addEventListener(
      'load',
      () => {
        if (track.track) {
          track.track.mode = 'showing';
          syncCuePositionAboveControls(videoRef.current, bottomBarVisibleRef.current);
        }
      },
      { once: true },
    );
    video.appendChild(track);
    if (track.track) {
      track.track.mode = 'showing';
    }
  };

  const adjustSubtitleOffset = (deltaSec: number, reset = false) => {
    const next = reset ? 0 : Math.max(-60, Math.min(60, Number((subtitleOffset + deltaSec).toFixed(1))));
    subtitleOffsetRef.current = next;
    setSubtitleOffset(next);
    if (rawVttRef.current !== null) {
      mountSubtitleFromRawVtt(rawVttRef.current, next);
    }
  };

  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [bufferedEnd, setBufferedEnd] = useState(0);
  const [isPlaying, setIsPlaying] = useState(false);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [controlsVisible, setControlsVisible] = useState(true);
  const [volume, setVolume] = useState<number>(() => {
    const saved = Number(localStorage.getItem('pistream_volume'));
    return Number.isFinite(saved) && saved >= 0 && saved <= 1 ? saved : 1;
  });
  const [isMuted, setIsMuted] = useState<boolean>(() => localStorage.getItem('pistream_muted') === '1');
  const [scrubTime, setScrubTime] = useState<number | null>(null);
  const [hoverTime, setHoverTime] = useState<number | null>(null);
  const [hoverPct, setHoverPct] = useState<number>(0);

  const hideTimerRef = useRef<number | null>(null);
  const hoveringTopBarRef = useRef<boolean>(false);
  const hoveringBottomBarRef = useRef<boolean>(false);
  const isScrubbingRef = useRef<boolean>(false);
  const lastTouchTimeRef = useRef<number>(0);
  const topBarRef = useRef<HTMLDivElement>(null);
  const seekBarRef = useRef<HTMLDivElement>(null);

  // 初始化時還原使用者的音量與靜音偏好
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    video.volume = volume;
    video.muted = isMuted;
  }, []);

  // 1. 取得串流決策（初次進入或切換畫質/音軌時觸發）
  useEffect(() => {
    let isMounted = true;
    const allowHEVCDirect = detectHEVCDirectSupport() && !forceH264;
    const allowHEVCHls = detectHEVCHlsSupport() && !forceH264;
    const allowEAC3 = detectEAC3Support();

    setLoading(true);
    setError(null);

    api
      .getStreamInfo(fileId, sessionRef.current, quality, selectedAudio, allowHEVCDirect, allowHEVCHls, allowEAC3)
      .then((info) => {
        if (!isMounted) return;
        setStreamInfo(info);

        try {
          const parsedAudio = JSON.parse(info.media.audio_tracks_json || '[]');
          setAudioTracks(Array.isArray(parsedAudio) ? parsedAudio : []);
        } catch {
          setAudioTracks([]);
        }

        try {
          const parsedSubs = JSON.parse(info.media.subtitles_json || '[]');
          setSubtitles(Array.isArray(parsedSubs) ? parsedSubs : []);
        } catch {
          setSubtitles([]);
        }
      })
      .catch((err: any) => {
        if (isMounted) setError(err.message || '無法取得串流決策');
      })
      .finally(() => {
        if (isMounted) setLoading(false);
      });

    return () => {
      isMounted = false;
    };
  }, [fileId, quality, selectedAudio, forceH264, reloadKey]);

  // 2. 初始化播放器 (Direct Play HTTP 206 vs HLS 切片串流)
  useEffect(() => {
    const video = videoRef.current;
    if (!video || !streamInfo) return;

    const { decision, progress, media } = streamInfo;
    setDuration(media.duration_seconds || 0);

    // 決定起始播放秒數：優先使用切換畫質/音軌前暫存的秒數，否則使用資料庫上次觀看進度
    let targetStartSec = 0;
    if (savedTimeRef.current !== null && savedTimeRef.current > 0) {
      targetStartSec = savedTimeRef.current;
      savedTimeRef.current = null;
    } else if (progress && progress.position_seconds > 5 && !progress.is_watched) {
      targetStartSec = progress.position_seconds;
    }

    if (hlsRef.current) {
      hlsRef.current.destroy();
      hlsRef.current = null;
    }

    const streamUrl = resolveUrl(decision.stream_url);

    let cleanupSeekingListener: (() => void) | null = null;

    if (decision.mode === 'direct') {
      video.src = streamUrl;
      const onLoaded = () => {
        if (targetStartSec > 0) {
          video.currentTime = targetStartSec;
        }
        video.play().catch(() => {});
      };
      video.addEventListener('loadedmetadata', onLoaded, { once: true });
    } else if (Hls.isSupported()) {
      // 重要：初始化時固定由 startPosition: 0 先載入 seg_00000.ts，確保 hls.js MP4Remuxer 的 _initPTS 鎖定為 0.000s；
      // 待首個切片載入完成後，再於 FRAG_BUFFERED 立即跳轉至 targetStartSec，徹底消除續播時因關鍵幀時間差造成的 initPTS 偏移。
      let pendingInitialSeek: number | null = targetStartSec > 0 ? targetStartSec : null;
      const configuredBufferSec =
        typeof streamInfo.buffer_seconds === 'number' &&
        Number.isFinite(streamInfo.buffer_seconds) &&
        streamInfo.buffer_seconds >= 16 &&
        streamInfo.buffer_seconds <= 120
          ? Math.round(streamInfo.buffer_seconds)
          : 45;

      const hls = new Hls({
        enableWorker: true,
        lowLatencyMode: false,
        startPosition: 0,
        backBufferLength: 30,
        maxBufferLength: configuredBufferSec,
        maxMaxBufferLength: Math.max(60, configuredBufferSec * 2),
        maxBufferHole: 2.5,
        nudgeOffset: 0.25,
        nudgeMaxRetry: 10,
        startFragPrefetch: true,
        fragLoadingTimeOut: 20000,
        fragLoadingMaxRetry: 2,
        manifestLoadingTimeOut: 10000,
      });

      let fatalNetworkRetries = 0;
      let fatalMediaRetries = 0;
      let needRestartLoadOnSeek = false;
      let ignoreNextSeeking = false;
      let lastHlsNudgeAt = 0;
      let lastOutBufferSeekTarget: number | null = null;
      let lastOutBufferSeekAt = 0;
      let lastObservedTime = 0;
      let recoveringUntil = 0;
      let pendingResumeAfterRecovery = false;

      // 當使用者拖拉時間軸至未緩衝區間時，重設所有切片的預期時間為標準切片網格 (sn * segDur)，
      // 消除 remux (-c:v copy) 模式下因關鍵幀長度 > segDur 所造成的 alignSliding 累積切片編號偏移，
      // 同時重設 hls.js Transmuxer（保留 initPTS、清除 lastFrag / isVideoContiguous / nextVideoTs），
      // 防止重啟 FFmpeg 後緊鄰切片 (snDiff === 1) 退回同一個關鍵幀時，MP4Remuxer.remuxVideo 誤判為連續切片而將畫面推遲一整個 GOP。
      const resetFragmentsForOutBufferSeek = () => {
        seekSeqRef.current += 1;
        const seq = seekSeqRef.current;
        const levelDetailsList = [
          hls.levels?.[0]?.details,
          (hls as any).latestLevelDetails,
        ];
        for (const details of levelDetailsList) {
          if (!details || !Array.isArray(details.fragments)) continue;
          const segDur = typeof details.targetduration === 'number' && details.targetduration > 0 ? details.targetduration : 4;
          for (const f of details.fragments as any[]) {
            if (typeof f.sn === 'number') {
              f.start = f.sn * segDur;
              f.duration = segDur;
              delete f.startPTS;
              delete f.endPTS;
              delete f.startDTS;
              delete f.endDTS;
              if (typeof f.relurl === 'string') {
                f.relurl = f.relurl.replace(/(&seek=\d+)?$/, `&seek=${seq}`);
                f._url = null;
              }
            }
          }
        }
        try {
          const sc = (hls as any).streamController;
          // 若首片尚未載完就提前跳轉時間軸，預鎖 initPTS=0 避免 computeInitPts 因關鍵幀落差產生永久時間軸偏移
          if (sc && Array.isArray(sc.initPTS) && !sc.initPTS[0]) {
            sc.initPTS[0] = { baseTime: 0, timescale: 90000, trackId: -1 };
          }
          sc?.resetTransmuxer?.();
          if (sc) {
            sc.fragPrevious = null;
          }
          sc?.fragmentTracker?.removeAllFragments?.();
        } catch {
          // ignore internal hls.js tracker/transmuxer cleanup errors
        }
      };

      const onVideoTimeTrack = () => {
        if (!video.seeking && Number.isFinite(video.currentTime)) {
          lastObservedTime = video.currentTime;
        }
      };

      const onVideoSeeking = () => {
        const cur = video.currentTime;
        const now = Date.now();
        if (ignoreNextSeeking) {
          ignoreNextSeeking = false;
          lastObservedTime = cur;
          if (needRestartLoadOnSeek && hlsRef.current) {
            needRestartLoadOnSeek = false;
            hlsRef.current.startLoad(cur);
          }
          return;
        }

        // 僅將來自 PiStream 介面（時間軸、±10s、方向鍵、續播定位）的跳轉標記為主動跳轉
        let isExplicitUserSeek = false;
        const pendingExplicit = explicitSeekRef.current;
        if (pendingExplicit && now - pendingExplicit.at < 2000 && Math.abs(cur - pendingExplicit.target) <= 1.5) {
          isExplicitUserSeek = true;
          explicitSeekRef.current = null;
        }

        if (pendingInitialSeek !== null && Math.abs(cur - pendingInitialSeek) > 1.5) {
          pendingInitialSeek = null;
        }
        const buffered = video.buffered;
        let inBuffer = false;
        for (let i = 0; i < buffered.length; i++) {
          if (cur >= buffered.start(i) - 0.15 && cur <= buffered.end(i) + 0.15) {
            inBuffer = true;
            break;
          }
        }
        if (!inBuffer) {
          const isNearActiveSeek =
            lastOutBufferSeekTarget !== null &&
            now - lastOutBufferSeekAt < 3500 &&
            cur >= lastOutBufferSeekTarget - 0.5 &&
            cur <= lastOutBufferSeekTarget + 2.0;

          if (isExplicitUserSeek) {
            if (!isNearActiveSeek) {
              lastOutBufferSeekTarget = cur;
              lastOutBufferSeekAt = now;
              resetFragmentsForOutBufferSeek();
            }
          } else {
            // 非主動跳轉：阻擋 recoverMediaError() 重載定位，以及 hls.js GapController 緩衝卡頓微調
            // （_tryNudgeBuffer / _trySkipBufferHole 會先同步寫入 media.currentTime 才發出 ERROR 事件），
            // 避免緩衝不足時誤觸發 seekSeq 遞增並強制重啟健康運作中的 FFmpeg，造成畫面閃跳與影音字幕不同步。
            const isRecovering = now < recoveringUntil;
            const isRecentHlsNudge = now - lastHlsNudgeAt < 2500;
            const isSmallForwardNudge = cur >= lastObservedTime - 0.5 && cur <= lastObservedTime + 6.0;
            if (!isRecovering && !isRecentHlsNudge && !isSmallForwardNudge && !isNearActiveSeek) {
              // 保留畫中畫 (PiP) 或作業系統媒體列遠距拖拉的跨緩衝跳轉支援
              lastOutBufferSeekTarget = cur;
              lastOutBufferSeekAt = now;
              resetFragmentsForOutBufferSeek();
            }
          }
        }
        lastObservedTime = cur;
        if (needRestartLoadOnSeek && hlsRef.current) {
          needRestartLoadOnSeek = false;
          hlsRef.current.startLoad(video.currentTime);
        }
      };
      // 使用 capture: true 確保在 hls.js 內部的 onMediaSeeking 選取切片前先完成網格校正與 Transmuxer 重設
      video.addEventListener('seeking', onVideoSeeking, { capture: true });
      video.addEventListener('timeupdate', onVideoTimeTrack);
      video.addEventListener('seeked', onVideoTimeTrack);
      video.addEventListener('loadedmetadata', onVideoTimeTrack);
      cleanupSeekingListener = () => {
        video.removeEventListener('seeking', onVideoSeeking, { capture: true });
        video.removeEventListener('timeupdate', onVideoTimeTrack);
        video.removeEventListener('seeked', onVideoTimeTrack);
        video.removeEventListener('loadedmetadata', onVideoTimeTrack);
      };

      hls.loadSource(streamUrl);
      hls.attachMedia(video);
      hlsRef.current = hls;

      hls.on(Hls.Events.MANIFEST_PARSED, () => {
        video.play().catch(() => {});
      });

      hls.on(Hls.Events.FRAG_LOADED, () => {
        fatalNetworkRetries = 0;
        fatalMediaRetries = 0;
        needRestartLoadOnSeek = false;
      });

      hls.on(Hls.Events.FRAG_BUFFERED, () => {
        if (pendingInitialSeek !== null) {
          const seekTarget = pendingInitialSeek;
          pendingInitialSeek = null;
          explicitSeekRef.current = { target: seekTarget, at: Date.now() };
          video.currentTime = seekTarget;
          video.play().catch(() => {});
          return;
        }
        if (pendingResumeAfterRecovery) {
          pendingResumeAfterRecovery = false;
          video.play().catch(() => {});
        }
        if (video.paused && !video.seeking) return;
        const buffered = video.buffered;
        const cur = video.currentTime;
        let inBuffer = false;
        let nearestStart: number | null = null;
        for (let i = 0; i < buffered.length; i++) {
          const s = buffered.start(i);
          const e = buffered.end(i);
          if (cur >= s - 0.1 && cur <= e) {
            inBuffer = true;
            break;
          }
          if (s > cur && s - cur <= 1.5 && e > s + 0.1) {
            if (nearestStart === null || s < nearestStart) {
              nearestStart = s;
            }
          }
        }
        if (!inBuffer && nearestStart !== null) {
          ignoreNextSeeking = true;
          video.currentTime = nearestStart + 0.02;
          video.play().catch(() => {});
        }
      });

      hls.on(Hls.Events.ERROR, (_, data) => {
        if (!data.fatal) {
          if (data.details === Hls.ErrorDetails.FRAG_LOAD_ERROR || data.details === Hls.ErrorDetails.FRAG_LOAD_TIMEOUT) {
            needRestartLoadOnSeek = true;
          }
          if (
            data.details === Hls.ErrorDetails.BUFFER_NUDGE_ON_STALL ||
            data.details === Hls.ErrorDetails.BUFFER_SEEK_OVER_HOLE
          ) {
            lastHlsNudgeAt = Date.now();
          }
          if (
            data.details === Hls.ErrorDetails.BUFFER_STALLED_ERROR ||
            data.details === Hls.ErrorDetails.BUFFER_SEEK_OVER_HOLE
          ) {
            const buffered = video.buffered;
            const cur = video.currentTime;
            for (let i = 0; i < buffered.length; i++) {
              const start = buffered.start(i);
              const end = buffered.end(i);
              if (start > cur && start - cur <= 5 && end > start + 0.1) {
                ignoreNextSeeking = true;
                video.currentTime = start + 0.05;
                break;
              }
            }
          }
          return;
        }
        switch (data.type) {
          case Hls.ErrorTypes.NETWORK_ERROR:
            if (fatalNetworkRetries < 2) {
              fatalNetworkRetries += 1;
              hls.startLoad();
            } else {
              hls.destroy();
              setSeeking(false);
              setError('跳轉或串流切片載入失敗，可回到上次播放位置或切換為 1080p 兼容轉碼模式');
            }
            break;
          case Hls.ErrorTypes.MEDIA_ERROR:
            if (fatalMediaRetries < 2) {
              fatalMediaRetries += 1;
              pendingResumeAfterRecovery = !video.paused;
              recoveringUntil = Date.now() + 3000;
              // 觸發 recoverMediaError() 重新掛載 MediaSource 前先還原切片標準時間網格、遞增 seekSeq、
              // 預鎖 initPTS=0 並重設 Transmuxer，確保重載後畫面、聲音與字幕精準同步。
              resetFragmentsForOutBufferSeek();
              hls.recoverMediaError();
            } else if (
              !forceH264 &&
              decision.mode === 'remux' &&
              (media.video_codec.toLowerCase() === 'hevc' || media.video_codec.toLowerCase() === 'h265')
            ) {
              if (video.currentTime > 1) {
                savedTimeRef.current = video.currentTime;
              }
              hls.destroy();
              setForceH264(true);
              setQuality('1080p');
              return;
            } else {
              hls.destroy();
              setSeeking(false);
              setError('串流解碼失敗，請嘗試切換畫質為 1080p 或 720p');
            }
            break;
          default:
            hls.destroy();
            setSeeking(false);
            setError('串流解碼中斷，請嘗試切換畫質為 1080p 或 720p');
            break;
        }
      });
    } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
      // iOS / macOS Safari 原生 HLS
      video.src = streamUrl;
      const onLoaded = () => {
        if (targetStartSec > 0) {
          video.currentTime = targetStartSec;
        }
        video.play().catch(() => {});
      };
      video.addEventListener('loadedmetadata', onLoaded, { once: true });
    } else {
      setError('目前瀏覽器不支援 HLS 串流播放');
    }

    return () => {
      if (cleanupSeekingListener) {
        cleanupSeekingListener();
      }
      if (hlsRef.current) {
        hlsRef.current.destroy();
        hlsRef.current = null;
      }
    };
  }, [streamInfo]);

  // 3. 定時每 5 秒與關閉前自動儲存個人觀看進度
  useEffect(() => {
    if (!streamInfo) return;

    const timer = window.setInterval(() => {
      const video = videoRef.current;
      if (video && !video.paused && video.currentTime > 2) {
        api.reportProgress(fileId, video.currentTime).catch(() => {});
      }
    }, 5000);

    return () => {
      clearInterval(timer);
      const video = videoRef.current;
      if (video && video.currentTime > 2) {
        api.reportProgress(fileId, video.currentTime).catch(() => {});
      }
    };
  }, [fileId, streamInfo]);

  // 3.5 關閉播放器、離開頁面或關閉瀏覽器分頁時，立刻通知後端終止 FFmpeg 轉碼釋放樹莓派 CPU
  useEffect(() => {
    const activeSession = sessionRef.current;
    const handleUnload = () => {
      api.stopStreamBeacon(activeSession);
    };
    window.addEventListener('pagehide', handleUnload);
    window.addEventListener('beforeunload', handleUnload);
    return () => {
      if (subFetchAbortRef.current) {
        subFetchAbortRef.current.abort();
        subFetchAbortRef.current = null;
      }
      rawVttRef.current = null;
      clearSubtitleTracks(videoRef.current);
      window.removeEventListener('pagehide', handleUnload);
      window.removeEventListener('beforeunload', handleUnload);
      api.stopStream(activeSession);
    };
  }, [fileId]);

  // 4. 切換外掛或內嵌字幕（抓回原始 WebVTT 文字快取於記憶體，透過重建 Blob <track> 實現零殘留即時偏移）
  const handleSubtitleChange = (subId: string) => {
    setSelectedSub(subId);
    if (subFetchAbortRef.current) {
      subFetchAbortRef.current.abort();
      subFetchAbortRef.current = null;
    }
    rawVttRef.current = null;
    clearSubtitleTracks(videoRef.current);

    if (subId === 'off') return;

    const controller = new AbortController();
    subFetchAbortRef.current = controller;
    const url = resolveUrl(`/api/stream/${fileId}/subtitles/${encodeURIComponent(subId)}`);

    fetch(url, { credentials: 'include', signal: controller.signal })
      .then((res) => {
        if (!res.ok) throw new Error(`subtitle fetch failed (${res.status})`);
        return res.text();
      })
      .then((text) => {
        if (controller.signal.aborted) return;
        const normalized = text.replace(/\r\n/g, '\n');
        rawVttRef.current = normalized;
        mountSubtitleFromRawVtt(normalized, subtitleOffsetRef.current);
      })
      .catch(() => {
        // ignore abort or fetch error
      });
  };

  // 5. 切換畫質或音軌時保留當前播放秒數
  const handleQualityChange = (newQuality: 'original' | '1080p' | '720p') => {
    if (videoRef.current && videoRef.current.currentTime > 1) {
      savedTimeRef.current = videoRef.current.currentTime;
    }
    setQuality(newQuality);
  };

  const handleAudioChange = (newAudioIdx: number) => {
    if (videoRef.current && videoRef.current.currentTime > 1) {
      savedTimeRef.current = videoRef.current.currentTime;
    }
    setSelectedAudio(newAudioIdx);
  };

  const getEffectiveDuration = () => {
    if (duration > 0) return duration;
    const vDur = videoRef.current?.duration;
    return typeof vDur === 'number' && Number.isFinite(vDur) && vDur > 0 ? vDur : 0;
  };

  const syncBufferedRange = (video: HTMLVideoElement | null) => {
    if (!video) return;
    const buffered = video.buffered;
    const cur = video.currentTime;
    let end = 0;
    for (let i = 0; i < buffered.length; i++) {
      const s = buffered.start(i);
      const e = buffered.end(i);
      if (cur >= s - 1 && cur <= e + 1) {
        end = Math.max(end, e);
      } else if (end === 0 && e > cur && s <= cur + 2) {
        end = e;
      }
    }
    setBufferedEnd(end);
  };

  const seekTo = (targetSec: number) => {
    const video = videoRef.current;
    if (!video) return;
    const maxDur = getEffectiveDuration();
    const clamped = maxDur > 0 ? Math.max(0, Math.min(maxDur, targetSec)) : Math.max(0, targetSec);
    if (Math.abs(video.currentTime - clamped) > 0.05) {
      explicitSeekRef.current = { target: clamped, at: Date.now() };
    }
    video.currentTime = clamped;
    setCurrentTime(clamped);
  };

  const seekRelative = (deltaSec: number) => {
    const video = videoRef.current;
    if (!video) return;
    seekTo(video.currentTime + deltaSec);
  };

  const togglePlayPause = () => {
    const video = videoRef.current;
    if (!video) return;
    if (video.paused) {
      video.play().catch(() => {});
    } else {
      video.pause();
    }
  };

  const applyVolume = (nextVol: number) => {
    const video = videoRef.current;
    const clamped = Math.max(0, Math.min(1, Number(nextVol.toFixed(2))));
    const nextMuted = clamped === 0;
    setVolume(clamped);
    setIsMuted(nextMuted);
    localStorage.setItem('pistream_volume', String(clamped));
    localStorage.setItem('pistream_muted', nextMuted ? '1' : '0');
    if (video) {
      video.volume = clamped;
      video.muted = nextMuted;
    }
  };

  const toggleMute = () => {
    const video = videoRef.current;
    const nextMuted = !(video ? video.muted : isMuted);
    if (!nextMuted && (video ? video.volume : volume) === 0) {
      applyVolume(0.5);
      return;
    }
    setIsMuted(nextMuted);
    localStorage.setItem('pistream_muted', nextMuted ? '1' : '0');
    if (video) {
      video.muted = nextMuted;
    }
  };

  const isDropdownOpen = () => Boolean(topBarRef.current?.querySelector('[aria-expanded="true"]'));

  const clearHideTimer = () => {
    if (hideTimerRef.current !== null) {
      window.clearTimeout(hideTimerRef.current);
      hideTimerRef.current = null;
    }
  };

  const scheduleAutoHide = () => {
    clearHideTimer();
    if (
      !isPlaying ||
      loading ||
      error ||
      hoveringTopBarRef.current ||
      hoveringBottomBarRef.current ||
      isScrubbingRef.current ||
      isDropdownOpen()
    ) {
      return;
    }
    hideTimerRef.current = window.setTimeout(() => {
      if (!hoveringTopBarRef.current && !hoveringBottomBarRef.current && !isScrubbingRef.current && !isDropdownOpen()) {
        setControlsVisible(false);
      }
    }, 2500);
  };

  const revealControls = () => {
    setControlsVisible(true);
    scheduleAutoHide();
  };

  const calcSeekTimeFromClientX = (clientX: number): { sec: number; pct: number } => {
    const bar = seekBarRef.current;
    const maxDur = getEffectiveDuration();
    if (!bar || maxDur <= 0) return { sec: 0, pct: 0 };
    const rect = bar.getBoundingClientRect();
    if (rect.width <= 0) return { sec: 0, pct: 0 };
    const ratio = Math.max(0, Math.min(1, (clientX - rect.left) / rect.width));
    return { sec: ratio * maxDur, pct: ratio * 100 };
  };

  const handleSeekPointerDown = (e: React.PointerEvent<HTMLDivElement>) => {
    if (e.button !== 0 && e.pointerType === 'mouse') return;
    e.preventDefault();
    e.stopPropagation();
    e.currentTarget.setPointerCapture(e.pointerId);
    isScrubbingRef.current = true;
    clearHideTimer();
    setControlsVisible(true);
    const { sec, pct } = calcSeekTimeFromClientX(e.clientX);
    setScrubTime(sec);
    setHoverTime(sec);
    setHoverPct(pct);
  };

  const handleSeekPointerMove = (e: React.PointerEvent<HTMLDivElement>) => {
    const { sec, pct } = calcSeekTimeFromClientX(e.clientX);
    setHoverTime(sec);
    setHoverPct(pct);
    if (isScrubbingRef.current) {
      setScrubTime(sec);
    }
  };

  const handleSeekPointerUp = (e: React.PointerEvent<HTMLDivElement>) => {
    if (!isScrubbingRef.current) return;
    e.stopPropagation();
    isScrubbingRef.current = false;
    const { sec } = calcSeekTimeFromClientX(e.clientX);
    setScrubTime(null);
    seekTo(sec);
    if (e.currentTarget.hasPointerCapture(e.pointerId)) {
      e.currentTarget.releasePointerCapture(e.pointerId);
    }
    scheduleAutoHide();
  };

  const handleSeekPointerCancel = () => {
    if (!isScrubbingRef.current) return;
    isScrubbingRef.current = false;
    setScrubTime(null);
    scheduleAutoHide();
  };

  const handleVideoTouchStart = () => {
    lastTouchTimeRef.current = Date.now();
  };

  const handleVideoClick = () => {
    if (Date.now() - lastTouchTimeRef.current < 500) {
      if (controlsVisible && isPlaying) {
        clearHideTimer();
        setControlsVisible(false);
      } else {
        revealControls();
      }
      return;
    }
    revealControls();
    togglePlayPause();
  };

  const handleVideoDoubleClick = () => {
    if (Date.now() - lastTouchTimeRef.current < 500) return;
    toggleFullscreen();
  };

  // 監聽系統全螢幕狀態變更（包含按 Esc 退出全螢幕）
  useEffect(() => {
    const handleFsChange = () => {
      const fs = Boolean(document.fullscreenElement);
      setIsFullscreen(fs);
    };
    document.addEventListener('fullscreenchange', handleFsChange);
    return () => {
      document.removeEventListener('fullscreenchange', handleFsChange);
      clearHideTimer();
    };
  }, []);

  // 播放中（含全視窗播放器、Home Assistant Ingress 與系統全螢幕）無操作 2.5 秒自動隱藏上下控制列；暫停或載入時保持顯示
  useEffect(() => {
    if (isPlaying && !loading && !error) {
      scheduleAutoHide();
    } else {
      clearHideTimer();
      setControlsVisible(true);
    }
    return () => clearHideTimer();
  }, [isFullscreen, isPlaying, loading, error]);

  const toggleFullscreen = () => {
    const container = containerRef.current;
    const video = videoRef.current as (HTMLVideoElement & { webkitEnterFullscreen?: () => void }) | null;
    if (!document.fullscreenElement) {
      if (container?.requestFullscreen) {
        container
          .requestFullscreen()
          .then(() => setIsFullscreen(true))
          .catch(() => {
            video?.webkitEnterFullscreen?.();
          });
      } else {
        video?.webkitEnterFullscreen?.();
      }
    } else {
      document
        .exitFullscreen()
        .then(() => setIsFullscreen(false))
        .catch(() => {});
    }
  };

  // 6. 鍵盤快捷鍵支援 (Space, 左右方向鍵快轉, 上下方向鍵音量, F, M, Esc)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const video = videoRef.current;
      if (!video) return;
      const targetTag = (e.target as HTMLElement | null)?.tagName;
      const isRangeInput = targetTag === 'INPUT';

      if (e.code === 'Space') {
        e.preventDefault();
        revealControls();
        togglePlayPause();
      } else if (e.code === 'ArrowRight' && !isRangeInput) {
        e.preventDefault();
        revealControls();
        seekRelative(10);
      } else if (e.code === 'ArrowLeft' && !isRangeInput) {
        e.preventDefault();
        revealControls();
        seekRelative(-10);
      } else if (e.code === 'ArrowUp' && !isRangeInput) {
        e.preventDefault();
        revealControls();
        applyVolume((video.muted ? 0 : video.volume) + 0.05);
      } else if (e.code === 'ArrowDown' && !isRangeInput) {
        e.preventDefault();
        revealControls();
        applyVolume((video.muted ? 0 : video.volume) - 0.05);
      } else if (e.code === 'KeyF') {
        e.preventDefault();
        toggleFullscreen();
      } else if (e.code === 'KeyM') {
        e.preventDefault();
        revealControls();
        toggleMute();
      } else if (e.code === 'Escape') {
        if (!document.fullscreenElement) {
          onClose();
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [duration, volume, isMuted, onClose, isFullscreen, isPlaying, loading, error]);

  const modeLabelMap: Record<string, string> = {
    direct: 'Direct Play',
    remux: 'Remux',
    transcode: 'HLS 轉碼',
  };

  const formatSubtitleOptionLabel = (sub: SubtitleTrack): string => {
    const rawTitle = (sub.title || '').trim();
    if (sub.is_external) {
      // 後端掃描器已可能帶有「外掛 (xxx.srt)」，避免重複前綴並簡化與片名重疊的長檔名
      const cleaned = rawTitle.replace(/^外掛\s*\((.+)\)$/, '$1');
      const mediaTitle = streamInfo?.media.title || '';
      const shortName =
        mediaTitle && cleaned.toLowerCase().startsWith(mediaTitle.toLowerCase())
          ? cleaned.slice(mediaTitle.length).replace(/^[._\-\s]+/, '') || cleaned
          : cleaned || `字幕 ${sub.index + 1}`;
      return `外掛：${shortName}`;
    }
    const base = rawTitle || `字幕 ${sub.index + 1}`;
    const lang = sub.language || sub.codec;
    return lang ? `內嵌：${base}（${lang}）` : `內嵌：${base}`;
  };

  const effectiveDuration = getEffectiveDuration();
  const displayTime = scrubTime !== null ? scrubTime : currentTime;
  const progressPct = effectiveDuration > 0 ? Math.max(0, Math.min(100, (displayTime / effectiveDuration) * 100)) : 0;
  const bufferedPct = effectiveDuration > 0 ? Math.max(0, Math.min(100, (bufferedEnd / effectiveDuration) * 100)) : 0;
  const effectiveVolume = isMuted ? 0 : volume;
  const volumePct = Math.round(effectiveVolume * 100);
  const hideControls = isPlaying && !loading && !error && !controlsVisible && scrubTime === null;
  const bottomBarVisible = !error && !hideControls;

  // 當底部控制列顯示／隱藏、或視窗與全螢幕尺寸改變時，自動將字幕上移至時間軸上方避免遮擋，隱藏時復位回底部
  useEffect(() => {
    bottomBarVisibleRef.current = bottomBarVisible;
    syncCuePositionAboveControls(videoRef.current, bottomBarVisible);
    if (!bottomBarVisible) return;
    const handleResize = () => syncCuePositionAboveControls(videoRef.current, true);
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, [bottomBarVisible, isFullscreen, selectedSub]);

  return (
    <div
      ref={containerRef}
      onMouseMove={revealControls}
      className={`fixed inset-0 z-50 bg-black flex flex-col items-center justify-center select-none ${
        hideControls ? 'cursor-none' : ''
      }`}
    >
      {/* Top Control Bar */}
      <div
        ref={topBarRef}
        onMouseEnter={() => {
          if (Date.now() - lastTouchTimeRef.current < 500) return;
          hoveringTopBarRef.current = true;
          clearHideTimer();
          setControlsVisible(true);
        }}
        onMouseLeave={() => {
          hoveringTopBarRef.current = false;
          scheduleAutoHide();
        }}
        onTouchStart={() => {
          lastTouchTimeRef.current = Date.now();
          hoveringTopBarRef.current = false;
          revealControls();
        }}
        className={`absolute top-0 left-0 right-0 z-20 px-4 pt-3 pb-8 sm:px-6 sm:pt-4 sm:pb-10 bg-gradient-to-b from-[#0f0f10]/95 via-[#0f0f10]/75 to-transparent flex flex-wrap items-center justify-between gap-3 transition-[opacity,transform] duration-200 ease-cinema ${
          hideControls ? 'opacity-0 -translate-y-2 pointer-events-none' : 'opacity-100 translate-y-0'
        }`}
      >
        <div className="flex items-center gap-3 min-w-0">
          <button
            type="button"
            onClick={onClose}
            className="w-9 h-9 rounded-lg bg-surface/95 hover:bg-raised text-ink-primary flex items-center justify-center border border-border flex-none cinema-focus transition-colors duration-150 ease-cinema"
            title="返回媒體庫（Esc）"
            aria-label="返回媒體庫"
          >
            <ArrowLeft size={16} strokeWidth={1.5} />
          </button>

          {streamInfo && (
            <div className="min-w-0">
              <h2 className="text-sm sm:text-base font-medium text-ink-primary truncate max-w-[220px] sm:max-w-md leading-snug">
                {streamInfo.media.title}
              </h2>
              <div className="flex items-center gap-3 mt-1 flex-wrap text-xs font-mono tabular-nums">
                <span className="text-ink-primary/85">
                  {modeLabelMap[streamInfo.decision.mode] || streamInfo.decision.mode}
                </span>
                <span className="text-ink-secondary uppercase">
                  {streamInfo.media.video_codec} / {streamInfo.media.audio_codec}
                </span>
                <span className="inline-flex items-center gap-1.5 text-ink-primary">
                  {isPlaying ? (
                    <Play size={12} strokeWidth={1.75} className="text-accent fill-current" />
                  ) : (
                    <Pause size={12} strokeWidth={1.75} className="text-ink-secondary" />
                  )}
                  <span>{formatDuration(currentTime)}</span>
                  <span className="text-ink-muted">/</span>
                  <span className="text-ink-primary/80">{formatDuration(duration)}</span>
                </span>
              </div>
            </div>
          )}
        </div>

        {/* Right Controls: Quick Seek, Quality, Audio Track, Subtitles, Fullscreen */}
        <div className="flex items-center flex-wrap gap-2">
          <button
            type="button"
            onClick={() => seekRelative(-10)}
            className="h-8 px-2.5 rounded-lg bg-surface/95 hover:bg-raised border border-border text-xs text-ink-secondary hover:text-ink-primary font-mono tabular-nums inline-flex items-center gap-1.5 cinema-focus transition-colors duration-150 ease-cinema"
            title="倒退 10 秒（←）"
          >
            <RotateCcw size={13} strokeWidth={1.5} />
            <span>10s</span>
          </button>
          <button
            type="button"
            onClick={() => seekRelative(10)}
            className="h-8 px-2.5 rounded-lg bg-surface/95 hover:bg-raised border border-border text-xs text-ink-secondary hover:text-ink-primary font-mono tabular-nums inline-flex items-center gap-1.5 cinema-focus transition-colors duration-150 ease-cinema"
            title="快轉 10 秒（→）"
          >
            <RotateCw size={13} strokeWidth={1.5} />
            <span>10s</span>
          </button>

          {/* 畫質/頻寬選擇 */}
          <CinemaSelect
            ariaLabel="畫質設定"
            value={quality}
            onChange={(v) => handleQualityChange(v)}
            options={[
              { value: 'original', label: '畫質：原畫' },
              { value: '1080p', label: '畫質：1080p（4 Mbps）' },
              { value: '720p', label: '畫質：720p（2 Mbps）' },
            ]}
            maxWidthClass="max-w-[170px]"
          />

          {/* 多音軌選擇 */}
          {audioTracks.length > 1 && (
            <CinemaSelect
              ariaLabel="音軌選擇"
              value={selectedAudio}
              onChange={(v) => handleAudioChange(v)}
              options={audioTracks.map((a) => ({
                value: a.index,
                label: `音軌 ${a.index + 1} · ${a.language || '預設'} · ${a.title || a.codec.toUpperCase()}`,
              }))}
              maxWidthClass="max-w-[180px]"
            />
          )}

          {/* 內外掛字幕選擇 */}
          {subtitles.length > 0 && (
            <>
              <CinemaSelect
                ariaLabel="字幕軌道"
                value={selectedSub}
                onChange={(v) => handleSubtitleChange(v)}
                options={[
                  { value: 'off', label: '字幕：關閉' },
                  ...subtitles.map((sub) => ({
                    value: sub.id,
                    label: formatSubtitleOptionLabel(sub),
                  })),
                ]}
                maxWidthClass="max-w-[180px]"
              />

              <CinemaSelect
                ariaLabel="字幕大小"
                value={subtitleScale}
                onChange={(v) => handleSubtitleScaleChange(v)}
                options={[
                  { value: 75, label: '字體：75%' },
                  { value: 100, label: '字體：100%' },
                  { value: 125, label: '字體：125%' },
                  { value: 150, label: '字體：150%' },
                  { value: 180, label: '字體：180%' },
                  { value: 220, label: '字體：220%' },
                ]}
                maxWidthClass="max-w-[120px]"
              />

              {selectedSub !== 'off' && (
                <div className="flex items-center h-8 rounded-lg bg-surface/95 border border-border overflow-hidden text-xs font-mono tabular-nums">
                  <button
                    type="button"
                    onClick={() => adjustSubtitleOffset(-0.5)}
                    className="h-full px-2 hover:bg-raised text-ink-secondary hover:text-ink-primary border-r border-border transition-colors duration-150 ease-cinema"
                    title="字幕提前 0.5 秒（字幕太慢時點擊）"
                  >
                    -0.5s
                  </button>
                  <button
                    type="button"
                    onClick={() => adjustSubtitleOffset(0, true)}
                    className={`h-full px-2.5 hover:bg-raised transition-colors duration-150 ease-cinema ${
                      subtitleOffset !== 0 ? 'text-accent font-medium' : 'text-ink-muted'
                    }`}
                    title="點擊重置字幕時間偏移為 0.0 秒"
                  >
                    {subtitleOffset > 0 ? `+${subtitleOffset.toFixed(1)}s` : `${subtitleOffset.toFixed(1)}s`}
                  </button>
                  <button
                    type="button"
                    onClick={() => adjustSubtitleOffset(0.5)}
                    className="h-full px-2 hover:bg-raised text-ink-secondary hover:text-ink-primary border-l border-border transition-colors duration-150 ease-cinema"
                    title="字幕延後 0.5 秒（字幕太快時點擊）"
                  >
                    +0.5s
                  </button>
                </div>
              )}
            </>
          )}

          <button
            type="button"
            onClick={toggleFullscreen}
            className="h-8 px-2.5 rounded-lg bg-surface/95 hover:bg-raised text-xs font-medium text-ink-secondary hover:text-ink-primary border border-border inline-flex items-center gap-1.5 cinema-focus transition-colors duration-150 ease-cinema"
            title="全螢幕切換（F）"
          >
            {isFullscreen ? (
              <Minimize2 size={14} strokeWidth={1.5} />
            ) : (
              <Maximize2 size={14} strokeWidth={1.5} />
            )}
            <span className="hidden md:inline">{isFullscreen ? '縮小' : '全螢幕'}</span>
            <kbd className="hidden lg:inline px-1 py-0.5 rounded bg-raised border border-border text-[10px] font-mono text-ink-muted leading-none">
              F
            </kbd>
          </button>
        </div>
      </div>

      {/* Main HTML5 Video Element（關閉原生 controls，由 PiStream 自訂底部控制列，避免右下角重複顯示 CC 與全螢幕圖示） */}
      <video
        ref={videoRef}
        playsInline
        onTouchStart={handleVideoTouchStart}
        onClick={handleVideoClick}
        onDoubleClick={handleVideoDoubleClick}
        style={
          {
            '--pistream-cue-size': `${((1.05 * subtitleScale) / 100).toFixed(2)}em`,
            '--pistream-cue-lift': `-${cueLiftPx}px`,
          } as React.CSSProperties
        }
        className={`w-full h-full object-contain focus:outline-none ${
          !supportsVttLineAlign && bottomBarVisible ? 'pistream-controls-visible' : ''
        }`}
        onLoadedMetadata={() => {
          const v = videoRef.current;
          if (!v) return;
          v.volume = volume;
          v.muted = isMuted;
          if (duration <= 0 && Number.isFinite(v.duration) && v.duration > 0) {
            setDuration(v.duration);
          }
          syncBufferedRange(v);
        }}
        onProgress={() => syncBufferedRange(videoRef.current)}
        onTimeUpdate={() => {
          const v = videoRef.current;
          if (!v) return;
          if (!isScrubbingRef.current) {
            setCurrentTime(v.currentTime);
          }
          syncBufferedRange(v);
          if (!v.seeking && v.readyState >= 3 && v.currentTime > 0) {
            lastValidTimeRef.current = v.currentTime;
          }
        }}
        onVolumeChange={() => {
          const v = videoRef.current;
          if (!v) return;
          setVolume(v.volume);
          setIsMuted(v.muted);
        }}
        onPlay={() => setIsPlaying(true)}
        onPause={() => setIsPlaying(false)}
        onSeeking={() => setSeeking(true)}
        onSeeked={() => setSeeking(false)}
        onWaiting={() => setSeeking(true)}
        onPlaying={() => setSeeking(false)}
        onCanPlay={() => setSeeking(false)}
        onCanPlayThrough={() => setSeeking(false)}
      />

      {/* Custom Bottom Control Bar（僅保留播放/暫停、進度條、時間與音量，不顯示右下角 CC 與全螢幕按鈕） */}
      {!error && (
        <div
          onMouseEnter={() => {
            if (Date.now() - lastTouchTimeRef.current < 500) return;
            hoveringBottomBarRef.current = true;
            clearHideTimer();
            setControlsVisible(true);
          }}
          onMouseLeave={() => {
            hoveringBottomBarRef.current = false;
            setHoverTime(null);
            scheduleAutoHide();
          }}
          onTouchStart={() => {
            lastTouchTimeRef.current = Date.now();
            hoveringBottomBarRef.current = false;
            revealControls();
          }}
          onClick={(e) => e.stopPropagation()}
          className={`absolute bottom-0 left-0 right-0 z-20 px-4 pt-10 pb-3 sm:px-6 sm:pb-4 bg-gradient-to-t from-[#0f0f10]/95 via-[#0f0f10]/75 to-transparent transition-[opacity,transform] duration-200 ease-cinema ${
            hideControls ? 'opacity-0 translate-y-2 pointer-events-none' : 'opacity-100 translate-y-0'
          }`}
        >
          {/* Interactive Timeline / Seek Bar */}
          <div
            ref={seekBarRef}
            role="slider"
            tabIndex={0}
            aria-label="播放進度"
            aria-valuemin={0}
            aria-valuemax={Math.round(effectiveDuration)}
            aria-valuenow={Math.round(displayTime)}
            aria-valuetext={`${formatDuration(displayTime)} / ${formatDuration(effectiveDuration)}`}
            onPointerDown={handleSeekPointerDown}
            onPointerMove={handleSeekPointerMove}
            onPointerUp={handleSeekPointerUp}
            onPointerCancel={handleSeekPointerCancel}
            onPointerLeave={() => {
              if (!isScrubbingRef.current) setHoverTime(null);
            }}
            onKeyDown={(e) => {
              if (e.code === 'Home') {
                e.preventDefault();
                seekTo(0);
              } else if (e.code === 'End') {
                e.preventDefault();
                seekTo(effectiveDuration);
              }
            }}
            className="relative h-5 flex items-center cursor-pointer touch-none group mb-2 cinema-focus rounded"
          >
            {hoverTime !== null && effectiveDuration > 0 && (
              <div
                style={{ left: `${Math.max(4, Math.min(96, hoverPct))}%` }}
                className="absolute -top-7 -translate-x-1/2 px-2 py-0.5 rounded bg-[#0f0f10]/95 border border-border-strong text-[11px] font-mono tabular-nums text-ink-primary pointer-events-none whitespace-nowrap shadow-lg"
              >
                {formatDuration(hoverTime)}
              </div>
            )}

            <div className="relative w-full h-1.5 group-hover:h-2 transition-[height] duration-150 ease-cinema rounded-full bg-white/20 overflow-hidden">
              <div
                style={{ width: `${bufferedPct}%` }}
                className="absolute inset-y-0 left-0 bg-white/30 rounded-full"
              />
              <div
                style={{ width: `${progressPct}%` }}
                className="absolute inset-y-0 left-0 bg-accent rounded-full"
              />
            </div>

            <div
              style={{ left: `${progressPct}%` }}
              className={`absolute top-1/2 -translate-x-1/2 -translate-y-1/2 w-3.5 h-3.5 rounded-full bg-accent border-2 border-white shadow transition-transform duration-150 ease-cinema pointer-events-none ${
                scrubTime !== null ? 'scale-110 opacity-100' : 'scale-0 group-hover:scale-100 opacity-0 group-hover:opacity-100'
              }`}
            />
          </div>

          {/* Bottom Controls Row: Play/Pause, Time, Volume (NO CC / NO Fullscreen) */}
          <div className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-3 min-w-0">
              <button
                type="button"
                onClick={togglePlayPause}
                className="w-9 h-9 rounded-lg bg-surface/95 hover:bg-raised text-ink-primary flex items-center justify-center border border-border flex-none cinema-focus transition-colors duration-150 ease-cinema"
                title={isPlaying ? '暫停（Space）' : '播放（Space）'}
                aria-label={isPlaying ? '暫停' : '播放'}
              >
                {isPlaying ? (
                  <Pause size={16} strokeWidth={1.75} className="fill-current" />
                ) : (
                  <Play size={16} strokeWidth={1.75} className="fill-current translate-x-[1px]" />
                )}
              </button>

              <div className="text-xs sm:text-sm font-mono tabular-nums text-ink-primary whitespace-nowrap">
                <span>{formatDuration(displayTime)}</span>
                <span className="text-ink-muted mx-1.5">/</span>
                <span className="text-ink-secondary">{formatDuration(effectiveDuration)}</span>
              </div>
            </div>

            <div className="flex items-center gap-2 flex-none">
              <button
                type="button"
                onClick={toggleMute}
                className="w-8 h-8 rounded-lg bg-surface/95 hover:bg-raised text-ink-secondary hover:text-ink-primary flex items-center justify-center border border-border cinema-focus transition-colors duration-150 ease-cinema"
                title={effectiveVolume === 0 ? '取消靜音（M）' : '靜音（M）'}
                aria-label={effectiveVolume === 0 ? '取消靜音' : '靜音'}
              >
                {effectiveVolume === 0 ? (
                  <VolumeX size={15} strokeWidth={1.6} />
                ) : effectiveVolume < 0.5 ? (
                  <Volume1 size={15} strokeWidth={1.6} />
                ) : (
                  <Volume2 size={15} strokeWidth={1.6} />
                )}
              </button>
              <input
                type="range"
                min={0}
                max={1}
                step={0.02}
                value={effectiveVolume}
                onChange={(e) => applyVolume(Number(e.target.value))}
                aria-label="音量"
                title={`音量：${volumePct}%`}
                style={{
                  background: `linear-gradient(to right, #e2a336 0%, #e2a336 ${volumePct}%, rgba(255, 255, 255, 0.22) ${volumePct}%, rgba(255, 255, 255, 0.22) 100%)`,
                }}
                className="cinema-range w-20 sm:w-24"
              />
            </div>
          </div>
        </div>
      )}

      {/* Seeking / Loading Spinner */}
      {(loading || seeking) && !error && (
        <div className="absolute inset-0 flex flex-col items-center justify-center bg-black/45 pointer-events-none">
          <div className="w-8 h-8 border-2 border-accent border-t-transparent rounded-full animate-spin mb-3" />
          <p className="text-xs font-medium text-ink-primary bg-surface/90 border border-border px-3 py-1.5 rounded-lg">
            {loading ? '載入串流中' : '定位畫面中'}
          </p>
        </div>
      )}

      {/* Error Overlay */}
      {error && (
        <div className="absolute inset-0 flex flex-col items-center justify-center bg-deepest/95 p-6 text-center">
          <p className="text-negative font-medium text-base mb-2">播放發生錯誤</p>
          <p className="text-sm text-ink-secondary mb-6 max-w-md leading-relaxed">{error}</p>
          <div className="flex flex-wrap items-center justify-center gap-3">
            {lastValidTimeRef.current > 0 && (
              <button
                type="button"
                onClick={() => {
                  savedTimeRef.current = lastValidTimeRef.current;
                  setError(null);
                  setReloadKey((k) => k + 1);
                }}
                className="h-9 px-4 rounded-lg bg-ink-primary hover:bg-white text-deepest font-medium text-xs btn-highlight font-mono tabular-nums"
              >
                回到上次位置（{formatDuration(lastValidTimeRef.current)}）
              </button>
            )}
            {(quality === 'original' || !forceH264) && (
              <button
                type="button"
                onClick={() => {
                  setForceH264(true);
                  handleQualityChange('1080p');
                }}
                className="h-9 px-4 rounded-lg bg-raised hover:bg-hover text-ink-primary font-medium text-xs border border-border-strong"
              >
                切換為 1080p 兼容轉碼
              </button>
            )}
            <button
              type="button"
              onClick={onClose}
              className="h-9 px-4 rounded-lg bg-surface hover:bg-raised text-xs font-medium text-ink-secondary hover:text-ink-primary border border-border"
            >
              返回媒體庫
            </button>
          </div>
        </div>
      )}
    </div>
  );
};
