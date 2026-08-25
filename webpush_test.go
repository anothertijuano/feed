package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVAPIDAuthHeader(t *testing.T) {
	v, err := loadOrCreateVAPID(filepath.Join(t.TempDir(), "vapid.json"), "mailto:test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	hdr, err := v.authHeader("https://push.example.com/xyz/123")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(hdr, ", ", 2)
	if len(parts) != 2 {
		t.Fatalf("header = %q", hdr)
	}
	token := strings.TrimPrefix(parts[0], "vapid t=")
	if got := strings.TrimPrefix(parts[1], "k="); got != v.PublicKey() {
		t.Fatalf("k = %q, want %q", got, v.PublicKey())
	}

	segs := strings.Split(token, ".")
	if len(segs) != 3 {
		t.Fatalf("token segments = %d", len(segs))
	}
	var h struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(b64uDecode(segs[0]), &h); err != nil || h.Alg != "ES256" {
		t.Fatalf("jwt header = %s (%v)", segs[0], err)
	}
	var claims struct {
		Aud string `json:"aud"`
		Sub string `json:"sub"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(b64uDecode(segs[1]), &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Aud != "https://push.example.com" {
		t.Fatalf("aud = %q", claims.Aud)
	}
	if claims.Sub != "mailto:test@example.com" {
		t.Fatalf("sub = %q", claims.Sub)
	}
	if claims.Exp < time.Now().Unix() {
		t.Fatal("exp in the past")
	}

	// Verify the ES256 signature over the JWT signing input.
	digest := sha256.Sum256([]byte(segs[0] + "." + segs[1]))
	if !ecdsa.VerifyASN1(&v.private.PublicKey, digest[:], b64uDecode(segs[2])) {
		t.Fatal("signature does not verify")
	}
}

func TestEncryptPayloadRoundtrip(t *testing.T) {
	// The subscriber's key pair (in production this lives in the browser).
	subPriv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	subPub := subPriv.PublicKey()
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		t.Fatal(err)
	}

	plaintext := []byte(`{"title":"hello","url":"https://example.com"}`)
	body, err := encryptPayload(b64u(subPub.Bytes()), b64u(auth), plaintext)
	if err != nil {
		t.Fatal(err)
	}
	// Small payloads are padded to one full record.
	if len(body) != webpushRecordSize {
		t.Fatalf("body length = %d, want %d", len(body), webpushRecordSize)
	}

	// Parse the header.
	salt := body[:16]
	rs := binary.BigEndian.Uint32(body[16:20])
	idLen := int(body[20])
	keyID := body[21 : 21+idLen]
	if rs != webpushRecordSize || idLen != 65 {
		t.Fatalf("rs=%d idLen=%d", rs, idLen)
	}

	// Derive the keys as a push service would.
	ephPub, err := ecdh.P256().NewPublicKey(keyID)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := subPriv.ECDH(ephPub)
	if err != nil {
		t.Fatal(err)
	}
	info := append(append([]byte("WebPush: info\x00"), subPub.Bytes()...), keyID...)
	km := hkdfSHA256(shared, salt, info, 32)

	block, err := aes.NewCipher(km[:16])
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	record, err := gcm.Open(nil, km[16:28], body[21+idLen:], nil)
	if err != nil {
		t.Fatal(err)
	}

	idx := bytes.IndexByte(record, 0x02)
	if idx < 0 {
		t.Fatal("no padding delimiter in record")
	}
	if got := record[:idx]; !bytes.Equal(got, plaintext) {
		t.Fatalf("plaintext = %q, want %q", got, plaintext)
	}
}
