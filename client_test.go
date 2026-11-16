package airportdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		c, err := NewClient(nil, Config{Key: "k"})
		require.NoError(t, err)
		assert.Equal(t, DefaultURL, c.url)
		assert.Equal(t, DefaultTimeout, c.httpClient.Timeout)
		assert.Equal(t, "k", c.key())
	})

	t.Run("trailing slash trimmed", func(t *testing.T) {
		c, err := NewClient(nil, Config{URL: "https://example.test/api/", Key: "k"})
		require.NoError(t, err)
		assert.Equal(t, "https://example.test/api", c.url)
	})

	t.Run("invalid URL", func(t *testing.T) {
		_, err := NewClient(nil, Config{URL: "http://bad\x7f", Key: "k"})
		require.Error(t, err)
	})

	t.Run("own HTTP client", func(t *testing.T) {
		hc := &http.Client{}
		c, err := NewClient(nil, Config{Key: "k", HTTPClient: hc})
		require.NoError(t, err)
		assert.Same(t, hc, c.httpClient)
	})
}

func TestGetAirport(t *testing.T) {
	tests := []struct {
		name    string
		icao    string
		status  int
		body    string
		wantErr error  // matched with errors.Is
		errText string // a substring of the error, when wantErr is nil
		want    string // the airport's name on success
	}{
		{name: "found", icao: "YBBN", status: 200, body: `{"ident":"YBBN","icao_code":"YBBN","name":"Brisbane"}`, want: "Brisbane"},
		{name: "404", icao: "ZZZZ", status: 404, body: `{}`, wantErr: ErrNotFound},
		{name: "empty record", icao: "YSWS", status: 200, body: `{"ident":"","name":""}`, wantErr: ErrNotFound},
		{name: "null", icao: "YSWS", status: 200, body: `null`, wantErr: ErrNotFound},
		{
			// airportdb.io's rate limit: a 429 whose JSON body shares no
			// fields with Airport.
			name: "rate limited", icao: "YBBN", status: 429,
			body:    `{"statusCode":429,"error":"Too many requests","message":"Rate limit exceeded"}`,
			errText: "status 429",
		},
		{name: "server error", icao: "YBBN", status: 500, body: "server error", errText: "status 500: server error"},
		{name: "invalid JSON", icao: "YBBN", status: 200, body: "not json", errText: "unable to parse response"},
		{name: "no code", icao: "  ", errText: "ICAO code is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/"+tt.icao, r.URL.Path)
				assert.Equal(t, "secret", r.URL.Query().Get(apiTokenKey))
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			c, err := NewClient(nil, Config{URL: srv.URL + "/api", Key: "secret"})
			require.NoError(t, err)

			airport, err := c.GetAirport(context.Background(), tt.icao)

			switch {
			case tt.wantErr != nil:
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, airport)
			case tt.errText != "":
				require.ErrorContains(t, err, tt.errText)
				assert.Nil(t, airport)
			default:
				require.NoError(t, err)
				assert.Equal(t, tt.want, airport.Name)
			}
		})
	}
}

func TestGetAirportEscapesCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/a%2Fb", r.URL.RawPath)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c, err := NewClient(nil, Config{URL: srv.URL, Key: "k"})
	require.NoError(t, err)

	_, err = c.GetAirport(context.Background(), "a/b")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestKeyFuncReadPerRequest(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Query().Get(apiTokenKey))
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	var n atomic.Int32
	keys := []string{"first", "second"}
	c, err := NewClient(nil, Config{
		URL:     srv.URL,
		Key:     "ignored",
		KeyFunc: func() string { return keys[n.Add(1)-1] },
	})
	require.NoError(t, err)

	_, _ = c.GetAirport(context.Background(), "YBBN")
	_, _ = c.GetAirport(context.Background(), "YBBN")

	assert.Equal(t, []string{"first", "second"}, seen)
}

func TestGetAirportNoKey(t *testing.T) {
	c, err := NewClient(nil, Config{URL: "http://unused.test", KeyFunc: func() string { return "" }})
	require.NoError(t, err)

	_, err = c.GetAirport(context.Background(), "YBBN")
	require.ErrorContains(t, err, "no API key set")
}

func TestGetAirportCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	c, err := NewClient(nil, Config{URL: srv.URL, Key: "k"})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = c.GetAirport(ctx, "YBBN")
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestQuoteTruncates(t *testing.T) {
	long := make([]byte, maxErrorBody+100)
	for i := range long {
		long[i] = 'x'
	}
	q := quote(long)
	assert.Len(t, q, maxErrorBody+3)
	assert.Equal(t, "...", q[len(q)-3:])
}
