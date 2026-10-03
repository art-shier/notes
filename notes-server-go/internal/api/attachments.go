package api

import (
	"bytes"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	_ "golang.org/x/image/webp"
	"gorm.io/gorm"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"shiji/internal/store"
	"strings"
)

func (a *App) RegisterAttachments(r *gin.Engine) {
	r.POST("/api/v1/attachments", a.Handle("attachments:write", false, a.upload))
	r.GET("/api/v1/attachments/:attachment_id", a.Handle("attachments:read", false, a.downloadImage))
}
func SafeAttachmentPath(dir, key string) (string, error) {
	if !strings.HasSuffix(key, ".bin") {
		return "", fmt.Errorf("unsafe attachment key")
	}
	id := strings.TrimSuffix(key, ".bin")
	if u, e := uuid.Parse(id); e != nil || u.String() != id {
		return "", fmt.Errorf("unsafe attachment key")
	}
	root, e := filepath.Abs(dir)
	if e != nil {
		return "", e
	}
	for part := root; ; part = filepath.Dir(part) {
		st, e := os.Lstat(part)
		if e != nil || st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
			return "", fmt.Errorf("unsafe attachment root")
		}
		if filepath.Dir(part) == part {
			break
		}
	}
	p := filepath.Join(root, key)
	if st, e := os.Lstat(p); e == nil && (!st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0) {
		return "", fmt.Errorf("unsafe attachment path")
	}
	return p, nil
}
func (a *App) upload(c *gin.Context, p Principal) error {
	select {
	case a.UploadSlots <- struct{}{}:
	default:
		return Fail(429, "upload_busy", "图片处理繁忙，请稍后重试。")
	}
	defer func() { <-a.UploadSlots }()
	limit := a.Settings.UploadLimit
	if limit <= 0 {
		limit = 10 << 20
	}
	c.Request.Body = io.NopCloser(io.LimitReader(c.Request.Body, limit+(1<<20)+1))
	if e := c.Request.ParseMultipartForm(1 << 20); e != nil {
		return Fail(413, "upload_limit", "图片超过 10 MiB 或表单无效。")
	}
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	f, _, e := c.Request.FormFile("file")
	if e != nil {
		return Fail(422, "validation_error", "请上传图片。")
	}
	defer f.Close()
	data, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil {
		return e
	}
	if int64(len(data)) > limit {
		return Fail(413, "upload_limit", "图片超过 10 MiB。")
	}
	cfg, format, e := image.DecodeConfig(bytes.NewReader(data))
	mimes := map[string]string{"jpeg": "image/jpeg", "png": "image/png", "gif": "image/gif", "webp": "image/webp"}
	if e != nil || mimes[format] == "" || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 40000000 {
		return Fail(422, "invalid_image", "请上传有效的 JPEG、PNG、WebP 或 GIF 图片。")
	}
	if _, _, e = image.Decode(bytes.NewReader(data)); e != nil {
		return Fail(422, "invalid_image", "请上传有效的 JPEG、PNG、WebP 或 GIF 图片。")
	}
	if e = os.MkdirAll(a.Settings.AttachmentsDir, 0700); e != nil {
		return e
	}
	id := store.ID()
	key := id + ".bin"
	path, e := SafeAttachmentPath(a.Settings.AttachmentsDir, key)
	if e != nil {
		return e
	}
	created := false
	e = a.Write(p.User.ID, func(tx *gorm.DB) error {
		res := tx.Model(&store.User{}).Where("id = ? AND used_bytes + ? <= quota_bytes", p.User.ID, len(data)).Update("used_bytes", gorm.Expr("used_bytes + ?", len(data)))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return Fail(413, "quota_exceeded", "空间配额不足。")
		}
		out, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		created = true
		_, e = out.Write(data)
		if e == nil {
			e = out.Sync()
		}
		closeErr := out.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
		row := store.Attachment{ID: id, UserID: p.User.ID, ObjectKey: key, MimeType: mimes[format], SizeBytes: int64(len(data)), CreatedAt: store.Now()}
		return tx.Create(&row).Error
	})
	if e != nil {
		if created {
			os.Remove(path)
		}
		return e
	}
	c.JSON(201, gin.H{"id": id, "url": "/api/v1/attachments/" + id, "mime_type": mimes[format], "size_bytes": len(data)})
	return nil
}
func (a *App) downloadImage(c *gin.Context, p Principal) error {
	var att store.Attachment
	if a.DB.First(&att, "id = ? AND user_id = ?", c.Param("attachment_id"), p.User.ID).Error != nil {
		return Fail(404, "not_found", "图片不存在。")
	}
	path, e := SafeAttachmentPath(a.Settings.AttachmentsDir, att.ObjectKey)
	if e != nil {
		return Fail(404, "file_missing", "图片文件暂时不可用。")
	}
	f, e := os.Open(path)
	if e != nil {
		return Fail(404, "file_missing", "图片文件暂时不可用。")
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() || st.Size() != att.SizeBytes {
		return Fail(404, "file_missing", "图片文件暂时不可用。")
	}
	c.Header("Cache-Control", "private, no-store")
	c.DataFromReader(200, st.Size(), att.MimeType, f, nil)
	return nil
}
