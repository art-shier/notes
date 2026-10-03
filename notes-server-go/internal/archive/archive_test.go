package archive

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gorm.io/gorm"
	"io"
	"os"
	"path/filepath"
	"shiji/internal/config"
	"shiji/internal/store"
	"strings"
	"testing"
)

func TestArchiveHistoryImageAndHashes(t *testing.T) {
	root := t.TempDir()
	db, e := store.Open("sqlite:///" + filepath.ToSlash(filepath.Join(root, "db")))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close(db)
	if e = store.Migrate(db); e != nil {
		t.Fatal(e)
	}
	u := store.User{ID: store.ID(), Email: "owner@example.test", DisplayName: "Owner", PasswordHash: "private-secret", Role: "member", QuotaBytes: 1000, CreatedAt: store.Now()}
	db.Create(&u)
	folder := store.Folder{ID: store.ID(), UserID: u.ID, Name: "Inbox", ParentKey: "root", IsInbox: true}
	db.Create(&folder)
	aid := store.ID()
	a := store.Attachment{ID: aid, UserID: u.ID, ObjectKey: aid + ".bin", MimeType: "image/png", SizeBytes: 4, CreatedAt: store.Now()}
	db.Create(&a)
	os.WriteFile(filepath.Join(root, a.ObjectKey), []byte("data"), 0600)
	doc := store.Map{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "hello"}}}}}
	note := store.Note{ID: store.ID(), UserID: u.ID, FolderID: folder.ID, Title: "<script>Title</script>", ContentJSON: doc, BlockIDs: store.List{}, Version: 2, Source: "web", CreatedAt: store.Now(), UpdatedAt: store.Now()}
	db.Create(&note)
	old := store.Map{"type": "doc", "content": []any{map[string]any{"type": "image", "attrs": map[string]any{"attachment_id": aid}}}}
	rev := store.Revision{ID: store.ID(), UserID: u.ID, NoteID: note.ID, Version: 1, Snapshot: store.Map{"title": "old", "content_json": old}, Action: "create", Actor: "web", SavedAt: store.Now()}
	db.Create(&rev)
	view := func(tx *gorm.DB, n store.Note) (map[string]any, error) {
		return map[string]any{"id": n.ID, "title": n.Title, "content_json": map[string]any(n.ContentJSON), "version": n.Version, "tags": []any{map[string]any{"name": "离线标签"}}, "updated_at": n.UpdatedAt, "trashed": n.Trashed}, nil
	}
	s := config.Settings{AttachmentsDir: root, ExportLimit: 1 << 20}
	p := filepath.Join(root, "archive.zip")
	m, e := Build(db, s, u.ID, p, Options{IncludeTrash: true, IncludeHistory: true}, view)
	if e != nil {
		t.Fatal(e)
	}
	if m.Counts["notes"] != 1 || m.Counts["revisions"] != 1 || m.Counts["attachments"] != 1 {
		t.Fatal(m.Counts)
	}
	z, e := zip.OpenReader(p)
	if e != nil {
		t.Fatal(e)
	}
	defer z.Close()
	files := map[string][]byte{}
	for _, f := range z.File {
		r, _ := f.Open()
		b, _ := io.ReadAll(r)
		r.Close()
		files[f.Name] = b
	}
	for _, f := range m.Files {
		b := files[f.Path]
		sum := sha256.Sum256(b)
		if int64(len(b)) != f.SizeBytes || hex.EncodeToString(sum[:]) != f.SHA256 {
			t.Fatal("manifest mismatch")
		}
	}
	if len(files) != len(m.Files)+1 {
		t.Fatal("manifest coverage")
	}
	if strings.Contains(string(files["notes/"+note.ID+".html"]), "<script>") {
		t.Fatal("HTML injection")
	}
	if !strings.Contains(string(files["notes/"+note.ID+".html"]), "离线标签") {
		t.Fatal("offline HTML lost tag metadata")
	}
	if string(files["attachments/"+aid+".png"]) != "data" {
		t.Fatal("history image missing")
	}
	var library map[string]any
	json.Unmarshal(files["library.json"], &library)
	if strings.Contains(string(files["library.json"]), "private-secret") {
		t.Fatal("credential leak")
	}
	without, e := Build(db, s, u.ID, filepath.Join(root, "no-history.zip"), Options{}, view)
	if e != nil || without.Counts["attachments"] != 0 || without.Counts["revisions"] != 0 {
		t.Fatal("history option ignored", e)
	}
	foreign := u
	foreign.ID = store.ID()
	foreign.Email = "history-foreign@test.invalid"
	if e = db.Create(&foreign).Error; e != nil {
		t.Fatal(e)
	}
	if e = db.Model(&store.Attachment{}).Where("id = ?", aid).Update("user_id", foreign.ID).Error; e != nil {
		t.Fatal(e)
	}
	if _, e = Build(db, s, u.ID, filepath.Join(root, "foreign-history.zip"), Options{IncludeHistory: true}, view); e == nil {
		t.Fatal("foreign history-only image accepted")
	}
	if e = db.Model(&store.Attachment{}).Where("id = ?", aid).Update("user_id", u.ID).Error; e != nil {
		t.Fatal(e)
	}
	s.ExportLimit = 20
	if _, e = Build(db, s, u.ID, filepath.Join(root, "limit.zip"), Options{}, view); e == nil {
		t.Fatal("size limit ignored")
	}
	os.Remove(filepath.Join(root, a.ObjectKey))
	s.ExportLimit = 1 << 20
	if _, e = Build(db, s, u.ID, filepath.Join(root, "missing.zip"), Options{IncludeHistory: true}, view); e == nil {
		t.Fatal("missing history image accepted")
	}
}

