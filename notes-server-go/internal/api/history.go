package api

import (
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"os"
	"shiji/internal/document"
	"shiji/internal/store"
	"strconv"
	"strings"
)

func snapshotNote(db *gorm.DB, n store.Note) (map[string]any, error) {
	tags, e := noteTags(db, n)
	if e != nil {
		return nil, e
	}
	out := map[string]any{"id": n.ID, "version": n.Version, "title": n.Title, "folder_id": n.FolderID, "content_json": n.ContentJSON, "block_ids": n.BlockIDs, "tags": tags, "favorite": n.Favorite, "trashed": n.Trashed, "source": n.Source, "excerpt": excerpt(n.PlainText), "created_at": n.CreatedAt, "updated_at": n.UpdatedAt}
	raw, e := json.Marshal(out)
	if e != nil {
		return nil, e
	}
	var cloned map[string]any
	e = json.Unmarshal(raw, &cloned)
	return cloned, e
}

// SnapshotNote returns an immutable, canonical snapshot without attachment URLs.
func SnapshotNote(db *gorm.DB, n store.Note) map[string]any {
	v, e := snapshotNote(db, n)
	if e != nil {
		panic(e)
	}
	return v
}
func RecordRevision(tx *gorm.DB, n *store.Note, action, actor string, restored *int) error {
	snapshot, e := snapshotNote(tx, *n)
	if e != nil {
		return e
	}
	r := store.Revision{ID: store.ID(), UserID: n.UserID, NoteID: n.ID, Version: n.Version, Snapshot: store.Map(snapshot), Action: action, Actor: actor, RestoredFrom: restored, SavedAt: n.UpdatedAt}
	if e = tx.Create(&r).Error; e != nil {
		return e
	}
	limit := 200
	if raw, ok := tx.Get("history_limit"); ok {
		if v, ok := raw.(int); ok && v > 0 {
			limit = v
		}
	}
	var versions []int
	if e = tx.Model(&store.Revision{}).Where("user_id = ? AND note_id = ?", n.UserID, n.ID).Order("version DESC").Offset(limit-1).Limit(1).Pluck("version", &versions).Error; e != nil {
		return e
	}
	if len(versions) > 0 {
		return tx.Where("user_id = ? AND note_id = ? AND version < ?", n.UserID, n.ID, versions[0]).Delete(&store.Revision{}).Error
	}
	return nil
}
func ownedRevision(db *gorm.DB, uid, nid string, version int) (store.Revision, error) {
	var r store.Revision
	e := db.Where("user_id = ? AND note_id = ? AND version = ?", uid, nid, version).Take(&r).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = Fail(404, "not_found", "历史版本不存在或已超出保留范围。")
	}
	return r, e
}
func revisionView(r store.Revision, detail bool) map[string]any {
	out := map[string]any{"version": r.Version, "title": r.Snapshot["title"], "excerpt": r.Snapshot["excerpt"], "action": r.Action, "actor": r.Actor, "restored_from": r.RestoredFrom, "saved_at": r.SavedAt}
	if detail {
		raw, e := json.Marshal(r.Snapshot)
		if e != nil {
			panic(e)
		}
		var snapshot map[string]any
		if e = json.Unmarshal(raw, &snapshot); e != nil {
			panic(e)
		}
		if doc, ok := snapshot["content_json"].(map[string]any); ok {
			snapshot["content_json"] = document.Hydrate(doc)
		}
		out["snapshot"] = snapshot
	}
	return out
}
func (a *App) listHistory(c *gin.Context, p Principal) error {
	limit, e := queryInt(c, "limit", 20, 1, 100)
	if e != nil {
		return e
	}
	before, e := queryInt(c, "before_version", 0, 1, 1<<31-1)
	if e != nil {
		return e
	}
	n, e := ownedNote(a.DB, p.User.ID, c.Param("note_id"))
	if e != nil {
		return e
	}
	q := a.DB.Where("user_id = ? AND note_id = ?", p.User.ID, n.ID)
	if before > 0 {
		q = q.Where("version < ?", before)
	}
	var records []store.Revision
	if e = q.Order("version DESC").Limit(limit + 1).Find(&records).Error; e != nil {
		return e
	}
	var next any
	if len(records) > limit {
		next = records[limit-1].Version
		records = records[:limit]
	}
	items := []map[string]any{}
	for _, r := range records {
		items = append(items, revisionView(r, false))
	}
	retention := a.Settings.HistoryLimit
	if retention <= 0 {
		retention = 200
	}
	c.JSON(200, gin.H{"items": items, "next_before_version": next, "current_version": n.Version, "retention_limit": retention})
	return nil
}
func historyVersion(c *gin.Context) (int, error) {
	v, e := strconv.Atoi(c.Param("version"))
	if e != nil {
		return 0, Fail(422, "validation_error", "版本号不正确。")
	}
	return v, nil
}
func (a *App) getHistory(c *gin.Context, p Principal) error {
	v, e := historyVersion(c)
	if e != nil {
		return e
	}
	if _, e = ownedNote(a.DB, p.User.ID, c.Param("note_id")); e != nil {
		return e
	}
	r, e := ownedRevision(a.DB, p.User.ID, c.Param("note_id"), v)
	if e != nil {
		return e
	}
	c.JSON(200, revisionView(r, true))
	return nil
}
func stringList(v any) ([]string, error) {
	switch xs := v.(type) {
	case []string:
		return append([]string{}, xs...), nil
	case store.List:
		return append([]string{}, xs...), nil
	case []any:
		out := []string{}
		for _, x := range xs {
			s, ok := x.(string)
			if !ok {
				return nil, Fail(500, "internal_error", "历史版本数据不正确。")
			}
			out = append(out, s)
		}
		return out, nil
	}
	return nil, Fail(500, "internal_error", "历史版本数据不正确。")
}
func (a *App) verifyImageFiles(tx *gorm.DB, uid string, content document.Doc) error {
	for _, id := range document.ImageIDs(content) {
		var attachment store.Attachment
		e := tx.Where("user_id = ? AND id = ?", uid, id).Take(&attachment).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return Fail(404, "not_found", "图片附件不存在。")
		}
		if e != nil {
			return e
		}
		path, e := SafeAttachmentPath(a.Settings.AttachmentsDir, attachment.ObjectKey)
		if e != nil {
			return Fail(404, "file_missing", "历史版本中的图片文件暂时不可用，正文未恢复。")
		}
		if st, e := os.Stat(path); e != nil || !st.Mode().IsRegular() || st.Size() != attachment.SizeBytes {
			return Fail(404, "file_missing", "历史版本中的图片文件暂时不可用，正文未恢复。")
		}
	}
	return nil
}
func (a *App) restoreHistory(c *gin.Context, p Principal) error {
	v, e := historyVersion(c)
	if e != nil {
		return e
	}
	b, e := decodeObject(c, "expected_version")
	if e != nil {
		return e
	}
	expected, e := expectedVersion(b)
	if e != nil {
		return e
	}
	var result map[string]any
	e = a.Write(p.User.ID, func(tx *gorm.DB) error {
		n, e := ownedNote(tx, p.User.ID, c.Param("note_id"))
		if e != nil {
			return e
		}
		if n.Trashed {
			return Fail(409, "note_trashed", "请先从回收站恢复笔记。")
		}
		if n.Version != expected {
			return versionConflict(n.Version)
		}
		r, e := ownedRevision(tx, p.User.ID, n.ID, v)
		if e != nil {
			return e
		}
		raw, ok := r.Snapshot["content_json"].(map[string]any)
		if !ok {
			return Fail(500, "internal_error", "历史版本数据不正确。")
		}
		content, e := document.Validate(raw, attachmentValidator(tx, p.User.ID))
		if e != nil {
			return documentError(e)
		}
		if e = a.verifyImageFiles(tx, p.User.ID, content); e != nil {
			return e
		}
		ids, e := stringList(r.Snapshot["block_ids"])
		if e != nil {
			return e
		}
		values := map[string]any{"title": r.Snapshot["title"], "content_json": store.Map(content), "block_ids": store.List(ids), "plain_text": strings.TrimSpace(document.PlainText(content))}
		result, e = atomicNoteUpdate(tx, p, n.ID, expected, values, nil, "history_restore", &v)
		return e
	})
	if e != nil {
		return e
	}
	c.JSON(200, result)
	return nil
}
