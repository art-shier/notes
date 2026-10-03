package api

import (
	"errors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"shiji/internal/store"
	"strings"
	"unicode/utf8"
)

func ownedFolder(db *gorm.DB, uid, id string) (store.Folder, error) {
	var f store.Folder
	e := db.Where("id = ? AND user_id = ?", id, uid).Take(&f).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = Fail(404, "not_found", "文件夹不存在。")
	}
	return f, e
}
func folderView(f store.Folder) map[string]any {
	return map[string]any{"id": f.ID, "name": f.Name, "parent_id": f.ParentID, "is_inbox": f.IsInbox}
}
func folderScope(db *gorm.DB, uid, id string) ([]string, error) {
	if _, e := ownedFolder(db, uid, id); e != nil {
		return nil, e
	}
	var all []store.Folder
	if e := db.Where("user_id = ?", uid).Find(&all).Error; e != nil {
		return nil, e
	}
	out := []string{id}
	seen := map[string]bool{id: true}
	for i := 0; i < len(out); i++ {
		for _, f := range all {
			if f.ParentID != nil && *f.ParentID == out[i] && !seen[f.ID] {
				out = append(out, f.ID)
				seen[f.ID] = true
			}
		}
	}
	return out, nil
}
func (a *App) listFolders(c *gin.Context, p Principal) error {
	var all []store.Folder
	if e := a.DB.Where("user_id = ?", p.User.ID).Order("is_inbox DESC, name, id").Find(&all).Error; e != nil {
		return e
	}
	items := []map[string]any{}
	for _, f := range all {
		items = append(items, folderView(f))
	}
	c.JSON(200, gin.H{"items": items})
	return nil
}
func (a *App) createFolder(c *gin.Context, p Principal) error {
	var b struct {
		Name     string  `json:"name"`
		ParentID *string `json:"parent_id"`
	}
	if e := Decode(c, &b); e != nil {
		return e
	}
	if utf8.RuneCountInString(b.Name) > 80 || strings.TrimSpace(b.Name) == "" {
		return Fail(422, "validation_error", "文件夹名称不正确。")
	}
	b.Name = strings.TrimSpace(b.Name)
	f := store.Folder{ID: store.ID(), UserID: p.User.ID, Name: b.Name, ParentID: b.ParentID, ParentKey: "root"}
	e := a.Write(p.User.ID, func(tx *gorm.DB) error {
		depth := 1
		parent := b.ParentID
		for parent != nil && *parent != "" {
			ancestor, e := ownedFolder(tx, p.User.ID, *parent)
			if e != nil {
				return e
			}
			depth++
			if depth > 3 {
				return Fail(422, "folder_depth", "文件夹最多三级。")
			}
			parent = ancestor.ParentID
		}
		if b.ParentID != nil && *b.ParentID != "" {
			f.ParentKey = *b.ParentID
		}
		var count int64
		if e := tx.Model(&store.Folder{}).Where("user_id = ? AND parent_key = ? AND name = ?", p.User.ID, f.ParentKey, f.Name).Count(&count).Error; e != nil {
			return e
		}
		if count > 0 {
			return Fail(409, "duplicate_folder", "当前位置已有同名文件夹。")
		}
		return tx.Create(&f).Error
	})
	if e != nil {
		return e
	}
	c.JSON(201, folderView(f))
	return nil
}
