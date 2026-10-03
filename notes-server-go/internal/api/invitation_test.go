package api

import (
	"path/filepath"
	"shiji/internal/store"
	"testing"
)

func TestBootstrapAndAdminInvitationGuards(t *testing.T) {
	db, e := store.Open("sqlite:///" + filepath.ToSlash(filepath.Join(t.TempDir(), "db")))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close(db)
	if e = store.Migrate(db); e != nil {
		t.Fatal(e)
	}
	raw, e := IssueInvitation(db, " Root@Example.test ", "", true)
	if e != nil || raw == "" {
		t.Fatal(e)
	}
	if _, e = IssueInvitation(db, "second@example.test", "", true); e == nil {
		t.Fatal("parallel pending bootstrap permitted")
	}
	var inv store.Invitation
	if e = db.First(&inv).Error; e != nil || inv.Email != "root@example.test" || inv.Role != "admin" {
		t.Fatal("invalid bootstrap invitation", e)
	}
	used := store.Now()
	if e = db.Model(&inv).Update("used_at", used).Error; e != nil {
		t.Fatal(e)
	}
	u := store.User{ID: store.ID(), Email: inv.Email, DisplayName: "Root", PasswordHash: "unused", Role: "admin", QuotaBytes: 100, CreatedAt: store.Now()}
	if e = db.Create(&u).Error; e != nil {
		t.Fatal(e)
	}
	if _, e = IssueInvitation(db, "member@example.test", u.Email, false); e != nil {
		t.Fatal(e)
	}
	if _, e = IssueInvitation(db, "another@example.test", "nobody@example.test", false); e == nil {
		t.Fatal("nonadmin issued invitation")
	}
	if _, e = IssueInvitation(db, "second@example.test", "", true); e == nil {
		t.Fatal("existing user allowed bootstrap")
	}
	db.Model(&u).Update("disabled", true)
	if _, e = IssueInvitation(db, "disabled@example.test", u.Email, false); e == nil {
		t.Fatal("disabled admin issued invitation")
	}
}
