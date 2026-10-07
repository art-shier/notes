package config

import "testing"

func TestProductionRequiresDatabaseURL(t *testing.T) {
	t.Setenv("REQUIRE_DATABASE_URL", "true")
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("production must reject a missing generated database URL")
	}
	t.Setenv("DATABASE_URL", "postgresql://example/notes")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}

func TestLocalDevelopmentRetainsSQLiteDefault(t *testing.T) {
	t.Setenv("REQUIRE_DATABASE_URL", "")
	t.Setenv("DATABASE_URL", "")
	s, err := Load()
	if err != nil || s.DatabaseURL != "sqlite:///data/notes.db" {
		t.Fatal(s.DatabaseURL, err)
	}
}

func TestOriginValidation(t *testing.T) {
	for _, row := range []struct {
		origin, secure string
		valid          bool
	}{{"http://127.0.0.1:5173", "false", true}, {"http://example.com", "false", false}, {"https://example.com", "true", true}, {"https://example.com/path", "true", false}, {"https://user@example.com", "true", false}, {"http://localhost", "true", false}} {
		t.Run(row.origin, func(t *testing.T) {
			t.Setenv("APP_ORIGIN", row.origin)
			t.Setenv("COOKIE_SECURE", row.secure)
			_, e := Load()
			if (e == nil) != row.valid {
				t.Fatal(row, e)
			}
		})
	}
}
