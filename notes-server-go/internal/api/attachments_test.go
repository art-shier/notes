package api

import (
	"bytes"
	"github.com/gin-gonic/gin"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http/httptest"
	"path/filepath"
	"shiji/internal/config"
	"shiji/internal/security"
	"shiji/internal/store"
	"testing"
	"time"
)

func TestPrivateImageQuotaAndValidation(t *testing.T) {
	dir := t.TempDir()
	db, e := store.Open("sqlite:///" + filepath.ToSlash(filepath.Join(dir, "db")))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close(db)
	if e = store.Migrate(db); e != nil {
		t.Fatal(e)
	}
	u := store.User{ID: store.ID(), Email: "image@example.test", DisplayName: "I", PasswordHash: "unused", Role: "member", QuotaBytes: 100000, CreatedAt: store.Now()}
	db.Create(&u)
	tok := store.Token{ID: store.ID(), UserID: u.ID, Name: "t", Prefix: "sj_x", SecretHash: security.Digest("image-token"), Scopes: store.List{"attachments:read", "attachments:write"}, ExpiresAt: store.Time{Time: time.Now().Add(time.Hour)}, CreatedAt: store.Now()}
	db.Create(&tok)
	a := New(db, config.Settings{AttachmentsDir: filepath.Join(dir, "images"), UploadLimit: 1024, ExportConcurrency: 2})
	r := gin.New()
	a.RegisterAttachments(r)
	upload := func(data []byte) *httptest.ResponseRecorder {
		b := new(bytes.Buffer)
		mw := multipart.NewWriter(b)
		f, _ := mw.CreateFormFile("file", "test.png")
		f.Write(data)
		mw.Close()
		req := httptest.NewRequest("POST", "/api/v1/attachments", b)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Authorization", "Bearer image-token")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	if w := upload([]byte("not an image")); w.Code != 422 {
		t.Fatal(w.Code, w.Body.String())
	}
	im := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	im.Set(0, 0, color.White)
	buf := new(bytes.Buffer)
	png.Encode(buf, im)
	if w := upload(buf.Bytes()); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var files []store.Attachment
	db.Find(&files)
	if len(files) != 1 {
		t.Fatal("attachment missing")
	}
	var got store.User
	db.First(&got, "id = ?", u.ID)
	if got.UsedBytes != int64(buf.Len()) {
		t.Fatal("quota not reserved")
	}
	db.Model(&got).Update("quota_bytes", got.UsedBytes)
	if w := upload(buf.Bytes()); w.Code != 413 {
		t.Fatal("quota exceeded allowed", w.Code)
	}
	req := httptest.NewRequest("GET", "/api/v1/attachments/"+files[0].ID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatal("public image access")
	}
}
