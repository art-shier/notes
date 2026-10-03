// Package archive writes bounded, portable private library exports.
package archive

import (
	"archive/zip"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"hash"
	"html"
	"io"
	"os"
	"path/filepath"
	"shiji/internal/config"
	"shiji/internal/document"
	"shiji/internal/store"
	"sort"
	"strings"
	"time"
)

type Error struct {
	Status        int
	Code, Message string
}

func (e *Error) Error() string { return e.Message }
func missing() error {
	return &Error{409, "attachment_missing", "笔记引用的图片不可用，导出未完成。"}
}

type Options struct {
	IncludeTrash   bool `json:"include_trash"`
	IncludeHistory bool `json:"include_history"`
}
type File struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}
type Manifest struct {
	Format        string         `json:"format"`
	SchemaVersion int            `json:"schema_version"`
	ExportedAt    string         `json:"exported_at"`
	Options       Options        `json:"options"`
	Counts        map[string]int `json:"counts"`
	Files         []File         `json:"files"`
}
type Snapshot func(*gorm.DB, store.Note) (map[string]any, error)
type writer struct {
	zip          *zip.Writer
	limit, total int64
	files        []File
}
type entryWriter struct {
	target io.Writer
	owner  *writer
	hash   hash.Hash
	size   int64
}

func (w *entryWriter) Write(p []byte) (int, error) {
	if w.owner.total+int64(len(p)) > w.owner.limit {
		return 0, &Error{413, "export_limit", "导出内容超过服务器设置的大小上限。"}
	}
	n, e := w.target.Write(p)
	w.owner.total += int64(n)
	w.size += int64(n)
	w.hash.Write(p[:n])
	return n, e
}
func (w *writer) entry(name string, fn func(io.Writer) error, record bool) error {
	target, e := w.zip.Create(name)
	if e != nil {
		return e
	}
	out := &entryWriter{target: target, owner: w, hash: sha256.New()}
	if e = fn(out); e != nil {
		return e
	}
	if record {
		w.files = append(w.files, File{name, out.size, hex.EncodeToString(out.hash.Sum(nil))})
	}
	return nil
}
func (w *writer) text(name, value string) error {
	return w.entry(name, func(out io.Writer) error { _, e := io.WriteString(out, value); return e }, true)
}
func (w *writer) json(name string, value any) error {
	return w.entry(name, func(out io.Writer) error { return json.NewEncoder(out).Encode(value) }, true)
}
func safeUUID(id string) bool { u, e := uuid.Parse(id); return e == nil && u.String() == id }
func SafeFile(root, key string) (*os.File, error) {
	if key == "" || filepath.Base(key) != key || strings.ContainsAny(key, "/\\") {
		return nil, missing()
	}
	abs, e := filepath.Abs(root)
	if e != nil {
		return nil, e
	}
	p := filepath.Join(abs, key)
	for part := p; ; part = filepath.Dir(part) {
		info, e := os.Lstat(part)
		if e != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil, missing()
		}
		if filepath.Dir(part) == part {
			break
		}
	}
	f, e := os.Open(p)
	if e != nil {
		return nil, missing()
	}
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, missing()
	}
	return f, nil
}

const style = `body{max-width:760px;margin:40px auto;padding:0 24px;font:16px/1.8 system-ui,sans-serif;color:#242428;background:#fff;overflow-wrap:anywhere}h1{line-height:1.4}img{max-width:100%;height:auto;display:block;margin:24px 0}pre{white-space:pre-wrap;background:#f5f5f7;padding:16px;border-radius:8px}blockquote{border-left:3px solid #ddd;padding-left:16px;margin-left:0;color:#666}a{color:#3269d4}.meta{font-size:13px;color:#686870}li{margin:6px 0}code{background:#f5f5f7}hr{border:0;border-top:1px solid #ddd}`

