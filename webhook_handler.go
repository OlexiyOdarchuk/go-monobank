package monobank

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// KeyProvider is anything that can fetch the bank's current signing key.
// [Client] satisfies it; tests can swap in a fake.
type KeyProvider interface {
	ServerKey(ctx context.Context) (*ServerKey, error)
}

// WebhookHandlerOptions configures [NewWebhookHandler].
type WebhookHandlerOptions struct {
	// Keys is required. The handler calls Keys.ServerKey on start-up to
	// load the bank's public key, and again whenever an incoming
	// X-Key-Id stops matching the cached one (i.e. mono rotated).
	Keys KeyProvider

	// OnEvent is required. It receives the verified, parsed webhook
	// payload. Returning a non-nil error makes the handler respond with
	// HTTP 500 so mono retries the delivery; returning nil acks with 200.
	OnEvent func(ctx context.Context, event *WebHookResponse) error

	// OnUnknownType is optional. When set, it receives payloads that pass
	// signature verification but whose top-level "type" is not a known
	// WebHookType* constant. If left nil the handler silently ACKs (200);
	// payloads are authentic, just unfamiliar.
	OnUnknownType func(ctx context.Context, raw []byte)

	// OnError is optional. It is invoked for internal failures the
	// handler can't surface through HTTP (e.g. a key refresh that fails
	// while still trying to verify a request). Use it for logging.
	OnError func(err error)
}

// WebhookHandler is a ready-to-mount http.Handler that receives signed
// webhooks from monobank. Behaviour:
//
//   - GET /your-webhook-path — returns 200 (mono pings the URL with GET
//     when you subscribe a webhook to confirm it's alive).
//   - POST — reads the body, verifies X-Sign against the cached server key
//     (re-fetching if X-Key-Id changed), decodes the payload and calls
//     OnEvent. Bad signature → 401; OnEvent error → 500; otherwise 200.
//
// Mount it on any router (net/http, gin via http.HandlerFunc, chi, …):
//
//	h, err := monobank.NewWebhookHandler(monobank.WebhookHandlerOptions{
//	    Keys:    client,
//	    OnEvent: func(ctx context.Context, e *monobank.WebHookResponse) error {
//	        log.Printf("%+v", e.Data.Transaction)
//	        return nil
//	    },
//	})
//	if err != nil { log.Fatal(err) }
//	http.Handle("/webhook", h)
type WebhookHandler struct {
	opts WebhookHandlerOptions

	mu  sync.RWMutex
	key *ServerKey
}

// Webhook-handler configuration errors.
var (
	ErrNilKeyProvider = errors.New("WebhookHandlerOptions.Keys is required")
	ErrNilOnEvent     = errors.New("WebhookHandlerOptions.OnEvent is required")
)

// NewWebhookHandler validates opts and primes the key cache. Use a
// non-cancellable context here unless start-up time is bounded by the
// caller — the handler is unusable without an initial key.
func NewWebhookHandler(ctx context.Context, opts WebhookHandlerOptions) (*WebhookHandler, error) {
	if opts.Keys == nil {
		return nil, ErrNilKeyProvider
	}
	if opts.OnEvent == nil {
		return nil, ErrNilOnEvent
	}
	h := &WebhookHandler{opts: opts}
	if err := h.refresh(ctx); err != nil {
		return nil, fmt.Errorf("initial ServerKey fetch: %w", err)
	}
	return h, nil
}

// KeyID returns the cached server-key identifier. Mainly for diagnostics.
func (h *WebhookHandler) KeyID() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.key == nil {
		return ""
	}
	return h.key.ID
}

func (h *WebhookHandler) refresh(ctx context.Context) error {
	sk, err := h.opts.Keys.ServerKey(ctx)
	if err != nil {
		return err
	}
	h.mu.Lock()
	h.key = sk
	h.mu.Unlock()
	return nil
}

func (h *WebhookHandler) currentKey() *ServerKey {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.key
}

// ServeHTTP implements http.Handler.
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Mono pings the URL with GET to verify the subscription.
		w.WriteHeader(http.StatusOK)
		return
	case http.MethodPost:
		// fall through
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}

	key := h.currentKey()
	if incomingKeyID := r.Header.Get("X-Key-Id"); incomingKeyID != "" && incomingKeyID != key.ID {
		if err := h.refresh(r.Context()); err != nil {
			h.reportError(fmt.Errorf("refresh ServerKey on X-Key-Id mismatch: %w", err))
		} else {
			key = h.currentKey()
		}
	}

	if err := key.Verify(body, r.Header.Get("X-Sign")); err != nil {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}

	event, err := ParseWebHook(body)
	if errors.Is(err, ErrUnknownWebHookType) {
		if h.opts.OnUnknownType != nil {
			h.opts.OnUnknownType(r.Context(), body)
		}
		w.WriteHeader(http.StatusOK)
		return
	}
	if err != nil {
		// Body is authenticated but unparseable — log and 400.
		h.reportError(fmt.Errorf("parse webhook: %w", err))
		http.Error(w, "malformed payload", http.StatusBadRequest)
		return
	}

	if err := h.opts.OnEvent(r.Context(), event); err != nil {
		h.reportError(fmt.Errorf("OnEvent: %w", err))
		// 5xx → mono will retry (after 60s and 600s).
		http.Error(w, "callback failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *WebhookHandler) reportError(err error) {
	if h.opts.OnError != nil {
		h.opts.OnError(err)
	}
}
