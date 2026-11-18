import React, { useEffect, useState } from 'react';
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
      setMessage('已成功新增使用者帳號');
      loadUsers();
    } catch (err: any) {
      setError(err.message);
    }
  };

  const handleDeleteUser = async (u: User) => {
    if (!window.confirm(`確定要刪除帳號「${u.username}」嗎？其觀看進度也將一併移除。`)) {
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
      setMessage('個人密碼已更新成功');
    } catch (err: any) {
      setError(err.message);
    }
  };

  return (
    <div className="fixed inset-0 z-50 bg-black/75 backdrop-blur-sm flex items-center justify-center p-4 overflow-y-auto">
      <div className="w-full max-w-2xl bg-surface border border-border rounded-2xl p-6 shadow-2xl max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between pb-4 border-b border-border mb-5">
          <div>
            <h2 className="text-lg font-bold text-white">
              帳號設定與多使用者管理
            </h2>
            <p className="text-xs text-slate-400">
              每個帳號皆擁有各自獨立的「繼續觀看」與播放進度紀錄
            </p>
          </div>
          <button
            onClick={onClose}
            className="w-8 h-8 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 flex items-center justify-center"
          >
            ✕
          </button>
        </div>

        {message && (
          <div className="mb-4 px-4 py-2.5 rounded-xl bg-emerald-500/15 border border-emerald-500/30 text-emerald-300 text-xs">
            {message}
          </div>
        )}
        {error && (
          <div className="mb-4 px-4 py-2.5 rounded-xl bg-red-500/15 border border-red-500/30 text-red-300 text-xs">
            {error}
          </div>
        )}

        {/* 1. 修改個人密碼 */}
        <div className="mb-6 p-4 rounded-xl bg-slate-900/70 border border-border">
          <h3 className="text-xs font-bold uppercase tracking-wider text-sky-400 mb-3">
            修改我的密碼 ({currentUser.username})
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
              className="px-3 py-2 rounded-xl bg-surface border border-border text-xs text-white"
            />
            <input
              type="password"
              required
              placeholder="新密碼 (至少 4 碼)"
              value={myNewPass}
              onChange={(e) => setMyNewPass(e.target.value)}
              className="px-3 py-2 rounded-xl bg-surface border border-border text-xs text-white"
            />
            <button
              type="submit"
              className="px-4 py-2 rounded-xl bg-sky-500 hover:bg-sky-400 text-slate-950 font-bold text-xs"
            >
              更新個人密碼
            </button>
          </form>
        </div>

        {/* 2. 管理員專區：新增與管理其他使用者 */}
        {currentUser.is_admin && (
          <div>
            <div className="mb-5 p-4 rounded-xl bg-slate-900/70 border border-border">
              <h3 className="text-xs font-bold uppercase tracking-wider text-sky-400 mb-3">
                新增家人/朋友觀看帳號 (關閉公開註冊)
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
                  className="flex-1 px-3 py-2 rounded-xl bg-surface border border-border text-xs text-white"
                />
                <input
                  type="password"
                  required
                  placeholder="初始密碼 (至少 4 碼)"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  className="flex-1 px-3 py-2 rounded-xl bg-surface border border-border text-xs text-white"
                />
                <label className="flex items-center gap-1.5 text-xs text-slate-300 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={newIsAdmin}
                    onChange={(e) => setNewIsAdmin(e.target.checked)}
                  />
                  管理員
                </label>
                <button
                  type="submit"
                  className="px-4 py-2 rounded-xl bg-emerald-500 hover:bg-emerald-400 text-slate-950 font-bold text-xs"
                >
                  ＋ 建立帳號
                </button>
              </form>
            </div>

            <h3 className="text-xs font-bold uppercase tracking-wider text-slate-400 mb-2.5">
              系統帳號清單 ({users.length})
            </h3>
            <div className="space-y-2">
              {users.map((u) => (
                <div
                  key={u.id}
                  className="p-3 rounded-xl bg-slate-900/60 border border-border flex items-center justify-between gap-2"
                >
                  <div className="flex items-center gap-2.5">
                    <span className="text-sm font-semibold text-white">
                      {u.username}
                    </span>
                    {u.is_admin && (
                      <span className="px-2 py-0.5 rounded bg-sky-500/20 text-[10px] font-bold text-sky-300">
                        管理員
                      </span>
                    )}
                  </div>
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => handleResetUserPass(u)}
                      className="px-2.5 py-1 rounded-lg bg-surface hover:bg-slate-800 border border-border text-xs text-slate-300"
                    >
                      重設密碼
                    </button>
                    {u.id !== currentUser.id && (
                      <button
                        onClick={() => handleDeleteUser(u)}
                        className="px-2.5 py-1 rounded-lg bg-red-500/15 hover:bg-red-500/30 border border-red-500/30 text-xs text-red-300"
                      >
                        刪除
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
