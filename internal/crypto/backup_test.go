package crypto

import (
	"bytes"
	"testing"
)

func TestBackupEncryptionRoundTrip(t *testing.T) {
	plain := []byte("accounts.json and profile data")
	password := "correct-horse-battery"

	encrypted, err := EncryptBackup(plain, password)
	if err != nil {
		t.Fatalf("EncryptBackup failed: %v", err)
	}
	if !bytes.HasPrefix(encrypted, []byte(BackupPrefix)) {
		t.Fatalf("encrypted backup missing prefix")
	}
	if bytes.Contains(encrypted, plain) {
		t.Fatalf("encrypted backup contains plaintext")
	}

	decrypted, err := DecryptBackup(encrypted, password)
	if err != nil {
		t.Fatalf("DecryptBackup failed: %v", err)
	}
	if !bytes.Equal(decrypted, plain) {
		t.Fatalf("round-trip mismatch")
	}
	if _, err := DecryptBackup(encrypted, "wrong-password-123"); err == nil {
		t.Fatalf("wrong password should fail")
	}
}

func TestBackupPasswordMinimumLength(t *testing.T) {
	plain := []byte("small backup")
	if _, err := EncryptBackup(plain, "1234"); err == nil {
		t.Fatal("expected a four-character backup password to be rejected")
	}

	encrypted, err := EncryptBackup(plain, "abcde")
	if err != nil {
		t.Fatalf("five-character backup password should be accepted: %v", err)
	}
	decrypted, err := DecryptBackup(encrypted, "abcde")
	if err != nil {
		t.Fatalf("decrypting with a five-character backup password failed: %v", err)
	}
	if !bytes.Equal(decrypted, plain) {
		t.Fatal("backup round-trip with five-character password mismatch")
	}
}
