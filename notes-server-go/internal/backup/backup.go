package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"shiji/internal/config"
	"shiji/internal/document"
	"shiji/internal/store"
	"sort"
	"strings"
	"time"
)

var tables = []string{"users", "invitations", "sessions", "folders", "notes", "attachments", "api_tokens", "tags", "note_tags", "idempotency_records", "note_revisions", "export_bundles"}

type RestoreResult struct {
	DatabaseKind        string           `json:"database_kind"`
	AlembicRevision     string           `json:"alembic_revision"`
	Counts              map[string]int64 `json:"counts"`
	SessionsInvalidated bool             `json:"sessions_invalidated"`
	ExportsCleared      bool             `json:"exports_cleared"`
}

func dbKind(raw string) (string, error) {
	if strings.HasPrefix(raw, "sqlite:///") {
		return "sqlite", nil
	}
	u, e := url.Parse(raw)
	if e == nil && (u.Scheme == "postgres" || u.Scheme == "postgresql") {
		return "postgresql", nil
	}
	return "", errors.New("only persistent SQLite and PostgreSQL are supported")
}
func sqlitePath(raw string) (string, error) {
	p := strings.TrimPrefix(raw, "sqlite:///")
	if p == "" || p == ":memory:" || strings.Contains(p, "?") {
		return "", errors.New("persistent SQLite path required")
	}
	if e := safePath(p); e != nil {
		return "", e
	}
	return filepath.Abs(p)
}
func openInspect(raw string) (*gorm.DB, error) { return store.Open(raw) }
func inventory(db *gorm.DB, root string) (map[string]int64, error) {
	if e := store.Check(db); e != nil {
		return nil, e
	}
	counts := map[string]int64{}
	for _, t := range tables {
		var n int64
		if e := db.Table(t).Count(&n).Error; e != nil {
			return nil, e
		}
		counts[t] = n
	}
	if db.Dialector.Name() == "sqlite" {
		var check string
		if e := db.Raw("PRAGMA integrity_check").Scan(&check).Error; e != nil || check != "ok" {
			return nil, errors.New("database integrity check failed")
		}
		rows, e := db.Raw("PRAGMA foreign_key_check").Rows()
		if e != nil {
			return nil, e
		}
		bad := rows.Next()
		rows.Close()
		if bad {
			return nil, errors.New("database foreign keys are invalid")
		}
	}
	var attachments []store.Attachment
	if e := db.Find(&attachments).Error; e != nil {
		return nil, e
	}
	owners := map[string]string{}
	if e := safePath(root); e != nil {
		return nil, e
	}
	info, e := os.Stat(root)
	if e != nil || !info.IsDir() {
		return nil, errors.New("missing attachment directory")
	}
	for _, a := range attachments {
		if !safeName("attachments/"+a.ObjectKey, "sqlite") {
			return nil, errors.New("unsafe attachment key")
		}
		p := filepath.Join(root, a.ObjectKey)
		if e := safePath(p); e != nil {
			return nil, e
		}
		info, e := os.Stat(p)
		if e != nil || !info.Mode().IsRegular() || info.Size() != a.SizeBytes {
			return nil, errors.New("missing or incomplete attachment")
		}
		owners[a.ID] = a.UserID
	}
	checkDoc := func(doc map[string]any, owner string) error {
		for _, id := range document.ImageIDs(doc) {
			if owners[id] != owner || owner == "" {
				return errors.New("missing or foreign image reference")
			}
		}
		return nil
	}
	var notes []store.Note
	if e = db.Select("id", "user_id", "content_json").Find(&notes).Error; e != nil {
		return nil, e
	}
	for _, n := range notes {
		if e = checkDoc(map[string]any(n.ContentJSON), n.UserID); e != nil {
			return nil, e
		}
	}
	var revisions []store.Revision
	if e = db.Find(&revisions).Error; e != nil {
		return nil, e
	}
	for _, r := range revisions {
		doc, ok := r.Snapshot["content_json"].(map[string]any)
		if !ok {
			return nil, errors.New("invalid history snapshot")
		}
		if e = checkDoc(doc, r.UserID); e != nil {
			return nil, e
		}
	}
	return counts, nil
}
func pgCommand(command, raw string, args []string) (*exec.Cmd, error) {
	u, e := url.Parse(raw)
	if e != nil {
		return nil, errors.New("invalid PostgreSQL URL")
	}
	password := ""
	if u.User != nil {
		password, _ = u.User.Password()
		u.User = url.User(u.User.Username())
	}
	q := u.Query()
	if p := q.Get("password"); p != "" {
		password = p
	}
	q.Del("password")
	u.RawQuery = q.Encode()
	env := []string{}
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "PGPASSWORD=") {
			env = append(env, v)
		}
	}
	if password != "" {
		env = append(env, "PGPASSWORD="+password)
	}
	cmd := exec.Command(command, append([]string{"--dbname=" + u.String()}, args...)...)
	cmd.Env = env
	return cmd, nil
}
func pgTool(command, raw string, args []string) error {
	cmd, e := pgCommand(command, raw, args)
	if e != nil {
		return e
	}
	version, e := exec.Command(command, "--version").Output()
	if e != nil {
		return fmt.Errorf("%s 16 is required", command)
	}
	parts := strings.Fields(string(version))
	if len(parts) < 3 || !strings.HasPrefix(parts[2], "16.") {
		return errors.New("PostgreSQL client major version must be 16")
	}
	if e = cmd.Run(); e != nil {
		return fmt.Errorf("%s failed; check availability, compatibility and permissions", command)
	}
	return nil
}
func pgVersion(db *gorm.DB) error {
	var v int
	if e := db.Raw("SHOW server_version_num").Scan(&v).Error; e != nil || v/10000 != 16 {
		return errors.New("PostgreSQL server major version must be 16")
	}
	return nil
}
func Create(s config.Settings, output string, appStopped bool) (*Manifest, error) {
	if !appStopped {
		return nil, errors.New("all application instances must be stopped; pass --app-stopped")
	}
	kind, e := dbKind(s.DatabaseURL)
	if e != nil {
		return nil, e
	}
	if e = safePath(output); e != nil {
		return nil, e
	}
	output, e = filepath.Abs(output)
	if e != nil {
		return nil, e
	}
	root, e := filepath.Abs(s.AttachmentsDir)
	if e != nil {
		return nil, e
	}
	if e = safePath(root); e != nil {
		return nil, e
	}
	if exists(output) || inside(output, root) {
		return nil, errors.New("backup destination exists or is inside attachments")
	}
	unlock, e := acquireDirLock(root)
	if e != nil {
		return nil, e
	}
	defer unlock()
	if e = os.MkdirAll(filepath.Dir(output), 0700); e != nil {
		return nil, e
	}
	stage, e := os.MkdirTemp(filepath.Dir(output), ".shiji-backup-")
	if e != nil {
		return nil, e
	}
	defer os.RemoveAll(stage)
	if e = os.Mkdir(filepath.Join(stage, "attachments"), 0700); e != nil {
		return nil, e
	}
	var db *gorm.DB
	dbFile := "database.dump"
	if kind == "sqlite" {
		p, e := sqlitePath(s.DatabaseURL)
		if e != nil {
			return nil, e
		}
		if !exists(p) {
			return nil, errors.New("source database missing")
		}
		raw, e := sql.Open("sqlite", p)
		if e != nil {
			return nil, e
		}
		defer raw.Close()
		conn, e := raw.Conn(context.Background())
		if e != nil {
			return nil, e
		}
		defer conn.Close()
		if _, e = conn.ExecContext(context.Background(), "PRAGMA busy_timeout=30000"); e != nil {
			return nil, e
		}
		if _, e = conn.ExecContext(context.Background(), "BEGIN IMMEDIATE"); e != nil {
			return nil, e
		}
		defer conn.ExecContext(context.Background(), "ROLLBACK")
		reader, e := sql.Open("sqlite", p)
		if e != nil {
			return nil, e
		}
		defer reader.Close()
		dbFile = "database.sqlite"
		if _, e = reader.Exec("VACUUM INTO ?", filepath.Join(stage, dbFile)); e != nil {
			return nil, e
		}
		db, e = openInspect("sqlite:///" + filepath.ToSlash(filepath.Join(stage, dbFile)))
		if e != nil {
			return nil, e
		}
	} else {
		db, e = openInspect(s.DatabaseURL)
		if e != nil {
			return nil, e
		}
		if e = pgVersion(db); e != nil {
			store.Close(db)
			return nil, e
		}
		if e = pgTool("pg_dump", s.DatabaseURL, []string{"--format=custom", "--no-owner", "--no-privileges", "--file=" + filepath.Join(stage, dbFile)}); e != nil {
			store.Close(db)
			return nil, e
		}
	}
	defer store.Close(db)
	counts, e := inventory(db, root)
	if e != nil {
		return nil, e
	}
	entries, e := os.ReadDir(root)
	if e != nil {
		return nil, e
	}
	for _, entry := range entries {
		if !safeName("attachments/"+entry.Name(), kind) {
			return nil, errors.New("unsafe attachment filename")
		}
		if e = copyFile(filepath.Join(root, entry.Name()), filepath.Join(stage, "attachments", entry.Name())); e != nil {
			return nil, e
		}
	}
	if kind == "sqlite" {
		if e = db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error; e != nil {
			return nil, e
		}
		if e = db.Exec("PRAGMA journal_mode=DELETE").Error; e != nil {
			return nil, e
		}
		store.Close(db)
		for _, suffix := range []string{"-wal", "-shm"} {
			os.Remove(filepath.Join(stage, dbFile) + suffix)
		}
	}
	m := &Manifest{Format: "shiji-server-backup", SchemaVersion: 1, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), DatabaseKind: kind, AlembicRevision: store.RevisionHead, Counts: counts, Files: []File{}}
	e = filepath.WalkDir(stage, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		if e = os.Chmod(p, 0600); e != nil {
			return e
		}
		n, h, e := hashFile(p)
		if e != nil {
			return e
		}
		rel, _ := filepath.Rel(stage, p)
		m.Files = append(m.Files, File{filepath.ToSlash(rel), n, h})
		return nil
	})
	if e != nil {
		return nil, e
	}
	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
	data, e := json.MarshalIndent(m, "", "  ")
	if e != nil {
		return nil, e
	}
	if e = os.WriteFile(filepath.Join(stage, "manifest.json"), data, 0600); e != nil {
		return nil, e
	}
	if _, e = Verify(stage); e != nil {
		return nil, e
	}
	if e = os.Mkdir(output, 0700); e != nil {
		return nil, e
	}
	published := false
	defer func() {
		if !published {
			os.RemoveAll(output)
		}
	}()
	for _, name := range []string{dbFile, "attachments", "manifest.json"} {
		if e = os.Rename(filepath.Join(stage, name), filepath.Join(output, name)); e != nil {
			return nil, e
		}
	}
	published = true
	return m, nil
}
func Restore(s config.Settings, path string, appStopped bool) (*RestoreResult, error) {
	if !appStopped {
		return nil, errors.New("all target application instances must be stopped; pass --app-stopped")
	}
	m, e := Verify(path)
	if e != nil {
		return nil, e
	}
	kind, e := dbKind(s.DatabaseURL)
	if e != nil {
		return nil, e
	}
	if kind != m.DatabaseKind || m.AlembicRevision != store.RevisionHead {
		return nil, errors.New("restore requires matching database type and migration version")
	}
	path, _ = filepath.Abs(path)
	root, e := filepath.Abs(s.AttachmentsDir)
	if e != nil {
		return nil, e
	}
	if e = safePath(root); e != nil {
		return nil, e
	}
	if inside(root, path) {
		return nil, errors.New("restore target cannot be inside backup")
	}
	if entries, e := os.ReadDir(root); e == nil && len(entries) > 0 {
		return nil, errors.New("target attachment directory must be empty")
	} else if e != nil && !os.IsNotExist(e) {
		return nil, e
	}
	dbTarget := ""
	if kind == "sqlite" {
		dbTarget, e = sqlitePath(s.DatabaseURL)
		if e != nil {
			return nil, e
		}
		if exists(dbTarget) || exists(dbTarget+"-wal") || exists(dbTarget+"-shm") || inside(dbTarget, path) || inside(dbTarget, root) {
			return nil, errors.New("target database already exists or is unsafe")
		}
	}
	if e = os.MkdirAll(filepath.Dir(root), 0700); e != nil {
		return nil, e
	}
	unlock, e := acquireDirLock(root)
	if e != nil {
		return nil, e
	}
	defer unlock()
	stage, e := os.MkdirTemp(filepath.Dir(root), ".shiji-restore-")
	if e != nil {
		return nil, e
	}
	defer os.RemoveAll(stage)
	for _, f := range m.Files {
		if strings.HasPrefix(f.Path, "attachments/") {
			if e = checkedCopy(filepath.Join(path, filepath.FromSlash(f.Path)), filepath.Join(stage, filepath.Base(f.Path)), f); e != nil {
				return nil, e
			}
		}
	}
	var db *gorm.DB
	dbstage := ""
	if kind == "sqlite" {
		if e = os.MkdirAll(filepath.Dir(dbTarget), 0700); e != nil {
			return nil, e
		}
		dbstage = filepath.Join(filepath.Dir(dbTarget), ".shiji-db-"+store.ID()+".sqlite")
		defer func() {
			for _, suffix := range []string{"", "-wal", "-shm"} {
				os.Remove(dbstage + suffix)
			}
		}()
		for _, f := range m.Files {
			if f.Path == "database.sqlite" {
				if e = checkedCopy(filepath.Join(path, f.Path), dbstage, f); e != nil {
					return nil, e
				}
			}
		}
		db, e = openInspect("sqlite:///" + filepath.ToSlash(dbstage))
		if e != nil {
			return nil, e
		}
	} else {
		db, e = openInspect(s.DatabaseURL)
		if e != nil {
			return nil, e
		}
		if e = pgVersion(db); e != nil {
			store.Close(db)
			return nil, e
		}
		var count int64
		e = db.Raw("SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%' AND c.relkind IN ('r','p','v','m','S','f')").Scan(&count).Error
		if e != nil || count > 0 {
			store.Close(db)
			return nil, errors.New("target PostgreSQL database must be empty")
		}
		if e = pgTool("pg_restore", s.DatabaseURL, []string{"--single-transaction", "--exit-on-error", "--no-owner", "--no-privileges", filepath.Join(path, "database.dump")}); e != nil {
			store.Close(db)
			return nil, e
		}
	}
	defer store.Close(db)
	counts, e := inventory(db, stage)
	if e != nil {
		return nil, e
	}
	if !reflect.DeepEqual(counts, m.Counts) {
		return nil, errors.New("restored database counts differ from manifest")
	}
	if e = db.Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec("DELETE FROM sessions").Error; e != nil {
			return e
		}
		return tx.Exec("DELETE FROM export_bundles").Error
	}); e != nil {
		return nil, e
	}
	if kind == "sqlite" {
		if e = db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error; e != nil {
			return nil, e
		}
		if e = db.Exec("PRAGMA journal_mode=DELETE").Error; e != nil {
			return nil, e
		}
	}
	store.Close(db)
	if e = os.Mkdir(root, 0700); e != nil && !os.IsExist(e) {
		return nil, e
	}
	entries, e := os.ReadDir(root)
	if e != nil || len(entries) > 0 {
		return nil, errors.New("target attachment directory must remain empty")
	}
	files, e := os.ReadDir(stage)
	if e != nil {
		return nil, e
	}
	for _, file := range files {
		src := filepath.Join(stage, file.Name())
		dst := filepath.Join(root, file.Name())
		// The target may be a mounted Docker volume on another filesystem.
		// Exclusive creation preserves no-overwrite without requiring hardlinks.
		if e = copyFile(src, dst); e != nil {
			return nil, e
		}
		if e = os.Remove(src); e != nil {
			return nil, e
		}
	}
	if kind == "sqlite" {
		if exists(dbTarget+"-wal") || exists(dbTarget+"-shm") {
			return nil, errors.New("target database sidecars appeared during restore")
		}
		if e = os.Link(dbstage, dbTarget); e != nil {
			return nil, e
		}
	}
	return &RestoreResult{kind, store.RevisionHead, counts, true, true}, nil
}
