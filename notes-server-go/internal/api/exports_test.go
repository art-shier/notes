package api

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"shiji/internal/config"
	"shiji/internal/security"
	"shiji/internal/store"
	"testing"
	"time"
)

func TestExportsCapacityScopesOwnerAndOpenDownload(t *testing.T) {
	root := t.TempDir()
	db, e := store.Open("sqlite:///" + filepath.ToSlash(filepath.Join(root, "db")))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close(db)
	if e = store.Migrate(db); e != nil {
		t.Fatal(e)
	}
	s := config.Settings{AttachmentsDir: filepath.Join(root, "images"), ExportsDir: filepath.Join(root, "exports"), ExportLimit: 1 << 20, ExportConcurrency: 2}
	os.Mkdir(s.AttachmentsDir, 0700)
	a := New(db, s)
	r := gin.New()
	a.RegisterExports(r)
	u := store.User{ID: store.ID(), Email: "export@example.test", DisplayName: "A", Role: "member", PasswordHash: "hash", CreatedAt: store.Now()}
	db.Create(&u)
	scopes := store.List{"notes:read", "folders:read", "tags:read", "attachments:read"}
	token := func(uid string, ss store.List) string {
		raw := security.Secret()
		row := store.Token{ID: store.ID(), UserID: uid, Name: "test", Prefix: "test", SecretHash: security.Digest(raw), Scopes: ss, CreatedAt: store.Now(), ExpiresAt: store.Time{Time: time.Now().Add(time.Hour)}}
		if e := db.Create(&row).Error; e != nil {
			t.Fatal(e)
		}
		return raw
	}
	secret := token(u.ID, scopes)
	call := func(method, path, raw string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString("{}"))
		req.Header.Set("Authorization", "Bearer "+raw)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	w := call("POST", "/api/v1/exports", secret)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var first map[string]any
	json.Unmarshal(w.Body.Bytes(), &first)
	bid := first["id"].(string)
	path := first["download_url"].(string)
	for i := range scopes {
		limited := append(store.List{}, scopes[:i]...)
		limited = append(limited, scopes[i+1:]...)
		raw := token(u.ID, limited)
		for _, req := range []struct{ m, p string }{{"GET", "/api/v1/exports"}, {"POST", "/api/v1/exports"}, {"GET", path}} {
			if w := call(req.m, req.p, raw); w.Code != 403 {
				t.Fatal("missing scope allowed", i, w.Code)
			}
		}
	}
	other := u
	other.ID = store.ID()
	other.Email = "other@example.test"
	db.Create(&other)
	if w = call("GET", path, token(other.ID, scopes)); w.Code != 404 {
		t.Fatal("foreign export exposed", w.Code)
	}
	if w = call("POST", "/api/v1/exports", secret); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call("POST", "/api/v1/exports", secret); w.Code != 429 {
		t.Fatal("capacity ignored", w.Code)
	}
	f, _, e := a.openExportDownload(u.ID, bid)
	if e != nil {
		t.Fatal(e)
	}
	db.Model(&store.Export{}).Where("id = ?", bid).Update("expires_at", store.Time{Time: time.Now().Add(-time.Minute)})
	if e = a.CleanupExports(); e != nil {
		t.Fatal(e)
	}
	data, e := io.ReadAll(f)
	f.Close()
	if e != nil || len(data) < 4 || string(data[:2]) != "PK" {
		t.Fatal("started download damaged", e)
	}
	if w = call("POST", "/api/v1/exports", secret); w.Code != 201 {
		t.Fatal("expired slot not released", w.Code, w.Body.String())
	}
	var current map[string]any
	json.Unmarshal(w.Body.Bytes(), &current)
	currentID := current["id"].(string)
	if e = db.Model(&store.Export{}).Where("id = ?", currentID).Update("expires_at", store.Time{Time: time.Now().Add(-time.Minute)}).Error; e != nil {
		t.Fatal(e)
	}
	if w = call("GET", current["download_url"].(string), secret); w.Code != 410 {
		t.Fatal("expired download not rejected", w.Code)
	}
	a.ExportSlots <- struct{}{}
	a.ExportSlots <- struct{}{}
	db.Exec("DELETE FROM export_bundles")
	if w = call("POST", "/api/v1/exports", secret); w.Code != 429 {
		t.Fatal("global capacity ignored", w.Code)
	}
	<-a.ExportSlots
	<-a.ExportSlots
	a.Settings.ExportLimit = 1
	if w = call("POST", "/api/v1/exports", secret); w.Code != 413 {
		t.Fatal("export limit not enforced", w.Code, w.Body.String())
	}
	var count int64
	if e = db.Model(&store.Export{}).Count(&count).Error; e != nil || count != 0 {
		t.Fatal("failed export reservation leaked", count, e)
	}
	parts, _ := filepath.Glob(filepath.Join(s.ExportsDir, "*.part"))
	if len(parts) > 0 {
		t.Fatal("partial archive leaked")
	}
}
