import React, { useState } from 'react';
import { api } from '../services/api';
import { User } from '../types';
import { BrandMark } from './BrandMark';

interface LoginViewProps {
  onLoginSuccess: (user: User) => void;
}

export const LoginView: React.FC<LoginViewProps> = ({ onLoginSuccess }) => {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!username.trim() || !password) return;

    setLoading(true);
    setError(null);
    try {
      const res = await api.login(username.trim(), password);
      onLoginSuccess(res.user);
    } catch (err: any) {
      setError(err.message || '登入失敗，請檢查帳號與密碼');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen bg-background flex items-center justify-center p-6">
      <div className="w-full max-w-[400px] bg-surface border border-border rounded-xl p-8">
        <div className="flex items-center gap-3 mb-8">
          <BrandMark size={32} />
          <div>
            <h1 className="text-lg font-medium text-ink-primary leading-snug">
              PiStream
            </h1>
            <p className="text-xs text-ink-muted leading-normal">
              家庭影音庫與遠端串流播放
            </p>
          </div>
        </div>

        {error && (
          <div className="mb-6 px-3 py-2.5 rounded-lg bg-negative/10 border border-negative/25 text-negative text-xs leading-normal">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label
              htmlFor="login-username"
              className="block text-xs font-medium text-ink-secondary mb-2"
            >
              帳號
            </label>
            <input
              id="login-username"
              type="text"
              required
              autoFocus
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="輸入使用者帳號"
              className="w-full h-10 px-3 rounded-lg bg-deepest border border-border text-sm text-ink-primary placeholder-ink-muted focus:outline-none focus:border-border-strong cinema-focus transition-colors duration-150 ease-cinema"
            />
          </div>

          <div>
            <label
              htmlFor="login-password"
              className="block text-xs font-medium text-ink-secondary mb-2"
            >
              密碼
            </label>
            <input
              id="login-password"
              type="password"
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="輸入登入密碼"
              className="w-full h-10 px-3 rounded-lg bg-deepest border border-border text-sm text-ink-primary placeholder-ink-muted focus:outline-none focus:border-border-strong cinema-focus transition-colors duration-150 ease-cinema"
            />
          </div>

          <div className="pt-2">
            <button
              type="submit"
              disabled={loading}
              className="w-full h-10 rounded-lg bg-ink-primary hover:bg-white disabled:opacity-50 text-deepest font-medium text-sm btn-highlight cinema-focus transition-colors transition-transform duration-150 ease-cinema active:scale-[0.98]"
            >
              {loading ? '驗證中…' : '登入'}
            </button>
          </div>
        </form>

        <p className="mt-6 text-center text-xs text-ink-muted leading-normal">
          本系統採私密帳號制，如需帳號請聯繫管理員
        </p>
      </div>
    </div>
  );
};
