package auth

import "testing"

func TestPasswordAndTokens(t *testing.T) {
	pw := "unique&safe_password_from_test_12345"
	stored, err := HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(pw, stored) {
		t.Fatal("valid password rejected")
	}
	if VerifyPassword("wrong", stored) {
		t.Fatal("invalid password accepted")
	}
	if VerifyPassword("wrong", "pbkdf2-sha256$600000$broken$x") {
		t.Fatal("invalid hash accepted")
	}
	a, err := Secret(32)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Secret(32)
	if err != nil {
		t.Fatal(err)
	}
	if a == b || len(a) != 43 {
		t.Fatalf("poor token entropy or encoding")
	}
	if HashToken(a) == a {
		t.Fatal("hashed tokens not distinct")
	}
	id, err := UUID()
	if err != nil || len(id) != 36 {
		t.Fatalf("invalid uuid: %v %s", err, id)
	}
}