func page(title, body string) string {
	return `<!doctype html><html lang="zh-CN"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src 'self' file:; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'"><title>` + html.EscapeString(title) + `</title><style>` + style + `</style></head><body>` + body + `</body></html>`
}
func title(value string) string {
	if value == "" {
		return "未命名笔记"
	}
	return value
}
func Build(db *gorm.DB, s config.Settings, userID, path string, options Options, snapshot Snapshot) (*Manifest, error) {
	file, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return nil, e
	}
	success := false
	defer func() {
		file.Close()
		if !success {
			os.Remove(path)
		}
	}()
	zipper := zip.NewWriter(file)
	w := &writer{zip: zipper, limit: s.ExportLimit, files: []File{}}
	exported := time.Now().UTC().Format(time.RFC3339Nano)
	var result *Manifest
	txOptions := &sql.TxOptions{ReadOnly: true}
	if db.Dialector.Name() == "postgres" {
		txOptions.Isolation = sql.LevelRepeatableRead
	}
	e = db.Transaction(func(tx *gorm.DB) error {
		var folders []store.Folder
		var tags []store.Tag
		if e := tx.Where("user_id = ?", userID).Order("name,id").Find(&folders).Error; e != nil {
			return e
		}
		if e := tx.Where("user_id = ?", userID).Order("name,id").Find(&tags).Error; e != nil {
			return e
		}
		folderNames := map[string]string{}
		folderJSON := []map[string]any{}
		tagJSON := []map[string]any{}
		for _, f := range folders {
			folderNames[f.ID] = f.Name
			folderJSON = append(folderJSON, map[string]any{"id": f.ID, "name": f.Name, "parent_id": f.ParentID, "is_inbox": f.IsInbox})
		}
		for _, t := range tags {
			tagJSON = append(tagJSON, map[string]any{"id": t.ID, "name": t.Name})
		}
		notesQuery := func() *gorm.DB {
			q := tx.Model(&store.Note{}).Where("user_id = ?", userID)
			if !options.IncludeTrash {
				q = q.Where("trashed = ?", false)
			}
			return q.Order("id")
		}
		// pgx allows one active result set per transaction. Close each bounded
		// batch before snapshot callbacks query tags on the same connection.
		eachNote := func(fn func(store.Note) error) error {
			cursor := ""
			for {
				var batch []store.Note
				query := notesQuery().Limit(20)
				if cursor != "" {
					query = query.Where("id > ?", cursor)
				}
				if err := query.Find(&batch).Error; err != nil {
					return err
				}
				if len(batch) == 0 {
					return nil
				}
				for _, note := range batch {
					if err := fn(note); err != nil {
						return err
					}
					cursor = note.ID
				}
			}
		}
		referenced := map[string]bool{}
		index := []store.Note{}
		noteCount, revisionCount := 0, 0
		e := w.entry("library.json", func(out io.Writer) error {
			header, e := json.Marshal(map[string]any{"schema_version": 2, "exported_at": exported, "folders": folderJSON, "tags": tagJSON})
			if e != nil {
				return e
			}
			if _, e = out.Write(append(header[:len(header)-1], []byte(",\"notes\":[")...)); e != nil {
				return e
			}
			e = eachNote(func(n store.Note) error {
				if !safeUUID(n.ID) {
					return errors.New("invalid note id")
				}
				view, e := snapshot(tx, n)
				if e != nil {
					return e
				}
				for _, id := range document.ImageIDs(map[string]any(n.ContentJSON)) {
					referenced[id] = true
				}
				if noteCount > 0 {
					if _, e = io.WriteString(out, ","); e != nil {
						return e
					}
				}
				if e = json.NewEncoder(out).Encode(view); e != nil {
					return e
				}
				index = append(index, store.Note{ID: n.ID, Title: n.Title, FolderID: n.FolderID, Trashed: n.Trashed})
				noteCount++
				return nil
			})
			if e != nil {
				return e
			}
			_, e = io.WriteString(out, "]}")
			return e
		}, true)
		if e != nil {
			return e
		}
		if options.IncludeHistory {
			q := tx.Model(&store.Revision{}).Where("user_id = ? AND note_id IN (?)", userID, notesQuery().Select("id")).Order("note_id,version")
			rows, e := q.Rows()
			if e != nil {
				return e
			}
			for rows.Next() {
				var r store.Revision
				if e = tx.ScanRows(rows, &r); e != nil {
					rows.Close()
					return e
				}
				if !safeUUID(r.NoteID) || r.Version < 1 {
					rows.Close()
					return errors.New("invalid history path")
				}
				doc, ok := r.Snapshot["content_json"].(map[string]any)
				if !ok {
					rows.Close()
					return errors.New("invalid history snapshot")
				}
				for _, id := range document.ImageIDs(doc) {
					referenced[id] = true
				}
				e = w.json(fmt.Sprintf("history/%s/%d.json", r.NoteID, r.Version), map[string]any{"version": r.Version, "actor": r.Actor, "action": r.Action, "restored_from": r.RestoredFrom, "saved_at": r.SavedAt, "snapshot": r.Snapshot})
				if e != nil {
					rows.Close()
					return e
				}
				revisionCount++
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
		}
		ids := make([]string, 0, len(referenced))
		for id := range referenced {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		images := map[string]string{}
		extensions := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp", "image/gif": "gif"}
		for _, id := range ids {
			if !safeUUID(id) {
				return missing()
			}
			var a store.Attachment
			if e := tx.Where("id = ? AND user_id = ?", id, userID).First(&a).Error; e != nil {
				return missing()
			}
			ext, ok := extensions[a.MimeType]
			if !ok {
				return missing()
			}
			source, e := SafeFile(s.AttachmentsDir, a.ObjectKey)
			if e != nil {
				return e
			}
			name := "attachments/" + id + "." + ext
			e = w.entry(name, func(out io.Writer) error {
				n, e := io.Copy(out, source)
				if e == nil && n != a.SizeBytes {
					return &Error{409, "attachment_changed", "图片文件不完整，导出未完成。"}
				}
				return e
			}, true)
			source.Close()
			if e != nil {
				return e
			}
			images[id] = name
		}
		e = eachNote(func(n store.Note) error {
			body, e := document.HTML(map[string]any(n.ContentJSON), images)
			if e != nil {
				return e
			}
			view, e := snapshot(tx, n)
			if e != nil {
				return e
			}
			metadata := []string{folderNames[n.FolderID]}
			if tags, ok := view["tags"].([]any); ok {
				for _, entry := range tags {
					if tag, ok := entry.(map[string]any); ok {
						if name, ok := tag["name"].(string); ok {
							metadata = append(metadata, name)
						}
					}
				}
			}
			metadata = append(metadata, n.UpdatedAt.UTC().Format(time.RFC3339Nano))
			meta := strings.Join(metadata, " · ")
			if n.Trashed {
				meta += " · 回收站"
			}
			e = w.text("notes/"+n.ID+".html", page(title(n.Title), `<a href="../index.html">返回笔记目录</a><h1>`+html.EscapeString(title(n.Title))+`</h1><p class="meta">`+html.EscapeString(meta)+`</p>`+body))
			return e
		})
		if e != nil {
			return e
		}
		var list strings.Builder
		for _, n := range index {
			list.WriteString(`<li><a href="notes/` + n.ID + `.html">` + html.EscapeString(title(n.Title)) + `</a><span class="meta"> · ` + html.EscapeString(folderNames[n.FolderID]))
			if n.Trashed {
				list.WriteString(" · 回收站")
			}
			list.WriteString("</span></li>")
		}
		if e = w.text("index.html", page("拾记导出", fmt.Sprintf(`<h1>拾记 · 笔记导出</h1><p class="meta">%s · %d 篇笔记</p><ul>%s</ul><p class="meta">原始数据位于 library.json；历史版本为 history 目录中的 JSON。这是当前账户的数据导出，不包含账户凭证，也不是整库备份。</p>`, exported, noteCount, list.String()))); e != nil {
			return e
		}
		result = &Manifest{"shiji-archive", 2, exported, options, map[string]int{"notes": noteCount, "revisions": revisionCount, "attachments": len(images)}, w.files}
		return w.entry("manifest.json", func(out io.Writer) error { return json.NewEncoder(out).Encode(result) }, false)
	}, txOptions)
	if e != nil {
		zipper.Close()
		return nil, e
	}
	if e = zipper.Close(); e != nil {
		return nil, e
	}
	if e = file.Sync(); e != nil {
		return nil, e
	}
	if e = file.Close(); e != nil {
		return nil, e
	}
	success = true
	return result, nil
}
