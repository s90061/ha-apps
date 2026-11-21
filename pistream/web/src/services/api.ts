import { FolderBrowseResult, MediaFile, SortOption, StreamInfo, User } from '../types';

export function resolveUrl(path: string): string {
  const clean = path.replace(/^\/+/, '');
  const base =
    typeof window !== 'undefined'
      ? window.location.pathname.replace(/\/?$/, '/')
      : '/';
  return base + clean;
}

// detectHEVCDirectSupport 偵測瀏覽器 <video> 原生直連 (Direct Play) 是否支援 HEVC 硬解（含 Windows Firefox）
export function detectHEVCDirectSupport(): boolean {
  try {
    const v = document.createElement('video');
    const canPlay =
      v.canPlayType('video/mp4; codecs="hev1.1.6.L93.B0"') ||
      v.canPlayType('video/mp4; codecs="hvc1.1.6.L93.B0"');
    if (canPlay === 'probably' || canPlay === 'maybe') {
      return true;
    }
    if (typeof window !== 'undefined' && 'MediaSource' in window && typeof MediaSource.isTypeSupported === 'function') {
      return (
        MediaSource.isTypeSupported('video/mp4; codecs="hev1.1.6.L93.B0"') ||
        MediaSource.isTypeSupported('video/mp4; codecs="hvc1.1.6.L93.B0"')
      );
    }
  } catch {
    // ignore
  }
  return false;
}

// detectHEVCHlsSupport 偵測瀏覽器 HLS (MSE / Safari 原生 HLS) 是否支援 HEVC 免重編碼換殼 (Remux)
// 不再寫死排除 Windows Firefox：現代 Firefox 134+ 搭配系統 HEVC 擴充可正常透過 MSE 硬解 HEVC，
// 若個別裝置實際解碼失敗，VideoPlayer 亦有自動降級至 H.264 轉碼的保護機制
export function detectHEVCHlsSupport(): boolean {
  try {
    if (typeof window !== 'undefined' && 'MediaSource' in window && typeof MediaSource.isTypeSupported === 'function') {
      if (
        MediaSource.isTypeSupported('video/mp4; codecs="hev1.1.6.L93.B0"') ||
        MediaSource.isTypeSupported('video/mp4; codecs="hvc1.1.6.L93.B0"') ||
        MediaSource.isTypeSupported('video/mp4; codecs="hvc1.2.4.L120.90"')
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

// detectEAC3Support 偵測瀏覽器是否原生支援杜比數位+ (EAC3 / Dolby Digital Plus) 音訊解碼
export function detectEAC3Support(): boolean {
  try {
    const v = document.createElement('video');
    return v.canPlayType('audio/mp4; codecs="ec-3"') === 'probably';
  } catch {
    // ignore
  }
  return false;
}

export const detectHEVCSupport = detectHEVCDirectSupport;

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
    allowHEVCDirect: boolean,
    allowHEVCHls: boolean = allowHEVCDirect,
    allowEAC3: boolean = false,
  ) =>
    request<StreamInfo>(
      `/api/stream/${fileId}/info?session=${encodeURIComponent(session)}&quality=${encodeURIComponent(quality)}&audio=${audioTrack}&hevc=${allowHEVCDirect ? '1' : '0'}&hevc_hls=${allowHEVCHls ? '1' : '0'}&eac3=${allowEAC3 ? '1' : '0'}`,
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
