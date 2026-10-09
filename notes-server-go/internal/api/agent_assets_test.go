package api

import (
	"encoding/json"
	"net/http/httptest"
	"shiji/internal/config"
	"strings"
	"testing"
)

func TestPublicSkillResources(t *testing.T) {
	r := New(nil, config.Settings{Origin: "https://notes.example.test"}).Router()
	for _, path := range []string{"/agent/SKILL.md", "/agent/scripts/notes.py", "/agent/scripts/agent_auth.py", "/agent/references/api.md", "/agent/install-client.py", "/agent/shiji-notes.zip", "/api/v1/agent-access"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d", path, w.Code)
		}
		if path == "/agent/SKILL.md" && !strings.Contains(w.Body.String(), "name: shiji-notes") {
			t.Fatal("invalid skill")
		}
		if path == "/api/v1/agent-access" {
			var v map[string]any
			json.Unmarshal(w.Body.Bytes(), &v)
			if v["skill_url"] != "https://notes.example.test/agent/SKILL.md" {
				t.Fatal(v)
			}
		}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/agent/missing", nil))
	if w.Code != 404 {
		t.Fatal("unknown skill path served SPA")
	}
}
