package api

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"shiji/internal/document"
	"shiji/internal/store"
	"strings"
)

func (a *App) modifyBlocks(c *gin.Context, p Principal) error {
	var b struct {
		ExpectedVersion int                  `json:"expected_version"`
		Operations      []document.Operation `json:"operations"`
	}
	if e := Decode(c, &b); e != nil {
		return e
	}
	if b.ExpectedVersion < 1 || len(b.Operations) < 1 || len(b.Operations) > 100 {
		return Fail(422, "validation_error", "块操作参数不正确。")
	}
	var result map[string]any
	e := a.Write(p.User.ID, func(tx *gorm.DB) error {
		n, e := ownedNote(tx, p.User.ID, c.Param("note_id"))
		if e != nil {
			return e
		}
		if n.Trashed {
			return Fail(409, "note_trashed", "请先恢复笔记。")
		}
		if n.Version != b.ExpectedVersion {
			return versionConflict(n.Version)
		}
		content, ids, e := document.ApplyOperations(document.Doc(n.ContentJSON), n.BlockIDs, b.Operations, attachmentValidator(tx, p.User.ID))
		if e != nil {
			return documentError(e)
		}
		values := map[string]any{"content_json": store.Map(content), "block_ids": store.List(ids), "plain_text": strings.TrimSpace(document.PlainText(content))}
		result, e = atomicNoteUpdate(tx, p, n.ID, b.ExpectedVersion, values, nil, "blocks", nil)
		return e
	})
	if e != nil {
		return e
	}
	c.JSON(200, result)
	return nil
}
