import React, { useEffect, useRef, useState } from 'react';
import Hls from 'hls.js';
import { AudioTrack, StreamInfo, SubtitleTrack } from '../types';
import { api, detectHEVCSupport, resolveUrl } from '../services/api';
import { formatDuration } from './VideoCard';

interface VideoPlayerProps {
  fileId: number;
  onClose: () => void;
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
  const [audioTracks, setAudioTracks] = useState<AudioTrack[]>([]);
  const [selectedAudio, setSelectedAudio] = useState<number>(0);

  const [subtitles, setSubtitles] = useState<SubtitleTrack[]>([]);
  const [selectedSub, setSelectedSub] = useState<string>('off');
  const [subtitleScale, setSubtitleScale] = useState<number>(() => {
    const saved = Number(localStorage.getItem('pistream_subtitle_scale'));
    return saved >= 75 && saved <= 220 ? saved : 100;
  });

  const handleSubtitleScaleChange = (scale: number) => {
    setSubtitleScale(scale);
    localStorage.setItem('pistream_subtitle_scale', String(scale));
  };

  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [isPlaying, setIsPlaying] = useState(false);
  const [isFullscreen, setIsFullscreen] = useState(false);

  // 1. 取得串流決策（初次進入或切換畫質/音軌時觸發）
  useEffect(() => {
    let isMounted = true;
    const allowHEVC = detectHEVCSupport();

    setLoading(true);
    setError(null);

    api
      .getStreamInfo(fileId, sessionRef.current, quality, selectedAudio, allowHEVC)
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
  }, [fileId, quality, selectedAudio, reloadKey]);

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
      const hls = new Hls({
        enableWorker: true,
        lowLatencyMode: false,
        startPosition: targetStartSec > 0 ? targetStartSec : -1,
        backBufferLength: 20,
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
          if (s > cur && s - cur <= 12 && e > s + 0.1) {
            if (nearestStart === null || s < nearestStart) {
              nearestStart = s;
            }
          }
        }
        if (!inBuffer && nearestStart !== null) {
          video.currentTime = nearestStart + 0.05;
          video.play().catch(() => {});
        }
      });

      const onVideoSeeking = () => {
        if (needRestartLoadOnSeek && hlsRef.current) {
          needRestartLoadOnSeek = false;
          hlsRef.current.startLoad(video.currentTime);
        }
      };
      video.addEventListener('seeking', onVideoSeeking);

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

  // 4. 切換外掛或內嵌字幕
  const handleSubtitleChange = (subId: string) => {
    setSelectedSub(subId);
    const video = videoRef.current;
    if (!video) return;

    while (video.querySelector('track')) {
      const t = video.querySelector('track');
      if (t) video.removeChild(t);
    }

    if (subId !== 'off') {
      const track = document.createElement('track');
      track.kind = 'subtitles';
      track.src = resolveUrl(`/api/stream/${fileId}/subtitles/${encodeURIComponent(subId)}`);
      track.srclang = 'zh';
      track.label = '字幕';
      track.default = true;
      video.appendChild(track);

      setTimeout(() => {
        if (video.textTracks && video.textTracks[0]) {
          video.textTracks[0].mode = 'showing';
        }
      }, 50);
    }
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
        if (video.paused) video.play();
        else video.pause();
      } else if (e.code === 'ArrowRight') {
        e.preventDefault();
        seekRelative(10);
      } else if (e.code === 'ArrowLeft') {
        e.preventDefault();
        seekRelative(-10);
      } else if (e.code === 'KeyF') {
        e.preventDefault();
        toggleFullscreen();
      } else if (e.code === 'KeyM') {
        e.preventDefault();
        video.muted = !video.muted;
      } else if (e.code === 'Escape') {
        if (!document.fullscreenElement) {
          onClose();
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [duration, onClose]);

  const modeLabelMap: Record<string, string> = {
    direct: 'HTTP 206 原生直連 (0% CPU)',
    remux: '免重編碼換殼 (-c:v copy)',
    transcode: '即時切片轉碼 (HLS)',
  };

  return (
    <div
      ref={containerRef}
      className="fixed inset-0 z-50 bg-black flex flex-col items-center justify-center select-none"
    >
      {/* Top Control Bar */}
      <div className="absolute top-0 left-0 right-0 z-20 p-4 sm:p-6 bg-gradient-to-b from-black/85 via-black/45 to-transparent flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3 min-w-0">
          <button
            onClick={onClose}
            className="w-9 h-9 sm:w-10 sm:h-10 rounded-full bg-surface/80 hover:bg-slate-700 text-white flex items-center justify-center border border-white/10 flex-none"
            title="返回媒體庫 (Esc)"
          >
            ←
          </button>

          {streamInfo && (
            <div className="min-w-0">
              <h2 className="text-sm sm:text-base font-bold text-white truncate max-w-xs sm:max-w-lg">
                {streamInfo.media.title}
              </h2>
              <div className="flex items-center gap-2 mt-0.5 flex-wrap">
                <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-sky-500/20 text-sky-300 border border-sky-400/30">
                  {modeLabelMap[streamInfo.decision.mode] || streamInfo.decision.mode}
                </span>
                <span className="text-[11px] text-slate-400 uppercase">
                  {streamInfo.media.video_codec} / {streamInfo.media.audio_codec}
                </span>
                <span className="text-[11px] text-slate-300 font-mono">
                  {isPlaying ? '▶' : '⏸'} {formatDuration(currentTime)} / {formatDuration(duration)}
                </span>
              </div>
            </div>
          )}
        </div>

        {/* Right Controls: Quick Seek, Quality, Audio Track, Subtitles, Fullscreen */}
        <div className="flex items-center flex-wrap gap-2">
          <button
            onClick={() => seekRelative(-10)}
            className="px-2.5 py-1.5 rounded-lg bg-surface/85 hover:bg-slate-700 border border-border text-xs text-slate-200 font-mono"
            title="倒退 10 秒 (←)"
          >
            -10s
          </button>
          <button
            onClick={() => seekRelative(10)}
            className="px-2.5 py-1.5 rounded-lg bg-surface/85 hover:bg-slate-700 border border-border text-xs text-slate-200 font-mono"
            title="快轉 10 秒 (→)"
          >
            +10s
          </button>

          {/* 畫質/頻寬選擇 */}
          <select
            value={quality}
            onChange={(e) => handleQualityChange(e.target.value as 'original' | '1080p' | '720p')}
            className="bg-surface/90 text-slate-200 border border-border rounded-lg px-2.5 py-1.5 text-xs font-medium focus:outline-none focus:border-primary"
          >
            <option value="original">畫質：原畫極速 (Direct/Remux)</option>
            <option value="1080p">畫質：1080p 省流 (4 Mbps)</option>
            <option value="720p">畫質：720p 極省流 (2 Mbps)</option>
          </select>

          {/* 多音軌選擇 */}
          {audioTracks.length > 1 && (
            <select
              value={selectedAudio}
              onChange={(e) => handleAudioChange(parseInt(e.target.value, 10))}
              className="bg-surface/90 text-slate-200 border border-border rounded-lg px-2.5 py-1.5 text-xs font-medium focus:outline-none focus:border-primary"
            >
              {audioTracks.map((a) => (
                <option key={a.index} value={a.index}>
                  音軌 {a.index + 1} [{a.language || '預設'}] {a.title || a.codec.toUpperCase()}
                </option>
              ))}
            </select>
          )}

          {/* 內外掛字幕選擇 */}
          {subtitles.length > 0 && (
            <>
              <select
                value={selectedSub}
                onChange={(e) => handleSubtitleChange(e.target.value)}
                className="bg-surface/90 text-slate-200 border border-border rounded-lg px-2.5 py-1.5 text-xs font-medium focus:outline-none focus:border-primary"
              >
                <option value="off">字幕：關閉</option>
                {subtitles.map((sub) => (
                  <option key={sub.id} value={sub.id}>
                    {sub.is_external ? '📄 ' : '💬 '}
                    {sub.title || `字幕 ${sub.index + 1}`} [{sub.language || sub.codec}]
                  </option>
                ))}
              </select>

              <select
                aria-label="字幕大小"
                value={subtitleScale}
                onChange={(e) => handleSubtitleScaleChange(Number(e.target.value))}
                className="bg-surface/90 text-slate-200 border border-border rounded-lg px-2.5 py-1.5 text-xs font-medium focus:outline-none focus:border-primary"
              >
                <option value={75}>字體：小 (75%)</option>
                <option value={100}>字體：標準 (100%)</option>
                <option value={125}>字體：中大 (125%)</option>
                <option value={150}>字體：大 (150%)</option>
                <option value={180}>字體：特大 (180%)</option>
                <option value={220}>字體：超大 (220%)</option>
              </select>
            </>
          )}

          <button
            onClick={toggleFullscreen}
            className="px-3 py-1.5 rounded-lg bg-surface/90 hover:bg-slate-700 text-xs font-medium text-slate-200 border border-border"
            title="全螢幕切換 (F)"
          >
            {isFullscreen ? '縮小' : '全螢幕'}
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
        <div className="absolute inset-0 flex flex-col items-center justify-center bg-black/40 pointer-events-none">
          <div className="w-11 h-11 border-4 border-sky-400 border-t-transparent rounded-full animate-spin mb-2.5" />
          <p className="text-xs sm:text-sm font-semibold text-slate-200 bg-black/60 px-3 py-1 rounded-full">
            {loading ? '正在載入串流...' : '極速定位關鍵幀中...'}
          </p>
        </div>
      )}

      {/* Error Overlay */}
      {error && (
        <div className="absolute inset-0 flex flex-col items-center justify-center bg-black/85 p-6 text-center">
          <p className="text-red-400 font-bold text-lg mb-2">播放發生錯誤</p>
          <p className="text-sm text-slate-400 mb-6 max-w-md">{error}</p>
          <div className="flex flex-wrap items-center justify-center gap-3">
            {lastValidTimeRef.current > 0 && (
              <button
                onClick={() => {
                  savedTimeRef.current = lastValidTimeRef.current;
                  setError(null);
                  setReloadKey((k) => k + 1);
                }}
                className="px-4 py-2 rounded-xl bg-emerald-500 hover:bg-emerald-400 text-slate-950 font-bold text-xs"
              >
                回到上次播放位置 ({formatDuration(lastValidTimeRef.current)})
              </button>
            )}
            {quality === 'original' && (
              <button
                onClick={() => handleQualityChange('1080p')}
                className="px-4 py-2 rounded-xl bg-sky-500 hover:bg-sky-400 text-slate-950 font-bold text-xs"
              >
                切換為 1080p 兼容轉碼模式
              </button>
            )}
            <button
              onClick={onClose}
              className="px-4 py-2 rounded-xl bg-surface hover:bg-slate-800 text-xs font-medium text-white border border-border"
            >
              返回媒體庫
            </button>
          </div>
        </div>
      )}
    </div>
  );
};
