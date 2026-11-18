package db

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	"pistream/internal/auth"

	_ "modernc.org/sqlite"
)

type User struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	IsAdmin      bool   `json:"is_admin"`
	CreatedAt    string `json:"created_at"`
}

type MediaFile struct {
	ID              int64   `json:"id"`
	Title           string  `json:"title"`
	FilePath        string  `json:"file_path"`
	RelPath         string  `json:"rel_path"`
	RelDir          string  `json:"rel_dir"`
	FileSize        int64   `json:"file_size"`
	Container       string  `json:"container"`
	VideoCodec      string  `json:"video_codec"`
	AudioCodec      string  `json:"audio_codec"`
	Width           int     `json:"width"`
	Height          int     `json:"height"`
	DurationSeconds float64 `json:"duration_seconds"`
	BitRate         int64   `json:"bit_rate"`
	AudioTracksJSON string  `json:"audio_tracks_json"`
	SubtitlesJSON   string  `json:"subtitles_json"`
	ThumbReady      bool    `json:"thumb_ready"`
}

type MediaFileWithProgress struct {
	MediaFile
	PositionSeconds float64 `json:"position_seconds"`
	IsWatched       bool    `json:"is_watched"`
	ProgressRatio   float64 `json:"progress_ratio"`
	LastWatchedAt   string  `json:"last_watched_at,omitempty"`
}

type FolderEntry struct {
	Name       string `json:"name"`
	RelPath    string `json:"rel_path"`
	VideoCount int    `json:"video_count"`
}

type FolderBrowseResult struct {
	CurrentPath string                  `json:"current_path"`
	ParentPath  string                  `json:"parent_path"`
	Folders     []FolderEntry           `json:"folders"`
	Files       []MediaFileWithProgress `json:"files"`
}

type PlaybackProgress struct {
	UserID          int64   `json:"user_id"`
	MediaFileID     int64   `json:"media_file_id"`
	PositionSeconds float64 `json:"position_seconds"`
	IsWatched       bool    `json:"is_watched"`
	UpdatedAt       string  `json:"updated_at"`
}

type Store struct {
	Conn *sql.DB
}

func InitDB(dbPath string) (*Store, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("無法開啟資料庫: %w", err)
	}

	schema := `
	PRAGMA journal_mode = WAL;
	PRAGMA synchronous = NORMAL;
	PRAGMA foreign_keys = ON;
	PRAGMA busy_timeout = 5000;

	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		is_admin BOOLEAN NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS media_files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		file_path TEXT NOT NULL UNIQUE,
		rel_path TEXT NOT NULL DEFAULT '',
		rel_dir TEXT NOT NULL DEFAULT '',
		file_size INTEGER NOT NULL,
		container TEXT NOT NULL,
		video_codec TEXT NOT NULL,
		audio_codec TEXT NOT NULL,
		width INTEGER DEFAULT 0,
		height INTEGER DEFAULT 0,
		duration_seconds REAL NOT NULL,
		bit_rate INTEGER DEFAULT 0,
		audio_tracks_json TEXT DEFAULT '[]',
		subtitles_json TEXT DEFAULT '[]',
		thumb_ready BOOLEAN NOT NULL DEFAULT 0,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_media_files_rel_dir ON media_files(rel_dir);

	CREATE TABLE IF NOT EXISTS playback_progress (
		user_id INTEGER NOT NULL,
		media_file_id INTEGER NOT NULL,
		position_seconds REAL NOT NULL DEFAULT 0,
		is_watched BOOLEAN NOT NULL DEFAULT 0,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (user_id, media_file_id),
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (media_file_id) REFERENCES media_files(id) ON DELETE CASCADE
	);`

	if _, err := conn.Exec(schema); err != nil {
		return nil, fmt.Errorf("初始化 Schema 失敗: %w", err)
	}

	return &Store{Conn: conn}, nil
}

