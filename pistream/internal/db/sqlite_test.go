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
