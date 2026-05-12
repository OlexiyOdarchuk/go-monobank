package monobank

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_commonClient_withAuth(t *testing.T) {
	c := newCommonClient(nil)

	auth := PersAuth{token: "123"}
	c.withAuth(auth)
	assert.Equal(t, auth, c.auth)
}

// TransactionsRange must page mono's 31-day-window endpoint until the full
// range is covered, concatenating results in chronological order.
func TestCommonClient_TransactionsRange(t *testing.T) {
	var calls atomic.Int32
	var seenWindows []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		// path: /personal/statement/{accountID}/{from}/{to}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		require.Len(t, parts, 5)
		from, _ := strconv.ParseInt(parts[3], 10, 64)
		seenWindows = append(seenWindows, parts[3]+"-"+parts[4])

		w.WriteHeader(http.StatusOK)
		body := `[{"id":"` + parts[3] + `","time":` + strconv.FormatInt(from, 10) + `,"amount":0}]`
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	cc := newCommonClient(server.Client())
	base, _ := url.Parse(server.URL)
	cc.baseURL = base
	cc.http = server.Client()

	from := time.Unix(1_700_000_000, 0)
	to := from.Add(70 * 24 * time.Hour) // 70 days → ⌈70/31⌉ = 3 windows

	got, err := cc.TransactionsRange(context.Background(), "acc-1", from, to)
	require.NoError(t, err)
	assert.Equal(t, int32(3), calls.Load(), "expected 3 windows for a 70-day range")
	assert.Len(t, got, 3)

	for i := 1; i < len(seenWindows); i++ {
		prevTo := strings.Split(seenWindows[i-1], "-")[1]
		curFrom := strings.Split(seenWindows[i], "-")[0]
		assert.Equal(t, prevTo, curFrom, "windows must be contiguous")
	}
	lastTo := strings.Split(seenWindows[len(seenWindows)-1], "-")[1]
	assert.Equal(t, strconv.FormatInt(to.Unix(), 10), lastTo)

	for i := 1; i < len(got); i++ {
		assert.Less(t, int64(got[i-1].Time.Unix()), int64(got[i].Time.Unix()))
	}
}

func TestCommonClient_TransactionsRange_zeroOrNegativeRange(t *testing.T) {
	cc := newCommonClient(nil)
	now := time.Now()

	got, err := cc.TransactionsRange(context.Background(), "x", now, time.Time{})
	require.NoError(t, err)
	assert.Nil(t, got, "zero `to` must return nil")

	got, err = cc.TransactionsRange(context.Background(), "x", now, now.Add(-time.Hour))
	require.NoError(t, err)
	assert.Nil(t, got, "`to` before `from` must return nil")
}

func TestCommonClient_TransactionsRange_singleWindow(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	cc := newCommonClient(server.Client())
	base, _ := url.Parse(server.URL)
	cc.baseURL = base
	cc.http = server.Client()

	from := time.Unix(1_700_000_000, 0)
	to := from.Add(7 * 24 * time.Hour) // 7 days < 31

	_, err := cc.TransactionsRange(context.Background(), "acc", from, to)
	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load())
}
