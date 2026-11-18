import React from 'react';
import { User } from '../types';

interface NavbarProps {
  user: User;
  viewMode: 'folders' | 'all';
  onViewModeChange: (mode: 'folders' | 'all') => void;
  searchQuery: string;
  onSearchChange: (q: string) => void;
  onScan: () => void;
  scanning: boolean;
  onOpenUserModal: () => void;
  onLogout: () => void;
}

export const Navbar: React.FC<NavbarProps> = ({
  user,
  viewMode,
  onViewModeChange,
  searchQuery,
  onSearchChange,
  onScan,
  scanning,
  onOpenUserModal,
  onLogout,
}) => {
  return (
    <header className="sticky top-0 z-30 bg-background/90 backdrop-blur-xl border-b border-border">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 h-16 flex items-center justify-between gap-3">
        {/* Brand & View Mode Tabs */}
        <div className="flex items-center gap-4 sm:gap-6">
          <div
            onClick={() => onViewModeChange('folders')}
            className="flex items-center gap-2.5 cursor-pointer"
          >
            <div className="w-9 h-9 rounded-xl bg-sky-500/20 border border-sky-400/30 flex items-center justify-center text-sky-400 font-bold">
              ▶
            </div>
            <span className="font-bold text-lg tracking-tight text-white hidden sm:inline">
              PiStream
            </span>
          </div>

          <nav className="flex items-center bg-surface border border-border rounded-xl p-1">
            <button
              onClick={() => onViewModeChange('folders')}
              className={`px-3 py-1.5 rounded-lg text-xs font-semibold transition-all ${
                viewMode === 'folders'
                  ? 'bg-sky-500 text-slate-950 shadow'
                  : 'text-slate-400 hover:text-white'
              }`}
            >
              📁 資料夾瀏覽
            </button>
            <button
              onClick={() => onViewModeChange('all')}
              className={`px-3 py-1.5 rounded-lg text-xs font-semibold transition-all ${
                viewMode === 'all'
                  ? 'bg-sky-500 text-slate-950 shadow'
                  : 'text-slate-400 hover:text-white'
              }`}
            >
              🎬 全部影片
            </button>
          </nav>
        </div>

        {/* Search Input */}
        <div className="flex-1 max-w-xs sm:max-w-sm">
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => {
              onSearchChange(e.target.value);
              if (e.target.value.trim() && viewMode !== 'all') {
                onViewModeChange('all');
              }
            }}
            placeholder="搜尋影片檔名或路徑..."
            className="w-full px-3.5 py-1.5 rounded-xl bg-surface border border-border text-xs sm:text-sm text-white placeholder-slate-500 focus:outline-none focus:border-primary"
          />
        </div>

        {/* Actions & User Menu */}
        <div className="flex items-center gap-2">
          <button
            onClick={onScan}
            disabled={scanning}
            title="重新掃描樹莓派影片目錄"
            className="px-3 py-1.5 rounded-xl bg-surface hover:bg-slate-800 border border-border text-xs font-medium text-slate-200 transition-all"
          >
            {scanning ? '掃描中...' : '↻ 掃描'}
          </button>

          <button
            onClick={onOpenUserModal}
            className="px-3 py-1.5 rounded-xl bg-surface hover:bg-slate-800 border border-border text-xs font-medium text-sky-400 transition-all flex items-center gap-1.5"
          >
            <span>👤 {user.username}</span>
            {user.is_admin && (
              <span className="px-1.5 py-0.5 rounded bg-sky-500/20 text-[10px] text-sky-300">
                管理員
              </span>
            )}
          </button>

          <button
            onClick={onLogout}
            title="登出帳號"
            className="px-2.5 py-1.5 rounded-xl bg-surface hover:bg-red-500/20 border border-border hover:border-red-500/30 text-xs text-slate-400 hover:text-red-300 transition-all"
          >
            登出
          </button>
        </div>
      </div>
    </header>
  );
};
