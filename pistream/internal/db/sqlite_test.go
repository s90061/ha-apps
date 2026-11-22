package db

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestGetContinueWatchingForUserLimit4(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pistream_test.db")
	store, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer store.Conn.Close()

	user, err := store.CreateUser("tester", "hash", false)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	for i := 1; i <= 6; i++ {
		m := &MediaFile{
			Title:           fmt.Sprintf("Video %d", i),
			FilePath:        fmt.Sprintf("/media/video_%d.mp4", i),
			RelPath:         fmt.Sprintf("video_%d.mp4", i),
			RelDir:          "",
			FileSize:        1024,
			Container:       "mp4",
			VideoCodec:      "h264",
			AudioCodec:      "aac",
			DurationSeconds: 1000,
		}
		if err := store.UpsertMediaFile(m); err != nil {
			t.Fatalf("UpsertMediaFile %d failed: %v", i, err)
		}
		if _, err := store.Conn.Exec(
			`INSERT INTO playback_progress (user_id, media_file_id, position_seconds, is_watched, updated_at)
			 VALUES (?, ?, ?, 0, datetime('2026-01-01 00:00:00', ?))`,
			user.ID, m.ID, 120.0, fmt.Sprintf("+%d minutes", i),
		); err != nil {
			t.Fatalf("insert progress %d failed: %v", i, err)
		}
	}

	items, err := store.GetContinueWatchingForUser(user.ID)
	if err != nil {
		t.Fatalf("GetContinueWatchingForUser failed: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(items))
	}

	wantTitles := []string{"Video 6", "Video 5", "Video 4", "Video 3"}
	for idx, want := range wantTitles {
		if items[idx].Title != want {
			t.Errorf("items[%d].Title = %q, want %q", idx, items[idx].Title, want)
		}
	}
}

func TestListMediaFilesNameSortCrossFolder(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pistream_sort_test.db")
	store, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer store.Conn.Close()

	user, err := store.CreateUser("sorter", "hash", false)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	samples := []struct {
		title  string
		relDir string
	}{
		{"HDR_HEVC_EAC3", "4K"},
		{"Clip_B", "Clips"},
		{"Tiny_2MB", "Movies"},
	}
	for _, s := range samples {
		if err := store.UpsertMediaFile(&MediaFile{
			Title:           s.title,
			FilePath:        fmt.Sprintf("/media/%s/%s.mp4", s.relDir, s.title),
			RelPath:         fmt.Sprintf("%s/%s.mp4", s.relDir, s.title),
			RelDir:          s.relDir,
			FileSize:        1024,
			Container:       "mp4",
			VideoCodec:      "h264",
			AudioCodec:      "aac",
			DurationSeconds: 60,
		}); err != nil {
			t.Fatalf("UpsertMediaFile %s failed: %v", s.title, err)
		}
	}

	asc, err := store.ListMediaFilesForUser(user.ID, "", "name_asc")
	if err != nil {
		t.Fatalf("name_asc failed: %v", err)
	}
	wantAsc := []string{"Clip_B", "HDR_HEVC_EAC3", "Tiny_2MB"}
	for i, want := range wantAsc {
		if asc[i].Title != want {
			t.Fatalf("name_asc[%d] = %q, want %q (all=%v)", i, asc[i].Title, want, asc)
		}
	}

	desc, err := store.ListMediaFilesForUser(user.ID, "", "name_desc")
	if err != nil {
		t.Fatalf("name_desc failed: %v", err)
	}
	wantDesc := []string{"Tiny_2MB", "HDR_HEVC_EAC3", "Clip_B"}
	for i, want := range wantDesc {
		if desc[i].Title != want {
			t.Fatalf("name_desc[%d] = %q, want %q", i, desc[i].Title, want)
		}
	}
}

func TestSyncMediaDirResetsProgressAndMediaIndex(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pistream_media_dir_test.db")
	store, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer store.Conn.Close()

	user, err := store.CreateUser("viewer", "hash", true)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	rootA := filepath.Join(t.TempDir(), "media_root")
	subB := filepath.Join(rootA, "movies")

	for i := 1; i <= 2; i++ {
		m := &MediaFile{
			Title:           fmt.Sprintf("Movie %d", i),
			FilePath:        filepath.Join(subB, fmt.Sprintf("movie_%d.mp4", i)),
			RelPath:         fmt.Sprintf("movies/movie_%d.mp4", i),
			RelDir:          "movies",
			FileSize:        4096,
			Container:       "mp4",
			VideoCodec:      "hevc",
			AudioCodec:      "aac",
			DurationSeconds: 1800,
		}
		if err := store.UpsertMediaFile(m); err != nil {
			t.Fatalf("UpsertMediaFile %d failed: %v", i, err)
		}
		if err := store.UpdateProgress(user.ID, m.ID, 300.0, m.DurationSeconds); err != nil {
			t.Fatalf("UpdateProgress %d failed: %v", i, err)
		}
	}

	// 1. 首次紀錄（模擬由舊版升級）：應僅寫入目前 media_dir，不得刪除既有觀看紀錄與影片索引
	changed, _, clearedProg, clearedMedia, err := store.SyncMediaDir(rootA)
	if err != nil {
		t.Fatalf("first SyncMediaDir failed: %v", err)
	}
	if changed || clearedProg != 0 || clearedMedia != 0 {
		t.Fatalf("首次升級不應清空資料: changed=%v clearedProg=%d clearedMedia=%d", changed, clearedProg, clearedMedia)
	}

	// 2. 路徑相同（僅尾端多斜線）：應視為未變更，保留全部資料
	changed, _, _, _, err = store.SyncMediaDir(rootA + string(filepath.Separator))
	if err != nil {
		t.Fatalf("same-dir SyncMediaDir failed: %v", err)
	}
	if changed {
		t.Fatalf("相同路徑僅尾端斜線差異不應視為路徑變更")
	}

	// 3. 變更為子目錄 subB（即使絕對 file_path 重疊，也必須立刻清空全部觀看紀錄與舊影片索引）
	changed, prevDir, clearedProg, clearedMedia, err := store.SyncMediaDir(subB)
	if err != nil {
		t.Fatalf("changed SyncMediaDir failed: %v", err)
	}
	if !changed || prevDir != normalizeMediaDir(rootA) || clearedProg != 2 || clearedMedia != 2 {
		t.Fatalf("預期清空 2 筆觀看紀錄與 2 筆影片索引，實際 changed=%v prev=%q prog=%d media=%d",
			changed, prevDir, clearedProg, clearedMedia)
	}

	var progCount, mediaCount int
	_ = store.Conn.QueryRow(`SELECT COUNT(1) FROM playback_progress`).Scan(&progCount)
	_ = store.Conn.QueryRow(`SELECT COUNT(1) FROM media_files`).Scan(&mediaCount)
	if progCount != 0 || mediaCount != 0 {
		t.Fatalf("預期 playback_progress 與 media_files 皆為 0，實際 prog=%d media=%d", progCount, mediaCount)
	}
	if cw, err := store.GetContinueWatchingForUser(user.ID); err != nil || len(cw) != 0 {
		t.Fatalf("預期繼續觀看已清空，實際 len=%d err=%v", len(cw), err)
	}

	// 使用者帳號必須完整保留
	if gotUser, err := store.GetUserByID(user.ID); err != nil || gotUser.Username != "viewer" {
		t.Fatalf("變更 media_dir 不得刪除使用者帳號: user=%v err=%v", gotUser, err)
	}

	// 4. 切到新路徑後重啟（維持 subB）：不應再次觸發清空
	newMedia := &MediaFile{
		Title:           "Fresh Movie",
		FilePath:        filepath.Join(subB, "fresh.mp4"),
		RelPath:         "fresh.mp4",
		RelDir:          "",
		FileSize:        2048,
		Container:       "mp4",
		VideoCodec:      "h264",
		AudioCodec:      "aac",
		DurationSeconds: 600,
	}
	if err := store.UpsertMediaFile(newMedia); err != nil {
		t.Fatalf("UpsertMediaFile fresh failed: %v", err)
	}
	if err := store.UpdateProgress(user.ID, newMedia.ID, 90.0, 600.0); err != nil {
		t.Fatalf("UpdateProgress fresh failed: %v", err)
	}
	changed, _, _, _, err = store.SyncMediaDir(subB)
	if err != nil || changed {
		t.Fatalf("維持新路徑重啟不應再次清空: changed=%v err=%v", changed, err)
	}
	_ = store.Conn.QueryRow(`SELECT COUNT(1) FROM playback_progress`).Scan(&progCount)
	if progCount != 1 {
		t.Fatalf("預期新路徑的觀看紀錄維持 1 筆，實際 %d", progCount)
	}
}

func TestSQLiteForeignKeysAcrossPoolConnections(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pistream_fk_pool_test.db")
	store, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer store.Conn.Close()

	user, err := store.CreateUser("fkuser", "hash", false)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	m := &MediaFile{
		Title:           "Cascade Check",
		FilePath:        "/media/cascade.mp4",
		RelPath:         "cascade.mp4",
		FileSize:        1024,
		Container:       "mp4",
		VideoCodec:      "h264",
		AudioCodec:      "aac",
		DurationSeconds: 300,
	}
	if err := store.UpsertMediaFile(m); err != nil {
		t.Fatalf("UpsertMediaFile failed: %v", err)
	}
	if err := store.UpdateProgress(user.ID, m.ID, 60.0, 300.0); err != nil {
		t.Fatalf("UpdateProgress failed: %v", err)
	}

	// 刻意佔住連線池第一條連線，迫使後續刪除在第二條新連線執行，驗證 ConnectionHook 讓所有連線皆啟用 foreign_keys=ON
	rows, err := store.Conn.Query(`SELECT id FROM media_files`)
	if err != nil {
		t.Fatalf("open holding query failed: %v", err)
	}
	defer rows.Close()

	var fkOn int
	if err := store.Conn.QueryRow(`PRAGMA foreign_keys`).Scan(&fkOn); err != nil {
		t.Fatalf("query PRAGMA foreign_keys on second conn failed: %v", err)
	}
	if fkOn != 1 {
		t.Fatalf("預期連線池第二條連線仍啟用 PRAGMA foreign_keys=1，實際 %d", fkOn)
	}

	if err := store.DeleteMediaFileByPath(m.FilePath); err != nil {
		t.Fatalf("DeleteMediaFileByPath failed: %v", err)
	}
	var progCount int
	if err := store.Conn.QueryRow(`SELECT COUNT(1) FROM playback_progress`).Scan(&progCount); err != nil {
		t.Fatalf("count playback_progress failed: %v", err)
	}
	if progCount != 0 {
		t.Fatalf("刪除影片後應由 ON DELETE CASCADE 連動刪除觀看紀錄，實際殘留 %d 筆", progCount)
	}
}
