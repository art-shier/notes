package store

import (
	"path/filepath"
	"testing"
)

func TestFreshSchemaAndAtomicRollback(t *testing.T) {
	db, err := Open("sqlite:///" + filepath.ToSlash(filepath.Join(t.TempDir(), "notes.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer Close(db)
	if err = Migrate(db); err != nil {
		t.Fatal(err)
	}
	u := User{ID: ID(), Email: "a@example.test", DisplayName: "A", PasswordHash: "hash", Role: "member", QuotaBytes: 100, CreatedAt: Now()}
	if err = db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	var got User
	if err = db.First(&got, "id = ?", u.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Email != u.Email || got.CreatedAt.IsZero() {
		t.Fatal("legacy columns/time not preserved")
	}
	if err = Migrate(db); err != nil {
		t.Fatal(err)
	}
	// Emulate an existing deployment with the legacy revision but no grant extension.
	if err = db.Migrator().DropTable(&AgentGrant{}); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(db); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable(&AgentGrant{}) {
		t.Fatal("missing additive migration")
	}
	if err = db.First(&got, "id = ?", u.ID).Error; err != nil || got.Email != u.Email {
		t.Fatal("upgrade lost account", err)
	}
}
