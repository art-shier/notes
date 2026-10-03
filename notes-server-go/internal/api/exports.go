package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"io"
	"os"
	"path/filepath"
	"shiji/internal/archive"
	"shiji/internal/document"
	"shiji/internal/store"
	"strings"
	"time"
)

func exportID(id string) bool { u, e := uuid.Parse(id); return e == nil && u.String() == id }
func (a *App) exportPath(id, suffix string) string {
	return filepath.Join(a.Settings.ExportsDir, id+suffix)
}
func exportView(e store.Export) map[string]any {
	counts := e.Manifest["counts"]
	if counts == nil {
		counts = map[string]any{}
	}
	return map[string]any{"id": e.ID, "ready": e.Ready, "download_url": "/api/v1/exports/" + e.ID + "/download", "size_bytes": e.SizeBytes, "expires_at": e.ExpiresAt, "counts": counts}
}
func exportsScopes(p Principal) error {
	if p.Token == nil {
		return nil
	}
	for _, required := range []string{"notes:read", "folders:read", "tags:read", "attachments:read"} {
		found := false
		for _, s := range p.Token.Scopes {
			if s == required {
				found = true
				break
			}
		}
		if !found {
			return Fail(403, "scope_denied", "访问凭证没有这项权限。")
		}
	}
	return nil
}
func (a *App) RegisterExports(r *gin.Engine) {
	wrap := func(fn Handler) gin.HandlerFunc {
		return a.Handle("notes:read", false, func(c *gin.Context, p Principal) error {
			if e := exportsScopes(p); e != nil {
				return e
			}
			return fn(c, p)
		})
	}
	r.GET("/api/v1/exports", wrap(a.listExports))
	r.POST("/api/v1/exports", wrap(a.createExport))
	r.GET("/api/v1/exports/:bundle_id/download", wrap(a.downloadExport))
}

