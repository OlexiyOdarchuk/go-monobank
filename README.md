# go-monobank

[![Godoc Reference][godoc-img]][godoc-url] [![CI][ci-img]][ci-url] [![codecov][codecov-img]][codecov-url]

Monobank REST API client.

## Features
- Personal API(with Token authorization).
- API for providers(corporate) with authorization.
- Webhooks(including API for providers).
- Webhook signature verification (ECDSA secp256k1) via `Client.ServerKey` and `VerifyWebhookSignature`.
- Ready-to-mount `WebhookHandler` (`http.Handler`) with key rotation and optional deduplication.
- Functional options (`New`, `NewPersonal`, `NewCorporate`) with HTTP retry + `Retry-After`.
- `TransactionsRange` paginates `/personal/statement` across arbitrary date ranges (mono's 31-day window).
- `CurrencyCode` and `MCC` enums with helpers for ISO 4217 names and merchant categorisation.
- Jars(only in Personal API).

## Installation
```shell
go get github.com/vtopc/go-monobank@latest
```
This will update yours go.mod file.

## Usage examples
NOTE: Do not forget to check errors.

#### Public client
```go
package main

import (
    "context"
    "fmt"

    "github.com/vtopc/go-monobank"
)

func main() {
    // Create public client.
    client := monobank.NewClient(nil)

    response, _ := client.Currency(context.Background())
    fmt.Println(response)
}
```

#### Personal client
```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/vtopc/go-monobank"
)

func main() {
    token := os.Getenv("TOKEN")

    // Create authorized client.
    client := monobank.NewPersonalClient(nil).WithAuth(monobank.NewPersonalAuthorizer(token))

    response, _ := client.ClientInfo(context.Background())
    fmt.Println(response)
}
```

#### Corporate client
```go
package main

import (
    "context"
    "fmt"

    "github.com/vtopc/go-monobank"
)

var secKey []byte // put here you private key

const webhook = "https://example.com/webhook"

func main() {
    // Create auth creator.
    authMaker, _ := monobank.NewCorpAuthMaker(secKey)

    // Create authorized client.
    client, _ := monobank.NewCorporateClient(nil, authMaker)

    // If the user is not authorized yet, do next:
    resp, _ := client.Auth(context.Background(), webhook, monobank.PermSt, monobank.PermPI)

    // Send `resp.AcceptURL` to the user and wait until it will authorize your client
    // in Monobank app on mobile, you will get GET request on `webhook` when it will be done.
    // See Documentation for details.
    // Store `resp.RequestID` somewhere.
    requestID := resp.RequestID

    // If user authorized already:
    response, _ := client.ClientInfo(context.Background(), requestID)
    fmt.Println(response)
}
```

## Documentation
- [Official](https://api.monobank.ua/docs/)
- [Unofficial](https://gist.github.com/Sominemo/8714a82e26a268c30e4a332b0b2fd943)

## Similar projects
- https://github.com/shal/mono (last update 16.05.2020)
- https://github.com/artemrys/go-monobank-api (no corporate API)

#### Webhook handler
```go
client := monobank.NewClient(nil)
h, err := monobank.NewWebhookHandler(ctx, monobank.WebhookHandlerOptions{
    Keys:  client,
    Dedup: monobank.NewMemoryDeduper(1024),
    OnEvent: func(ctx context.Context, e *monobank.WebHookResponse) error {
        t := e.Data.Transaction
        log.Printf("%s %s: %d %s [%s]", e.Data.AccountID, t.ID,
            t.Amount, monobank.CurrencyCode(t.CurrencyCode), t.MCCCode().Category())
        return nil
    },
})
if err != nil { log.Fatal(err) }
http.Handle("/webhook", h)
```

The handler responds 200 to mono's GET ping, verifies `X-Sign` against the
cached `ServerKey` (re-fetching on `X-Key-Id` rotation), parses the body,
runs `OnEvent`, and ACKs duplicates via `Dedup`. Returning a non-nil error
from `OnEvent` yields HTTP 500 so mono retries.

## TODO
- More unit tests

[godoc-img]: https://godoc.org/github.com/vtopc/go-monobank?status.svg
[godoc-url]: https://godoc.org/github.com/vtopc/go-monobank

[ci-img]: https://github.com/vtopc/go-monobank/workflows/CI/badge.svg
[ci-url]: https://github.com/vtopc/go-monobank/actions?query=workflow%3A%22CI%22

[codecov-img]: https://codecov.io/gh/vtopc/go-monobank/branch/master/graph/badge.svg
[codecov-url]: https://codecov.io/gh/vtopc/go-monobank
