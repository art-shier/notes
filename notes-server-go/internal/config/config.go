package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Settings struct {
	DatabaseURL, AttachmentsDir, ExportsDir, Origin, WebDir, Listen string
	CookieSecure                                                    bool
	QuotaBytes, UploadLimit, ExportLimit                            int64
	HistoryLimit                                                    int
	ExportConcurrency                                               int
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func Load() (Settings, error) {
	database, err := databaseURL()
	if err != nil {
		return Settings{}, err
	}
	s := Settings{DatabaseURL: database, AttachmentsDir: env("ATTACHMENTS_DIR", "data/attachments"), Origin: strings.TrimRight(env("APP_ORIGIN", "http://127.0.0.1:5173"), "/"), CookieSecure: env("COOKIE_SECURE", "false") == "true", WebDir: os.Getenv("WEB_DIR"), Listen: env("LISTEN_ADDR", "127.0.0.1:8000"), QuotaBytes: 1 << 30, UploadLimit: 10 << 20, HistoryLimit: 200, ExportLimit: 2 << 30, ExportConcurrency: 2}
	s.ExportsDir = env("EXPORTS_DIR", filepath.Join(filepath.Dir(s.AttachmentsDir), "exports"))
	u, e := url.Parse(s.Origin)
	if e != nil || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Host == "" {
		return s, fmt.Errorf("invalid APP_ORIGIN")
	}
	if s.CookieSecure {
		if u.Scheme != "https" {
			return s, fmt.Errorf("COOKIE_SECURE requires HTTPS")
		}
	} else if u.Scheme != "http" || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") {
		return s, fmt.Errorf("non-local deployment requires COOKIE_SECURE=true")
	}
	if s.HistoryLimit, e = strconv.Atoi(env("NOTE_HISTORY_LIMIT", "200")); e != nil || s.HistoryLimit < 2 || s.HistoryLimit > 2000 {
		return s, fmt.Errorf("invalid history limit")
	}
	if s.ExportLimit, e = strconv.ParseInt(env("EXPORT_LIMIT_BYTES", "2147483648"), 10, 64); e != nil || s.ExportLimit < 1 {
		return s, fmt.Errorf("invalid export limit")
	}
	return s, nil
}

// Keep the ctl snapshot authoritative: derive a URL only when it supplies no
// canonical DATABASE_URL. Errors name fields and never include secret values.
func databaseURL() (string, error) {
	production := env("REQUIRE_DATABASE_URL", "false") == "true"
	if raw := os.Getenv("DATABASE_URL"); raw != "" {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "", fmt.Errorf("invalid DATABASE_URL")
		}
		if production && (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" || parsed.Hostname() == "" || parsed.Path == "" || parsed.Path == "/") {
			return "", fmt.Errorf("invalid DATABASE_URL: production requires PostgreSQL")
		}
		return raw, nil
	}
	fields := []string{"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_SSLMODE"}
	provided := false
	for _, key := range fields {
		provided = provided || os.Getenv(key) != ""
	}
	if !provided && !production {
		return "sqlite:///data/notes.db", nil
	}
	missing := []string{}
	for _, key := range []string{"DB_HOST", "DB_USER", "DB_PASSWORD"} {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) != 0 {
		return "", fmt.Errorf("missing %s (or DATABASE_URL)", strings.Join(missing, ", "))
	}
	host := os.Getenv("DB_HOST")
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	if net.ParseIP(host) == nil && !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`).MatchString(host) {
		return "", fmt.Errorf("invalid DB_HOST")
	}
	port, err := strconv.Atoi(env("DB_PORT", "5432"))
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("invalid DB_PORT")
	}
	name, ssl := env("DB_NAME", "notes"), env("DB_SSLMODE", "require")
	for _, key := range []string{"DB_USER", "DB_PASSWORD", "DB_NAME"} {
		if strings.ContainsRune(env(key, name), 0) {
			return "", fmt.Errorf("invalid %s", key)
		}
	}
	if ssl != "require" && ssl != "verify-ca" && ssl != "verify-full" {
		return "", fmt.Errorf("invalid DB_SSLMODE")
	}
	query := url.Values{"sslmode": {ssl}, "connect_timeout": {"10"}}
	parsed := url.URL{Scheme: "postgresql", User: url.UserPassword(os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD")), Host: net.JoinHostPort(host, strconv.Itoa(port)), Path: "/" + name, RawPath: "/" + url.PathEscape(name), RawQuery: query.Encode()}
	return parsed.String(), nil
}
