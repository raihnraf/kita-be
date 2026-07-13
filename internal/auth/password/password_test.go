package password_test

import (
	"testing"

	pwdsvc "kita-be/internal/auth/password"
)

func TestHashAndVerify(t *testing.T) {
	svc := pwdsvc.NewService()

	hash, err := svc.Hash("my-secret-password")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

	if !svc.Verify("my-secret-password", hash) {
		t.Fatal("expected password to verify correctly")
	}
}

func TestVerifyWrongPassword(t *testing.T) {
	svc := pwdsvc.NewService()

	hash, err := svc.Hash("correct-password")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	if svc.Verify("wrong-password", hash) {
		t.Fatal("expected wrong password to fail verification")
	}
}

func TestHashProducesDifferentResults(t *testing.T) {
	svc := pwdsvc.NewService()

	hash1, err := svc.Hash("password")
	if err != nil {
		t.Fatalf("failed to hash: %v", err)
	}

	hash2, err := svc.Hash("password")
	if err != nil {
		t.Fatalf("failed to hash: %v", err)
	}

	if hash1 == hash2 {
		t.Fatal("expected different hashes for same password due to salt")
	}
}
