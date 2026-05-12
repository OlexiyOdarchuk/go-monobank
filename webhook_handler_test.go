package monobank

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeKeyProvider is a KeyProvider whose returned key and error are settable
// at runtime. It also counts calls so tests can assert refresh behaviour.
type fakeKeyProvider struct {
	key   *ServerKey
	err   error
	calls atomic.Int32
}

func (f *fakeKeyProvider) ServerKey(_ context.Context) (*ServerKey, error) {
	f.calls.Add(1)
	if f.err != nil {
		return nil, f.err
	}
	return f.key, nil
}

func newTestHandler(t *testing.T, opts WebhookHandlerOptions) (*WebhookHandler, *fakeKeyProvider) {
	t.Helper()
	pub := testServerPubKey(t)
	prov := &fakeKeyProvider{key: &ServerKey{ID: testServerKeyID, PubKey: pub}}
	opts.Keys = prov
	h, err := NewWebhookHandler(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, int32(1), prov.calls.Load())
	return h, prov
}

func signedPOST(body, sign, keyID string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	r.Header.Set("X-Sign", sign)
	r.Header.Set("X-Key-Id", keyID)
	return r
}

func TestNewWebhookHandler_validation(t *testing.T) {
	_, err := NewWebhookHandler(context.Background(), WebhookHandlerOptions{
		OnEvent: func(context.Context, *WebHookResponse) error { return nil },
	})
	assert.ErrorIs(t, err, ErrNilKeyProvider)

	_, err = NewWebhookHandler(context.Background(), WebhookHandlerOptions{
		Keys: &fakeKeyProvider{key: &ServerKey{}},
	})
	assert.ErrorIs(t, err, ErrNilOnEvent)
}

func TestNewWebhookHandler_initialFetchFails(t *testing.T) {
	prov := &fakeKeyProvider{err: errors.New("boom")}
	_, err := NewWebhookHandler(context.Background(), WebhookHandlerOptions{
		Keys:    prov,
		OnEvent: func(context.Context, *WebHookResponse) error { return nil },
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "initial ServerKey fetch")
}

func TestWebhookHandler_GET_returns200(t *testing.T) {
	h, _ := newTestHandler(t, WebhookHandlerOptions{
		OnEvent: func(context.Context, *WebHookResponse) error {
			t.Fatal("OnEvent must not be called for GET")
			return nil
		},
	})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/webhook", http.NoBody))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestWebhookHandler_validPOST_callsOnEvent(t *testing.T) {
	var seen *WebHookResponse
	h, _ := newTestHandler(t, WebhookHandlerOptions{
		OnEvent: func(_ context.Context, e *WebHookResponse) error {
			seen = e
			return nil
		},
	})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedPOST(testWebhookBody, testWebhookSign, testServerKeyID))

	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, seen)
	assert.Equal(t, "XfHmJ0KH0p1jVAw58w", seen.Data.Transaction.ID)
}

func TestWebhookHandler_badSignature_rejects401(t *testing.T) {
	called := false
	h, _ := newTestHandler(t, WebhookHandlerOptions{
		OnEvent: func(context.Context, *WebHookResponse) error {
			called = true
			return nil
		},
	})

	tampered := strings.Replace(testWebhookBody, "Поповнення", "Hijacked", 1)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedPOST(tampered, testWebhookSign, testServerKeyID))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.False(t, called, "OnEvent must not run on bad signature")
}

func TestWebhookHandler_callbackError_500(t *testing.T) {
	h, _ := newTestHandler(t, WebhookHandlerOptions{
		OnEvent: func(context.Context, *WebHookResponse) error {
			return errors.New("downstream unavailable")
		},
	})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedPOST(testWebhookBody, testWebhookSign, testServerKeyID))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestWebhookHandler_unknownType_ackedButCallbackSkipped(t *testing.T) {
	const unknown = `{"type":"FutureEvent","data":{}}`

	// Deterministic keypair (any constant 32-byte seed works) so the test
	// doesn't depend on rand.Reader.
	seed := [32]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
		0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20}
	priv := secp256k1.PrivKeyFromBytes(seed[:]).ToECDSA()
	digest := sha256.Sum256([]byte(unknown))
	r, s, err := ecdsa.Sign(rand.Reader, priv, digest[:])
	require.NoError(t, err)
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	signB64 := base64.StdEncoding.EncodeToString(sig)

	var onEventCalled, onUnknownCalled bool
	prov := &fakeKeyProvider{key: &ServerKey{ID: "test", PubKey: &priv.PublicKey}}
	h, err := NewWebhookHandler(context.Background(), WebhookHandlerOptions{
		Keys: prov,
		OnEvent: func(context.Context, *WebHookResponse) error {
			onEventCalled = true
			return nil
		},
		OnUnknownType: func(_ context.Context, raw []byte) {
			onUnknownCalled = true
			assert.Equal(t, unknown, string(raw))
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedPOST(unknown, signB64, "test"))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.False(t, onEventCalled, "OnEvent must not run for unknown type")
	assert.True(t, onUnknownCalled, "OnUnknownType must run")
}

func TestWebhookHandler_keyRotation(t *testing.T) {
	pub := testServerPubKey(t)
	prov := &fakeKeyProvider{key: &ServerKey{ID: "stale", PubKey: pub}}

	h, err := NewWebhookHandler(context.Background(), WebhookHandlerOptions{
		Keys:    prov,
		OnEvent: func(context.Context, *WebHookResponse) error { return nil },
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), prov.calls.Load())
	assert.Equal(t, "stale", h.KeyID())

	// Mono now reports a different key id; provider returns the up-to-date one.
	prov.key = &ServerKey{ID: testServerKeyID, PubKey: pub}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedPOST(testWebhookBody, testWebhookSign, testServerKeyID))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, int32(2), prov.calls.Load(), "handler must re-fetch on X-Key-Id mismatch")
	assert.Equal(t, testServerKeyID, h.KeyID())
}

func TestWebhookHandler_endToEnd_overHTTP(t *testing.T) {
	var got *WebHookResponse
	h, _ := newTestHandler(t, WebhookHandlerOptions{
		OnEvent: func(_ context.Context, e *WebHookResponse) error {
			got = e
			return nil
		},
	})
	server := httptest.NewServer(h)
	defer server.Close()

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/webhook", strings.NewReader(testWebhookBody))
	req.Header.Set("X-Sign", testWebhookSign)
	req.Header.Set("X-Key-Id", testServerKeyID)

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, got)
	assert.Equal(t, "ACAB-X504-7TP3-143T", got.Data.Transaction.ReceiptID)
}
