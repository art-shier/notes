package api

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"path/filepath"
	"shiji/internal/config"
	"shiji/internal/security"
	"shiji/internal/store"
	"testing"
	"time"
)

func TestInvitationCookieCSRFAndTokens(t *testing.T) {
	db, e := store.Open("sqlite:///" + filepath.ToSlash(filepath.Join(t.TempDir(), "db")))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close(db)
	if e = store.Migrate(db); e != nil {
		t.Fatal(e)
	}
	s := config.Settings{Origin: "http://127.0.0.1:5173", QuotaBytes: 10000, ExportConcurrency: 2}
	a := New(db, s)
	r := gin.New()
	a.RegisterAuth(r)
	raw := security.Secret()
	inv := store.Invitation{ID: store.ID(), Email: "a@example.test", Role: "admin", SecretHash: security.Digest(raw), ExpiresAt: store.Time{Time: time.Now().Add(time.Hour)}}
	if e = db.Create(&inv).Error; e != nil {
		t.Fatal(e)
	}
	call := func(method, path string, body any, cookie, csrf string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", s.Origin)
		if cookie != "" {
			req.Header.Set("Cookie", cookie)
		}
		req.Header.Set("X-CSRF-Token", csrf)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	body := map[string]any{"email": inv.Email, "password": "test-password-123", "display_name": "A", "invitation": raw}
	w := call("POST", "/api/v1/auth/register", body, "", "")
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var account map[string]any
	json.Unmarshal(w.Body.Bytes(), &account)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].Path != "/api" {
		t.Fatal("cookie policy")
	}
	cookie := cookies[0].Name + "=" + cookies[0].Value
	csrf := account["csrf_token"].(string)
	if w = call("POST", "/api/v1/auth/register", body, "", ""); w.Code != 422 {
		t.Fatal("invitation reuse", w.Code)
	}
	if w = call("POST", "/api/v1/auth/logout", nil, cookie, ""); w.Code != 403 {
		t.Fatal("missing csrf accepted", w.Code)
	}
	w = call("POST", "/api/v1/tokens", map[string]any{"name": "Agent", "scopes": []string{"notes:read"}}, cookie, csrf)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var token map[string]any
	json.Unmarshal(w.Body.Bytes(), &token)
	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token["secret"].(string))
	out := httptest.NewRecorder()
	r.ServeHTTP(out, req)
	if out.Code != 403 {
		t.Fatal("agent accessed account", out.Code)
	}
	if w = call("POST", "/api/v1/auth/logout", nil, cookie, csrf); w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call("GET", "/api/v1/me", nil, cookie, ""); w.Code != 401 {
		t.Fatal("old session valid")
	}
	w = call("POST", "/api/v1/auth/login", map[string]any{"email": inv.Email, "password": "test-password-123"}, "", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}
