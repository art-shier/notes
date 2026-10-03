package config

import "testing"

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
