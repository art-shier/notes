package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"shiji/internal/config"
	"shiji/internal/security"
	"shiji/internal/store"
	"testing"
	"time"
)

func TestAgentBrowserAuthorization(t *testing.T) {
	database := os.Getenv("NOTES_AUTH_TEST_DATABASE_URL")
	if database == "" {
		database = "sqlite:///" + filepath.ToSlash(filepath.Join(t.TempDir(), "db"))
	}
	db, err := store.Open(database)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close(db)
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	a := New(db, config.Settings{Origin: "https://notes.example.test", ExportConcurrency: 1})
	r := a.Router()
	u := store.User{ID: store.ID(), Email: "a@example.test", DisplayName: "A", PasswordHash: "unused", Role: "user", CreatedAt: store.Now()}
	if err = db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	session, err := newSession(db, u)
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path string, body any, auth string, csrf bool) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", a.Settings.Origin)
		if auth == "browser" {
			req.Header.Set("Cookie", Cookie+"="+session)
		} else if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		if csrf {
			req.Header.Set("X-CSRF-Token", security.CSRF(session))
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	object := func(w *httptest.ResponseRecorder, want int) map[string]any {
		t.Helper()
		if w.Code != want {
			t.Fatalf("status %d want %d: %s", w.Code, want, w.Body.String())
		}
		var v map[string]any
		if json.Unmarshal(w.Body.Bytes(), &v) != nil {
			t.Fatal("invalid JSON")
		}
		return v
	}
	raw := "sj_" + security.Secret()
	request := func() map[string]any {
		return object(call("POST", "/api/v1/auth/agent/request", map[string]any{"name": "Test Agent", "token_hash": security.Digest(raw), "token_prefix": raw[:10]}, "", false), 201)
	}
	grant := request()
	code := grant["user_code"].(string)
	device := map[string]any{"device_code": grant["device_code"]}
	if got := object(call("POST", "/api/v1/auth/agent/poll", device, "", false), 200)["status"]; got != "pending" {
		t.Fatal(got)
	}
	approval := map[string]any{"approve": true, "account_id": u.ID, "access": "read", "allow_trash": false, "expires_days": 30}
	if w := call("POST", "/api/v1/auth/agent/requests/"+code, approval, "browser", false); w.Code != 403 {
		t.Fatal("approval bypassed CSRF", w.Code)
	}
	wrongAccount := map[string]any{"approve": true, "account_id": store.ID(), "access": "read", "expires_days": 30}
	if w := call("POST", "/api/v1/auth/agent/requests/"+code, wrongAccount, "browser", true); w.Code != 409 {
		t.Fatal("account mismatch accepted", w.Code)
	}
	object(call("POST", "/api/v1/auth/agent/requests/"+code, approval, "browser", true), 200)
	if w := call("POST", "/api/v1/auth/agent/requests/"+code, approval, "browser", true); w.Code != 409 {
		t.Fatal("repeated approval", w.Code)
	}
	for range 2 {
		poll := object(call("POST", "/api/v1/auth/agent/poll", device, "", false), 200)
		if poll["status"] != "approved" || poll["secret"] != nil {
			t.Fatal(poll)
		}
	}
	me := object(call("GET", "/api/v1/auth/agent/me", nil, raw, false), 200)
	if me["account"].(map[string]any)["id"] != u.ID {
		t.Fatal(me)
	}
	var tok store.Token
	if err = db.First(&tok, "secret_hash = ?", security.Digest(raw)).Error; err != nil {
		t.Fatal(err)
	}
	if len(tok.Scopes) != 4 || tok.Scopes[0] != "notes:read" {
		t.Fatal(tok.Scopes)
	}
	if w := call("POST", "/api/v1/auth/agent/requests/"+code, approval, raw, false); w.Code != 403 {
		t.Fatal("bearer approved", w.Code)
	}
	object(call("POST", "/api/v1/auth/agent/cancel", device, "", false), 200)
	if w := call("GET", "/api/v1/auth/agent/me", nil, raw, false); w.Code != 401 {
		t.Fatal("canceled token active", w.Code)
	}
	denied := request()
	dc := map[string]any{"device_code": denied["device_code"]}
	object(call("POST", "/api/v1/auth/agent/requests/"+denied["user_code"].(string), map[string]any{"approve": false}, "browser", true), 200)
	if object(call("POST", "/api/v1/auth/agent/poll", dc, "", false), 200)["status"] != "denied" {
		t.Fatal("denial lost")
	}
	expired := request()
	if e := db.Exec("UPDATE agent_grants SET expires_at = ? WHERE user_code = ?", store.Time{Time: time.Now().Add(-time.Minute)}, expired["user_code"]).Error; e != nil {
		t.Fatal(e)
	}
	if object(call("POST", "/api/v1/auth/agent/poll", map[string]any{"device_code": expired["device_code"]}, "", false), 200)["status"] != "expired" {
		t.Fatal("expired grant usable")
	}
	if w := call("POST", "/api/v1/auth/agent/requests/"+expired["user_code"].(string), approval, "browser", true); w.Code != 409 {
		t.Fatal("expired approval accepted", w.Code)
	}
	// Even different user-row locks must not allow two accounts to approve the same code.
	u2 := store.User{ID: store.ID(), Email: "b@example.test", DisplayName: "B", PasswordHash: "unused", Role: "member", CreatedAt: store.Now()}
	if e := db.Create(&u2).Error; e != nil {
		t.Fatal(e)
	}
	s2, e := newSession(db, u2)
	if e != nil {
		t.Fatal(e)
	}
	raw = "sj_" + security.Secret()
	raced := request()
	out := make(chan int, 2)
	for _, account := range []struct{ session, id string }{{session, u.ID}, {s2, u2.ID}} {
		go func(s, id string) {
			b, _ := json.Marshal(map[string]any{"approve": true, "account_id": id, "access": "read", "expires_days": 30})
			req := httptest.NewRequest("POST", "/api/v1/auth/agent/requests/"+raced["user_code"].(string), bytes.NewReader(b))
			req.Header.Set("Cookie", Cookie+"="+s)
			req.Header.Set("Origin", a.Settings.Origin)
			req.Header.Set("X-CSRF-Token", security.CSRF(s))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			out <- w.Code
		}(account.session, account.id)
	}
	c1, c2 := <-out, <-out
	if !(c1 == 200 && c2 == 409 || c2 == 200 && c1 == 409) {
		t.Fatal("approval race", c1, c2)
	}
	var n int64
	if e := db.Model(&store.Token{}).Where("secret_hash = ?", security.Digest(raw)).Count(&n).Error; e != nil || n != 1 {
		t.Fatal("duplicate token", n, e)
	}
	var g store.AgentGrant
	if e := db.First(&g, "user_code = ?", raced["user_code"]).Error; e != nil {
		t.Fatal(e)
	}
	if g.DeviceHash == raced["device_code"] || g.TokenHash == raw {
		t.Fatal("plaintext stored")
	}
}
