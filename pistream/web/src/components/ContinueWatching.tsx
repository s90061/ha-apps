import React from 'react';
import { MediaFile } from '../types';
import { resolveUrl } from '../services/api';
import { formatDuration, getResolutionBadge } from './VideoCard';

interface ContinueWatchingProps {
  items: MediaFile[];
  onPlay: (fileId: number) => void;
  onClearProgress: (fileId: number, e: React.MouseEvent) => void;
}

export const ContinueWatching: React.FC<ContinueWatchingProps> = ({
  items,
  onPlay,
  onClearProgress,
}) => {
  if (!items || items.length === 0) return null;

  return (
    <section className="mb-8">
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-base sm:text-lg font-bold text-white flex items-center gap-2">
          <span className="text-sky-400">⏱</span> 繼續觀看
        </h2>
        <span className="text-xs text-slate-400">自動從上次中斷秒數極速續播</span>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
        {items.map((item) => {
          const pct = item.progress_ratio
            ? Math.min(100, Math.max(2, Math.round(item.progress_ratio * 100)))
            : 0;
          const resBadge = getResolutionBadge(item.width, item.height);

          return (
            <div
              key={item.id}
              onClick={() => onPlay(item.id)}
              className="group relative cursor-pointer rounded-2xl bg-surface border border-border hover:border-sky-400/60 overflow-hidden transition-all"
            >
              <div className="relative aspect-video bg-slate-900">
                <img
                  src={resolveUrl(`/api/thumbs/${item.id}`)}
                  alt={item.title}
                  loading="lazy"
                  className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
                />
                <div className="absolute inset-0 bg-gradient-to-t from-black/90 via-black/30 to-transparent" />

                <button
                  onClick={(e) => onClearProgress(item.id, e)}
                  title="從繼續觀看中移除"
                  className="absolute top-2 right-2 w-7 h-7 rounded-full bg-black/70 hover:bg-red-500/80 text-slate-300 hover:text-white text-xs flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity"
                >
                  ✕
                </button>

                {resBadge && (
                  <span className="absolute top-2 left-2 px-2 py-0.5 rounded text-[10px] font-bold bg-sky-500/90 text-slate-950">
                    {resBadge}
                  </span>
                )}

                <div className="absolute bottom-3 left-3 right-3">
                  <p className="text-xs sm:text-sm font-semibold text-white truncate">
                    {item.title}
                  </p>
                  <div className="flex items-center justify-between text-[11px] text-sky-300 font-mono mt-0.5">
                    <span>
                      續播 {formatDuration(item.position_seconds || 0)}
                    </span>
                    <span className="text-slate-400">
                      / {formatDuration(item.duration_seconds)}
                    </span>
                  </div>
                </div>

                <div className="absolute bottom-0 left-0 right-0 h-1.5 bg-white/20">
                  <div
                    className="h-full bg-sky-400"
                    style={{ width: `${pct}%` }}
                  />
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </section>
  );
};
