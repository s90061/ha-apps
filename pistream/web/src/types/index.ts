export interface User {
  id: number;
  username: string;
  is_admin: boolean;
  created_at: string;
}

export interface AudioTrack {
  index: number;
  stream_idx: number;
  codec: string;
  channels: number;
  language: string;
  title: string;
}

export interface SubtitleTrack {
  id: string;
  index: number;
  codec: string;
  language: string;
  title: string;
  is_external: boolean;
}

export interface MediaFile {
  id: number;
  title: string;
  file_path: string;
  rel_path: string;
  rel_dir: string;
  file_size: number;
  container: string;
  video_codec: string;
  audio_codec: string;
  width: number;
  height: number;
  duration_seconds: number;
  bit_rate: number;
  audio_tracks_json: string;
  subtitles_json: string;
  thumb_ready: boolean;
  file_mod_time?: number;
  position_seconds?: number;
  is_watched?: boolean;
  progress_ratio?: number;
}

export type SortOption = 'date_desc' | 'date_asc' | 'name_asc' | 'name_desc' | 'size_desc';

export interface FolderEntry {
  name: string;
  rel_path: string;
  video_count: number;
}

export interface FolderBrowseResult {
  current_path: string;
  parent_path: string;
  folders: FolderEntry[];
  files: MediaFile[];
}

export interface StreamDecision {
  mode: 'direct' | 'remux' | 'transcode';
  quality: 'original' | '1080p' | '720p';
  audio_track: number;
  stream_url: string;
}

export interface PlaybackProgress {
  user_id: number;
  media_file_id: number;
  position_seconds: number;
  is_watched: boolean;
  updated_at: string;
}

export interface StreamInfo {
  media: MediaFile;
  decision: StreamDecision;
  progress: PlaybackProgress | null;
  buffer_seconds?: number;
}
