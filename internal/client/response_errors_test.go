package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponseErrorTypes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		kind   string
		agile  bool
	}{
		{"forbidden", 403, `{"errorCode":-39742}`, "http", false},
		{"not found status", 404, `{"errorCode":0}`, "http", false},
		{"server failure", 500, `{"errorCode":-1}`, "http", false},
		{"agile API code", 200, `{"errorCode":-39742}`, "api", true},
		{"other API code", 200, `{"errorCode":-1001,"msg":"-39742"}`, "api", false},
		{"protocol", 200, `{"msg":"-39742"}`, "protocol", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			resp, err := responseClient(server).doSiteRequest(context.Background(), "site", http.MethodDelete, "/probe", nil)
			if err == nil || resp != nil {
				t.Fatal("error response returned as success or usable envelope")
			}
			err = fmt.Errorf("wrapped: %w", err)
			var httpErr *HTTPError
			var apiErr *APIError
			var protocolErr *ProtocolError
			if errors.As(err, &httpErr) != (tc.kind == "http") || errors.As(err, &apiErr) != (tc.kind == "api") || errors.As(err, &protocolErr) != (tc.kind == "protocol") {
				t.Fatalf("wrong error type: %v", err)
			}
			if httpErr != nil && httpErr.StatusCode != tc.status {
				t.Fatal("lost HTTP status")
			}
			if isAgileSeriesError(err) != tc.agile {
				t.Fatal("incorrect Agile fallback classification")
			}
		})
	}
	t.Run("transport", func(t *testing.T) {
		cause := errors.New(responseSecret)
		c := &Client{baseURL: "http://fixture.invalid", omadacID: "controller", token: responseSecret, httpClient: &http.Client{Transport: responseRoundTripper(func(*http.Request) (*http.Response, error) { return nil, cause })}}
		err := c.DeleteNetwork(context.Background(), "site", "network")
		var transportErr *TransportError
		if !errors.As(err, &transportErr) || !errors.Is(err, cause) {
			t.Fatal("transport type/cause lost")
		}
		if strings.Contains(err.Error(), responseSecret) {
			t.Fatal("transport message exposed")
		}
	})
}

type responseCountingBody struct {
	remaining, read int
	closed          bool
}

func (b *responseCountingBody) Read(p []byte) (int, error) {
	n := len(p)
	if n > b.remaining {
		n = b.remaining
	}
	for i := 0; i < n; i++ {
		p[i] = ' '
	}
	b.remaining -= n
	b.read += n
	return n, nil
}
func (b *responseCountingBody) Close() error { b.closed = true; return nil }

func TestResponseReadBound(t *testing.T) {
	for _, status := range []int{200, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			body := &responseCountingBody{remaining: 32 << 20}
			c := &Client{baseURL: "http://fixture.invalid", omadacID: "controller", token: "token", httpClient: &http.Client{Transport: responseRoundTripper(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Body: body, Header: make(http.Header)}, nil
			})}}
			if err := c.DeleteNetwork(context.Background(), "site", "network"); err == nil {
				t.Fatal("expected failure")
			}
			want := maxAPIResponseBytes + 1
			if status == 500 {
				want = 0
			}
			if body.read != want || !body.closed {
				t.Errorf("read=%d closed=%v; want %d and true", body.read, body.closed, want)
			}
		})
	}
}
