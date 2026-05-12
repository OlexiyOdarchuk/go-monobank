package monobank

import (
	"context"
	"net/http"
	"time"
)

type PersonalAPI interface {
	CommonAPI

	// ClientInfo - https://api.monobank.ua/docs/#/definitions/UserInfo
	ClientInfo(context.Context) (*ClientInfo, error)

	// Transactions - gets bank account statements
	// https://api.monobank.ua/docs/#/definitions/StatementItems
	Transactions(ctx context.Context, accountID string, from, to time.Time) (Transactions, error)
}

type PersonalClient struct {
	commonClient
}

func NewPersonalClient(client *http.Client) PersonalClient {
	return PersonalClient{
		commonClient: newCommonClient(client),
	}
}

// NewPersonal returns a PersonalClient built from the supplied options.
// Auth is set up via the WithAuth method on the returned value.
//
//	c := monobank.NewPersonal(monobank.WithRetry(5, 0, 0)).
//	    WithAuth(monobank.NewPersonalAuthorizer(token))
func NewPersonal(opts ...Option) PersonalClient {
	return PersonalClient{commonClient: commonClient{Client: New(opts...)}}
}

// WithAuth returns copy of PersonalClient with authorizer
func (c PersonalClient) WithAuth(auth Authorizer) PersonalClient {
	c.withAuth(auth)

	return c
}

func (c PersonalClient) SetWebHook(ctx context.Context, uri string) error {
	const urlPath = "/personal/webhook"

	return c.setWebHook(ctx, uri, urlPath)
}
