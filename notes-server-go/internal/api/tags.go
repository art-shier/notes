package api

import (
	"errors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"shiji/internal/store"
	"sort"
	"strings"
	"unicode/utf8"
)

func ownedTag(db *gorm.DB, uid, id string) (store.Tag, error) {
	var t store.Tag
	e := db.Where("id = ? AND user_id = ?", id, uid).Take(&t).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = Fail(404, "not_found", "标签不存在。")
	}
	return t, e
}
func tagView(t store.Tag) map[string]any { return map[string]any{"id": t.ID, "name": t.Name} }
func validateTagIDs(db *gorm.DB, uid string, ids []string) ([]string, error) {
	seen := map[string]bool{}
	wanted := []string{}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			wanted = append(wanted, id)
		}
	}
	sort.Strings(wanted)
	if len(wanted) == 0 {
		return wanted, nil
	}
	var count int64
	if e := db.Model(&store.Tag{}).Where("user_id = ? AND id IN ?", uid, wanted).Count(&count).Error; e != nil {
		return nil, e
	}
	if count != int64(len(wanted)) {
		return nil, Fail(404, "not_found", "标签不存在。")
	}
	return wanted, nil
}
func noteTags(db *gorm.DB, n store.Note) ([]map[string]any, error) {
	var tags []store.Tag
	e := db.Table("tags").Select("tags.*").Joins("JOIN note_tags ON note_tags.tag_id = tags.id").Where("tags.user_id = ? AND note_tags.user_id = ? AND note_tags.note_id = ?", n.UserID, n.UserID, n.ID).Order("tags.name, tags.id").Find(&tags).Error
	out := []map[string]any{}
	for _, t := range tags {
		out = append(out, tagView(t))
	}
	return out, e
}
func assignTags(db *gorm.DB, uid, nid string, ids []string) error {
	if e := db.Where("user_id = ? AND note_id = ?", uid, nid).Delete(&store.NoteTag{}).Error; e != nil {
		return e
	}
	for _, id := range ids {
		if e := db.Create(&store.NoteTag{UserID: uid, NoteID: nid, TagID: id}).Error; e != nil {
			return e
		}
	}
	return nil
}
func (a *App) listTags(c *gin.Context, p Principal) error {
	var tags []store.Tag
	if e := a.DB.Where("user_id = ?", p.User.ID).Order("name, id").Find(&tags).Error; e != nil {
		return e
	}
	out := []map[string]any{}
	for _, t := range tags {
		out = append(out, tagView(t))
	}
	c.JSON(200, gin.H{"items": out})
	return nil
}
func decodeTagName(c *gin.Context) (string, error) {
	var b struct {
		Name string `json:"name"`
	}
	if e := Decode(c, &b); e != nil {
		return "", e
	}
	if strings.TrimSpace(b.Name) == "" || utf8.RuneCountInString(b.Name) > 40 {
		return "", Fail(422, "validation_error", "标签名称不正确。")
	}
	return strings.TrimSpace(b.Name), nil
}
func (a *App) createTag(c *gin.Context, p Principal) error {
	name, e := decodeTagName(c)
	if e != nil {
		return e
	}
	tag := store.Tag{ID: store.ID(), UserID: p.User.ID, Name: name}
	e = a.Write(p.User.ID, func(tx *gorm.DB) error { return tx.Create(&tag).Error })
	if e != nil {
		return e
	}
	c.JSON(201, tagView(tag))
	return nil
}
func (a *App) renameTag(c *gin.Context, p Principal) error {
	name, e := decodeTagName(c)
	if e != nil {
		return e
	}
	var tag store.Tag
	e = a.Write(p.User.ID, func(tx *gorm.DB) error {
		var e error
		tag, e = ownedTag(tx, p.User.ID, c.Param("tag_id"))
		if e != nil {
			return e
		}
		tag.Name = name
		return tx.Model(&store.Tag{}).Where("id = ? AND user_id = ?", tag.ID, p.User.ID).Update("name", name).Error
	})
	if e != nil {
		return e
	}
	c.JSON(200, tagView(tag))
	return nil
}
func (a *App) deleteTag(c *gin.Context, p Principal) error {
	e := a.Write(p.User.ID, func(tx *gorm.DB) error {
		t, e := ownedTag(tx, p.User.ID, c.Param("tag_id"))
		if e != nil {
			return e
		}
		var notes []store.Note
		sub := tx.Model(&store.NoteTag{}).Select("note_id").Where("user_id = ? AND tag_id = ?", p.User.ID, t.ID)
		if e = tx.Where("user_id = ? AND id IN (?)", p.User.ID, sub).Order("id").Find(&notes).Error; e != nil {
			return e
		}
		for i := range notes {
			n := &notes[i]
			now := store.Now()
			r := tx.Model(&store.Note{}).Where("id = ? AND user_id = ? AND version = ?", n.ID, p.User.ID, n.Version).Updates(map[string]any{"version": n.Version + 1, "updated_at": now})
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 {
				return versionConflict(n.Version)
			}
			n.Version++
			n.UpdatedAt = now
		}
		if e = tx.Where("user_id = ? AND tag_id = ?", p.User.ID, t.ID).Delete(&store.NoteTag{}).Error; e != nil {
			return e
		}
		for i := range notes {
			if e = RecordRevision(tx, &notes[i], "tag_delete", p.Actor(), nil); e != nil {
				return e
			}
		}
		return tx.Where("id = ? AND user_id = ?", t.ID, p.User.ID).Delete(&store.Tag{}).Error
	})
	if e != nil {
		return e
	}
	c.Status(204)
	return nil
}
