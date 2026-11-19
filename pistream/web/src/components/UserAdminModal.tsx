import React, { useEffect, useState } from 'react';
import { KeyRound, Plus, Trash2, X } from './Icons';
import { api } from '../services/api';
import { User } from '../types';

interface UserAdminModalProps {
  currentUser: User;
  onClose: () => void;
}

export const UserAdminModal: React.FC<UserAdminModalProps> = ({
  currentUser,
  onClose,
}) => {
  const [users, setUsers] = useState<User[]>([]);
  const [newUsername, setNewUsername] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [newIsAdmin, setNewIsAdmin] = useState(false);

  const [myOldPass, setMyOldPass] = useState('');
  const [myNewPass, setMyNewPass] = useState('');
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const loadUsers = () => {
    if (!currentUser.is_admin) return;
    api
      .listUsers()
      .then(setUsers)
      .catch((err) => setError(err.message));
  };

  useEffect(() => {
    loadUsers();
  }, [currentUser.is_admin]);

  const handleCreateUser = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setMessage(null);
    try {
      await api.createUser(newUsername.trim(), newPassword, newIsAdmin);
      setNewUsername('');
      setNewPassword('');
      setNewIsAdmin(false);
      setMessage('已新增使用者帳號');
      loadUsers();
    } catch (err: any) {
      setError(err.message);
    }
  };

  const handleDeleteUser = async (u: User) => {
    if (!window.confirm(`確定要刪除帳號「${u.username}」嗎？其觀看進度將一併移除。`)) {
      return;
    }
    setError(null);
    setMessage(null);
    try {
      await api.deleteUser(u.id);
      setMessage(`已刪除帳號 ${u.username}`);
      loadUsers();
    } catch (err: any) {
      setError(err.message);
    }
  };

  const handleResetUserPass = async (u: User) => {
    const pwd = window.prompt(`請輸入帳號「${u.username}」的新密碼（至少 4 碼）：`);
    if (!pwd) return;
    setError(null);
    setMessage(null);
    try {
      await api.resetUserPassword(u.id, pwd);
      setMessage(`已重設 ${u.username} 的密碼`);
    } catch (err: any) {
      setError(err.message);
    }
  };

  const handleChangeMyPass = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setMessage(null);
    try {
      await api.changeMyPassword(myOldPass, myNewPass);
      setMyOldPass('');
      setMyNewPass('');
      setMessage('個人密碼已更新');
    } catch (err: any) {
      setError(err.message);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 bg-black/75 flex items-center justify-center p-4 overflow-y-auto"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="w-full max-w-2xl bg-surface border border-border rounded-xl p-6 max-h-[90vh] overflow-y-auto">
        <div className="flex items-start justify-between gap-4 pb-4 border-b border-border mb-6">
          <div>
            <h2 className="text-base font-medium text-ink-primary leading-snug">
              帳號設定與成員管理
            </h2>
            <p className="text-xs text-ink-muted mt-1">
              每個帳號皆保有獨立的「繼續觀看」與播放進度紀錄
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="關閉視窗"
            className="w-8 h-8 rounded-lg bg-raised hover:bg-hover border border-border text-ink-secondary hover:text-ink-primary flex items-center justify-center cinema-focus transition-colors duration-150 ease-cinema"
          >
            <X size={16} strokeWidth={1.5} />
          </button>
        </div>

        {message && (
          <div className="mb-4 px-3 py-2.5 rounded-lg bg-positive/10 border border-positive/25 text-positive text-xs">
            {message}
          </div>
        )}
        {error && (
          <div className="mb-4 px-3 py-2.5 rounded-lg bg-negative/10 border border-negative/25 text-negative text-xs">
            {error}
          </div>
        )}

        {/* 1. 修改個人密碼 */}
        <section className="mb-6 p-4 rounded-xl bg-deepest border border-border">
          <h3 className="text-xs font-medium text-ink-secondary mb-3">
            修改個人密碼（{currentUser.username}）
          </h3>
          <form
            onSubmit={handleChangeMyPass}
            className="grid grid-cols-1 sm:grid-cols-3 gap-3"
          >
            <input
              type="password"
              required
              placeholder="目前密碼"
              value={myOldPass}
              onChange={(e) => setMyOldPass(e.target.value)}
              className="h-9 px-3 rounded-lg bg-surface border border-border text-xs text-ink-primary placeholder-ink-muted focus:outline-none focus:border-border-strong cinema-focus"
            />
            <input
              type="password"
              required
              placeholder="新密碼（至少 4 碼）"
              value={myNewPass}
              onChange={(e) => setMyNewPass(e.target.value)}
              className="h-9 px-3 rounded-lg bg-surface border border-border text-xs text-ink-primary placeholder-ink-muted focus:outline-none focus:border-border-strong cinema-focus"
            />
            <button
              type="submit"
              className="h-9 px-4 rounded-lg bg-ink-primary hover:bg-white text-deepest font-medium text-xs btn-highlight cinema-focus transition-colors duration-150 ease-cinema active:scale-[0.98]"
            >
              更新密碼
            </button>
          </form>
        </section>

        {/* 2. 管理員專區：新增與管理其他使用者 */}
        {currentUser.is_admin && (
          <div>
            <section className="mb-6 p-4 rounded-xl bg-deepest border border-border">
              <h3 className="text-xs font-medium text-ink-secondary mb-3">
                新增家人或朋友帳號
              </h3>
              <form
                onSubmit={handleCreateUser}
                className="flex flex-col sm:flex-row items-stretch sm:items-center gap-3"
              >
                <input
                  type="text"
                  required
                  placeholder="新帳號名稱"
                  value={newUsername}
                  onChange={(e) => setNewUsername(e.target.value)}
                  className="flex-1 h-9 px-3 rounded-lg bg-surface border border-border text-xs text-ink-primary placeholder-ink-muted focus:outline-none focus:border-border-strong cinema-focus"
                />
                <input
                  type="password"
                  required
                  placeholder="初始密碼（至少 4 碼）"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  className="flex-1 h-9 px-3 rounded-lg bg-surface border border-border text-xs text-ink-primary placeholder-ink-muted focus:outline-none focus:border-border-strong cinema-focus"
                />
                <label className="flex items-center gap-2 text-xs text-ink-secondary cursor-pointer px-1">
                  <input
                    type="checkbox"
                    checked={newIsAdmin}
                    onChange={(e) => setNewIsAdmin(e.target.checked)}
                    className="rounded border-border bg-surface accent-accent"
                  />
                  <span>管理員</span>
                </label>
                <button
                  type="submit"
                  className="h-9 px-4 rounded-lg bg-raised hover:bg-hover border border-border-strong text-ink-primary font-medium text-xs flex items-center justify-center gap-1.5 cinema-focus transition-colors duration-150 ease-cinema"
                >
                  <Plus size={14} strokeWidth={1.5} />
                  <span>建立帳號</span>
                </button>
              </form>
            </section>

            <div className="flex items-baseline gap-2 mb-3">
              <h3 className="text-xs font-medium text-ink-secondary">帳號清單</h3>
              <span className="text-xs font-mono text-ink-muted tabular-nums">
                {users.length}
              </span>
            </div>
            <div className="space-y-2">
              {users.map((u) => (
                <div
                  key={u.id}
                  className="px-4 py-3 rounded-lg bg-deepest border border-border flex items-center justify-between gap-3"
                >
                  <div className="flex items-center gap-2 min-w-0">
                    <span className="text-sm font-medium text-ink-primary truncate">
                      {u.username}
                    </span>
                    {u.is_admin && (
                      <span className="px-2 py-0.5 rounded bg-raised border border-border text-xs text-accent leading-none">
                        管理員
                      </span>
                    )}
                  </div>
                  <div className="flex items-center gap-2 flex-none">
                    <button
                      type="button"
                      onClick={() => handleResetUserPass(u)}
                      className="h-8 px-3 rounded-lg bg-surface hover:bg-raised border border-border text-xs text-ink-secondary hover:text-ink-primary flex items-center gap-1.5 cinema-focus transition-colors duration-150 ease-cinema"
                    >
                      <KeyRound size={13} strokeWidth={1.5} />
                      <span>重設密碼</span>
                    </button>
                    {u.id !== currentUser.id && (
                      <button
                        type="button"
                        onClick={() => handleDeleteUser(u)}
                        className="h-8 px-3 rounded-lg bg-surface hover:bg-negative/15 border border-border hover:border-negative/30 text-xs text-ink-muted hover:text-negative flex items-center gap-1.5 cinema-focus transition-colors duration-150 ease-cinema"
                      >
                        <Trash2 size={13} strokeWidth={1.5} />
                        <span>刪除</span>
                      </button>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
