package backup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"shiji/internal/config"
	"shiji/internal/store"
	"strings"
	"testing"
)

func fixture(t *testing.T) (config.Settings, string) {
	t.Helper()
	root := t.TempDir()
	s := config.Settings{DatabaseURL: "sqlite:///" + filepath.ToSlash(filepath.Join(root, "source.db")), AttachmentsDir: filepath.Join(root, "images")}
	if e := os.Mkdir(s.AttachmentsDir, 0700); e != nil {
		t.Fatal(e)
	}
	db, e := store.Open(s.DatabaseURL)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { store.Close(db) })
	if e = store.Migrate(db); e != nil {
		t.Fatal(e)
	}
	u := store.User{ID: store.ID(), Email: "owner@example.test", DisplayName: "Owner", PasswordHash: "secret-hash", Role: "member", QuotaBytes: 1000, CreatedAt: store.Now()}
	if e = db.Create(&u).Error; e != nil {
		t.Fatal(e)
	}
	a := store.Attachment{ID: store.ID(), UserID: u.ID, ObjectKey: store.ID() + ".bin", MimeType: "image/png", SizeBytes: 4, CreatedAt: store.Now()}
	if e = db.Create(&a).Error; e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(s.AttachmentsDir, a.ObjectKey), []byte("data"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = db.Create(&store.Session{ID: store.ID(), UserID: u.ID, SecretHash: "session", ExpiresAt: store.Now()}).Error; e != nil {
		t.Fatal(e)
	}
	if e = db.Create(&store.Token{ID: store.ID(), UserID: u.ID, Name: "preserved", Prefix: "test", SecretHash: "token-hash", Scopes: store.List{"notes:read"}, ExpiresAt: store.Now(), CreatedAt: store.Now()}).Error; e != nil {
		t.Fatal(e)
	}
	if e = db.Create(&store.Export{ID: store.ID(), UserID: u.ID, Ready: true, Manifest: store.Map{}, CreatedAt: store.Now(), ExpiresAt: store.Now()}).Error; e != nil {
		t.Fatal(e)
	}
	return s, root
}
func TestBackupRestoreWALAndInvalidateSessions(t *testing.T) {
	s, root := fixture(t)
	out := filepath.Join(root, "backup")
	if _, e := Create(s, out, false); e == nil {
		t.Fatal("accepted online backup")
	}
	m, e := Create(s, out, true)
	if e != nil {
		t.Fatal(e)
	}
	if m.Counts["users"] != 1 || m.Counts["attachments"] != 1 {
		t.Fatal(m.Counts)
	}
	if _, e = Verify(out); e != nil {
		t.Fatal(e)
	}
	target := s
	target.DatabaseURL = "sqlite:///" + filepath.ToSlash(filepath.Join(root, "restored.db"))
	target.AttachmentsDir = filepath.Join(root, "restored-images")
	if _, e = Restore(target, out, true); e != nil {
		t.Fatal(e)
	}
	db, e := store.Open(target.DatabaseURL)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close(db)
	var count int64
	db.Model(&store.User{}).Count(&count)
	if count != 1 {
		t.Fatal("user missing")
	}
	db.Model(&store.Session{}).Count(&count)
	if count != 0 {
		t.Fatal("old session survived")
	}
	db.Model(&store.Export{}).Count(&count)
	if count != 0 {
		t.Fatal("export tasks survived")
	}
	var tok store.Token
	if e = db.First(&tok).Error; e != nil || tok.SecretHash != "token-hash" {
		t.Fatal("token not preserved", e)
	}
	var attachment store.Attachment
	if e = db.First(&attachment).Error; e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(target.AttachmentsDir, attachment.ObjectKey))
	if e != nil || string(b) != "data" {
		t.Fatal("attachment not restored", e)
	}
	if _, e = Restore(target, out, true); e == nil {
		t.Fatal("overwrote existing target")
	}
	if _, e = Create(s, out, true); e == nil {
		t.Fatal("overwrote backup")
	}
}

func TestRejectSymlinksAndNonEmptyDestination(t *testing.T) {
	s, root := fixture(t)
	out := filepath.Join(root, "backup")
	if _, e := Create(s, out, true); e != nil {
		t.Fatal(e)
	}
	target := s
	target.DatabaseURL = "sqlite:///" + filepath.ToSlash(filepath.Join(root, "new.db"))
	target.AttachmentsDir = filepath.Join(root, "new-images")
	os.Mkdir(target.AttachmentsDir, 0700)
	keep := filepath.Join(target.AttachmentsDir, "keep")
	os.WriteFile(keep, []byte("keep"), 0600)
	if _, e := Restore(target, out, true); e == nil {
		t.Fatal("nonempty target accepted")
	}
	if b, _ := os.ReadFile(keep); string(b) != "keep" {
		t.Fatal("existing data altered")
	}
	link := filepath.Join(root, "backup-link")
	if e := os.Symlink(out, link); e != nil {
		t.Skip("symlink creation unavailable: " + e.Error())
	}
	if _, e := Verify(link); e == nil {
		t.Fatal("backup symlink accepted")
	}
	entries, _ := os.ReadDir(filepath.Join(out, "attachments"))
	image := filepath.Join(out, "attachments", entries[0].Name())
	if e := os.Remove(image); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(keep, image); e != nil {
		t.Fatal(e)
	}
	if _, e := Verify(out); e == nil {
		t.Fatal("attachment symlink accepted")
	}
}

func TestPGCommandNeverContainsPassword(t *testing.T) {
	for _, raw := range []string{"postgresql://notes:sensitive-password@db/notes", "postgresql://notes@db/notes?password=sensitive-password"} {
		cmd, e := pgCommand("pg_dump", raw, []string{"--format=custom"})
		if e != nil {
			t.Fatal(e)
		}
		for _, arg := range cmd.Args {
			if strings.Contains(arg, "sensitive-password") {
				t.Fatal("password in argv")
			}
		}
		found := false
		for _, v := range cmd.Env {
			if v == "PGPASSWORD=sensitive-password" {
				found = true
			}
		}
		if !found {
			t.Fatal("password not passed through environment")
		}
	}
}
func TestRejectDamageAndUnsafePaths(t *testing.T) {
	for _, damage := range []string{"hash", "extra", "escape", "missing"} {
		t.Run(damage, func(t *testing.T) {
			s, root := fixture(t)
			out := filepath.Join(root, "backup")
			if _, e := Create(s, out, true); e != nil {
				t.Fatal(e)
			}
			switch damage {
			case "hash":
				os.WriteFile(filepath.Join(out, "database.sqlite"), []byte("broken"), 0600)
			case "extra":
				os.WriteFile(filepath.Join(out, "extra"), []byte("extra"), 0600)
			case "missing":
				os.Remove(filepath.Join(out, "database.sqlite"))
			case "escape":
				b, _ := os.ReadFile(filepath.Join(out, "manifest.json"))
				var m Manifest
				json.Unmarshal(b, &m)
				m.Files[0].Path = "../outside"
				b, _ = json.Marshal(m)
				os.WriteFile(filepath.Join(out, "manifest.json"), b, 0600)
			}
			if _, e := Verify(out); e == nil {
				t.Fatal("accepted damage")
			}
		})
	}
}
