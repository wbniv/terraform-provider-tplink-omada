package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSessionRedirectPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, location, version, path string
		status                        int
		duplicate, want               bool
	}{
		{"root relative", "/controller/login", "6.1.0.19", "/controller/api/v2/sites", 302, false, true},
		{"absolute", "SELF/controller/login", "6.1.0.19", "/controller/api/v2/sites", 302, false, true},
		{"unknown version", "/controller/login", "6.1.0.20", "/controller/api/v2/sites", 302, false, false},
		{"no version", "/controller/login", "", "/controller/api/v2/sites", 302, false, false},
		{"wrong controller", "/other/login", "6.1.0.19", "/controller/api/v2/sites", 302, false, false},
		{"other page", "/controller/dashboard", "6.1.0.19", "/controller/api/v2/sites", 302, false, false},
		{"foreign origin", "https://example.invalid/controller/login", "6.1.0.19", "/controller/api/v2/sites", 302, false, false},
		{"scheme relative foreign", "//example.invalid/controller/login", "6.1.0.19", "/controller/api/v2/sites", 302, false, false},
		{"query", "/controller/login?token=secret", "6.1.0.19", "/controller/api/v2/sites", 302, false, false},
		{"empty query", "/controller/login?", "6.1.0.19", "/controller/api/v2/sites", 302, false, false},
		{"fragment", "/controller/login#secret", "6.1.0.19", "/controller/api/v2/sites", 302, false, false},
		{"empty fragment", "/controller/login#", "6.1.0.19", "/controller/api/v2/sites", 302, false, false},
		{"userinfo", "http://secret@HOST/controller/login", "6.1.0.19", "/controller/api/v2/sites", 302, false, false},
		{"encoded path", "/controller/%6cogin", "6.1.0.19", "/controller/api/v2/sites", 302, false, false},
		{"duplicate location", "/controller/login", "6.1.0.19", "/controller/api/v2/sites", 302, true, false},
		{"empty location", "", "6.1.0.19", "/controller/api/v2/sites", 302, false, false},
		{"malformed location", "%ZZ", "6.1.0.19", "/controller/api/v2/sites", 302, false, false},
		{"301", "/controller/login", "6.1.0.19", "/controller/api/v2/sites", 301, false, false},
		{"307", "/controller/login", "6.1.0.19", "/controller/api/v2/sites", 307, false, false},
		{"308", "/controller/login", "6.1.0.19", "/controller/api/v2/sites", 308, false, false},
		{"discovery", "/controller/login", "6.1.0.19", "/api/info", 302, false, false},
		{"login", "/controller/login", "6.1.0.19", "/controller/api/v2/login", 302, false, false},
		{"logout", "/controller/login", "6.1.0.19", "/controller/api/v2/logout", 302, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logins, requests, followed atomic.Int32
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/controller/login" {
					followed.Add(1)
					w.WriteHeader(500)
					return
				}
				if r.URL.Path == "/controller/api/v2/login" && r.Method == http.MethodPost {
					logins.Add(1)
					fmt.Fprint(w, `{"errorCode":0,"result":{"token":"fresh"}}`)
					return
				}
				requests.Add(1)
				if r.Header.Get("Csrf-Token") == "fresh" {
					fmt.Fprint(w, `{"errorCode":0,"result":{}}`)
					return
				}
				location := strings.ReplaceAll(tc.location, "SELF", server.URL)
				location = strings.ReplaceAll(location, "HOST", strings.TrimPrefix(server.URL, "http://"))
				w.Header().Set("Location", location)
				if tc.duplicate {
					w.Header().Add("Location", location)
				}
				w.WriteHeader(tc.status)
			}))
			defer server.Close()
			c := &Client{baseURL: server.URL, omadacID: "controller", controllerVersion: tc.version, token: "stale", httpClient: server.Client()}
			_, err := c.doRequest(context.Background(), http.MethodGet, server.URL+tc.path, nil)
			expectedLogins, expectedRequests := int32(0), int32(1)
			if tc.want {
				expectedLogins, expectedRequests = 1, 2
			}
			if (err == nil) != tc.want || logins.Load() != expectedLogins || requests.Load() != expectedRequests || followed.Load() != 0 {
				t.Fatalf("err=%v logins=%d requests=%d followed=%d", err, logins.Load(), requests.Load(), followed.Load())
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("redirect diagnostic leaked a secret")
			}
		})
	}
}
