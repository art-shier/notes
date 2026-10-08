package config

import (
	"net/url"
	"strings"
	"testing"
)

func databaseEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{"DATABASE_URL", "DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_SSLMODE"} {
		t.Setenv(key, "")
	}
	t.Setenv("REQUIRE_DATABASE_URL", "true")
}

func TestDatabaseFieldsBuildEscapedPostgreSQLURL(t *testing.T) {
	databaseEnvironment(t)
	t.Setenv("DB_HOST", "2001:db8::1")
	t.Setenv("DB_USER", "notes@app")
	password := "p @:/?#%'\n"
	t.Setenv("DB_PASSWORD", password)
	settings, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(settings.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	actual, _ := parsed.User.Password()
	if parsed.Host != "[2001:db8::1]:5432" || parsed.User.Username() != "notes@app" || actual != password || parsed.Path != "/notes" || parsed.Query().Get("sslmode") != "require" {
		t.Fatal("database fields did not round-trip safely")
	}
	t.Setenv("DB_HOST", "[2001:db8::1]")
	t.Setenv("DB_PORT", "15432")
	t.Setenv("DB_NAME", "notes_custom")
	t.Setenv("DB_SSLMODE", "verify-full")
	settings, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ = url.Parse(settings.DatabaseURL)
	if parsed.Port() != "15432" || parsed.Path != "/notes_custom" || parsed.Query().Get("sslmode") != "verify-full" {
		t.Fatal("database overrides lost")
	}
}

func TestCanonicalDatabaseURLTakesPrecedence(t *testing.T) {
	databaseEnvironment(t)
	canonical := "postgresql://existing:password@db.example.test:5432/existing?sslmode=require"
	t.Setenv("DATABASE_URL", canonical)
	t.Setenv("DB_HOST", "invalid/host")
	settings, err := Load()
	if err != nil || settings.DatabaseURL != canonical {
		t.Fatal("canonical DATABASE_URL must retain precedence", err)
	}
}

func TestProductionDatabaseValidationDoesNotExposeValues(t *testing.T) {
	for _, row := range []struct{ field, value string }{
		{"DATABASE_URL", "sqlite:///sensitive-path"}, {"DATABASE_URL", "postgresql://private-secret@bad%host/notes"},
		{"DB_HOST", "private-host/bad"}, {"DB_PORT", "65536"}, {"DB_SSLMODE", "disable"},
	} {
		t.Run(row.field+row.value, func(t *testing.T) {
			databaseEnvironment(t)
			t.Setenv("DB_HOST", "db.example.test")
			t.Setenv("DB_USER", "notes_app")
			t.Setenv("DB_PASSWORD", "private-password")
			t.Setenv(row.field, row.value)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), row.field) || strings.Contains(err.Error(), row.value) || strings.Contains(err.Error(), "private-password") {
				t.Fatal("expected a field-only database validation error")
			}
		})
	}
	databaseEnvironment(t)
	_, err := Load()
	if err == nil {
		t.Fatal("missing database fields accepted")
	}
	for _, key := range []string{"DB_HOST", "DB_USER", "DB_PASSWORD"} {
		if !strings.Contains(err.Error(), key) {
			t.Fatal("missing field not named", key)
		}
	}
}

func TestProductionRequiresDatabaseURL(t *testing.T) {
	t.Setenv("REQUIRE_DATABASE_URL", "true")
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("production must reject missing database configuration")
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
