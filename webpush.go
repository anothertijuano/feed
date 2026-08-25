package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

/* Web Push: VAPID (ES256 JWT) + RFC 8291 aes128gcm payload encryption. */

// PushSub is a client's push subscription (the Web Push API PushSubscription
// JSON shape).
type PushSub struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	CreatedAt string `json:"createdAt,omitempty"`
}

func b64u(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func b64uDecode(s string) []byte {
	b, _ := base64.RawURLEncoding.DecodeString(s)
	return b
}

// VAPID holds the application's VAPID identity (P-256 / ES256).
type VAPID struct {
	public  *ecdh.PublicKey
	private *ecdsa.PrivateKey
	rawPub  []byte // uncompressed 65-byte public key
	subject string
}

// GenerateVAPID creates a fresh key pair, returning the base64url values.
func GenerateVAPID() (pub, priv string, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	rawPub := elliptic.Marshal(elliptic.P256(), key.X, key.Y)
	rawPriv := key.D.FillBytes(make([]byte, 32))
	return b64u(rawPub), b64u(rawPriv), nil
}

// loadOrCreateVAPID loads the key pair from a small JSON file, generating
// and persisting one on first use.
func loadOrCreateVAPID(path, subject string) (*VAPID, error) {
	var rawPub, rawPriv []byte
	b, err := os.ReadFile(path)
	if err == nil {
		var k struct {
			Public  string `json:"public"`
			Private string `json:"private"`
		}
		if err := json.Unmarshal(b, &k); err == nil {
			rawPub, rawPriv = b64uDecode(k.Public), b64uDecode(k.Private)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	if len(rawPub) == 0 || len(rawPriv) == 0 {
		pub, priv, err := GenerateVAPID()
		if err != nil {
			return nil, err
		}
		rawPub, rawPriv = b64uDecode(pub), b64uDecode(priv)
		data, _ := json.MarshalIndent(map[string]string{"public": pub, "private": priv}, "", "  ")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return nil, err
		}
	}

	if len(rawPub) != 65 || len(rawPriv) != 32 {
		return nil, fmt.Errorf("invalid VAPID key material in %s", path)
	}

	curve := elliptic.P256()
	x, y := curve.ScalarBaseMult(rawPriv)
	priv := &ecdsa.PrivateKey{
		D:         new(big.Int).SetBytes(rawPriv),
		PublicKey: ecdsa.PublicKey{Curve: curve, X: x, Y: y},
	}
	pub, err := ecdh.P256().NewPublicKey(rawPub)
	if err != nil {
		return nil, err
	}
	return &VAPID{public: pub, private: priv, rawPub: rawPub, subject: subject}, nil
}

// PublicKey returns the base64url public key (sent to the client for
// pushManager.subscribe).
func (v *VAPID) PublicKey() string { return b64u(v.rawPub) }

// authHeader builds the `vapid t=…, k=…` Authorization header value.
func (v *VAPID) authHeader(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	aud := u.Scheme + "://" + u.Host

	header := `{"typ":"JWT","alg":"ES256"}`
	claims := fmt.Sprintf(`{"aud":%q,"exp":%d,"sub":%q}`,
		aud, time.Now().Add(12*time.Hour).Unix(), v.subject)
	token := b64u([]byte(header)) + "." + b64u([]byte(claims))

	digest := sha256.Sum256([]byte(token))
	sig, err := ecdsa.SignASN1(rand.Reader, v.private, digest[:])
	if err != nil {
		return "", err
	}
	return "vapid t=" + token + "." + b64u(sig) + ", k=" + b64u(v.rawPub), nil
}

// hkdfSHA256 implements RFC 5869 HKDF with SHA-256.
func hkdfSHA256(ikm, salt, info []byte, length int) []byte {
	if len(salt) == 0 {
		salt = make([]byte, 32)
	}
	extract := hmac.New(sha256.New, salt)
	extract.Write(ikm)
	prk := extract.Sum(nil)

	out := make([]byte, 0, length)
	var t []byte
	for i := byte(1); len(out) < length; i++ {
		expand := hmac.New(sha256.New, prk)
		expand.Write(t)
		expand.Write(info)
		expand.Write([]byte{i})
		t = expand.Sum(nil)
		out = append(out, t...)
	}
	return out[:length]
}

const (
	webpushRecordSize = 4096
	webpushKeyIDLen   = 65
)

// encryptPayload implements RFC 8291 (aes128gcm) for Web Push, returning
// the full request body (header + ciphertext).
func encryptPayload(p256dh, auth string, plaintext []byte) ([]byte, error) {
	userPub, err := ecdh.P256().NewPublicKey(b64uDecode(p256dh))
	if err != nil {
		return nil, fmt.Errorf("p256dh: %w", err)
	}
	authSecret := b64uDecode(auth)
	if len(authSecret) != 16 {
		return nil, fmt.Errorf("auth secret must be 16 bytes")
	}

	eph, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	shared, err := eph.ECDH(userPub)
	if err != nil {
		return nil, err
	}

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}

	ephPub := eph.PublicKey().Bytes() // 65-byte uncompressed point
	info := make([]byte, 0, 144)
	info = append(info, "WebPush: info\x00"...)
	info = append(info, userPub.Bytes()...)
	info = append(info, ephPub...)

	keyMaterial := hkdfSHA256(shared, salt, info, 32)
	cek, nonce := keyMaterial[:16], keyMaterial[16:28]

	// Plaintext: payload || 0x02 delimiter || padding, so that the record
	// (86-byte header + plaintext + 16-byte tag) is at least rs bytes.
	record := append([]byte{}, plaintext...)
	record = append(record, 0x02)
	pad := webpushRecordSize - (len(record) + 86 + 16)
	if pad < 0 {
		pad = 0
	}
	record = append(record, make([]byte, pad)...)

	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, record, nil)

	header := make([]byte, 0, 86)
	header = append(header, salt...)
	var rsBuf [4]byte
	binary.BigEndian.PutUint32(rsBuf[:], webpushRecordSize)
	header = append(header, rsBuf[:]...)
	header = append(header, byte(webpushKeyIDLen))
	header = append(header, ephPub...)

	return append(header, ciphertext...), nil
}

// pushDeliveryError is a non-2xx response from a push service.
type pushDeliveryError struct {
	status int
	body   string
}

func (e *pushDeliveryError) Error() string {
	if e.body != "" {
		return fmt.Sprintf("push: HTTP %d: %s", e.status, e.body)
	}
	return fmt.Sprintf("push: HTTP %d", e.status)
}

// Gone reports whether the subscription is dead and should be pruned.
func (e *pushDeliveryError) Gone() bool {
	return e.status == http.StatusNotFound || e.status == http.StatusGone
}

// WebPusher delivers encrypted payloads to push subscriptions.
type WebPusher struct {
	vapid  *VAPID
	client *http.Client
	log    *slog.Logger
}

func newWebPusher(vapid *VAPID, client *http.Client, log *slog.Logger) *WebPusher {
	return &WebPusher{vapid: vapid, client: client, log: log}
}

// Send delivers a payload to one subscription.
func (s *WebPusher) Send(sub PushSub, payload []byte, ttl int) error {
	body, err := encryptPayload(sub.Keys.P256dh, sub.Keys.Auth, payload)
	if err != nil {
		return err
	}
	authHeader, err := s.vapid.authHeader(sub.Endpoint)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", strconv.Itoa(ttl))
	req.Header.Set("Authorization", authHeader)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		return nil
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return &pushDeliveryError{status: resp.StatusCode, body: strings.TrimSpace(string(b))}
}
