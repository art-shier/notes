package api

import (
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"shiji/internal/store"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNotesTagDeletionSerializesNewAssociation(t *testing.T) {
	f := newNotesFixture(t)
	tag := f.must("POST", "/api/v1/tags", map[string]any{"name": "concurrent tag"}, 201)
	n := f.create("initially untagged")
	path := "/api/v1/notes/" + n["id"].(string)
	enumerated := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	e := f.app.DB.Callback().Query().After("gorm:query").Register("notes_test:pause_tag_delete", func(tx *gorm.DB) {
		if tx.Statement.Table == "notes" && strings.Contains(tx.Statement.SQL.String(), "note_tags") {
			once.Do(func() { close(enumerated); <-release })
		}
	})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { f.app.DB.Callback().Query().Remove("notes_test:pause_tag_delete") })
	deletion := make(chan int, 1)
	go func() { code, _ := f.req("DELETE", "/api/v1/tags/"+tag["id"].(string), nil, ""); deletion <- code }()
	select {
	case <-enumerated:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("tag delete did not enumerate notes")
	}
	association := make(chan int, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		code, _ := f.req("PATCH", path, map[string]any{"expected_version": 1, "tag_ids": []any{tag["id"]}}, "")
		association <- code
	}()
	<-started
	select {
	case code := <-association:
		close(release)
		t.Fatalf("association escaped transaction lock: %d", code)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if code := <-deletion; code != 204 {
		t.Fatal(code)
	}
	if code := <-association; code != 404 {
		t.Fatal(code)
	}
	after := f.must("GET", path, nil, 200)
	if after["version"] != float64(1) || len(after["tags"].([]any)) != 0 {
		t.Fatal(after)
	}
	h := f.must("GET", path+"/history", nil, 200)
	if len(h["items"].([]any)) != 1 {
		t.Fatal(h)
	}
}

func TestNotesHistoryRestorePreservesSettingsAndChecksFiles(t *testing.T) {
	f := newNotesFixture(t)
	var tok store.Token
	if e := f.app.DB.Where("secret_hash = ?", hexDigest(f.secret)).Take(&tok).Error; e != nil {
		t.Fatal(e)
	}
	att := store.Attachment{ID: store.ID(), UserID: tok.UserID, MimeType: "image/png", SizeBytes: 3, CreatedAt: store.Now()}
	att.ObjectKey = att.ID + ".bin"
	if e := f.app.DB.Create(&att).Error; e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(f.app.Settings.AttachmentsDir, att.ObjectKey)
	if e := os.WriteFile(file, []byte("png"), 0600); e != nil {
		t.Fatal(e)
	}
	image := map[string]any{"type": "doc", "content": []any{map[string]any{"type": "image", "attrs": map[string]any{"attachment_id": att.ID}}}}
	n := f.must("POST", "/api/v1/notes", map[string]any{"folder_id": f.folder, "title": "old", "content_json": image}, 201)
	path := "/api/v1/notes/" + n["id"].(string)
	folder := f.must("POST", "/api/v1/folders", map[string]any{"name": "new location"}, 201)
	tag := f.must("POST", "/api/v1/tags", map[string]any{"name": "new tag"}, 201)
	newDoc := map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph"}}}
	f.must("PATCH", path, map[string]any{"expected_version": 1, "title": "new", "content_json": newDoc, "folder_id": folder["id"], "tag_ids": []any{tag["id"]}, "favorite": true}, 200)
	if e := os.WriteFile(file, []byte("corrupt size"), 0600); e != nil {
		t.Fatal(e)
	}
	bad := f.must("POST", path+"/history/1/restore", map[string]any{"expected_version": 2}, 404)
	if bad["error"].(map[string]any)["code"] != "file_missing" {
		t.Fatal(bad)
	}
	after := f.must("GET", path, nil, 200)
	if after["version"] != float64(2) || after["title"] != "new" {
		t.Fatal(after)
	}
	if e := os.Remove(file); e != nil {
		t.Fatal(e)
	}
	f.must("POST", path+"/history/1/restore", map[string]any{"expected_version": 2}, 404)
	if e := os.WriteFile(file, []byte("png"), 0600); e != nil {
		t.Fatal(e)
	}
	restored := f.must("POST", path+"/history/1/restore", map[string]any{"expected_version": 2}, 200)
	if restored["version"] != float64(3) || restored["title"] != "old" || restored["folder_id"] != folder["id"] || restored["favorite"] != true || len(restored["tags"].([]any)) != 1 {
		t.Fatal(restored)
	}
	if restored["blocks"].([]any)[0].(map[string]any)["id"] != n["blocks"].([]any)[0].(map[string]any)["id"] {
		t.Fatal("historical block identity lost")
	}
	r := f.must("GET", path+"/history/3", nil, 200)
	if r["restored_from"] != float64(1) || r["action"] != "history_restore" {
		t.Fatal(r)
	}
	f.must("PATCH", path, map[string]any{"expected_version": 3, "content_json": newDoc}, 200)
	if e := f.app.DB.Delete(&att).Error; e != nil {
		t.Fatal(e)
	}
	f.must("POST", path+"/history/3/restore", map[string]any{"expected_version": 4}, 404)
}

func TestNotesLegacyIdempotencyJSON(t *testing.T) {
	value := map[string]any{"unicode": "中文\u2028\u2029", "html": "<>&", "literal": `\u2028`}
	raw, e := idempotencyJSON(value)
	if e != nil {
		t.Fatal(e)
	}
	want := "{\"html\":\"<>&\",\"literal\":\"\\\\u2028\",\"unicode\":\"中文\u2028\u2029\"}"
	if string(raw) != want {
		t.Fatalf("Python-compatible JSON got %q want %q", raw, want)
	}
}
