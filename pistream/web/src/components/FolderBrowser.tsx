import React from 'react';
import { ArrowLeft, ChevronRight, Folder, Home } from './Icons';
import { FolderBrowseResult, SortOption } from '../types';
import { VideoCard } from './VideoCard';
import { SortSelect } from './SortSelect';

interface FolderBrowserProps {
  data: FolderBrowseResult;
  sortBy: SortOption;
  onSortChange: (sort: SortOption) => void;
  onNavigateFolder: (relPath: string) => void;
  onPlayVideo: (fileId: number) => void;
}

export const FolderBrowser: React.FC<FolderBrowserProps> = ({
  data,
  sortBy,
  onSortChange,
  onNavigateFolder,
  onPlayVideo,
}) => {
  const pathSegments = data.current_path
    ? data.current_path.split('/').filter(Boolean)
    : [];

  return (
    <div>
      {/* Breadcrumb Navigation */}
      <div className="flex items-center flex-wrap gap-2 mb-6 px-3 py-2 rounded-xl bg-surface border border-border text-xs sm:text-sm">
        <button
          type="button"
          onClick={() => onNavigateFolder('')}
          className={`h-8 px-3 rounded-lg font-medium flex items-center gap-2 cinema-focus transition-colors duration-150 ease-cinema ${
            pathSegments.length === 0
              ? 'bg-raised text-ink-primary btn-highlight'
              : 'text-ink-secondary hover:bg-raised hover:text-ink-primary'
          }`}
        >
          <Home size={15} strokeWidth={1.5} className={pathSegments.length === 0 ? 'text-accent' : 'text-ink-muted'} />
          <span>媒體根目錄</span>
        </button>

        {pathSegments.map((seg, idx) => {
          const targetRel = pathSegments.slice(0, idx + 1).join('/');
          const isLast = idx === pathSegments.length - 1;
          return (
            <React.Fragment key={targetRel}>
              <ChevronRight size={14} strokeWidth={1.5} className="text-ink-muted" />
              <button
                type="button"
                onClick={() => onNavigateFolder(targetRel)}
                className={`h-8 px-3 rounded-lg font-medium flex items-center gap-2 cinema-focus transition-colors duration-150 ease-cinema ${
                  isLast
                    ? 'bg-raised text-ink-primary btn-highlight'
                    : 'text-ink-secondary hover:bg-raised hover:text-ink-primary'
                }`}
              >
                <Folder size={15} strokeWidth={1.5} className={isLast ? 'text-accent' : 'text-ink-muted'} />
                <span>{seg}</span>
              </button>
            </React.Fragment>
          );
        })}

        <div className="ml-auto flex items-center gap-2">
          {data.current_path !== '' && (
            <button
              type="button"
              onClick={() => onNavigateFolder(data.parent_path)}
              className="h-8 px-3 rounded-lg bg-raised hover:bg-hover border border-border text-xs font-medium text-ink-secondary hover:text-ink-primary flex items-center gap-1.5 cinema-focus transition-colors duration-150 ease-cinema"
            >
              <ArrowLeft size={14} strokeWidth={1.5} />
              <span>返回上層</span>
            </button>
          )}
          <SortSelect value={sortBy} onChange={onSortChange} />
        </div>
      </div>

      {/* Subfolders Grid */}
      {data.folders.length > 0 && (
        <section className="mb-8">
          <div className="flex items-baseline gap-2 mb-3">
            <h3 className="text-sm font-medium text-ink-secondary">子資料夾</h3>
            <span className="text-xs font-mono text-ink-muted tabular-nums">
              {data.folders.length}
            </span>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
            {data.folders.map((folder) => (
              <div
                key={folder.rel_path}
                onClick={() => onNavigateFolder(folder.rel_path)}
                role="button"
                tabIndex={0}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    onNavigateFolder(folder.rel_path);
                  }
                }}
                className="group cursor-pointer p-4 rounded-xl bg-surface hover:bg-raised border border-border hover:border-border-strong flex items-center justify-between gap-3 cinema-focus transition-colors duration-150 ease-cinema"
              >
                <div className="flex items-center gap-3 min-w-0">
                  <div className="w-9 h-9 rounded-lg bg-raised group-hover:bg-hover border border-border flex items-center justify-center flex-none transition-colors duration-150 ease-cinema">
                    <Folder size={17} strokeWidth={1.5} className="text-ink-secondary group-hover:text-accent transition-colors duration-150 ease-cinema" />
                  </div>
                  <div className="min-w-0">
                    <p className="text-sm font-medium text-ink-primary truncate leading-snug">
                      {folder.name}
                    </p>
                    <p className="text-xs text-ink-muted tabular-nums mt-0.5">
                      {folder.video_count} 部影片
                    </p>
                  </div>
                </div>
                <ChevronRight
                  size={16}
                  strokeWidth={1.5}
                  className="text-ink-muted group-hover:text-ink-primary transition-colors duration-150 ease-cinema"
                />
              </div>
            ))}
          </div>
        </section>
      )}

      {/* Videos in Current Folder */}
      {data.files.length > 0 && (
        <section>
          <div className="flex items-baseline gap-2 mb-3">
            <h3 className="text-sm font-medium text-ink-secondary">此資料夾影片</h3>
            <span className="text-xs font-mono text-ink-muted tabular-nums">
              {data.files.length}
            </span>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
            {data.files.map((file) => (
              <VideoCard key={file.id} file={file} onPlay={onPlayVideo} />
            ))}
          </div>
        </section>
      )}

      {data.folders.length === 0 && data.files.length === 0 && (
        <div className="py-16 px-6 text-center rounded-xl bg-surface border border-border">
          <p className="text-sm font-medium text-ink-primary mb-2">
            目前資料夾內尚無影片
          </p>
          <p className="text-xs text-ink-muted">
            請確認媒體目錄已掛載至容器 /media，或點擊上方「掃描」更新片庫
          </p>
        </div>
      )}
    </div>
  );
};
