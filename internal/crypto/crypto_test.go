package crypto

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"
)

func randKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, k); err != nil {
		t.Fatal(err)
	}
	return k
}

func TestRoundTrip(t *testing.T) {
	key := randKey(t)
	pt := []byte("https://portal-renovacoes.aima.gov.pt/ords/r/aima/aima-pr/validar?p71_lang=pt&token=abc123")
	ct, err := Encrypt(pt, key)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ct, pt) {
		t.Fatal("ciphertext leaks plaintext")
	}
	got, err := Decrypt(ct, key)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, pt) {
		t.Fatalf("got %q, want %q", got, pt)
	}
}

func TestEncryptIsRandomized(t *testing.T) {
	key := randKey(t)
	pt := []byte("same plaintext")
	a, err := Encrypt(pt, key)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Encrypt(pt, key)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a, b) {
		t.Fatal("two encryptions of same plaintext produced identical ciphertext (nonce reuse)")
	}
}

func TestWrongKeyFails(t *testing.T) {
	k1, k2 := randKey(t), randKey(t)
	ct, err := Encrypt([]byte("secret"), k1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(ct, k2); err == nil {
		t.Fatal("expected decrypt to fail with wrong key")
	}
}

func TestTamperFails(t *testing.T) {
	key := randKey(t)
	ct, err := Encrypt([]byte("secret"), key)
	if err != nil {
		t.Fatal(err)
	}
	ct[len(ct)-1] ^= 0x01 // flip a bit in the auth tag
	if _, err := Decrypt(ct, key); err == nil {
		t.Fatal("expected decrypt to fail on tamper")
	}
}

func TestKeyLength(t *testing.T) {
	if _, err := Encrypt([]byte("x"), make([]byte, 16)); err == nil {
		t.Fatal("expected error for 16-byte key")
	}
}
