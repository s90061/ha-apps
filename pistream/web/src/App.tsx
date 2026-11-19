import React, { useCallback, useEffect, useState } from 'react';
import { FolderBrowseResult, MediaFile, SortOption, User } from './types';
import { api } from './services/api';
import { LoginView } from './components/LoginView';
import { Navbar } from './components/Navbar';
import { ContinueWatching } from './components/ContinueWatching';
import { FolderBrowser } from './components/FolderBrowser';
import { VideoCard } from './components/VideoCard';
import { VideoPlayer } from './components/VideoPlayer';
import { UserAdminModal } from './components/UserAdminModal';
import { SortSelect } from './components/SortSelect';

const VALID_SORTS: SortOption[] = ['date_desc', 'date_asc', 'name_asc', 'name_desc', 'size_desc'];

export const App: React.FC = () => {
  const [user, setUser] = useState<User | null>(null);
  const [authChecking, setAuthChecking] = useState(true);

  const [viewMode, setViewMode] = useState<'folders' | 'all'>('folders');
  const [currentFolder, setCurrentFolder] = useState<string>('');
  const [folderData, setFolderData] = useState<FolderBrowseResult | null>(null);
  const [sortBy, setSortBy] = useState<SortOption>(() => {
    const saved = localStorage.getItem('pistream_sort_by') as SortOption | null;
    return saved && VALID_SORTS.includes(saved) ? saved : 'date_desc';
  });

  const handleSortChange = (next: SortOption) => {
    setSortBy(next);
    localStorage.setItem('pistream_sort_by', next);
  };

  const [allFiles, setAllFiles] = useState<MediaFile[]>([]);
  const [continueItems, setContinueItems] = useState<MediaFile[]>([]);
  const [searchQuery, setSearchQuery] = useState('');

  const [scanning, setScanning] = useState(false);
  const [activeFileId, setActiveFileId] = useState<number | null>(null);
  const [showUserModal, setShowUserModal] = useState(false);

  // 1. 檢查登入狀態
  useEffect(() => {
    api
      .getMe()
      .then((u) => setUser(u))
      .catch(() => setUser(null))
      .finally(() => setAuthChecking(false));
  }, []);

  // 2. 載入媒體庫資料
  const refreshLibrary = useCallback(() => {
    if (!user) return;

    api.getContinueWatching().then(setContinueItems).catch(() => {});

    if (viewMode === 'folders') {
      api.browseFolder(currentFolder, sortBy).then(setFolderData).catch(() => {});
    } else {
      api.listAllFiles(searchQuery, sortBy).then(setAllFiles).catch(() => {});
    }
  }, [user, viewMode, currentFolder, searchQuery, sortBy]);

  useEffect(() => {
    refreshLibrary();
  }, [refreshLibrary]);

  const handleScan = async () => {
    setScanning(true);
    try {
      await api.triggerScan();
      setTimeout(() => {
        refreshLibrary();
        setScanning(false);
      }, 1800);
    } catch {
      setScanning(false);
    }
  };

  const handleClearProgress = async (fileId: number, e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await api.clearProgress(fileId);
      refreshLibrary();
    } catch {
      // ignore
    }
  };

  const handleLogout = async () => {
    await api.logout().catch(() => {});
    setUser(null);
  };

  if (authChecking) {
    return (
      <div className="min-h-screen bg-background flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-accent border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  if (!user) {
    return <LoginView onLoginSuccess={(u) => setUser(u)} />;
  }

  return (
    <div className="min-h-screen bg-background text-ink-primary flex flex-col">
      <Navbar
        user={user}
        viewMode={viewMode}
        onViewModeChange={(mode) => {
          setViewMode(mode);
          if (mode === 'folders') setSearchQuery('');
        }}
        searchQuery={searchQuery}
        onSearchChange={setSearchQuery}
        onScan={handleScan}
        scanning={scanning}
        onOpenUserModal={() => setShowUserModal(true)}
        onLogout={handleLogout}
      />

      <main className="flex-1 max-w-[1440px] w-full mx-auto px-4 sm:px-6 py-6 sm:py-8">
        {/* 個人化繼續觀看橫列 (僅在無搜尋字串時顯示) */}
        {!searchQuery.trim() && (
          <ContinueWatching
            items={continueItems}
            onPlay={(id) => setActiveFileId(id)}
            onClearProgress={handleClearProgress}
          />
        )}

        {/* 模式一：資料夾階層瀏覽 */}
        {viewMode === 'folders' && folderData && (
          <FolderBrowser
            data={folderData}
            sortBy={sortBy}
            onSortChange={handleSortChange}
            onNavigateFolder={(relPath) => setCurrentFolder(relPath)}
            onPlayVideo={(id) => setActiveFileId(id)}
          />
        )}

        {/* 模式二：全部影片網格與即時搜尋 */}
        {viewMode === 'all' && (
          <section>
            <div className="flex items-center justify-between gap-4 mb-4">
              <div className="flex items-baseline gap-2">
                <h2 className="text-base font-medium text-ink-primary">
                  {searchQuery.trim() ? `搜尋「${searchQuery}」` : '全部影片'}
                </h2>
                <span className="text-xs font-mono text-ink-muted tabular-nums">
                  {allFiles.length}
                </span>
              </div>
              <SortSelect value={sortBy} onChange={handleSortChange} />
            </div>

            {allFiles.length > 0 ? (
              <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
                {allFiles.map((file) => (
                  <VideoCard
                    key={file.id}
                    file={file}
                    onPlay={(id) => setActiveFileId(id)}
                    showFolderHint
                  />
                ))}
              </div>
            ) : (
              <div className="py-16 px-6 text-center rounded-xl bg-surface border border-border">
                <p className="text-sm font-medium text-ink-primary mb-2">
                  找不到符合的影片
                </p>
                <p className="text-xs text-ink-muted">
                  請嘗試其他關鍵字，或點擊上方「掃描」更新片庫
                </p>
              </div>
            )}
          </section>
        )}
      </main>

      {/* 全螢幕自適應播放器 */}
      {activeFileId !== null && (
        <VideoPlayer
          fileId={activeFileId}
          onClose={() => {
            setActiveFileId(null);
            refreshLibrary();
          }}
        />
      )}

      {/* 帳號設定與管理員多使用者後台 */}
      {showUserModal && (
        <UserAdminModal
          currentUser={user}
          onClose={() => setShowUserModal(false)}
        />
      )}
    </div>
  );
};

export default App;
