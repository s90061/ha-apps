import React from 'react';
import { Check, Folder, Play } from './Icons';
import { MediaFile } from '../types';
import { resolveUrl } from '../services/api';

interface VideoCardProps {
  file: MediaFile;
  onPlay: (fileId: number) => void;
  showFolderHint?: boolean;
}

export function formatDuration(sec: number): string {
  if (!Number.isFinite(sec) || sec <= 0) return '00:00';
  const hrs = Math.floor(sec / 3600);
  const mins = Math.floor((sec % 3600) / 60);
  const secs = Math.floor(sec % 60);
  const pad = (n: number) => n.toString().padStart(2, '0');
  return hrs > 0 ? `${hrs}:${pad(mins)}:${pad(secs)}` : `${pad(mins)}:${pad(secs)}`;
}

export function formatFileSize(bytes: number): string {
  if (!bytes || bytes <= 0) return '';
  const gb = bytes / (1024 * 1024 * 1024);
  if (gb >= 1) return `${gb.toFixed(1)} GB`;
  const mb = bytes / (1024 * 1024);
  if (mb >= 1) return `${mb.toFixed(0)} MB`;
  const kb = bytes / 1024;
  return `${kb.toFixed(0)} KB`;
}

export function getResolutionBadge(width: number, height: number): string {
  if (width >= 3800 || height >= 2100) return '4K';
  if (width >= 2500 || height >= 1400) return '2K';
  if (width >= 1900 || height >= 1000) return '1080p';
  if (width >= 1200 || height >= 700) return '720p';
  if (height > 0) return `${height}p`;
  return '';
}

export function formatShortDate(epochSec?: number): string {
  if (!epochSec || epochSec <= 0) return '';
  const d = new Date(epochSec * 1000);
  if (Number.isNaN(d.getTime())) return '';
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${y}/${m}/${day}`;
}

export const VideoCard: React.FC<VideoCardProps> = ({ file, onPlay, showFolderHint }) => {
  const resBadge = getResolutionBadge(file.width, file.height);
  const progressPct = file.progress_ratio
    ? Math.min(100, Math.max(0, Math.round(file.progress_ratio * 100)))
    : 0;
  const sizeLabel = formatFileSize(file.file_size);
  const dateLabel = formatShortDate(file.file_mod_time);
  const codecLabel = file.video_codec ? file.video_codec.toUpperCase() : '';
  const containerLabel = file.container ? file.container.toUpperCase() : '';

  return (
    <div
      onClick={() => onPlay(file.id)}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onPlay(file.id);
        }
      }}
      className="group cursor-pointer rounded-xl bg-surface border border-border hover:border-border-strong overflow-hidden flex flex-col cinema-focus transition-colors duration-150 ease-cinema"
    >
      {/* 16:9 Clean Thumbnail — Moments */}
      <div className="relative aspect-video bg-deepest overflow-hidden">
        <img
          src={resolveUrl(`/api/thumbs/${file.id}`)}
          alt={file.title}
          loading="lazy"
          className="w-full h-full object-cover"
        />
        <div className="absolute inset-0 bg-gradient-to-t from-[#0f0f10]/60 via-transparent to-transparent" />

        {/* Subtle Hover Play Indicator */}
        <div className="absolute inset-0 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity duration-150 ease-cinema pointer-events-none">
          <div className="w-10 h-10 rounded-full bg-ink-primary text-deepest flex items-center justify-center">
            <Play size={16} strokeWidth={1.75} className="translate-x-[1px] fill-current" />
          </div>
        </div>

        {/* Bottom-Right Mono Timecode (Signature) */}
        <div className="absolute bottom-2 right-2 px-1.5 py-0.5 rounded bg-deepest/85 border border-border text-xs font-mono text-ink-primary tabular-nums leading-tight">
          {formatDuration(file.duration_seconds)}
        </div>

        {/* 2px Warm Amber Progress Track */}
        {progressPct > 0 && !file.is_watched && (
          <div className="absolute bottom-0 left-0 right-0 h-[2px] bg-ink-primary/15">
            <div
              className="h-full bg-accent"
              style={{ width: `${progressPct}%` }}
            />
          </div>
        )}
      </div>

      {/* Info Footer — Metadata relocated cleanly below thumbnail */}
      <div className="p-4 flex-1 flex flex-col justify-between gap-3">
        <div>
          <h3
            title={file.title}
            className="text-sm font-medium text-ink-primary group-hover:text-white line-clamp-2 leading-snug"
          >
            {file.title}
          </h3>
          {(showFolderHint && file.rel_dir) || dateLabel ? (
            <div className="flex items-center justify-between gap-2 text-xs text-ink-muted mt-1.5 min-w-0">
              {showFolderHint && file.rel_dir ? (
                <div className="flex items-center gap-1.5 min-w-0">
                  <Folder size={13} strokeWidth={1.5} className="flex-none text-ink-muted" />
                  <span className="truncate">{file.rel_dir}</span>
                </div>
              ) : (
                <span />
              )}
              {dateLabel && (
                <span className="font-mono tabular-nums flex-none">{dateLabel}</span>
              )}
            </div>
          ) : null}
        </div>

        <div className="flex items-center justify-between gap-2 text-xs font-mono text-ink-muted tabular-nums">
          <div className="flex items-center gap-2 min-w-0 truncate">
            {resBadge && (
              <span className={resBadge === '4K' ? 'text-accent font-medium' : 'text-ink-secondary'}>
                {resBadge}
              </span>
            )}
            {codecLabel && <span>{codecLabel}</span>}
            {containerLabel && <span>{containerLabel}</span>}
          </div>
          <div className="flex items-center gap-2 flex-none">
            {file.is_watched && (
              <span className="inline-flex items-center gap-1 text-positive font-sans">
                <Check size={13} strokeWidth={1.75} />
                <span>已看</span>
              </span>
            )}
            {sizeLabel && <span>{sizeLabel}</span>}
          </div>
        </div>
      </div>
    </div>
  );
};