// EnsureDefaultAdmin 確保系統至少存在一個管理員帳號；若設定 FORCE_ADMIN_PASSWORD=true 則同步更新該管理員密碼
func (s *Store) EnsureDefaultAdmin(adminUser, adminPass string) error {
	var count int
	if err := s.Conn.QueryRow(`SELECT COUNT(1) FROM users`).Scan(&count); err != nil {
		return err
	}
	forceSync := strings.EqualFold(os.Getenv("FORCE_ADMIN_PASSWORD"), "true")
	if count > 0 && !forceSync {
		return nil
	}
	hash, err := auth.HashPassword(adminPass)
	if err != nil {
		return err
	}
	if count > 0 && forceSync {
		res, err := s.Conn.Exec(`UPDATE users SET password_hash = ?, is_admin = 1 WHERE username = ?`, hash, adminUser)
		if err != nil {
			return err
		}
		if rows, _ := res.RowsAffected(); rows > 0 {
			return nil
		}
	}
	_, err = s.Conn.Exec(`INSERT INTO users (username, password_hash, is_admin) VALUES (?, ?, 1)`, adminUser, hash)
	return err
}

func (s *Store) GetUserByUsername(username string) (*User, error) {
	row := s.Conn.QueryRow(`SELECT id, username, password_hash, is_admin, created_at FROM users WHERE username = ?`, username)
	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) GetUserByID(id int64) (*User, error) {
	row := s.Conn.QueryRow(`SELECT id, username, password_hash, is_admin, created_at FROM users WHERE id = ?`, id)
	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.Conn.Query(`SELECT id, username, is_admin, created_at FROM users ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, u)
	}
	return list, nil
}

func (s *Store) CreateUser(username, passwordHash string, isAdmin bool) (*User, error) {
	res, err := s.Conn.Exec(`INSERT INTO users (username, password_hash, is_admin) VALUES (?, ?, ?)`, username, passwordHash, isAdmin)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetUserByID(id)
}

func (s *Store) UpdateUserPassword(id int64, passwordHash string) error {
	_, err := s.Conn.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, id)
	return err
}

func (s *Store) DeleteUser(id int64) error {
	_, err := s.Conn.Exec(`DELETE FROM users WHERE id = ?`, id)
	return err
}

func (s *Store) UpsertMediaFile(m *MediaFile) error {
	query := `
	INSERT INTO media_files (
		title, file_path, rel_path, rel_dir, file_size, container,
		video_codec, audio_codec, width, height, duration_seconds,
		bit_rate, audio_tracks_json, subtitles_json, thumb_ready, updated_at
	)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(file_path) DO UPDATE SET
		title = excluded.title,
		rel_path = excluded.rel_path,
		rel_dir = excluded.rel_dir,
		file_size = excluded.file_size,
		container = excluded.container,
		video_codec = excluded.video_codec,
		audio_codec = excluded.audio_codec,
		width = excluded.width,
		height = excluded.height,
		duration_seconds = excluded.duration_seconds,
		bit_rate = excluded.bit_rate,
		audio_tracks_json = excluded.audio_tracks_json,
		subtitles_json = excluded.subtitles_json,
		thumb_ready = CASE WHEN excluded.thumb_ready = 1 THEN 1 ELSE media_files.thumb_ready END,
		updated_at = CURRENT_TIMESTAMP;`

	res, err := s.Conn.Exec(query,
		m.Title, m.FilePath, m.RelPath, m.RelDir, m.FileSize, m.Container,
		m.VideoCodec, m.AudioCodec, m.Width, m.Height, m.DurationSeconds,
		m.BitRate, m.AudioTracksJSON, m.SubtitlesJSON, m.ThumbReady,
	)
	if err != nil {
		return err
	}
	var id int64
	_ = s.Conn.QueryRow(`SELECT id, thumb_ready FROM media_files WHERE file_path = ?`, m.FilePath).Scan(&id, &m.ThumbReady)
	if id == 0 {
		id, _ = res.LastInsertId()
	}
	m.ID = id
	return nil
}

func (s *Store) MarkThumbReady(id int64) error {
	_, err := s.Conn.Exec(`UPDATE media_files SET thumb_ready = 1 WHERE id = ?`, id)
	return err
}

func (s *Store) DeleteMediaFileByPath(filePath string) error {
	_, err := s.Conn.Exec(`DELETE FROM media_files WHERE file_path = ?`, filePath)
	return err
}

func (s *Store) CleanupMissingFiles(validPaths map[string]bool) (int, error) {
	rows, err := s.Conn.Query(`SELECT id, file_path FROM media_files`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var toDelete []int64
	for rows.Next() {
		var id int64
		var fp string
		if err := rows.Scan(&id, &fp); err == nil {
			if !validPaths[fp] {
				toDelete = append(toDelete, id)
			}
		}
	}
	for _, id := range toDelete {
		_, _ = s.Conn.Exec(`DELETE FROM media_files WHERE id = ?`, id)
	}
	return len(toDelete), nil
}

func (s *Store) GetMediaFile(id int64) (*MediaFile, error) {
	row := s.Conn.QueryRow(`
		SELECT id, title, file_path, rel_path, rel_dir, file_size, container,
		       video_codec, audio_codec, width, height, duration_seconds,
		       bit_rate, audio_tracks_json, subtitles_json, thumb_ready
		FROM media_files WHERE id = ?`, id)
	var m MediaFile
	if err := row.Scan(
		&m.ID, &m.Title, &m.FilePath, &m.RelPath, &m.RelDir, &m.FileSize, &m.Container,
		&m.VideoCodec, &m.AudioCodec, &m.Width, &m.Height, &m.DurationSeconds,
		&m.BitRate, &m.AudioTracksJSON, &m.SubtitlesJSON, &m.ThumbReady,
	); err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) ListMediaFilesForUser(userID int64, searchQuery string) ([]MediaFileWithProgress, error) {
	query := `
		SELECT f.id, f.title, f.file_path, f.rel_path, f.rel_dir, f.file_size, f.container,
		       f.video_codec, f.audio_codec, f.width, f.height, f.duration_seconds,
		       f.bit_rate, f.audio_tracks_json, f.subtitles_json, f.thumb_ready,
		       COALESCE(p.position_seconds, 0), COALESCE(p.is_watched, 0)
		FROM media_files f
		LEFT JOIN playback_progress p ON f.id = p.media_file_id AND p.user_id = ?
	`
	args := []any{userID}
	if strings.TrimSpace(searchQuery) != "" {
		like := "%" + strings.TrimSpace(searchQuery) + "%"
		query += ` WHERE f.title LIKE ? OR f.rel_path LIKE ? `
		args = append(args, like, like)
	}
	query += ` ORDER BY f.rel_dir ASC, f.title COLLATE NOCASE ASC`

	rows, err := s.Conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]MediaFileWithProgress, 0)
	for rows.Next() {
		var item MediaFileWithProgress
		if err := rows.Scan(
			&item.ID, &item.Title, &item.FilePath, &item.RelPath, &item.RelDir, &item.FileSize, &item.Container,
			&item.VideoCodec, &item.AudioCodec, &item.Width, &item.Height, &item.DurationSeconds,
			&item.BitRate, &item.AudioTracksJSON, &item.SubtitlesJSON, &item.ThumbReady,
			&item.PositionSeconds, &item.IsWatched,
		); err != nil {
			return nil, err
		}
		if item.DurationSeconds > 0 {
			item.ProgressRatio = item.PositionSeconds / item.DurationSeconds
		}
		list = append(list, item)
	}
	return list, nil
}

// BrowseFolderForUser 回傳指定相對資料夾下的子資料夾清單與當前層級影片清單
func (s *Store) BrowseFolderForUser(userID int64, folderRelPath string) (*FolderBrowseResult, error) {
	cleanPath := strings.Trim(strings.ReplaceAll(folderRelPath, "\\", "/"), "/")

	allFiles, err := s.ListMediaFilesForUser(userID, "")
	if err != nil {
		return nil, err
	}

	subFolderCounts := make(map[string]int)
	var directFiles []MediaFileWithProgress

	prefix := ""
	if cleanPath != "" {
		prefix = cleanPath + "/"
	}

	for _, f := range allFiles {
		if f.RelDir == cleanPath {
			directFiles = append(directFiles, f)
			continue
		}
		if cleanPath == "" && f.RelDir != "" {
			parts := strings.SplitN(f.RelDir, "/", 2)
			subFolderCounts[parts[0]]++
		} else if cleanPath != "" && strings.HasPrefix(f.RelDir, prefix) {
			rest := strings.TrimPrefix(f.RelDir, prefix)
			if rest != "" {
				parts := strings.SplitN(rest, "/", 2)
				subFolderCounts[parts[0]]++
			}
		}
	}

	folders := make([]FolderEntry, 0, len(subFolderCounts))
	for name, count := range subFolderCounts {
		rel := name
		if cleanPath != "" {
			rel = cleanPath + "/" + name
		}
		folders = append(folders, FolderEntry{
			Name:       name,
			RelPath:    rel,
			VideoCount: count,
		})
	}

	// 按資料夾名稱排序
	for i := 0; i < len(folders); i++ {
		for j := i + 1; j < len(folders); j++ {
			if strings.ToLower(folders[i].Name) > strings.ToLower(folders[j].Name) {
				folders[i], folders[j] = folders[j], folders[i]
			}
		}
	}

	parentPath := ""
	if cleanPath != "" {
		if idx := strings.LastIndex(cleanPath, "/"); idx >= 0 {
			parentPath = cleanPath[:idx]
		}
	}

	if directFiles == nil {
		directFiles = []MediaFileWithProgress{}
	}

	return &FolderBrowseResult{
		CurrentPath: cleanPath,
		ParentPath:  parentPath,
		Folders:     folders,
		Files:       directFiles,
	}, nil
}

func (s *Store) UpdateProgress(userID, fileID int64, pos float64, duration float64) error {
	isWatched := false
	if duration > 0 && (pos/duration) >= 0.93 {
		isWatched = true
	}
	query := `
	INSERT INTO playback_progress (user_id, media_file_id, position_seconds, is_watched, updated_at)
	VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(user_id, media_file_id) DO UPDATE SET
		position_seconds = excluded.position_seconds,
		is_watched = excluded.is_watched,
		updated_at = CURRENT_TIMESTAMP;`
	_, err := s.Conn.Exec(query, userID, fileID, pos, isWatched)
	return err
}

func (s *Store) DeleteProgress(userID, fileID int64) error {
	_, err := s.Conn.Exec(`DELETE FROM playback_progress WHERE user_id = ? AND media_file_id = ?`, userID, fileID)
	return err
}

func (s *Store) GetProgress(userID, fileID int64) (*PlaybackProgress, error) {
	row := s.Conn.QueryRow(`
		SELECT user_id, media_file_id, position_seconds, is_watched, updated_at
		FROM playback_progress
		WHERE user_id = ? AND media_file_id = ?`, userID, fileID)
	var p PlaybackProgress
	if err := row.Scan(&p.UserID, &p.MediaFileID, &p.PositionSeconds, &p.IsWatched, &p.UpdatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) GetContinueWatchingForUser(userID int64) ([]MediaFileWithProgress, error) {
	query := `
		SELECT f.id, f.title, f.file_path, f.rel_path, f.rel_dir, f.file_size, f.container,
		       f.video_codec, f.audio_codec, f.width, f.height, f.duration_seconds,
		       f.bit_rate, f.audio_tracks_json, f.subtitles_json, f.thumb_ready,
		       p.position_seconds, p.is_watched, p.updated_at
		FROM playback_progress p
		JOIN media_files f ON p.media_file_id = f.id
		WHERE p.user_id = ? AND p.is_watched = 0 AND p.position_seconds > 5
		ORDER BY p.updated_at DESC
		LIMIT 20;`

	rows, err := s.Conn.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]MediaFileWithProgress, 0)
	for rows.Next() {
		var item MediaFileWithProgress
		if err := rows.Scan(
			&item.ID, &item.Title, &item.FilePath, &item.RelPath, &item.RelDir, &item.FileSize, &item.Container,
			&item.VideoCodec, &item.AudioCodec, &item.Width, &item.Height, &item.DurationSeconds,
			&item.BitRate, &item.AudioTracksJSON, &item.SubtitlesJSON, &item.ThumbReady,
			&item.PositionSeconds, &item.IsWatched, &item.LastWatchedAt,
		); err != nil {
			return nil, err
		}
		if item.DurationSeconds > 0 {
			item.ProgressRatio = item.PositionSeconds / item.DurationSeconds
		}
		list = append(list, item)
	}
	return list, nil
}
