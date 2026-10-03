package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"shiji/internal/config"
	"shiji/internal/store"
	"sync"
	"testing"
	"time"
)

type notesFixture struct {
	t              *testing.T
	app            *App
	router         *gin.Engine
	secret, folder string
}

func newNotesFixture(t *testing.T) *notesFixture {
	t.Helper()
	db, e := store.Open("sqlite:///:memory:")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { store.Close(db) })
	if e = store.Migrate(db); e != nil {
		t.Fatal(e)
	}
	u := store.User{ID: store.ID(), Email: store.ID() + "@example.com", DisplayName: "测试", PasswordHash: "unused", Role: "user", QuotaBytes: 1 << 30, CreatedAt: store.Now()}
	if e = db.Create(&u).Error; e != nil {
		t.Fatal(e)
	}
	f := store.Folder{ID: store.ID(), UserID: u.ID, Name: "收件箱", ParentKey: "root", IsInbox: true}
	if e = db.Create(&f).Error; e != nil {
		t.Fatal(e)
	}
	secret := "sj_" + store.ID()
	digest := sha256.Sum256([]byte(secret))
	tok := store.Token{ID: store.ID(), UserID: u.ID, Name: "tests", Prefix: "sj_tests", SecretHash: hex.EncodeToString(digest[:]), Scopes: store.List{"notes:read", "notes:create", "notes:update", "notes:trash", "folders:read", "folders:write", "tags:read", "tags:write"}, ExpiresAt: store.Time{Time: time.Now().Add(time.Hour)}, CreatedAt: store.Now()}
	if e = db.Create(&tok).Error; e != nil {
		t.Fatal(e)
	}
	a := New(db, config.Settings{HistoryLimit: 3, ExportConcurrency: 2, AttachmentsDir: t.TempDir()})
	return &notesFixture{t, a, a.Router(), secret, f.ID}
}
func (f *notesFixture) req(method, path string, body any, key string) (int, map[string]any) {
	f.t.Helper()
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, bytes.NewReader(raw))
	r.Header.Set("Authorization", "Bearer "+f.secret)
	r.Header.Set("Content-Type", "application/json")
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	var out map[string]any
	if w.Body.Len() > 0 {
		if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
			f.t.Fatalf("invalid response: %s", w.Body.String())
		}
	}
	return w.Code, out
}
func (f *notesFixture) must(method, path string, body any, status int) map[string]any {
	f.t.Helper()
	got, out := f.req(method, path, body, "")
	if got != status {
		f.t.Fatalf("%s %s: status %d want %d: %#v", method, path, got, status, out)
	}
	return out
}
func (f *notesFixture) create(title string) map[string]any {
	return f.must("POST", "/api/v1/notes", map[string]any{"folder_id": f.folder, "title": title, "content_json": map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "中文正文"}}}}}}, 201)
}

