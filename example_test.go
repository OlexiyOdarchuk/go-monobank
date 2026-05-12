package monobank_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/vtopc/go-monobank"
)

func ExampleNewClient() {
	client := monobank.NewClient(nil)

	rates, err := client.Currency(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d currency pairs\n", len(rates))
}

func ExampleNew_withRetry() {
	// New + WithRetry — automatic backoff on 5xx and 429, honouring
	// Retry-After when mono sends it.
	client := monobank.New(
		monobank.WithRetry(5, 500*time.Millisecond, 30*time.Second),
	)
	_ = client
}

func ExampleClient_ServerKey() {
	client := monobank.NewClient(nil)

	sk, err := client.ServerKey(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("keyId=%s serverTime=%s\n", sk.ID, sk.ServerTime.UTC().Format(time.RFC3339))
}

func ExampleNewPersonalClient() {
	token := os.Getenv("MONO_TOKEN")
	client := monobank.NewPersonalClient(nil).
		WithAuth(monobank.NewPersonalAuthorizer(token))

	info, err := client.ClientInfo(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("hello, %s; %d accounts\n", info.Name, len(info.Accounts))
}

func ExampleNewWebhookHandler() {
	client := monobank.NewClient(nil)

	h, err := monobank.NewWebhookHandler(context.Background(), monobank.WebhookHandlerOptions{
		Keys: client,
		OnEvent: func(_ context.Context, e *monobank.WebHookResponse) error {
			t := e.Data.Transaction
			log.Printf("%s %d %q", e.Data.AccountID, t.Amount, t.Description)
			return nil
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	http.Handle("/webhook", h)
}

func ExampleCurrencyCode() {
	c := monobank.CurrencyCode(840)
	fmt.Println(c)
	fmt.Println(c == monobank.USD)
	// Output:
	// USD
	// true
}
