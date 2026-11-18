import React, { useCallback, useEffect, useState } from 'react';
import { FolderBrowseResult, MediaFile, User } from './types';
import { api } from './services/api';
import { LoginView } from './components/LoginView';
import { Navbar } from './components/Navbar';
import { ContinueWatching } from './components/ContinueWatching';
import { FolderBrowser } from './components/FolderBrowser';
import { VideoCard } from './components/VideoCard';
import { VideoPlayer } from './components/VideoPlayer';
import { UserAdminModal } from './components/UserAdminModal';

export const App: React.FC = () => {
  const [user, setUser] = useState<User | null>(null);
  const [authChecking, setAuthChecking] = useState(true);

  const [viewMode, setViewMode] = useState<'folders' | 'all'>('folders');
  const [currentFolder, setCurrentFolder] = useState<string>('');
  const [folderData, setFolderData] = useState<FolderBrowseResult | null>(null);

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
      api.browseFolder(currentFolder).then(setFolderData).catch(() => {});
    } else {
      api.listAllFiles(searchQuery).then(setAllFiles).catch(() => {});
    }
  }, [user, viewMode, currentFolder, searchQuery]);

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
        <div className="w-10 h-10 border-4 border-sky-400 border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  if (!user) {
    return <LoginView onLoginSuccess={(u) => setUser(u)} />;
  }

  return (
    <div className="min-h-screen bg-background text-slate-100 flex flex-col">
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

      <main className="flex-1 max-w-7xl w-full mx-auto px-4 sm:px-6 py-6">
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
            onNavigateFolder={(relPath) => setCurrentFolder(relPath)}
            onPlayVideo={(id) => setActiveFileId(id)}
          />
        )}

        {/* 模式二：全部影片網格與即時搜尋 */}
        {viewMode === 'all' && (
          <div>
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-base sm:text-lg font-bold text-white">
                {searchQuery.trim()
                  ? `🔍 搜尋「${searchQuery}」結果 (${allFiles.length})`
                  : `🎬 全部影片 (${allFiles.length})`}
              </h2>
            </div>

            {allFiles.length > 0 ? (
              <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-5">
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
              <div className="py-20 text-center rounded-2xl bg-surface/50 border border-border">
                <p className="text-base font-semibold text-slate-300 mb-1">
                  找不到符合的影片
                </p>
                <p className="text-xs text-slate-500">
                  請嘗試其他關鍵字或點擊右上角「↻ 掃描」更新媒體庫
                </p>
              </div>
            )}
          </div>
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
