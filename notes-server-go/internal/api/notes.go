package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"math"
	"regexp"
	"shiji/internal/document"
	"shiji/internal/store"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func (a *App) RegisterNotes(r *gin.Engine) {
	r.GET("/api/v1/folders", a.Handle("folders:read", false, a.listFolders))
	r.POST("/api/v1/folders", a.Handle("folders:write", false, a.createFolder))
	r.GET("/api/v1/tags", a.Handle("tags:read", false, a.listTags))
	r.POST("/api/v1/tags", a.Handle("tags:write", false, a.createTag))
	r.PATCH("/api/v1/tags/:tag_id", a.Handle("tags:write", false, a.renameTag))
	r.DELETE("/api/v1/tags/:tag_id", a.Handle("tags:write", false, a.deleteTag))
	r.GET("/api/v1/notes", a.Handle("notes:read", false, a.listNotes))
	r.POST("/api/v1/notes", a.Handle("notes:create", false, a.createNote))
	r.POST("/api/v1/notes/validate-content", a.Handle("notes:read", false, a.previewContent))
	r.GET("/api/v1/notes/:note_id", a.Handle("notes:read", false, a.getNote))
	r.PATCH("/api/v1/notes/:note_id", a.Handle("notes:update", false, a.patchNote))
	r.DELETE("/api/v1/notes/:note_id", a.Handle("notes:trash", false, a.trashNote))
	r.POST("/api/v1/notes/:note_id/restore", a.Handle("notes:trash", false, a.restoreNote))
	r.POST("/api/v1/notes/:note_id/blocks", a.Handle("notes:update", false, a.modifyBlocks))
	r.GET("/api/v1/notes/:note_id/history", a.Handle("notes:read", false, a.listHistory))
	r.GET("/api/v1/notes/:note_id/history/:version", a.Handle("notes:read", false, a.getHistory))
	r.POST("/api/v1/notes/:note_id/history/:version/restore", a.Handle("notes:update", false, a.restoreHistory))
}
func ownedNote(db *gorm.DB, uid, id string) (store.Note, error) {
	var n store.Note
	e := db.Where("id = ? AND user_id = ?", id, uid).Take(&n).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = Fail(404, "not_found", "笔记不存在。")
	}
	return n, e
}
func documentError(e error) error {
	var d *document.Error
	if errors.As(e, &d) {
		return Fail(d.Status, d.Code, d.Message)
	}
	return e
}
func attachmentValidator(db *gorm.DB, uid string) document.AttachmentValidator {
	return func(ids []string) error {
		for _, id := range ids {
			var count int64
			if e := db.Model(&store.Attachment{}).Where("id = ? AND user_id = ?", id, uid).Count(&count).Error; e != nil {
				return e
			}
			if count != 1 {
				return Fail(404, "not_found", "图片附件不存在。")
			}
		}
		return nil
	}
}
func excerpt(s string) string {
	r := []rune(s)
	if len(r) > 220 {
		r = r[:220]
	}
	return strings.ReplaceAll(string(r), "\n", " ")
}
func nodes(d document.Doc) []document.Doc {
	out := []document.Doc{}
	switch all := d["content"].(type) {
	case []any:
		for _, x := range all {
			if n, ok := x.(map[string]any); ok {
				out = append(out, n)
			}
		}
	case []document.Doc:
		out = all
	}
	return out
}
func noteView(db *gorm.DB, n store.Note, detail bool, format string) (map[string]any, error) {
	tags, e := noteTags(db, n)
	if e != nil {
		return nil, e
	}
	var thumb any
	url := document.Thumbnail(document.Doc(n.ContentJSON))
	if url != "" {
		thumb = url
	}
	out := map[string]any{"id": n.ID, "folder_id": n.FolderID, "title": n.Title, "excerpt": excerpt(n.PlainText), "thumbnail": thumb, "favorite": n.Favorite, "trashed": n.Trashed, "source": n.Source, "version": n.Version, "tags": tags, "created_at": n.CreatedAt, "updated_at": n.UpdatedAt}
	if detail {
		doc := document.Doc(n.ContentJSON)
		out["content_json"] = document.Hydrate(doc)
		blocks := []map[string]any{}
		for i, node := range nodes(doc) {
			if i < len(n.BlockIDs) {
				blocks = append(blocks, map[string]any{"id": n.BlockIDs[i], "content": document.Hydrate(node)})
			}
		}
		out["blocks"] = blocks
		md, safe := document.ExportMarkdown(doc)
		out["markdown_roundtrip_safe"] = safe
		if format == "markdown" {
			out["markdown"] = md
		}
	}
	return out, nil
}

