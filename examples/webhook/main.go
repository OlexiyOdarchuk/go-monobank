// Command webhook is a runnable example of receiving and authenticating
// personal-API webhooks from monobank.
//
// Subscribe a public URL via PersonalClient.SetWebHook and every statement
// event will arrive here as POST /webhook with the body, X-Sign and X-Key-Id
// headers. We fetch the bank's signing key once at start-up and refresh it
// whenever mono rotates it (incoming X-Key-Id no longer matches).
package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/vtopc/go-monobank"
)

type keyCache struct {
	mu     sync.RWMutex
	client monobank.Client
	key    *monobank.ServerKey
}

func (c *keyCache) refresh(ctx context.Context) error {
	sk, err := c.client.ServerKey(ctx)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.key = sk
	c.mu.Unlock()
	return nil
}

func (c *keyCache) get() *monobank.ServerKey {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.key
}

func main() {
	keys := &keyCache{client: monobank.NewClient(nil)}
	if err := keys.refresh(context.Background()); err != nil {
		log.Fatalf("initial ServerKey: %v", err)
	}

	http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// Mono pings the URL with GET when you subscribe a webhook.
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}

		sk := keys.get()
		if r.Header.Get("X-Key-Id") != sk.ID {
			if err := keys.refresh(r.Context()); err != nil {
				log.Printf("refresh ServerKey: %v", err)
			}
			sk = keys.get()
		}
		if err := sk.Verify(body, r.Header.Get("X-Sign")); err != nil {
			http.Error(w, "bad signature", http.StatusUnauthorized)
			return
		}

		event, err := monobank.ParseWebHook(body)
		if err != nil {
			log.Printf("parse webhook: %v", err)
			return // already authenticated, just unknown
		}
		t := event.Data.Transaction
		log.Printf("account=%s amount=%d %q hold=%v",
			event.Data.AccountID, t.Amount, t.Description, t.Hold)
	})

	log.Printf("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
