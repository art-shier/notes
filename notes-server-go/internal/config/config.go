package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
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
	s := Settings{DatabaseURL: env("DATABASE_URL", "sqlite:///data/notes.db"), AttachmentsDir: env("ATTACHMENTS_DIR", "data/attachments"), Origin: strings.TrimRight(env("APP_ORIGIN", "http://127.0.0.1:5173"), "/"), CookieSecure: env("COOKIE_SECURE", "false") == "true", WebDir: os.Getenv("WEB_DIR"), Listen: env("LISTEN_ADDR", "127.0.0.1:8000"), QuotaBytes: 1 << 30, UploadLimit: 10 << 20, HistoryLimit: 200, ExportLimit: 2 << 30, ExportConcurrency: 2}
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
