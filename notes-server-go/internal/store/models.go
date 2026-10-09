package store

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"time"
)

type Time struct{ time.Time }

func Now() Time { return Time{time.Now().UTC().Truncate(time.Microsecond)} }
func (t Time) Value() (driver.Value, error) {
	return t.Time.UTC().Format("2006-01-02 15:04:05.000000"), nil
}
func (t *Time) Scan(v any) error {
	if v == nil {
		t.Time = time.Time{}
		return nil
	}
	if x, ok := v.(time.Time); ok {
		t.Time = x.UTC()
		return nil
	}
	var s string
	switch x := v.(type) {
	case string:
		s = x
	case []byte:
		s = string(x)
	default:
		return fmt.Errorf("unsupported time")
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999999", "2006-01-02T15:04:05.999999999", "2006-01-02 15:04:05-07:00"} {
		if x, e := time.Parse(layout, s); e == nil {
			t.Time = x.UTC()
			return nil
		}
	}
	return fmt.Errorf("invalid timestamp")
}
func (t Time) MarshalJSON() ([]byte, error) { return json.Marshal(t.UTC().Format(time.RFC3339Nano)) }
func (Time) GormDataType() string           { return "datetime" }

type Map map[string]any
type List []string

func jsonScan(v any, out any) error {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []byte:
		return json.Unmarshal(x, out)
	case string:
		return json.Unmarshal([]byte(x), out)
	}
	return fmt.Errorf("invalid JSON column")
}
func (m *Map) Scan(v any) error            { return jsonScan(v, m) }
func (m Map) Value() (driver.Value, error) { b, e := json.Marshal(m); return string(b), e }
func (Map) GormDataType() string           { return "json" }
func (m *List) Scan(v any) error           { return jsonScan(v, m) }
func (m List) Value() (driver.Value, error) {
	if m == nil {
		m = List{}
	}
	b, e := json.Marshal(m)
	return string(b), e
}
func (List) GormDataType() string { return "json" }
func ID() string                  { return uuid.NewString() }

type User struct {
	ID, Email, DisplayName, PasswordHash, Role string
	Disabled                                   bool
	QuotaBytes, UsedBytes                      int64
	CreatedAt                                  Time
}
type Invitation struct {
	ID, SecretHash, Email, Role string
	ExpiresAt                   Time
	UsedAt                      *Time
}
type Session struct {
	ID, UserID, SecretHash string
	ExpiresAt              Time
}

func (Session) TableName() string { return "sessions" }

type Folder struct {
	ID, UserID      string
	ParentID        *string
	ParentKey, Name string
	IsInbox         bool
}
type Note struct {
	ID, UserID, FolderID, Title string
	ContentJSON                 Map  `gorm:"column:content_json"`
	BlockIDs                    List `gorm:"column:block_ids"`
	PlainText                   string
	Favorite, Trashed           bool
	Source                      string
	Version                     int
	CreatedAt, UpdatedAt        Time
}
type Attachment struct {
	ID, UserID, ObjectKey, MimeType string
	SizeBytes                       int64
	CreatedAt                       Time
}
type Token struct {
	ID, UserID, Name, Prefix, SecretHash string
	Scopes                               List
	ExpiresAt                            Time
	RevokedAt                            *Time
	CreatedAt                            Time
}

func (Token) TableName() string { return "api_tokens" }

// AgentGrant contains hashes only; the credential is generated and kept by the client.
type AgentGrant struct {
	ID, DeviceHash, UserCode, Name, TokenHash, TokenPrefix, Status string
	TokenID                                                        *string
	CreatedAt, ExpiresAt                                           Time
}

type Tag struct{ ID, UserID, Name string }
type NoteTag struct {
	UserID string
	NoteID string `gorm:"primaryKey"`
	TagID  string `gorm:"primaryKey"`
}
type Idempotency struct {
	ID, UserID, ActorKey, Route, Key, RequestHash string
	Response                                      Map
	ExpiresAt                                     Time
}

func (Idempotency) TableName() string { return "idempotency_records" }

type Revision struct {
	ID, UserID, NoteID string
	Version            int
	Snapshot           Map
	Action, Actor      string
	RestoredFrom       *int
	SavedAt            Time
}

func (Revision) TableName() string { return "note_revisions" }

type Export struct {
	ID, UserID           string
	Ready                bool
	SizeBytes            int64
	Manifest             Map
	CreatedAt, ExpiresAt Time
}

func (Export) TableName() string { return "export_bundles" }
