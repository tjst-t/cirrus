package network

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestGenerateWireGuardKeypair(t *testing.T) {
	priv, pub, err := GenerateWireGuardKeypair()
	if err != nil {
		t.Fatalf("GenerateWireGuardKeypair: %v", err)
	}
	if len(priv) != 32 {
		t.Errorf("expected private key length 32, got %d", len(priv))
	}
	if len(pub) != 32 {
		t.Errorf("expected public key length 32, got %d", len(pub))
	}
	// Verify Curve25519 clamping was applied.
	if priv[0]&7 != 0 {
		t.Error("expected lowest 3 bits of private key byte 0 to be cleared (Curve25519 clamping)")
	}
	if priv[31]&128 != 0 {
		t.Error("expected highest bit of private key byte 31 to be cleared (Curve25519 clamping)")
	}
	if priv[31]&64 == 0 {
		t.Error("expected bit 6 of private key byte 31 to be set (Curve25519 clamping)")
	}
}

func TestGenerateWireGuardKeypair_UniquePairs(t *testing.T) {
	priv1, pub1, err := GenerateWireGuardKeypair()
	if err != nil {
		t.Fatalf("first keypair: %v", err)
	}
	priv2, pub2, err := GenerateWireGuardKeypair()
	if err != nil {
		t.Fatalf("second keypair: %v", err)
	}
	if bytes.Equal(priv1, priv2) {
		t.Error("two generated private keys should not be equal")
	}
	if bytes.Equal(pub1, pub2) {
		t.Error("two generated public keys should not be equal")
	}
}

func TestEncryptDecryptAESGCM(t *testing.T) {
	key := make([]byte, 32) // AES-256
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}
	plaintext := []byte("wireguard-private-key-32bytelong!")

	ciphertext, err := EncryptAESGCM(key, plaintext)
	if err != nil {
		t.Fatalf("EncryptAESGCM: %v", err)
	}
	if bytes.Equal(ciphertext, plaintext) {
		t.Error("ciphertext should not equal plaintext")
	}

	decrypted, err := DecryptAESGCM(key, ciphertext)
	if err != nil {
		t.Fatalf("DecryptAESGCM: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptAESGCM_WrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	if _, err := rand.Read(key1); err != nil {
		t.Fatalf("generate key1: %v", err)
	}
	if _, err := rand.Read(key2); err != nil {
		t.Fatalf("generate key2: %v", err)
	}

	ciphertext, err := EncryptAESGCM(key1, []byte("secret"))
	if err != nil {
		t.Fatalf("EncryptAESGCM: %v", err)
	}
	if _, err := DecryptAESGCM(key2, ciphertext); err == nil {
		t.Error("expected error when decrypting with wrong key")
	}
}

func TestDecryptAESGCM_ShortCiphertext(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if _, err := DecryptAESGCM(key, []byte("short")); err == nil {
		t.Error("expected error for too-short ciphertext")
	}
}

func TestEncryptAESGCM_Nondeterministic(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}
	plaintext := []byte("same plaintext")

	ct1, err := EncryptAESGCM(key, plaintext)
	if err != nil {
		t.Fatalf("EncryptAESGCM (1): %v", err)
	}
	ct2, err := EncryptAESGCM(key, plaintext)
	if err != nil {
		t.Fatalf("EncryptAESGCM (2): %v", err)
	}
	if bytes.Equal(ct1, ct2) {
		t.Error("AES-GCM encryption should produce different ciphertexts for same plaintext (random nonce)")
	}
}
