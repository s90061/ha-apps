import React, { useEffect, useRef, useState } from 'react';
import Hls from 'hls.js';
import { ArrowLeft, Maximize2, Minimize2, Pause, Play, RotateCcw, RotateCw } from './Icons';
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
  const [isPlaying, setIsPlaying] = useState(false);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [controlsVisible, setControlsVisible] = useState(true);
  const hideTimerRef = useRef<number | null>(null);
  const hoveringTopBarRef = useRef<boolean>(false);
  const topBarRef = useRef<HTMLDivElement>(null);

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

      const hls = new Hls({
        enableWorker: true,
        lowLatencyMode: false,
        startPosition: 0,
        backBufferLength: 60,
        maxBufferLength: 16,
        maxMaxBufferLength: 32,
        maxBufferHole: 2.5,
        nudgeOffset: 0.25,
        nudgeMaxRetry: 10,
        startFragPrefetch: true,
        fragLoadingTimeOut: 20000,
        fragLoadingMaxRetry: 1,
        manifestLoadingTimeOut: 10000,
      });

      let fatalNetworkRetries = 0;
      let fatalMediaRetries = 0;
      let needRestartLoadOnSeek = false;

      // 當使用者拖拉時間軸至未緩衝區間時，重設所有切片的預期時間為標準 4 秒網格 (sn * 4)，
      // 消除 remux (-c:v copy) 模式下因關鍵幀長度 > 4 秒所造成的 alignSliding 累積切片編號偏移，
      // 並附帶遞增的 seek 序號通知後端立即重啟 FFmpeg 定位至正確秒數。
      const resetFragmentsForOutBufferSeek = () => {
        seekSeqRef.current += 1;
        const seq = seekSeqRef.current;
        const levelDetailsList = [
          hls.levels?.[0]?.details,
          (hls as any).latestLevelDetails,
        ];
        for (const details of levelDetailsList) {
          if (!details || !Array.isArray(details.fragments)) continue;
          for (const f of details.fragments as any[]) {
            if (typeof f.sn === 'number') {
              f.start = f.sn * 4;
              f.duration = 4;
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
          (hls as any).streamController?.fragmentTracker?.removeAllFragments?.();
        } catch {
          // ignore internal hls.js tracker cleanup errors
        }
      };

      const onVideoSeeking = () => {
        const cur = video.currentTime;
        const buffered = video.buffered;
        let inBuffer = false;
        for (let i = 0; i < buffered.length; i++) {
          if (cur >= buffered.start(i) - 0.15 && cur <= buffered.end(i) + 0.15) {
            inBuffer = true;
            break;
          }
        }
        if (!inBuffer) {
          resetFragmentsForOutBufferSeek();
        }
        if (needRestartLoadOnSeek && hlsRef.current) {
          needRestartLoadOnSeek = false;
          hlsRef.current.startLoad(video.currentTime);
        }
      };
      // 使用 capture: true 確保在 hls.js 內部的 onMediaSeeking 選取切片前先完成 4 秒網格校正
      video.addEventListener('seeking', onVideoSeeking, { capture: true });
      cleanupSeekingListener = () => {
        video.removeEventListener('seeking', onVideoSeeking, { capture: true });
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
          video.currentTime = seekTarget;
          video.play().catch(() => {});
          return;
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
            data.details === Hls.ErrorDetails.BUFFER_STALLED_ERROR ||
            data.details === Hls.ErrorDetails.BUFFER_SEEK_OVER_HOLE
          ) {
            const buffered = video.buffered;
            const cur = video.currentTime;
            for (let i = 0; i < buffered.length; i++) {
              const start = buffered.start(i);
              const end = buffered.end(i);
              if (start > cur && start - cur <= 5 && end > start + 0.1) {
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

  const seekRelative = (deltaSec: number) => {
    const video = videoRef.current;
    if (!video) return;
    const maxDur = duration || video.duration || 0;
    const nextPos = Math.max(0, Math.min(maxDur, video.currentTime + deltaSec));
    video.currentTime = nextPos;
    setCurrentTime(nextPos);
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
    if (!isPlaying || loading || error || hoveringTopBarRef.current || isDropdownOpen()) {
      return;
    }
    hideTimerRef.current = window.setTimeout(() => {
      if (!hoveringTopBarRef.current && !isDropdownOpen()) {
        setControlsVisible(false);
      }
    }, 2500);
  };

  const revealControls = () => {
    setControlsVisible(true);
    scheduleAutoHide();
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

  // 播放中（含全視窗播放器、Home Assistant Ingress 與系統全螢幕）無操作 2.5 秒自動隱藏頂部資訊與控制列；暫停或載入時保持顯示
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
    if (!containerRef.current) return;
    if (!document.fullscreenElement) {
      containerRef.current
        .requestFullscreen()
        .then(() => setIsFullscreen(true))
        .catch(() => {});
    } else {
      document
        .exitFullscreen()
        .then(() => setIsFullscreen(false))
        .catch(() => {});
    }
  };

  // 6. 鍵盤快捷鍵支援 (Space, 左右方向鍵快轉, F, M, Esc)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const video = videoRef.current;
      if (!video) return;

      if (e.code === 'Space') {
        e.preventDefault();
        revealControls();
        if (video.paused) video.play();
        else video.pause();
      } else if (e.code === 'ArrowRight') {
        e.preventDefault();
        revealControls();
        seekRelative(10);
      } else if (e.code === 'ArrowLeft') {
        e.preventDefault();
        revealControls();
        seekRelative(-10);
      } else if (e.code === 'KeyF') {
        e.preventDefault();
        toggleFullscreen();
      } else if (e.code === 'KeyM') {
        e.preventDefault();
        revealControls();
        video.muted = !video.muted;
      } else if (e.code === 'Escape') {
        if (!document.fullscreenElement) {
          onClose();
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [duration, onClose, isFullscreen, isPlaying, loading, error]);

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

  const hideTopBar = isPlaying && !loading && !error && !controlsVisible;

  return (
    <div
      ref={containerRef}
      onMouseMove={revealControls}
      onTouchStart={revealControls}
      className={`fixed inset-0 z-50 bg-black flex flex-col items-center justify-center select-none ${
        hideTopBar ? 'cursor-none' : ''
      }`}
    >
      {/* Top Control Bar */}
      <div
        ref={topBarRef}
        onMouseEnter={() => {
          hoveringTopBarRef.current = true;
          clearHideTimer();
          setControlsVisible(true);
        }}
        onMouseLeave={() => {
          hoveringTopBarRef.current = false;
          scheduleAutoHide();
        }}
        className={`absolute top-0 left-0 right-0 z-20 px-4 pt-3 pb-8 sm:px-6 sm:pt-4 sm:pb-10 bg-gradient-to-b from-[#0f0f10]/95 via-[#0f0f10]/75 to-transparent flex flex-wrap items-center justify-between gap-3 transition-[opacity,transform] duration-200 ease-cinema ${
          hideTopBar ? 'opacity-0 -translate-y-2 pointer-events-none' : 'opacity-100 translate-y-0'
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

      {/* Main HTML5 Video Element */}
      <video
        ref={videoRef}
        controls
        playsInline
        style={{ '--pistream-cue-size': `${((1.05 * subtitleScale) / 100).toFixed(2)}em` } as React.CSSProperties}
        className="w-full h-full object-contain focus:outline-none"
        onTimeUpdate={() => {
          const v = videoRef.current;
          if (!v) return;
          setCurrentTime(v.currentTime);
          if (!v.seeking && v.readyState >= 3 && v.currentTime > 0) {
            lastValidTimeRef.current = v.currentTime;
          }
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
