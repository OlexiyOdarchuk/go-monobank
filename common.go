package monobank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type CommonAPI interface {
	PublicAPI

	// SetWebHook - sets webhook for statements
	SetWebHook(ctx context.Context, uri string) error
}

// commonClient contains common to Personal and Corporate API
type commonClient struct {
	Client
}

func newCommonClient(client *http.Client) commonClient {
	return commonClient{
		Client: NewClient(client),
	}
}

func (c commonClient) ClientInfo(ctx context.Context) (*ClientInfo, error) {
	const urlPath = "/personal/client-info"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlPath, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	var v ClientInfo
	err = c.do(req, &v, http.StatusOK)

	return &v, err
}

// MaxStatementWindow is the longest range /personal/statement accepts in a
// single call. Use [commonClient.TransactionsRange] to span longer ranges.
const MaxStatementWindow = 31 * 24 * time.Hour

func (c commonClient) Transactions(ctx context.Context, accountID string, from, to time.Time) (
	Transactions, error) {

	const urlPath = "/personal/statement"
	uri := fmt.Sprintf("%s/%s/%d/%d", urlPath, accountID, from.Unix(), to.Unix())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	var v Transactions
	err = c.do(req, &v, http.StatusOK)

	return v, err
}

// TransactionsRange returns statements across an arbitrary date range by
// splitting it into successive 31-day windows (mono's single-call limit)
// and concatenating the results in chronological order.
//
// If to is zero or before from the call returns nil, nil.
func (c commonClient) TransactionsRange(ctx context.Context, accountID string, from, to time.Time) (
	Transactions, error) {

	if to.IsZero() || !to.After(from) {
		return nil, nil
	}

	var all Transactions
	for cursor := from; cursor.Before(to); {
		end := cursor.Add(MaxStatementWindow)
		if end.After(to) {
			end = to
		}
		chunk, err := c.Transactions(ctx, accountID, cursor, end)
		if err != nil {
			return nil, fmt.Errorf("range %s..%s: %w", cursor.Format(time.RFC3339), end.Format(time.RFC3339), err)
		}
		all = append(all, chunk...)
		cursor = end
	}
	return all, nil
}

func (c commonClient) setWebHook(ctx context.Context, uri, urlPath string) error {
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(WebHookRequest{WebHookURL: uri})
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlPath, &buf)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	return c.do(req, nil, http.StatusOK)
}