// NoteView returns the complete public representation. Database failures propagate
// as panics to the request recovery boundary, preventing partial export results.
func NoteView(db *gorm.DB, n store.Note) map[string]any {
	out, e := noteView(db, n, true, "json")
	if e != nil {
		panic(e)
	}
	return out
}
func decodeObject(c *gin.Context, allowed string) (map[string]any, error) {
	var b map[string]any
	if e := Decode(c, &b); e != nil {
		return nil, e
	}
	if b == nil {
		return nil, Fail(422, "validation_error", "请求参数不正确。")
	}
	set := map[string]bool{}
	for _, k := range strings.Fields(allowed) {
		set[k] = true
	}
	for k := range b {
		if !set[k] {
			return nil, Fail(422, "validation_error", "请求参数不正确。")
		}
	}
	return b, nil
}
func requiredString(b map[string]any, key string, max int) (string, error) {
	v, ok := b[key].(string)
	if !ok || max > 0 && utf8.RuneCountInString(v) > max {
		return "", Fail(422, "validation_error", "请求参数不正确。")
	}
	return v, nil
}
func expectedVersion(b map[string]any) (int, error) {
	v, ok := b["expected_version"].(float64)
	if !ok || v < 1 || v != math.Trunc(v) || v > float64(1<<31-1) {
		return 0, Fail(422, "validation_error", "版本号不正确。")
	}
	return int(v), nil
}
func queryInt(c *gin.Context, key string, defaultValue, min, max int) (int, error) {
	s, has := c.GetQuery(key)
	if !has {
		return defaultValue, nil
	}
	v, e := strconv.Atoi(s)
	if e != nil || v < min || max > 0 && v > max {
		return 0, Fail(422, "validation_error", "请求参数不正确。")
	}
	return v, nil
}
func queryBool(c *gin.Context, key string, def bool) (bool, error) {
	s, has := c.GetQuery(key)
	if !has {
		return def, nil
	}
	switch strings.ToLower(s) {
	case "true", "1", "yes", "on", "t", "y":
		return true, nil
	case "false", "0", "no", "off", "f", "n":
		return false, nil
	}
	return false, Fail(422, "validation_error", "请求参数不正确。")
}
func tagIDInput(b map[string]any) ([]string, error) {
	raw, ok := b["tag_ids"]
	if !ok {
		return []string{}, nil
	}
	all, ok := raw.([]any)
	if !ok || len(all) > 50 {
		return nil, Fail(422, "validation_error", "标签参数不正确。")
	}
	out := []string{}
	for _, x := range all {
		s, ok := x.(string)
		if !ok {
			return nil, Fail(422, "validation_error", "标签参数不正确。")
		}
		out = append(out, s)
	}
	return out, nil
}
func decodeContent(b map[string]any, empty bool) (document.Doc, []document.Warning, error) {
	warnings := []document.Warning{}
	format := "json"
	if f, has := b["content_format"]; has {
		v, ok := f.(string)
		if !ok || (v != "json" && v != "markdown") {
			return nil, nil, Fail(422, "validation_error", "正文格式不正确。")
		}
		format = v
	}
	content := b["content"]
	raw := b["content_json"]
	if raw != nil && content != nil {
		return nil, nil, Fail(422, "ambiguous_content", "只能提交一份正文。")
	}
	if format == "markdown" {
		text, ok := content.(string)
		if !ok || raw != nil {
			return nil, nil, Fail(422, "invalid_format", "Markdown 格式需要字符串 content。")
		}
		doc, w, e := document.FromMarkdown(text)
		if w == nil {
			w = warnings
		}
		return doc, w, documentError(e)
	}
	if raw == nil {
		raw = content
	}
	if raw == nil && empty {
		return document.Doc{"type": "doc", "content": []any{document.Doc{"type": "paragraph"}}}, warnings, nil
	}
	doc, ok := raw.(map[string]any)
	if !ok {
		return nil, nil, Fail(422, "invalid_format", "JSON 格式需要对象正文。")
	}
	return doc, warnings, nil
}