func TestNotesCASHistoryAndRetention(t *testing.T) {
	f := newNotesFixture(t)
	n := f.create("原文")
	path := "/api/v1/notes/" + n["id"].(string)
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, _ := f.req("PATCH", path, map[string]any{"expected_version": 1, "title": "修改"}, "")
			codes <- c
		}()
	}
	wg.Wait()
	close(codes)
	ok, conflict := 0, 0
	for c := range codes {
		if c == 200 {
			ok++
		}
		if c == 409 {
			conflict++
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("CAS successes=%d conflicts=%d", ok, conflict)
	}
	old := f.must("GET", path+"/history/1", nil, 200)
	if old["snapshot"].(map[string]any)["title"] != "原文" {
		t.Fatal(old)
	}
	for v := 2; v <= 4; v++ {
		f.must("PATCH", path, map[string]any{"expected_version": v, "title": "后来"}, 200)
	}
	h := f.must("GET", path+"/history?limit=1", nil, 200)
	if h["current_version"] != float64(5) || h["next_before_version"] != float64(5) {
		t.Fatal(h)
	}
	f.must("GET", path+"/history/2", nil, 404)
	r := f.must("POST", path+"/history/3/restore", map[string]any{"expected_version": 5}, 200)
	if r["version"] != float64(6) {
		t.Fatal(r)
	}
	f.must("DELETE", path+"?expected_version=6", nil, 200)
	f.must("POST", path+"/history/6/restore", map[string]any{"expected_version": 7}, 409)
	f.must("POST", path+"/restore", map[string]any{"expected_version": 7}, 200)
}
func TestNotesIdempotentCreateAndRollback(t *testing.T) {
	f := newNotesFixture(t)
	body := map[string]any{"folder_id": f.folder, "title": "一次"}
	code, a := f.req("POST", "/api/v1/notes", body, "create-once-123")
	if code != 201 {
		t.Fatal(code, a)
	}
	code, b := f.req("POST", "/api/v1/notes", body, "create-once-123")
	if code != 201 || a["id"] != b["id"] {
		t.Fatal(code, a, b)
	}
	body["title"] = "不同"
	code, _ = f.req("POST", "/api/v1/notes", body, "create-once-123")
	if code != 409 {
		t.Fatal(code)
	}
	body["content_json"] = map[string]any{"type": "script"}
	code, _ = f.req("POST", "/api/v1/notes", body, "rollback-key-123")
	if code != 422 {
		t.Fatal(code)
	}
	delete(body, "content_json")
	code, _ = f.req("POST", "/api/v1/notes", body, "rollback-key-123")
	if code != 201 {
		t.Fatal(code)
	}
}
func TestNotesFoldersTagsAndLiteralSearch(t *testing.T) {
	f := newNotesFixture(t)
	folder := f.must("POST", "/api/v1/folders", map[string]any{"name": " 工作 "}, 201)
	child := f.must("POST", "/api/v1/folders", map[string]any{"name": "二级", "parent_id": folder["id"]}, 201)
	leaf := f.must("POST", "/api/v1/folders", map[string]any{"name": "三级", "parent_id": child["id"]}, 201)
	f.must("POST", "/api/v1/folders", map[string]any{"name": "四级", "parent_id": leaf["id"]}, 422)
	f.must("POST", "/api/v1/folders", map[string]any{"name": "工作"}, 409)
	tag := f.must("POST", "/api/v1/tags", map[string]any{"name": " 项目 "}, 201)
	f.must("POST", "/api/v1/tags", map[string]any{"name": "项目"}, 409)
	n := f.create("literal%_")
	path := "/api/v1/notes/" + n["id"].(string)
	f.must("PATCH", path, map[string]any{"expected_version": 1, "folder_id": leaf["id"], "tag_ids": []any{tag["id"]}}, 200)
	f.create("other")
	items := f.must("GET", "/api/v1/notes?query=%25_", nil, 200)["items"].([]any)
	if len(items) != 1 {
		t.Fatal(items)
	}
	items = f.must("GET", "/api/v1/notes?folder_id="+folder["id"].(string), nil, 200)["items"].([]any)
	if len(items) != 1 {
		t.Fatal(items)
	}
	f.must("DELETE", "/api/v1/tags/"+tag["id"].(string), nil, http.StatusNoContent)
	n = f.must("GET", path, nil, 200)
	if n["version"] != float64(3) || len(n["tags"].([]any)) != 0 {
		t.Fatal(n)
	}
	rev := f.must("GET", path+"/history/3", nil, 200)
	if rev["action"] != "tag_delete" {
		t.Fatal(rev)
	}
}
func TestNotesBlocksAtomicAndUnknownFields(t *testing.T) {
	f := newNotesFixture(t)
	n := f.create("blocks")
	path := "/api/v1/notes/" + n["id"].(string)
	id := n["blocks"].([]any)[0].(map[string]any)["id"]
	n = f.must("POST", path+"/blocks", map[string]any{"expected_version": 1, "operations": []any{map[string]any{"op": "insert", "after_id": id, "content": map[string]any{"type": "paragraph"}}}}, 200)
	if len(n["blocks"].([]any)) != 2 {
		t.Fatal(n)
	}
	f.must("POST", path+"/blocks", map[string]any{"expected_version": 2, "operations": []any{map[string]any{"op": "delete", "block_id": id}, map[string]any{"op": "delete", "block_id": "missing"}}}, 422)
	n = f.must("GET", path, nil, 200)
	if n["version"] != float64(2) || len(n["blocks"].([]any)) != 2 {
		t.Fatal(n)
	}
	f.must("PATCH", path, map[string]any{"expected_version": 2, "title": nil}, 422)
	f.must("PATCH", path, map[string]any{"expected_version": 2, "unknown": true}, 422)
	f.must("GET", "/api/v1/notes?cursor=bad", nil, 422)
}

func TestNotesOwnershipScopesAndPagination(t *testing.T) {
	f := newNotesFixture(t)
	n := f.create("owner")
	f.create("second")
	path := "/api/v1/notes/" + n["id"].(string)
	first := f.must("GET", "/api/v1/notes?limit=1", nil, 200)
	second := f.must("GET", "/api/v1/notes?limit=1&cursor="+first["next_cursor"].(string), nil, 200)
	if first["items"].([]any)[0].(map[string]any)["id"] == second["items"].([]any)[0].(map[string]any)["id"] || second["next_cursor"] != nil {
		t.Fatal(first, second)
	}
	var token store.Token
	f.app.DB.Where("secret_hash = ?", hexDigest(f.secret)).Take(&token)
	token.Scopes = store.List{"notes:read"}
	if e := f.app.DB.Model(&token).Update("scopes", token.Scopes).Error; e != nil {
		t.Fatal(e)
	}
	f.must("GET", path+"/history/1", nil, 200)
	f.must("PATCH", path, map[string]any{"expected_version": 1, "title": "scope denied"}, 403)
	f.must("GET", "/api/v1/tags", nil, 403)
	foreign := store.User{ID: store.ID(), Email: store.ID() + "@example.com", DisplayName: "other", PasswordHash: "unused", Role: "user", QuotaBytes: 1 << 30, CreatedAt: store.Now()}
	if e := f.app.DB.Create(&foreign).Error; e != nil {
		t.Fatal(e)
	}
	token.UserID = foreign.ID
	token.Scopes = store.List{"notes:read", "notes:update", "notes:create", "folders:write", "tags:write"}
	if e := f.app.DB.Model(&token).Updates(map[string]any{"user_id": token.UserID, "scopes": token.Scopes}).Error; e != nil {
		t.Fatal(e)
	}
	f.must("GET", path, nil, 404)
	f.must("PATCH", path, map[string]any{"expected_version": 1, "title": "cross user"}, 404)
	f.must("GET", path+"/history/1", nil, 404)
	f.must("POST", "/api/v1/notes", map[string]any{"folder_id": f.folder}, 404)
	f.must("POST", "/api/v1/folders", map[string]any{"name": "foreign", "parent_id": f.folder}, 404)
	if len(f.must("GET", "/api/v1/notes", nil, 200)["items"].([]any)) != 0 {
		t.Fatal("foreign notes visible")
	}
}
func hexDigest(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }

