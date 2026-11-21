import { FolderBrowseResult, MediaFile, SortOption, StreamInfo, User } from '../types';

export function resolveUrl(path: string): string {
  const clean = path.replace(/^\/+/, '');
  const base =
    typeof window !== 'undefined'
      ? window.location.pathname.replace(/\/?$/, '/')
      : '/';
  return base + clean;
}

export function detectHEVCSupport(): boolean {
  try {
    if (typeof navigator !== 'undefined' && /\(Windows.+Firefox\//i.test(navigator.userAgent)) {
      return false;
    }
    if (typeof window !== 'undefined' && 'MediaSource' in window && typeof MediaSource.isTypeSupported === 'function') {
      if (
        MediaSource.isTypeSupported('video/mp4; codecs="hev1.1.6.L93.B0"') ||
        MediaSource.isTypeSupported('video/mp4; codecs="hvc1.1.6.L93.B0"')
      ) {
        return true;
      }
    }
    const v = document.createElement('video');
    if (v.canPlayType('application/vnd.apple.mpegurl')) {
      const canPlay =
        v.canPlayType('video/mp4; codecs="hev1.1.6.L93.B0"') ||
        v.canPlayType('video/mp4; codecs="hvc1.1.6.L93.B0"');
      if (canPlay === 'probably' || canPlay === 'maybe') {
        return true;
      }
    }
  } catch {
    // ignore
  }
  return false;
}

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(resolveUrl(url), {
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(options?.headers || {}),
    },
    ...options,
  });

  if (!res.ok) {
    let errMsg = `請求失敗 (${res.status})`;
    try {
      const data = await res.json();
      if (data.error) errMsg = data.error;
    } catch {
      // ignore
    }
    throw new Error(errMsg);
  }
  return res.json();
}

export const api = {
  // 驗證與帳號
  login: (username: string, password: string) =>
    request<{ token: string; user: User }>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),

  logout: () =>
    request<{ status: string }>('/api/auth/logout', { method: 'POST' }),

  getMe: () => request<User>('/api/auth/me'),

  changeMyPassword: (current_password: string, new_password: string) =>
    request<{ status: string }>('/api/auth/password', {
      method: 'PUT',
      body: JSON.stringify({ current_password, new_password }),
    }),

  // 管理員：使用者管理
  listUsers: () => request<User[]>('/api/admin/users'),

  createUser: (username: string, password: string, is_admin: boolean) =>
    request<User>('/api/admin/users', {
      method: 'POST',
      body: JSON.stringify({ username, password, is_admin }),
    }),

  resetUserPassword: (userId: number, password: string) =>
    request<{ status: string }>(`/api/admin/users/${userId}/password`, {
      method: 'PUT',
      body: JSON.stringify({ password }),
    }),

  deleteUser: (userId: number) =>
    request<{ status: string }>(`/api/admin/users/${userId}`, {
      method: 'DELETE',
    }),

  // 媒體庫與資料夾導覽
  browseFolder: (path: string, sort: SortOption = 'date_desc') =>
    request<FolderBrowseResult>(
      `/api/library/folders?path=${encodeURIComponent(path)}&sort=${encodeURIComponent(sort)}`,
    ),

  listAllFiles: (query = '', sort: SortOption = 'date_desc') =>
    request<MediaFile[]>(
      `/api/library/files?q=${encodeURIComponent(query)}&sort=${encodeURIComponent(sort)}`,
    ),

  getContinueWatching: () => request<MediaFile[]>('/api/library/continue'),

  triggerScan: () =>
    request<{ status: string }>('/api/library/scan', { method: 'POST' }),

  // 串流與播放進度
  getStreamInfo: (
    fileId: number,
    session: string,
    quality: string,
    audioTrack: number,
    allowHEVC: boolean,
  ) =>
    request<StreamInfo>(
      `/api/stream/${fileId}/info?session=${encodeURIComponent(session)}&quality=${encodeURIComponent(quality)}&audio=${audioTrack}&hevc=${allowHEVC ? '1' : '0'}`,
    ),

  reportProgress: (fileId: number, positionSeconds: number) =>
    request<{ status: string }>(`/api/stream/${fileId}/progress`, {
      method: 'POST',
      body: JSON.stringify({ position_seconds: positionSeconds }),
    }),

  clearProgress: (fileId: number) =>
    request<{ status: string }>(`/api/stream/${fileId}/progress`, {
      method: 'DELETE',
    }),

  stopStream: (session: string) => {
    if (!session) return;
    const url = resolveUrl(`/api/stream/stop?session=${encodeURIComponent(session)}`);
    fetch(url, {
      method: 'POST',
      credentials: 'include',
      keepalive: true,
    }).catch(() => {});
  },

  stopStreamBeacon: (session: string) => {
    if (!session) return;
    const url = resolveUrl(`/api/stream/stop?session=${encodeURIComponent(session)}`);
    if (typeof navigator !== 'undefined' && typeof navigator.sendBeacon === 'function') {
      try {
        if (navigator.sendBeacon(url)) {
          return;
        }
      } catch {
        // fallback to keepalive fetch
      }
    }
    fetch(url, {
      method: 'POST',
      credentials: 'include',
      keepalive: true,
    }).catch(() => {});
  },
};
