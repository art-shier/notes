// Package backup implements private offline server backups compatible with v1.
package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type File struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}
type Manifest struct {
	Format          string           `json:"format"`
	SchemaVersion   int              `json:"schema_version"`
	CreatedAt       string           `json:"created_at"`
	DatabaseKind    string           `json:"database_kind"`
	AlembicRevision string           `json:"alembic_revision"`
	Counts          map[string]int64 `json:"counts"`
	Files           []File           `json:"files"`
}

func safePath(path string) error {
	abs, e := filepath.Abs(path)
	if e != nil {
		return e
	}
	for p := abs; ; p = filepath.Dir(p) {
		info, e := os.Lstat(p)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if e == nil && info.Mode()&os.ModeSymlink != 0 {
			return errors.New("symlinks are not allowed")
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
	}
	return nil
}
func inside(path, root string) bool {
	rel, e := filepath.Rel(root, path)
	return e == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}
func safeName(name, kind string) bool {
	db := "database.dump"
	if kind == "sqlite" {
		db = "database.sqlite"
	}
	if name == db {
		return true
	}
	if !strings.HasPrefix(name, "attachments/") || strings.Contains(name, "\\") {
		return false
	}
	base := strings.TrimPrefix(name, "attachments/")
	if !strings.HasSuffix(base, ".bin") {
		return false
	}
	id := strings.TrimSuffix(base, ".bin")
	u, e := uuid.Parse(id)
	return e == nil && u.String() == id
}
func hashFile(path string) (int64, string, error) {
	if e := safePath(path); e != nil {
		return 0, "", e
	}
	f, e := os.Open(path)
	if e != nil {
		return 0, "", e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() {
		return 0, "", errors.New("unsafe file")
	}
	h := sha256.New()
	n, e := io.Copy(h, f)
	return n, hex.EncodeToString(h.Sum(nil)), e
}
func copyFile(src, dst string) error {
	if e := safePath(src); e != nil {
		return e
	}
	if e := safePath(dst); e != nil {
		return e
	}
	in, e := os.Open(src)
	if e != nil {
		return e
	}
	defer in.Close()
	info, e := in.Stat()
	if e != nil || !info.Mode().IsRegular() {
		return errors.New("unsafe source file")
	}
	out, e := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = io.Copy(out, in)
	if e == nil {
		e = out.Sync()
	}
	ce := out.Close()
	if e == nil {
		e = ce
	}
	return e
}
func checkedCopy(src, dst string, f File) error {
	if e := copyFile(src, dst); e != nil {
		return e
	}
	n, h, e := hashFile(dst)
	if e != nil {
		return e
	}
	if n != f.SizeBytes || h != f.SHA256 {
		return errors.New("copied backup file differs from manifest")
	}
	return nil
}
func exists(path string) bool { _, e := os.Lstat(path); return e == nil || !os.IsNotExist(e) }
func Verify(path string) (*Manifest, error) {
	if e := safePath(path); e != nil {
		return nil, e
	}
	root, e := filepath.Abs(path)
	if e != nil {
		return nil, e
	}
	mp := filepath.Join(root, "manifest.json")
	if e = safePath(mp); e != nil {
		return nil, e
	}
	info, e := os.Stat(mp)
	if e != nil || !info.Mode().IsRegular() || info.Size() > 16<<20 {
		return nil, errors.New("missing, unsafe or oversized manifest")
	}
	data, e := os.ReadFile(mp)
	if e != nil {
		return nil, e
	}
	var m Manifest
	if e = json.Unmarshal(data, &m); e != nil {
		return nil, errors.New("invalid backup manifest")
	}
	if m.Format != "shiji-server-backup" || m.SchemaVersion != 1 || (m.DatabaseKind != "sqlite" && m.DatabaseKind != "postgresql") || m.Counts == nil || m.Files == nil {
		return nil, errors.New("unsupported backup format")
	}
	seen := map[string]bool{"manifest.json": true}
	for _, f := range m.Files {
		if !safeName(f.Path, m.DatabaseKind) || seen[f.Path] || f.SizeBytes < 0 {
			return nil, errors.New("unsafe or duplicate manifest path")
		}
		n, h, e := hashFile(filepath.Join(root, filepath.FromSlash(f.Path)))
		if e != nil || n != f.SizeBytes || h != f.SHA256 {
			return nil, errors.New("missing, unsafe or damaged backup file")
		}
		seen[f.Path] = true
	}
	db := "database.dump"
	if m.DatabaseKind == "sqlite" {
		db = "database.sqlite"
	}
	if !seen[db] {
		return nil, errors.New("missing backup database")
	}
	e = filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if p == root {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("unsafe backup symlink")
		}
		if d.IsDir() {
			if rel != "attachments" {
				return errors.New("unexpected backup directory")
			}
			return nil
		}
		if !seen[rel] {
			return errors.New("unexpected backup file")
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	return &m, nil
}
func acquireDirLock(root string) (func(), error) {
	p := root + ".backup.lock"
	f, e := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return nil, fmt.Errorf("attachment directory is locked; if no backup/restore is running, inspect and remove %s", p)
	}
	f.Close()
	return func() { os.Remove(p) }, nil
}