type noteCursor struct {
	Time string `json:"time"`
	ID   string `json:"id"`
}

func decodeCursor(s string) (time.Time, string, error) {
	invalid := Fail(422, "invalid_cursor", "分页参数不正确。")
	b, e := base64.URLEncoding.DecodeString(s)
	if e != nil {
		b, e = base64.RawURLEncoding.DecodeString(s)
	}
	if e != nil {
		return time.Time{}, "", invalid
	}
	var value noteCursor
	if e = json.Unmarshal(b, &value); e != nil {
		return time.Time{}, "", invalid
	}
	parsed, e := time.Parse("2006-01-02T15:04:05.999999999", value.Time)
	if e != nil || value.ID == "" {
		return time.Time{}, "", invalid
	}
	return parsed, value.ID, nil
}
func (a *App) listNotes(c *gin.Context, p Principal) error {
	query := c.Query("query")
	if utf8.RuneCountInString(query) > 200 {
		return Fail(422, "validation_error", "搜索词过长。")
	}
	limit, e := queryInt(c, "limit", 20, 1, 100)
	if e != nil {
		return e
	}
	trash, e := queryBool(c, "trash", false)
	if e != nil {
		return e
	}
	q := a.DB.Where("user_id = ? AND trashed = ?", p.User.ID, trash)
	if id := c.Query("folder_id"); id != "" {
		ids, e := folderScope(a.DB, p.User.ID, id)
		if e != nil {
			return e
		}
		q = q.Where("folder_id IN ?", ids)
	}
	if id := c.Query("tag_id"); id != "" {
		if _, e := ownedTag(a.DB, p.User.ID, id); e != nil {
			return e
		}
		q = q.Where("id IN (?)", a.DB.Model(&store.NoteTag{}).Select("note_id").Where("user_id = ? AND tag_id = ?", p.User.ID, id))
	}
	if _, has := c.GetQuery("favorite"); has {
		v, e := queryBool(c, "favorite", false)
		if e != nil {
			return e
		}
		q = q.Where("favorite = ?", v)
	}
	if query != "" {
		pattern := "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(query) + "%"
		q = q.Where("(LOWER(title) LIKE LOWER(?) ESCAPE '\\' OR LOWER(plain_text) LIKE LOWER(?) ESCAPE '\\')", pattern, pattern)
	}
	if cursor, has := c.GetQuery("cursor"); has {
		t, id, e := decodeCursor(cursor)
		if e != nil {
			return e
		}
		stamp := store.Time{Time: t}
		q = q.Where("(updated_at < ? OR (updated_at = ? AND id < ?))", stamp, stamp, id)
	}
	var records []store.Note
	if e = q.Order("updated_at DESC, id DESC").Limit(limit + 1).Find(&records).Error; e != nil {
		return e
	}
	var next any
	if len(records) > limit {
		last := records[limit-1]
		b, _ := json.Marshal(noteCursor{last.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999"), last.ID})
		next = base64.URLEncoding.EncodeToString(b)
		records = records[:limit]
	}
	items := []map[string]any{}
	for _, n := range records {
		v, e := noteView(a.DB, n, false, "json")
		if e != nil {
			return e
		}
		items = append(items, v)
	}
	c.JSON(200, gin.H{"items": items, "next_cursor": next})
	return nil
}

var idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{8,128}$`)

// Python's existing records hash sorted compact JSON with ensure_ascii=False.
// Encoder disables HTML escapes; JSON's optional line-separator escapes are
// removed only when they are actual escapes, preserving literal backslashes.
func idempotencyJSON(value any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if e := enc.Encode(value); e != nil {
		return nil, e
	}
	raw := bytes.TrimSuffix(b.Bytes(), []byte("\n"))
	out := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); {
		if raw[i] == '\\' && i+1 < len(raw) {
			if i+6 <= len(raw) && (string(raw[i:i+6]) == `\u2028` || string(raw[i:i+6]) == `\u2029`) {
				if raw[i+5] == '8' {
					out = append(out, []byte("\u2028")...)
				} else {
					out = append(out, []byte("\u2029")...)
				}
				i += 6
				continue
			}
			out = append(out, raw[i], raw[i+1])
			i += 2
			continue
		}
		out = append(out, raw[i])
		i++
	}
	return out, nil
}

func claimCreate(tx *gorm.DB, p Principal, key string, b map[string]any) (*store.Idempotency, map[string]any, error) {
	if !idempotencyKeyPattern.MatchString(key) {
		return nil, nil, Fail(422, "invalid_idempotency_key", "幂等键需要 8–128 个字母、数字或 _.:-。")
	}
	payload := map[string]any{"folder_id": b["folder_id"], "title": "", "tag_ids": []any{}, "content_format": "json", "content": nil, "content_json": nil}
	for k, v := range b {
		payload[k] = v
	}
	raw, e := idempotencyJSON(payload)
	if e != nil {
		return nil, nil, e
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	where := map[string]any{"user_id": p.User.ID, "actor_key": p.ActorKey(), "route": "POST /api/v1/notes", "key": key}
	if e = tx.Where(where).Where("expires_at <= ?", store.Now()).Delete(&store.Idempotency{}).Error; e != nil {
		return nil, nil, e
	}
	var existing store.Idempotency
	e = tx.Where(where).Take(&existing).Error
	if e == nil {
		if existing.RequestHash != hash {
			return nil, nil, Fail(409, "idempotency_conflict", "这个幂等键已用于不同的请求。")
		}
		if existing.Response == nil {
			return nil, nil, Fail(409, "idempotency_pending", "请求尚未完成，请稍后查询。")
		}
		return nil, map[string]any(existing.Response), nil
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil, e
	}
	record := store.Idempotency{ID: store.ID(), UserID: p.User.ID, ActorKey: p.ActorKey(), Route: "POST /api/v1/notes", Key: key, RequestHash: hash, ExpiresAt: store.Time{Time: time.Now().UTC().Add(24 * time.Hour)}}
	if e = tx.Create(&record).Error; e != nil {
		return nil, nil, e
	}
	return &record, nil, nil
}
func (a *App) createNote(c *gin.Context, p Principal) error {
	b, e := decodeObject(c, "folder_id title tag_ids content_format content content_json")
	if e != nil {
		return e
	}
	fid, e := requiredString(b, "folder_id", 0)
	if e != nil {
		return e
	}
	title := ""
	if _, has := b["title"]; has {
		title, e = requiredString(b, "title", 300)
		if e != nil {
			return e
		}
	}
	ids, e := tagIDInput(b)
	if e != nil {
		return e
	}
	var result map[string]any
	e = a.Write(p.User.ID, func(tx *gorm.DB) error {
		var record *store.Idempotency
		if _, has := c.Request.Header["Idempotency-Key"]; has {
			var replay map[string]any
			var e error
			record, replay, e = claimCreate(tx, p, c.GetHeader("Idempotency-Key"), b)
			if e != nil {
				return e
			}
			if replay != nil {
				result = replay
				return nil
			}
		}
		if _, e := ownedFolder(tx, p.User.ID, fid); e != nil {
			return e
		}
		raw, w, e := decodeContent(b, true)
		if e != nil {
			return e
		}
		content, e := document.Validate(raw, attachmentValidator(tx, p.User.ID))
		if e != nil {
			return documentError(e)
		}
		tags, e := validateTagIDs(tx, p.User.ID, ids)
		if e != nil {
			return e
		}
		now := store.Now()
		n := store.Note{ID: store.ID(), UserID: p.User.ID, FolderID: fid, Title: title, ContentJSON: store.Map(content), PlainText: strings.TrimSpace(document.PlainText(content)), BlockIDs: store.List(document.AlignIDs(nil, nil, nodes(content))), Source: p.Actor(), Version: 1, CreatedAt: now, UpdatedAt: now}
		if e = tx.Create(&n).Error; e != nil {
			return e
		}
		if e = assignTags(tx, p.User.ID, n.ID, tags); e != nil {
			return e
		}
		result, e = noteView(tx, n, true, "json")
		if e != nil {
			return e
		}
		result["warnings"] = w
		if record != nil {
			if e = tx.Model(record).Update("response", store.Map(result)).Error; e != nil {
				return e
			}
		}
		return RecordRevision(tx, &n, "create", p.Actor(), nil)
	})
	if e != nil {
		return e
	}
	c.JSON(201, result)
	return nil
}
func (a *App) previewContent(c *gin.Context, p Principal) error {
	b, e := decodeObject(c, "content_format content content_json")
	if e != nil {
		return e
	}
	raw, w, e := decodeContent(b, false)
	if e != nil {
		return e
	}
	content, e := document.Validate(raw, attachmentValidator(a.DB, p.User.ID))
	if e != nil {
		return documentError(e)
	}
	md, safe := document.ExportMarkdown(content)
	c.JSON(200, gin.H{"content_json": document.Hydrate(content), "markdown": md, "markdown_roundtrip_safe": safe, "warnings": w})
	return nil
}
func (a *App) getNote(c *gin.Context, p Principal) error {
	format := c.DefaultQuery("content_format", "json")
	if format != "json" && format != "markdown" {
		return Fail(422, "validation_error", "正文格式不正确。")
	}
	n, e := ownedNote(a.DB, p.User.ID, c.Param("note_id"))
	if e != nil {
		return e
	}
	v, e := noteView(a.DB, n, true, format)
	if e != nil {
		return e
	}
	c.JSON(200, v)
	return nil
}
func versionConflict(version int) *Error {
	e := Fail(409, "version_conflict", "笔记已被其他客户端修改。")
	e.Details = map[string]any{"current_version": version}
	return e
}
func atomicNoteUpdate(tx *gorm.DB, p Principal, id string, version int, values map[string]any, tags []string, action string, restored *int) (map[string]any, error) {
	n, e := ownedNote(tx, p.User.ID, id)
	if e != nil {
		return nil, e
	}
	values["version"] = gorm.Expr("version + 1")
	values["updated_at"] = store.Now()
	row := tx.Model(&store.Note{}).Where("id = ? AND user_id = ? AND version = ?", id, p.User.ID, version).Updates(values)
	if row.Error != nil {
		return nil, row.Error
	}
	if row.RowsAffected != 1 {
		return nil, versionConflict(n.Version)
	}
	if tags != nil {
		if e = assignTags(tx, p.User.ID, id, tags); e != nil {
			return nil, e
		}
	}
	n, e = ownedNote(tx, p.User.ID, id)
	if e != nil {
		return nil, e
	}
	if e = RecordRevision(tx, &n, action, p.Actor(), restored); e != nil {
		return nil, e
	}
	return noteView(tx, n, true, "json")
}
func (a *App) patchNote(c *gin.Context, p Principal) error {
	b, e := decodeObject(c, "expected_version title folder_id favorite tag_ids content_format content content_json")
	if e != nil {
		return e
	}
	version, e := expectedVersion(b)
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
			return Fail(409, "note_trashed", "请先恢复笔记。")
		}
		values := map[string]any{}
		for _, k := range []string{"title", "folder_id", "favorite", "tag_ids"} {
			if v, has := b[k]; has && v == nil {
				return Fail(422, "null_field", "笔记字段不能为 null。")
			}
		}
		for _, k := range []string{"title", "folder_id"} {
			if _, has := b[k]; has {
				max := 0
				if k == "title" {
					max = 300
				}
				v, e := requiredString(b, k, max)
				if e != nil {
					return e
				}
				values[k] = v
				if k == "folder_id" {
					if _, e = ownedFolder(tx, p.User.ID, v); e != nil {
						return e
					}
				}
			}
		}
		if raw, has := b["favorite"]; has {
			v, ok := raw.(bool)
			if !ok {
				return Fail(422, "validation_error", "收藏参数不正确。")
			}
			values["favorite"] = v
		}
		var tags []string
		if _, has := b["tag_ids"]; has {
			raw, e := tagIDInput(b)
			if e != nil {
				return e
			}
			tags, e = validateTagIDs(tx, p.User.ID, raw)
			if e != nil {
				return e
			}
		}
		warnings := []document.Warning{}
		_, h1 := b["content"]
		_, h2 := b["content_json"]
		_, h3 := b["content_format"]
		if h1 || h2 || h3 {
			if b["content_format"] == "markdown" {
				if _, safe := document.ExportMarkdown(document.Doc(n.ContentJSON)); !safe {
					return Fail(422, "unsafe_markdown_overwrite", "这篇笔记不能无损转换为 Markdown，请使用 JSON 或按块修改。")
				}
			}
			raw, w, e := decodeContent(b, false)
			if e != nil {
				return e
			}
			warnings = w
			content, e := document.Validate(raw, attachmentValidator(tx, p.User.ID))
			if e != nil {
				return documentError(e)
			}
			values["content_json"] = store.Map(content)
			values["plain_text"] = strings.TrimSpace(document.PlainText(content))
			values["block_ids"] = store.List(document.AlignIDs(nodes(document.Doc(n.ContentJSON)), n.BlockIDs, nodes(content)))
		}
		if len(values) == 0 && tags == nil {
			return Fail(422, "empty_patch", "没有需要修改的字段。")
		}
		result, e = atomicNoteUpdate(tx, p, n.ID, version, values, tags, "update", nil)
		if e != nil {
			return e
		}
		result["warnings"] = warnings
		return nil
	})
	if e != nil {
		return e
	}
	c.JSON(200, result)
	return nil
}
func (a *App) trashNote(c *gin.Context, p Principal) error {
	v, e := queryInt(c, "expected_version", 0, 1, 1<<31-1)
	if e != nil {
		return e
	}
	if v == 0 {
		return Fail(422, "validation_error", "版本号不正确。")
	}
	return a.changeTrash(c, p, v, true)
}
func (a *App) restoreNote(c *gin.Context, p Principal) error {
	b, e := decodeObject(c, "expected_version")
	if e != nil {
		return e
	}
	v, e := expectedVersion(b)
	if e != nil {
		return e
	}
	return a.changeTrash(c, p, v, false)
}
func (a *App) changeTrash(c *gin.Context, p Principal, v int, trash bool) error {
	action := "restore"
	if trash {
		action = "trash"
	}
	var result map[string]any
	e := a.Write(p.User.ID, func(tx *gorm.DB) error {
		var e error
		result, e = atomicNoteUpdate(tx, p, c.Param("note_id"), v, map[string]any{"trashed": trash}, nil, action, nil)
		return e
	})
	if e != nil {
		return e
	}
	c.JSON(200, result)
	return nil
}
