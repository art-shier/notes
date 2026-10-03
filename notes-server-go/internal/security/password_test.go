package security

import "testing"

func TestPasswordCompatibility(t *testing.T) {
	h, e := HashPassword("test-password-123")
	if e != nil {
		t.Fatal(e)
	}
	if !VerifyPassword(h, "test-password-123") || VerifyPassword(h, "wrong") {
		t.Fatal("argon2 verification broken")
	}
	for _, h := range []string{"bad", "$argon2id$v=19$m=4294967295,t=3,p=4$AAAA$AAAA"} {
		if VerifyPassword(h, "x") {
			t.Fatal("invalid accepted")
		}
	}
}
