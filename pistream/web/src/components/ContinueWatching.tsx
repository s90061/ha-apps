import React from 'react';
import { Play, X } from './Icons';
import { MediaFile } from '../types';
import { resolveUrl } from '../services/api';
import { formatDuration } from './VideoCard';

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
    <section className="mb-10">
      <div className="flex items-baseline justify-between gap-4 mb-4">
        <div className="flex items-baseline gap-2">
          <h2 className="text-base font-medium text-ink-primary">繼續觀看</h2>
          <span className="text-xs font-mono text-ink-muted tabular-nums">
            {items.length}
          </span>
        </div>
        <span className="text-xs text-ink-muted hidden sm:inline">
          從上次中斷處接續播放
        </span>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
        {items.map((item) => {
          const pct = item.progress_ratio
            ? Math.min(100, Math.max(2, Math.round(item.progress_ratio * 100)))
            : 0;

          return (
            <div
              key={item.id}
              onClick={() => onPlay(item.id)}
              role="button"
              tabIndex={0}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  onPlay(item.id);
                }
              }}
              className="group relative cursor-pointer rounded-xl bg-surface border border-border hover:border-border-strong overflow-hidden cinema-focus transition-colors duration-150 ease-cinema"
            >
              <div className="relative aspect-video bg-deepest overflow-hidden">
                <img
                  src={resolveUrl(`/api/thumbs/${item.id}`)}
                  alt={item.title}
                  loading="lazy"
                  className="w-full h-full object-cover"
                />
                <div className="absolute inset-0 bg-gradient-to-t from-[#0f0f10]/90 via-[#0f0f10]/20 to-transparent" />

                <button
                  type="button"
                  onClick={(e) => onClearProgress(item.id, e)}
                  title="從繼續觀看中移除"
                  aria-label="從繼續觀看中移除"
                  className="absolute top-2 right-2 w-7 h-7 rounded-md bg-deepest/80 hover:bg-raised border border-border text-ink-secondary hover:text-ink-primary flex items-center justify-center opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity duration-150 ease-cinema"
                >
                  <X size={14} strokeWidth={1.5} />
                </button>

                <div className="absolute inset-0 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity duration-150 ease-cinema pointer-events-none">
                  <div className="w-10 h-10 rounded-full bg-ink-primary text-deepest flex items-center justify-center">
                    <Play size={16} strokeWidth={1.75} className="translate-x-[1px] fill-current" />
                  </div>
                </div>

                <div className="absolute bottom-3 left-3 right-3">
                  <p className="text-sm font-medium text-ink-primary truncate leading-snug">
                    {item.title}
                  </p>
                  <div className="flex items-center justify-between text-xs font-mono tabular-nums mt-1">
                    <span className="text-accent">
                      {formatDuration(item.position_seconds || 0)}
                    </span>
                    <span className="text-ink-muted">
                      {formatDuration(item.duration_seconds)}
                    </span>
                  </div>
                </div>

                <div className="absolute bottom-0 left-0 right-0 h-[2px] bg-ink-primary/15">
                  <div
                    className="h-full bg-accent"
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
