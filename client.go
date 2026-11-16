// Package airportdb is a client for the AirportDB API (https://airportdb.io),
// which returns an airport's details, runways and navaids by ICAO code.
package airportdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	// DefaultURL is AirportDB's API base, used when Config.URL is empty.
	DefaultURL = "https://airportdb.io/api/v1/airport"

	// DefaultTimeout bounds a request when Config.HTTPClient is unset.
	DefaultTimeout = 10 * time.Second

	apiTokenKey = "apiToken"

	// maxErrorBody caps how much of an error response is quoted in the
	// error, so a large HTML error page doesn't end up in a log line.
	maxErrorBody = 512
)

// ErrNotFound is returned for an airport AirportDB doesn't have: a 404, or
// the empty record it sometimes answers with instead (YSWS, for example,
// is filed under another ident in its source data).
var ErrNotFound = errors.New("airportdb: airport not found")

// Config configures a Client. Only a key (Key or KeyFunc) is required. The
// mapstructure tags let a host decode it straight from viper or similar.
type Config struct {
	// URL is the API base. Empty means DefaultURL.
	URL string `json:"url" mapstructure:"url"`

	// Key is the API token.
	Key string `json:"key" mapstructure:"key"`

	// KeyFunc, when set, is called for every request in place of Key, so
	// a host can change the key (from a settings page, say) without
	// building a new client.
	KeyFunc func() string `json:"-" mapstructure:"-"`

	// HTTPClient sends the requests. Nil means a client with
	// DefaultTimeout.
	HTTPClient *http.Client `json:"-" mapstructure:"-"`
}

// Client is a client for the AirportDB API. It is safe for concurrent use.
type Client struct {
	logger     *zap.Logger
	httpClient *http.Client
	url        string
	key        func() string
}

// NewClient creates an AirportDB client. A nil logger logs nothing.
func NewClient(logger *zap.Logger, cfg Config) (*Client, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	base := strings.TrimRight(cfg.URL, "/")
	if base == "" {
		base = DefaultURL
	}
	if _, err := url.Parse(base); err != nil {
		return nil, fmt.Errorf("airportdb: invalid URL: %w", err)
	}

	key := cfg.KeyFunc
	if key == nil {
		static := cfg.Key
		key = func() string { return static }
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultTimeout}
	}

	return &Client{
		logger:     logger.Named("airportdb"),
		httpClient: httpClient,
		url:        base,
		key:        key,
	}, nil
}

// GetAirport returns the airport with the given ICAO code, or ErrNotFound
// when AirportDB doesn't have it.
func (c *Client) GetAirport(ctx context.Context, icao string) (*Airport, error) {
	icao = strings.TrimSpace(icao)
	if icao == "" {
		return nil, errors.New("airportdb: an ICAO code is required")
	}

	key := c.key()
	if key == "" {
		return nil, errors.New("airportdb: no API key set")
	}

	req, err := c.newRequest(ctx, icao, key)
	if err != nil {
		return nil, err
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("airportdb: request failed: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("airportdb: unable to read response: %w", err)
	}

	switch {
	case res.StatusCode == http.StatusNotFound:
		return nil, ErrNotFound
	case res.StatusCode != http.StatusOK:
		// A rate-limited request is a 429 with a JSON body that would
		// otherwise decode into an empty Airport, so the status is checked
		// before decoding.
		return nil, fmt.Errorf("airportdb: status %d: %s", res.StatusCode, quote(body))
	}

	var airport *Airport
	if err := json.Unmarshal(body, &airport); err != nil {
		c.logger.Debug("unable to parse response", zap.String("icao", icao), zap.ByteString("body", body))
		return nil, fmt.Errorf("airportdb: unable to parse response: %w", err)
	}
	if airport == nil || (airport.Name == "" && airport.ICAOCode == "") {
		return nil, ErrNotFound
	}

	return airport, nil
}

// newRequest builds the GET for an airport, with the key as the apiToken
// query parameter.
func (c *Client) newRequest(ctx context.Context, icao, key string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/"+url.PathEscape(icao), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("airportdb: unable to build request: %w", err)
	}

	query := req.URL.Query()
	query.Set(apiTokenKey, key)
	req.URL.RawQuery = query.Encode()

	return req, nil
}

// quote returns an error response body fit for an error message: trimmed,
// and cut short if it's long.
func quote(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > maxErrorBody {
		s = s[:maxErrorBody] + "..."
	}
	return s
}
