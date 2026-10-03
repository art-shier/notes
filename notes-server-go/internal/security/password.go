package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"golang.org/x/crypto/argon2"
	"strings"
)

var slots = make(chan struct{}, 2)

func Secret() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func Digest(s string) string { b := sha256.Sum256([]byte(s)); return hex.EncodeToString(b[:]) }
func CSRF(s string) string   { return Digest(s + ":csrf") }
func HashPassword(password string) (string, error) {
	slots <- struct{}{}
	defer func() { <-slots }()
	salt := make([]byte, 16)
	if _, e := rand.Read(salt); e != nil {
		return "", e
	}
	key := argon2.IDKey([]byte(password), salt, 3, 65536, 4, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=4$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}
func VerifyPassword(hash, password string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var memory, rounds, parallel uint32
	if n, e := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &rounds, &parallel); e != nil || n != 3 || memory < 8 || memory > 262144 || rounds < 1 || rounds > 10 || parallel < 1 || parallel > 16 {
		return false
	}
	salt, e := base64.RawStdEncoding.DecodeString(parts[4])
	if e != nil || len(salt) < 8 || len(salt) > 64 {
		return false
	}
	want, e := base64.RawStdEncoding.DecodeString(parts[5])
	if e != nil || len(want) < 16 || len(want) > 64 {
		return false
	}
	slots <- struct{}{}
	defer func() { <-slots }()
	got := argon2.IDKey([]byte(password), salt, rounds, memory, uint8(parallel), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}