// CleanupExports removes expired reservations, then files, preserving active readers.
func (a *App) CleanupExports() error {
	if a.Settings.ExportsDir == "" {
		return nil
	}
	if e := os.MkdirAll(a.Settings.ExportsDir, 0700); e != nil {
		return e
	}
	info, e := os.Lstat(a.Settings.ExportsDir)
	if e != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("unsafe export directory")
	}
	now := store.Now()
	var expired []store.Export
	if e = a.DB.Where("expires_at <= ?", now).Find(&expired).Error; e != nil {
		return e
	}
	for _, bundle := range expired {
		result := a.DB.Where("id = ? AND expires_at <= ?", bundle.ID, now).Delete(&store.Export{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 && exportID(bundle.ID) {
			os.Remove(a.exportPath(bundle.ID, ".zip"))
			os.Remove(a.exportPath(bundle.ID, ".part"))
		}
	}
	var active []string
	if e = a.DB.Model(&store.Export{}).Pluck("id", &active).Error; e != nil {
		return e
	}
	set := map[string]bool{}
	for _, id := range active {
		set[id] = true
	}
	files, e := os.ReadDir(a.Settings.ExportsDir)
	if e != nil {
		return e
	}
	for _, f := range files {
		ext := filepath.Ext(f.Name())
		id := strings.TrimSuffix(f.Name(), ext)
		if (ext != ".zip" && ext != ".part") || !exportID(id) || set[id] || f.IsDir() {
			continue
		}
		info, e := f.Info()
		if e == nil && info.ModTime().Before(time.Now().Add(-time.Hour)) {
			os.Remove(filepath.Join(a.Settings.ExportsDir, f.Name()))
		}
	}
	return nil
}

// RunExportCleanup should be launched once by the server and canceled on shutdown.
func (a *App) RunExportCleanup(ctx context.Context) {
	a.CleanupExports()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.CleanupExports()
		}
	}
}
func (a *App) listExports(c *gin.Context, p Principal) error {
	if e := a.CleanupExports(); e != nil {
		return e
	}
	var rows []store.Export
	if e := a.DB.Where("user_id = ? AND expires_at > ?", p.User.ID, store.Now()).Order("created_at DESC").Find(&rows).Error; e != nil {
		return e
	}
	items := []map[string]any{}
	for _, row := range rows {
		items = append(items, exportView(row))
	}
	c.JSON(200, gin.H{"items": items})
	return nil
}
func archiveError(e error) error {
	var archiveErr *archive.Error
	if errors.As(e, &archiveErr) {
		return Fail(archiveErr.Status, archiveErr.Code, archiveErr.Message)
	}
	var docErr *document.Error
	if errors.As(e, &docErr) {
		return Fail(docErr.Status, docErr.Code, docErr.Message)
	}
	return e
}
func (a *App) createExport(c *gin.Context, p Principal) error {
	var body struct {
		IncludeTrash   *bool `json:"include_trash"`
		IncludeHistory *bool `json:"include_history"`
	}
	if e := Decode(c, &body); e != nil {
		return e
	}
	options := archive.Options{IncludeTrash: true, IncludeHistory: true}
	if body.IncludeTrash != nil {
		options.IncludeTrash = *body.IncludeTrash
	}
	if body.IncludeHistory != nil {
		options.IncludeHistory = *body.IncludeHistory
	}
	if e := a.CleanupExports(); e != nil {
		return e
	}
	select {
	case a.ExportSlots <- struct{}{}:
		defer func() { <-a.ExportSlots }()
	default:
		return Fail(429, "export_capacity", "服务器正在准备其他导出，请稍后重试。")
	}
	now := store.Now()
	bundle := store.Export{ID: store.ID(), UserID: p.User.ID, CreatedAt: now, ExpiresAt: store.Time{Time: now.Add(time.Hour)}, Manifest: store.Map{}}
	e := a.Write(p.User.ID, func(tx *gorm.DB) error {
		var count int64
		if e := tx.Model(&store.Export{}).Where("user_id = ? AND expires_at > ?", p.User.ID, store.Now()).Count(&count).Error; e != nil {
			return e
		}
		if count >= 2 {
			return Fail(429, "export_capacity", "已有两个未过期的导出，请先下载现有文件或稍后再试。")
		}
		return tx.Create(&bundle).Error
	})
	if e != nil {
		return e
	}
	partial := a.exportPath(bundle.ID, ".part")
	complete := a.exportPath(bundle.ID, ".zip")
	done := false
	defer func() {
		if !done {
			a.DB.Where("id = ? AND user_id = ?", bundle.ID, p.User.ID).Delete(&store.Export{})
			os.Remove(partial)
			os.Remove(complete)
		}
	}()
	manifest, e := archive.Build(a.DB, a.Settings, p.User.ID, partial, options, func(tx *gorm.DB, n store.Note) (map[string]any, error) { return snapshotNote(tx, n) })
	if e != nil {
		return archiveError(e)
	}
	if e = os.Link(partial, complete); e != nil {
		return e
	}
	if e = os.Remove(partial); e != nil {
		return e
	}
	info, e := os.Stat(complete)
	if e != nil {
		return e
	}
	data, e := json.Marshal(manifest)
	if e != nil {
		return e
	}
	var meta store.Map
	if e = json.Unmarshal(data, &meta); e != nil {
		return e
	}
	bundle.Ready = true
	bundle.Manifest = meta
	bundle.SizeBytes = info.Size()
	bundle.ExpiresAt = store.Time{Time: time.Now().UTC().Add(15 * time.Minute)}
	result := a.DB.Model(&store.Export{}).Where("id = ? AND user_id = ? AND expires_at > ?", bundle.ID, p.User.ID, store.Now()).Updates(map[string]any{"ready": true, "manifest": meta, "size_bytes": bundle.SizeBytes, "expires_at": bundle.ExpiresAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return Fail(410, "export_expired", "导出准备时间过长，请重新生成。")
	}
	done = true
	c.JSON(201, exportView(bundle))
	return nil
}
func (a *App) openExportDownload(userID, id string) (*os.File, store.Export, error) {
	var bundle store.Export
	if !exportID(id) {
		return nil, bundle, Fail(404, "not_found", "导出文件不存在。")
	}
	if e := a.DB.Where("id = ? AND user_id = ?", id, userID).First(&bundle).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return nil, bundle, Fail(404, "not_found", "导出文件不存在。")
		}
		return nil, bundle, e
	}
	if !bundle.ExpiresAt.After(time.Now()) {
		a.CleanupExports()
		return nil, bundle, Fail(410, "export_expired", "导出文件已过期，请重新生成。")
	}
	if !bundle.Ready {
		return nil, bundle, Fail(409, "export_pending", "导出尚未准备完成。")
	}
	file, e := archive.SafeFile(a.Settings.ExportsDir, id+".zip")
	if e != nil {
		return nil, bundle, Fail(404, "file_missing", "导出文件暂时不可用，请重新生成。")
	}
	return file, bundle, nil
}
func (a *App) downloadExport(c *gin.Context, p Principal) error {
	file, bundle, e := a.openExportDownload(p.User.ID, c.Param("bundle_id"))
	if e != nil {
		return e
	}
	defer func() {
		file.Close()
		if !bundle.ExpiresAt.After(time.Now()) {
			os.Remove(a.exportPath(bundle.ID, ".zip"))
		}
	}()
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="shiji-notes-%s.zip"`, bundle.CreatedAt.Format("20060102")))
	c.Header("Content-Length", fmt.Sprint(bundle.SizeBytes))
	c.Status(200)
	_, e = io.Copy(c.Writer, file)
	if e != nil {
		c.Abort()
	}
	return nil
}