func TestNotesIdempotencyConcurrentActorsExpiry(t *testing.T) {
	f := newNotesFixture(t)
	body := map[string]any{"folder_id": f.folder, "title": "concurrent"}
	var wg sync.WaitGroup
	out := make(chan map[string]any, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, r := f.req("POST", "/api/v1/notes", body, "concurrent-once-123")
			if code != 201 {
				t.Errorf("status %d: %v", code, r)
			}
			out <- r
		}()
	}
	wg.Wait()
	close(out)
	var id any
	for r := range out {
		if id != nil && id != r["id"] {
			t.Fatal("duplicate create", id, r)
		}
		id = r["id"]
	}
	if len(f.must("GET", "/api/v1/notes", nil, 200)["items"].([]any)) != 1 {
		t.Fatal("duplicate persisted")
	}
	var token store.Token
	f.app.DB.Where("secret_hash = ?", hexDigest(f.secret)).Take(&token)
	token.ID = store.ID()
	secondSecret := "sj_" + store.ID()
	token.SecretHash = hexDigest(secondSecret)
	if e := f.app.DB.Create(&token).Error; e != nil {
		t.Fatal(e)
	}
	f.secret = secondSecret
	code, r := f.req("POST", "/api/v1/notes", body, "concurrent-once-123")
	if code != 201 || r["id"] == id {
		t.Fatal("actor scopes collapsed", code, r)
	}
	if e := f.app.DB.Model(&store.Idempotency{}).Where("1 = 1").Update("expires_at", store.Time{Time: time.Now().Add(-time.Minute)}).Error; e != nil {
		t.Fatal(e)
	}
	code, expired := f.req("POST", "/api/v1/notes", body, "concurrent-once-123")
	if code != 201 || expired["id"] == r["id"] {
		t.Fatal(code, expired)
	}
	code, _ = f.req("POST", "/api/v1/notes", body, "bad")
	if code != 422 {
		t.Fatal(code)
	}
}

func TestNotesRevisionFailureRollsBack(t *testing.T) {
	f := newNotesFixture(t)
	n := f.create("before")
	path := "/api/v1/notes/" + n["id"].(string)
	if e := f.app.DB.Exec("CREATE TRIGGER reject_revision BEFORE INSERT ON note_revisions BEGIN SELECT RAISE(ABORT, 'revision failure'); END").Error; e != nil {
		t.Fatal(e)
	}
	f.must("PATCH", path, map[string]any{"expected_version": 1, "title": "must rollback"}, 500)
	after := f.must("GET", path, nil, 200)
	if after["version"] != float64(1) || after["title"] != "before" {
		t.Fatal(after)
	}
	h := f.must("GET", path+"/history", nil, 200)
	if len(h["items"].([]any)) != 1 {
		t.Fatal(h)
	}
}

func TestNotesMarkdownPreviewAndUnsafeOverwrite(t *testing.T) {
	f := newNotesFixture(t)
	n := f.must("POST", "/api/v1/notes", map[string]any{"folder_id": f.folder, "content_format": "markdown", "content": "# 标题\n\n**正文**"}, 201)
	path := "/api/v1/notes/" + n["id"].(string)
	n = f.must("GET", path+"?content_format=markdown", nil, 200)
	if n["markdown_roundtrip_safe"] != true || n["markdown"] == "" {
		t.Fatal(n)
	}
	f.must("PATCH", path, map[string]any{"expected_version": 1, "content_format": "markdown", "content": n["markdown"]}, 200)
	p := f.must("POST", "/api/v1/notes/validate-content", map[string]any{"content_format": "markdown", "content": "| A | B |\n| --- | --- |\n| 1 | 2 |"}, 200)
	if len(p["warnings"].([]any)) == 0 {
		t.Fatal(p)
	}
	underline := map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "underline", "marks": []any{map[string]any{"type": "underline"}}}}}}}
	n = f.must("POST", "/api/v1/notes", map[string]any{"folder_id": f.folder, "content_json": underline}, 201)
	f.must("PATCH", "/api/v1/notes/"+n["id"].(string), map[string]any{"expected_version": 1, "content_format": "markdown", "content": "overwrite"}, 422)
}
