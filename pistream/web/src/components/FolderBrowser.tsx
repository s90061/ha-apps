import React from 'react';
import { FolderBrowseResult } from '../types';
import { VideoCard } from './VideoCard';

interface FolderBrowserProps {
  data: FolderBrowseResult;
  onNavigateFolder: (relPath: string) => void;
  onPlayVideo: (fileId: number) => void;
}

export const FolderBrowser: React.FC<FolderBrowserProps> = ({
  data,
  onNavigateFolder,
  onPlayVideo,
}) => {
  const pathSegments = data.current_path
    ? data.current_path.split('/').filter(Boolean)
    : [];

  return (
    <div>
      {/* Breadcrumb Navigation */}
      <div className="flex items-center flex-wrap gap-1.5 mb-5 p-3 rounded-xl bg-surface/80 border border-border text-xs sm:text-sm">
        <button
          onClick={() => onNavigateFolder('')}
          className={`px-2.5 py-1 rounded-lg font-semibold transition-colors ${
            pathSegments.length === 0
              ? 'bg-sky-500/20 text-sky-300 border border-sky-500/30'
              : 'text-slate-300 hover:bg-slate-800 hover:text-white'
          }`}
        >
          🏠 媒體根目錄
        </button>

        {pathSegments.map((seg, idx) => {
          const targetRel = pathSegments.slice(0, idx + 1).join('/');
          const isLast = idx === pathSegments.length - 1;
          return (
            <React.Fragment key={targetRel}>
              <span className="text-slate-600">/</span>
              <button
                onClick={() => onNavigateFolder(targetRel)}
                className={`px-2.5 py-1 rounded-lg font-medium transition-colors ${
                  isLast
                    ? 'bg-sky-500/20 text-sky-300 border border-sky-500/30 font-semibold'
                    : 'text-slate-300 hover:bg-slate-800 hover:text-white'
                }`}
              >
                📁 {seg}
              </button>
            </React.Fragment>
          );
        })}

        {data.current_path !== '' && (
          <button
            onClick={() => onNavigateFolder(data.parent_path)}
            className="ml-auto px-3 py-1 rounded-lg bg-slate-800 hover:bg-slate-700 text-xs text-slate-200 transition-colors"
          >
            ← 返回上層
          </button>
        )}
      </div>

      {/* Subfolders Grid */}
      {data.folders.length > 0 && (
        <div className="mb-7">
          <h3 className="text-xs font-bold uppercase tracking-wider text-slate-400 mb-3">
            子資料夾 ({data.folders.length})
          </h3>
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3.5">
            {data.folders.map((folder) => (
              <div
                key={folder.rel_path}
                onClick={() => onNavigateFolder(folder.rel_path)}
                className="group cursor-pointer p-4 rounded-2xl bg-surface hover:bg-slate-800/90 border border-border hover:border-sky-400/50 transition-all flex items-center justify-between gap-3"
              >
                <div className="flex items-center gap-3 min-w-0">
                  <div className="w-10 h-10 rounded-xl bg-sky-500/15 border border-sky-400/20 flex items-center justify-center text-lg flex-none">
                    📁
                  </div>
                  <div className="min-w-0">
                    <p className="text-sm font-semibold text-slate-100 group-hover:text-sky-400 truncate">
                      {folder.name}
                    </p>
                    <p className="text-xs text-slate-400">
                      共 {folder.video_count} 部影片
                    </p>
                  </div>
                </div>
                <span className="text-slate-500 group-hover:text-sky-400 text-sm">
                  →
                </span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Videos in Current Folder */}
      {data.files.length > 0 && (
        <div>
          <h3 className="text-xs font-bold uppercase tracking-wider text-slate-400 mb-3">
            此資料夾影片 ({data.files.length})
          </h3>
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-5">
            {data.files.map((file) => (
              <VideoCard key={file.id} file={file} onPlay={onPlayVideo} />
            ))}
          </div>
        </div>
      )}

      {data.folders.length === 0 && data.files.length === 0 && (
        <div className="py-20 text-center rounded-2xl bg-surface/50 border border-border">
          <p className="text-base font-semibold text-slate-300 mb-1">
            目前資料夾內尚無影片檔案
          </p>
          <p className="text-xs text-slate-500">
            請確認樹莓派外接硬碟已掛載至容器 /media 目錄，或點擊右上角「↻ 掃描」
          </p>
        </div>
      )}
    </div>
  );
};
