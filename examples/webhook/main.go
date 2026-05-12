// Command webhook receives signed monobank personal-API webhooks and prints
// a one-line summary of each transaction. It uses monobank.WebhookHandler
// which takes care of signature verification, key rotation and parsing.
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/vtopc/go-monobank"
)

func main() {
	client := monobank.NewClient(nil)

	h, err := monobank.NewWebhookHandler(context.Background(), monobank.WebhookHandlerOptions{
		Keys: client,
		OnEvent: func(_ context.Context, event *monobank.WebHookResponse) error {
			t := event.Data.Transaction
			log.Printf("account=%s amount=%d %q hold=%v",
				event.Data.AccountID, t.Amount, t.Description, t.Hold)
			return nil
		},
		OnError: func(err error) { log.Printf("webhook: %v", err) },
	})
	if err != nil {
		log.Fatal(err)
	}

	http.Handle("/webhook", h)
	log.Printf("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
