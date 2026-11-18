import React from 'react';
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
  return `${mb.toFixed(0)} MB`;
}

export function getResolutionBadge(width: number, height: number): string {
  if (width >= 3800 || height >= 2100) return '4K';
  if (width >= 2500 || height >= 1400) return '2K';
  if (width >= 1900 || height >= 1000) return '1080p';
  if (width >= 1200 || height >= 700) return '720p';
  if (height > 0) return `${height}p`;
  return '';
}

export const VideoCard: React.FC<VideoCardProps> = ({ file, onPlay, showFolderHint }) => {
  const resBadge = getResolutionBadge(file.width, file.height);
  const progressPct = file.progress_ratio
    ? Math.min(100, Math.max(0, Math.round(file.progress_ratio * 100)))
    : 0;

  return (
    <div
      onClick={() => onPlay(file.id)}
      className="group cursor-pointer rounded-2xl bg-surface border border-border hover:border-sky-400/50 overflow-hidden transition-all duration-200 hover:-translate-y-0.5 hover:shadow-xl hover:shadow-sky-500/10 flex flex-col"
    >
      {/* 16:9 Thumbnail Area */}
      <div className="relative aspect-video bg-slate-900 overflow-hidden">
        <img
          src={resolveUrl(`/api/thumbs/${file.id}`)}
          alt={file.title}
          loading="lazy"
          className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
        />
        <div className="absolute inset-0 bg-gradient-to-t from-black/80 via-transparent to-black/30 opacity-80 group-hover:opacity-60 transition-opacity" />

        {/* Top Badges: Resolution & Codec */}
        <div className="absolute top-2.5 left-2.5 flex items-center gap-1.5">
          {resBadge && (
            <span
              className={`px-2 py-0.5 rounded-md text-[10px] font-bold uppercase tracking-wider backdrop-blur-md ${
                resBadge === '4K'
                  ? 'bg-amber-500/90 text-slate-950'
                  : 'bg-sky-500/80 text-slate-950'
              }`}
            >
              {resBadge}
            </span>
          )}
          <span className="px-2 py-0.5 rounded-md text-[10px] font-semibold uppercase bg-black/70 text-slate-200 border border-white/10 backdrop-blur-md">
            {file.video_codec}
          </span>
        </div>

        {/* Watched Checkmark */}
        {file.is_watched && (
          <div className="absolute top-2.5 right-2.5 px-2 py-0.5 rounded-md bg-emerald-500/90 text-slate-950 text-[10px] font-bold">
            ✓ 已看完
          </div>
        )}

        {/* Play Hover Button */}
        <div className="absolute inset-0 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity">
          <div className="w-12 h-12 rounded-full bg-sky-500 text-slate-950 flex items-center justify-center font-bold text-lg shadow-lg shadow-sky-500/40">
            ▶
          </div>
        </div>

        {/* Duration Bottom Right */}
        <div className="absolute bottom-2.5 right-2.5 px-2 py-0.5 rounded bg-black/80 text-[11px] font-mono text-slate-200">
          {formatDuration(file.duration_seconds)}
        </div>

        {/* Progress Bar */}
        {progressPct > 0 && !file.is_watched && (
          <div className="absolute bottom-0 left-0 right-0 h-1 bg-white/20">
            <div
              className="h-full bg-sky-400"
              style={{ width: `${progressPct}%` }}
            />
          </div>
        )}
      </div>

      {/* Info Footer */}
      <div className="p-3.5 flex-1 flex flex-col justify-between">
        <div>
          <h3
            title={file.title}
            className="text-sm font-semibold text-slate-100 group-hover:text-sky-400 transition-colors line-clamp-2 leading-snug"
          >
            {file.title}
          </h3>
          {showFolderHint && file.rel_dir && (
            <p className="text-[11px] text-slate-500 mt-1 truncate">
              📁 {file.rel_dir}
            </p>
          )}
        </div>

        <div className="mt-2.5 flex items-center justify-between text-[11px] text-slate-400">
          <span className="uppercase">{file.container}</span>
          <span>{formatFileSize(file.file_size)}</span>
        </div>
      </div>
    </div>
  );
};
