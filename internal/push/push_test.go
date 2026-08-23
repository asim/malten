package push

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newSubscription plays the browser: it generates the keypair a real
// PushManager subscription would hold, so the test can decrypt what we send.
func newSubscription(t *testing.T, endpoint string) (Subscription, *ecdh.PrivateKey, []byte) {
	t.Helper()
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	authSecret := make([]byte, 16)
	if _, err := rand.Read(authSecret); err != nil {
		t.Fatal(err)
	}
	var sub Subscription
	sub.Endpoint = endpoint
	sub.Keys.P256dh = b64.EncodeToString(priv.PublicKey().Bytes())
	sub.Keys.Auth = b64.EncodeToString(authSecret)
	return sub, priv, authSecret
}

// decrypt is the browser side of RFC 8291 — if this recovers the plaintext, a
// real browser will too.
func decrypt(t *testing.T, body []byte, priv *ecdh.PrivateKey, authSecret []byte) []byte {
	t.Helper()
	if len(body) < 21 {
		t.Fatalf("body too short: %d bytes", len(body))
	}
	salt := body[:16]
	idLen := int(body[20])
	asBytes := body[21 : 21+idLen]
	sealed := body[21+idLen:]

	as, err := ecdh.P256().NewPublicKey(asBytes)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := priv.ECDH(as)
	if err != nil {
		t.Fatal(err)
	}
	info := append([]byte("WebPush: info\x00"), priv.PublicKey().Bytes()...)
	info = append(info, asBytes...)
	ikm, err := hkdf.Key(sha256.New, shared, authSecret, string(info), 32)
	if err != nil {
		t.Fatal(err)
	}
	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		t.Fatal(err)
	}
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if n := len(plain); n == 0 || plain[n-1] != 0x02 {
		t.Errorf("missing aes128gcm record delimiter")
	} else {
		plain = plain[:n-1]
	}
	return plain
}

func TestSendRoundTrip(t *testing.T) {
	var got struct {
		body    []byte
		headers http.Header
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.body, _ = io.ReadAll(r.Body)
		got.headers = r.Header.Clone()
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	privKey, _, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	sender, err := NewSender(privKey, "mailto:ops@example.com")
	if err != nil {
		t.Fatal(err)
	}
	sub, browserKey, authSecret := newSubscription(t, srv.URL+"/push/abc")

	payload := []byte(`{"title":"New ground","body":"TQ 15 69 is a ten-minute walk north."}`)
	if err := sender.Send(sub, payload, 6*time.Hour); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if ce := got.headers.Get("Content-Encoding"); ce != "aes128gcm" {
		t.Errorf("Content-Encoding = %q, want aes128gcm", ce)
	}
	if ttl := got.headers.Get("TTL"); ttl != "21600" {
		t.Errorf("TTL = %q, want 21600", ttl)
	}

	// The payload must be readable by the subscriber, and only by them.
	if plain := decrypt(t, got.body, browserKey, authSecret); string(plain) != string(payload) {
		t.Errorf("decrypted %q, want %q", plain, payload)
	}
	// …and only by them: another browser's key must not open it.
	if openable(got.body) {
		t.Errorf("payload decrypted with the wrong key")
	}
}

// openable reports whether the record can be opened with a freshly generated
// (i.e. wrong) subscriber key — it must not be.
func openable(body []byte) bool {
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return false
	}
	authSecret := make([]byte, 16)
	if _, err := rand.Read(authSecret); err != nil {
		return false
	}
	salt, idLen := body[:16], int(body[20])
	asBytes, sealed := body[21:21+idLen], body[21+idLen:]
	as, err := ecdh.P256().NewPublicKey(asBytes)
	if err != nil {
		return false
	}
	shared, err := priv.ECDH(as)
	if err != nil {
		return false
	}
	info := append([]byte("WebPush: info\x00"), priv.PublicKey().Bytes()...)
	info = append(info, asBytes...)
	ikm, err := hkdf.Key(sha256.New, shared, authSecret, string(info), 32)
	if err != nil {
		return false
	}
	cek, _ := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	nonce, _ := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	block, err := aes.NewCipher(cek)
	if err != nil {
		return false
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return false
	}
	_, err = gcm.Open(nil, nonce, sealed, nil)
	return err == nil
}

// The VAPID header is what the push service checks before it accepts anything.
func TestVapidAuthorization(t *testing.T) {
	privKey, pubKey, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	sender, err := NewSender(privKey, "mailto:ops@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if sender.PublicKey() != pubKey {
		t.Errorf("public key doesn't round-trip through NewSender")
	}

	auth, err := sender.authorization("https://fcm.googleapis.com/fcm/send/abc123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(auth, "vapid t=") || !strings.Contains(auth, ", k="+pubKey) {
		t.Fatalf("malformed header: %q", auth)
	}
	token := strings.TrimPrefix(strings.SplitN(auth, ",", 2)[0], "vapid t=")
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT has %d parts", len(parts))
	}

	// Audience must be the push service's origin, not the full endpoint.
	claimsJSON, err := b64.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims struct {
		Aud string `json:"aud"`
		Sub string `json:"sub"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Aud != "https://fcm.googleapis.com" {
		t.Errorf("aud = %q, want the origin only", claims.Aud)
	}
	if claims.Sub != "mailto:ops@example.com" {
		t.Errorf("sub = %q", claims.Sub)
	}
	if claims.Exp <= time.Now().Unix() {
		t.Errorf("token already expired")
	}

	// And the signature must verify against the advertised public key.
	sig, err := b64.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		t.Fatalf("signature is %d bytes: %v", len(sig), err)
	}
	pubBytes, _ := b64.DecodeString(pubKey)
	x, y := elliptic.Unmarshal(elliptic.P256(), pubBytes)
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(&ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, sum[:],
		new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Errorf("signature does not verify")
	}
}

// A dead subscription must be reported as such, so the caller can forget it.
func TestSendGone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	defer srv.Close()

	privKey, _, _ := GenerateKey()
	sender, _ := NewSender(privKey, "")
	sub, _, _ := newSubscription(t, srv.URL+"/push/dead")
	if err := sender.Send(sub, []byte("hi"), time.Hour); err != ErrGone {
		t.Errorf("err = %v, want ErrGone", err)
	}
}
