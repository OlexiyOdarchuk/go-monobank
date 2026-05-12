package monobank

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests exercise http.ServeMux's Go 1.22 method+pattern routing
// against the client. Compared to a catch-all HandlerFunc + hasSuffix:
//
//   - a request to a wrong path/method gets 404 from the mux instead of
//     being silently accepted;
//   - URL segments are extracted by name (r.PathValue) rather than from a
//     formatted string.

func TestClient_Currency_servemuxPattern(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /bank/currency", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	base, _ := url.Parse(server.URL)
	c := Client{baseURL: base, http: server.Client(), auth: NewPublicAuthorizer()}

	out, err := c.Currency(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, out)
}

func TestCommonClient_Transactions_pathValues(t *testing.T) {
	var gotAccount, gotFrom, gotTo string

	mux := http.NewServeMux()
	mux.HandleFunc("GET /personal/statement/{accountID}/{from}/{to}",
		func(w http.ResponseWriter, r *http.Request) {
			gotAccount = r.PathValue("accountID")
			gotFrom = r.PathValue("from")
			gotTo = r.PathValue("to")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		})
	server := httptest.NewServer(mux)
	defer server.Close()

	cc := newCommonClient(server.Client())
	base, _ := url.Parse(server.URL)
	cc.baseURL = base
	cc.http = server.Client()

	from := time.Unix(1700000000, 0)
	to := time.Unix(1700001000, 0)
	_, err := cc.Transactions(context.Background(), "acc-123", from, to)
	require.NoError(t, err)

	assert.Equal(t, "acc-123", gotAccount)
	assert.Equal(t, strconv.FormatInt(from.Unix(), 10), gotFrom)
	assert.Equal(t, strconv.FormatInt(to.Unix(), 10), gotTo)
}
