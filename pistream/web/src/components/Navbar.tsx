import React from 'react';
import { Film, Folder, LogOut, RefreshCw, Search, UserIcon } from './Icons';
import { User } from '../types';
import { BrandMark } from './BrandMark';

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
    <header className="sticky top-0 z-30 bg-deepest border-b border-border">
      <div className="max-w-[1440px] mx-auto px-3 sm:px-6 py-2 lg:py-0 lg:h-14 flex flex-wrap lg:flex-nowrap items-center justify-between gap-x-2 sm:gap-x-4 gap-y-2">
        {/* Left: Brand & View Switcher */}
        <div className="flex items-center gap-2 sm:gap-4 lg:gap-6 flex-none min-w-0">
          <button
            type="button"
            onClick={() => onViewModeChange('folders')}
            className="flex items-center gap-2 text-left cinema-focus rounded-lg flex-none"
          >
            <BrandMark size={28} />
            <span className="text-base font-medium text-ink-primary hidden sm:inline leading-none">
              PiStream
            </span>
          </button>

          <nav
            aria-label="瀏覽檢視"
            className="flex items-center bg-surface border border-border rounded-lg p-0.5 sm:p-1 gap-0.5 sm:gap-1"
          >
            <button
              type="button"
              onClick={() => onViewModeChange('folders')}
              className={`h-7 px-2 sm:px-3 rounded-md text-xs font-medium flex items-center gap-1.5 sm:gap-2 cinema-focus transition-colors duration-150 ease-cinema whitespace-nowrap ${
                viewMode === 'folders'
                  ? 'bg-raised text-ink-primary btn-highlight'
                  : 'text-ink-muted hover:text-ink-primary'
              }`}
            >
              <Folder size={14} strokeWidth={1.5} className={`hidden min-[400px]:inline ${viewMode === 'folders' ? 'text-accent' : 'text-ink-muted'}`} />
              <span>資料夾</span>
            </button>
            <button
              type="button"
              onClick={() => onViewModeChange('all')}
              className={`h-7 px-2 sm:px-3 rounded-md text-xs font-medium flex items-center gap-1.5 sm:gap-2 cinema-focus transition-colors duration-150 ease-cinema whitespace-nowrap ${
                viewMode === 'all'
                  ? 'bg-raised text-ink-primary btn-highlight'
                  : 'text-ink-muted hover:text-ink-primary'
              }`}
            >
              <Film size={14} strokeWidth={1.5} className={`hidden min-[400px]:inline ${viewMode === 'all' ? 'text-accent' : 'text-ink-muted'}`} />
              <span>全部影片</span>
            </button>
          </nav>
        </div>

        {/* Center: Search (wraps cleanly onto full-width second row on mobile & tablet < lg) */}
        <div className="order-last w-full lg:order-none lg:w-auto lg:flex-1 lg:max-w-sm min-w-0">
          <div className="relative flex items-center">
            <Search
              size={15}
              strokeWidth={1.5}
              className="absolute left-3 text-ink-muted pointer-events-none"
            />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => {
                onSearchChange(e.target.value);
                if (e.target.value.trim() && viewMode !== 'all') {
                  onViewModeChange('all');
                }
              }}
              placeholder="搜尋影片"
              aria-label="搜尋影片"
              className="w-full h-9 pl-9 pr-3 rounded-lg bg-surface border border-border text-sm text-ink-primary placeholder-ink-muted focus:outline-none focus:border-border-strong cinema-focus transition-colors duration-150 ease-cinema"
            />
          </div>
        </div>

        {/* Right: Actions & Account */}
        <div className="flex items-center gap-1.5 sm:gap-2 flex-none min-w-0">
          {user.is_admin && (
            <button
              type="button"
              onClick={onScan}
              disabled={scanning}
              title="掃描媒體目錄"
              className="h-9 px-2.5 sm:px-3 rounded-lg bg-surface hover:bg-raised disabled:opacity-50 border border-border text-xs font-medium text-ink-secondary hover:text-ink-primary flex items-center gap-2 cinema-focus transition-colors duration-150 ease-cinema"
            >
              <RefreshCw
                size={15}
                strokeWidth={1.5}
                className={`text-ink-muted ${scanning ? 'animate-spin' : ''}`}
              />
              <span className="hidden md:inline">{scanning ? '掃描中' : '掃描'}</span>
            </button>
          )}

          <button
            type="button"
            onClick={onOpenUserModal}
            title="帳號設定"
            className="h-9 px-2 sm:px-3 rounded-lg bg-surface hover:bg-raised border border-border text-xs font-medium text-ink-primary flex items-center gap-1.5 sm:gap-2 min-w-0 cinema-focus transition-colors duration-150 ease-cinema"
          >
            <UserIcon size={15} strokeWidth={1.5} className="text-ink-muted flex-none" />
            <span className="truncate max-w-[64px] sm:max-w-[120px]">{user.username}</span>
            {user.is_admin && (
              <span className="px-1.5 py-0.5 rounded bg-raised border border-border text-xs text-ink-secondary leading-none hidden sm:inline flex-none">
                管理員
              </span>
            )}
          </button>

          <button
            type="button"
            onClick={onLogout}
            title="登出"
            aria-label="登出"
            className="h-9 px-2.5 rounded-lg bg-surface hover:bg-raised border border-border text-xs font-medium text-ink-muted hover:text-negative flex items-center gap-1.5 cinema-focus transition-colors duration-150 ease-cinema"
          >
            <LogOut size={15} strokeWidth={1.5} />
            <span className="hidden lg:inline">登出</span>
          </button>
        </div>
      </div>
    </header>
  );
};