func TestCurrentImageOfflinePathAndPinnedSnapshot(t *testing.T) {
	root := t.TempDir()
	url := "sqlite:///" + filepath.ToSlash(filepath.Join(root, "db"))
	db, e := store.Open(url)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close(db)
	if e = store.Migrate(db); e != nil {
		t.Fatal(e)
	}
	u := store.User{ID: store.ID(), Email: "snapshot@test.invalid", Role: "member", DisplayName: "Owner", CreatedAt: store.Now()}
	if e = db.Create(&u).Error; e != nil {
		t.Fatal(e)
	}
	f := store.Folder{ID: store.ID(), UserID: u.ID, ParentKey: "root", Name: "Inbox"}
	db.Create(&f)
	a := store.Attachment{ID: store.ID(), UserID: u.ID, ObjectKey: store.ID() + ".bin", MimeType: "image/png", SizeBytes: 4, CreatedAt: store.Now()}
	db.Create(&a)
	os.WriteFile(filepath.Join(root, a.ObjectKey), []byte("data"), 0600)
	n := store.Note{ID: store.ID(), UserID: u.ID, FolderID: f.ID, Title: "Before", ContentJSON: store.Map{"type": "doc", "content": []any{map[string]any{"type": "image", "attrs": map[string]any{"attachment_id": a.ID}}}}, BlockIDs: store.List{}, Source: "web", Version: 1, CreatedAt: store.Now(), UpdatedAt: store.Now()}
	if e = db.Create(&n).Error; e != nil {
		t.Fatal(e)
	}
	other, e := store.Open(url)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close(other)
	changed := false
	snapshot := func(tx *gorm.DB, n store.Note) (map[string]any, error) {
		var count int64
		if e := tx.Model(&store.Tag{}).Where("user_id = ?", u.ID).Count(&count).Error; e != nil {
			return nil, e
		}
		if !changed {
			changed = true
			if e := other.Model(&store.Note{}).Where("id = ?", n.ID).Update("title", "After").Error; e != nil {
				return nil, e
			}
		}
		return map[string]any{"id": n.ID, "title": n.Title, "content_json": n.ContentJSON}, nil
	}
	path := filepath.Join(root, "test.zip")
	if _, e = Build(db, config.Settings{AttachmentsDir: root, ExportLimit: 1 << 20}, u.ID, path, Options{}, snapshot); e != nil {
		t.Fatal(e)
	}
	z, e := zip.OpenReader(path)
	if e != nil {
		t.Fatal(e)
	}
	defer z.Close()
	for _, file := range z.File {
		if file.Name == "notes/"+n.ID+".html" {
			r, _ := file.Open()
			b, _ := io.ReadAll(r)
			r.Close()
			body := string(b)
			if !strings.Contains(body, `src="../attachments/`+a.ID+`.png"`) || strings.Contains(body, "After") || !strings.Contains(body, "Before") {
				t.Fatal("offline path or pinned snapshot broken", body)
			}
		}
	}
	foreign := u
	foreign.ID = store.ID()
	foreign.Email = "foreign@test.invalid"
	if e = other.Create(&foreign).Error; e != nil {
		t.Fatal(e)
	}
	if e = other.Model(&store.Attachment{}).Where("id = ?", a.ID).Update("user_id", foreign.ID).Error; e != nil {
		t.Fatal(e)
	}
	if _, e = Build(db, config.Settings{AttachmentsDir: root, ExportLimit: 1 << 20}, u.ID, filepath.Join(root, "foreign.zip"), Options{}, snapshot); e == nil {
		t.Fatal("foreign image accepted")
	}
}
