package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const responseSecret = "Synthetic-Response-Secret-987!"
const responseSuccess = `{"errorCode":0,"result":{"omadacId":"controller","token":"fresh","id":"uploaded","cerId":"uploaded"}}`

var responseOperations = []struct {
	name string
	call func(context.Context, *Client) error
}{
	{"delete", func(ctx context.Context, c *Client) error { return c.DeleteNetwork(ctx, "site", "network") }},
	{"patch", func(ctx context.Context, c *Client) error {
		_, err := c.doSiteRequest(ctx, "site", http.MethodPatch, "/setting", map[string]string{"password": responseSecret})
		return err
	}},
	{"discovery", func(ctx context.Context, c *Client) error { return c.getControllerInfo(ctx) }},
	{"login", func(ctx context.Context, c *Client) error { _, err := c.login(ctx); return err }},
	{"multipart", func(ctx context.Context, c *Client) error {
		_, err := c.UploadCertificate(ctx, []byte(responseSecret), "fixture.pem")
		return err
	}},
}

func responseClient(server *httptest.Server) *Client {
	return &Client{baseURL: server.URL, omadacID: "controller", token: responseSecret, username: "fixture", password: responseSecret, httpClient: server.Client()}
}

// This test file also compiles on the unmodified E baseline.
func TestResponseValidation(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		ok     bool
	}{
		{"success", 200, responseSuccess, true},
		{"created", 201, responseSuccess, true},
		{"forbidden JSON", 403, `{"message":"` + responseSecret + `"}`, false},
		{"forbidden success envelope", 403, responseSuccess, false},
		{"unauthorized success envelope", 401, responseSuccess, false},
		{"missing route", 404, responseSuccess, false},
		{"rate limited", 429, responseSuccess, false},
		{"server JSON", 500, responseSuccess, false},
		{"proxy HTML", 502, "<html>" + responseSecret + "</html>", false},
		{"proxy text", 503, responseSecret, false},
		{"missing code", 200, `{"result":{"omadacId":"controller","token":"fresh","id":"uploaded","cerId":"uploaded"}}`, false},
		{"null code", 200, `{"errorCode":null,"result":{"omadacId":"controller","token":"fresh","id":"uploaded","cerId":"uploaded"}}`, false},
		{"string code", 200, `{"errorCode":"0","msg":"` + responseSecret + `"}`, false},
		{"fractional code", 200, `{"errorCode":0.5}`, false},
		{"bool code", 200, `{"errorCode":false}`, false},
		{"duplicate code", 200, `{"errorCode":-1001,"errorCode":0,"result":{"omadacId":"controller","token":"fresh","id":"uploaded","cerId":"uploaded"}}`, false},
		{"duplicate result", 200, `{"errorCode":0,"result":null,"result":{"omadacId":"controller","token":"fresh","id":"uploaded","cerId":"uploaded"}}`, false},
		{"wrong case code", 200, `{"ErrorCode":0,"result":{"omadacId":"controller","token":"fresh","id":"uploaded","cerId":"uploaded"}}`, false},
		{"null envelope", 200, "null", false},
		{"array envelope", 200, "[]", false},
		{"empty object", 200, "{}", false},
		{"empty body", 200, "", false},
		{"empty 204", 204, "", false},
		{"whitespace", 200, " \n\t", false},
		{"truncated", 200, `{"msg":"` + responseSecret, false},
		{"trailing JSON", 200, responseSuccess + ` {"msg":"` + responseSecret + `"}`, false},
		{"trailing text", 200, responseSuccess + responseSecret, false},
		{"invalid message", 200, `{"errorCode":0,"msg":{"password":"` + responseSecret + `"}}`, false},
		{"API failure", 200, `{"errorCode":-1001,"msg":"` + responseSecret + `"}`, false},
		{"large API message", 200, `{"errorCode":-1001,"msg":"` + strings.Repeat(responseSecret, 200) + `"}`, false},
	}
	for _, op := range responseOperations {
		t.Run(op.name, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.WriteHeader(tc.status)
						fmt.Fprint(w, tc.body)
					}))
					defer server.Close()
					err := op.call(context.Background(), responseClient(server))
					if (err == nil) != tc.ok {
						t.Errorf("success=%v, want %v", err == nil, tc.ok)
					}
					if err != nil && (strings.Contains(err.Error(), responseSecret) || len(err.Error()) > 512) {
						t.Error("diagnostic contains response secret or exceeds 512 bytes")
					}
				})
			}
		})
	}
}

func TestResponseEmptyResult(t *testing.T) {
	for _, body := range []string{`{"errorCode":0}`, `{"errorCode":0,"result":null}`, `{"errorCode":0,"result":{}}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			c := responseClient(server)
			if err := c.DeleteNetwork(context.Background(), "site", "network"); err != nil {
				t.Fatal(err)
			}
			if _, err := c.doSiteRequest(context.Background(), "site", http.MethodPatch, "/setting", nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestResponseRedirect(t *testing.T) {
	for _, op := range responseOperations {
		for _, status := range []int{301, 302, 303, 307, 308} {
			t.Run(fmt.Sprintf("%s/%d", op.name, status), func(t *testing.T) {
				var forwarded atomic.Int32
				target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { forwarded.Add(1); fmt.Fprint(w, responseSuccess) }))
				defer target.Close()
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Redirect(w, r, target.URL+"/?token="+responseSecret, status)
				}))
				defer server.Close()
				if err := op.call(context.Background(), responseClient(server)); err == nil {
					t.Error("redirect accepted as success")
				}
				if forwarded.Load() != 0 {
					t.Error("redirect forwarded an API request")
				}
			})
		}
	}
}

type responseRoundTripper func(*http.Request) (*http.Response, error)

func (f responseRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type responseBrokenBody struct {
	cause  error
	closed bool
}

func (b *responseBrokenBody) Read([]byte) (int, error) { return 0, b.cause }
func (b *responseBrokenBody) Close() error             { b.closed = true; return nil }

func TestResponseTransport(t *testing.T) {
	for _, op := range responseOperations {
		for _, mode := range []string{"transport", "read", "cancel", "deadline", "invalid URL"} {
			t.Run(op.name+"/"+mode, func(t *testing.T) {
				cause := errors.New(responseSecret)
				if mode == "cancel" {
					cause = context.Canceled
				}
				if mode == "deadline" {
					cause = context.DeadlineExceeded
				}
				body := &responseBrokenBody{cause: cause}
				c := &Client{baseURL: "http://fixture.invalid", omadacID: "controller", token: responseSecret, password: responseSecret, httpClient: &http.Client{Transport: responseRoundTripper(func(*http.Request) (*http.Response, error) {
					if mode == "read" {
						return &http.Response{StatusCode: 200, Body: body, Header: make(http.Header)}, nil
					}
					return nil, cause
				})}}
				if mode == "invalid URL" {
					c.baseURL = "http://" + responseSecret + "/%zz"
				}
				err := op.call(context.Background(), c)
				if err == nil {
					t.Fatal("expected failure")
				}
				if strings.Contains(err.Error(), responseSecret) || strings.Contains(err.Error(), "http://") {
					t.Error("diagnostic exposes transport detail or URL")
				}
				if mode != "invalid URL" && !errors.Is(err, cause) {
					t.Error("transport cause no longer unwraps")
				}
				if mode == "read" && !body.closed {
					t.Error("response body not closed")
				}
			})
		}
	}
}

func TestResponseSessionRetry(t *testing.T) {
	for _, tc := range []struct {
		name             string
		status           int
		body             string
		repeat           bool
		requests, logins int32
		ok               bool
	}{
		{"valid expiry", 200, `{"errorCode":-1}`, false, 2, 1, true},
		{"repeated expiry", 200, `{"errorCode":-1}`, true, 2, 1, false},
		{"HTTP unauthorized", 401, `{"errorCode":-1}`, false, 1, 0, false},
		{"HTTP forbidden", 403, `{"errorCode":-1}`, false, 1, 0, false},
		{"HTTP server error", 500, `{"errorCode":-1}`, false, 1, 0, false},
		{"malformed expiry", 200, `{"errorCode":-1,"errorCode":0}`, false, 1, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests, logins atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/login") {
					logins.Add(1)
					fmt.Fprint(w, responseSuccess)
					return
				}
				n := requests.Add(1)
				if n == 1 || tc.repeat {
					w.WriteHeader(tc.status)
					fmt.Fprint(w, tc.body)
					return
				}
				if r.Header.Get("Csrf-Token") != "fresh" || r.URL.Query().Get("token") != "fresh" {
					t.Error("refreshed tokens differ")
				}
				fmt.Fprint(w, responseSuccess)
			}))
			defer server.Close()
			err := responseClient(server).DeleteNetwork(context.Background(), "site", "network")
			if (err == nil) != tc.ok || requests.Load() != tc.requests || logins.Load() != tc.logins {
				t.Errorf("success=%v requests/logins=%d/%d; want %v %d/%d", err == nil, requests.Load(), logins.Load(), tc.ok, tc.requests, tc.logins)
			}
		})
	}
}

func TestResponseSizeLimit(t *testing.T) {
	const limit = 16 << 20
	for _, size := range []int{limit, limit + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			body := responseSuccess + strings.Repeat(" ", size-len(responseSuccess))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, body) }))
			defer server.Close()
			err := responseClient(server).DeleteNetwork(context.Background(), "site", "network")
			if (err == nil) != (size == limit) {
				t.Errorf("success=%v for %d-byte response", err == nil, size)
			}
		})
	}
}

func TestResponseDiscoveryIdentity(t *testing.T) {
	for _, body := range []string{`{"errorCode":0,"result":{}}`, `{"errorCode":0,"result":null}`, `{"errorCode":0,"result":{"omadacId":""}}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			if err := responseClient(server).getControllerInfo(context.Background()); err == nil {
				t.Error("missing controller identity accepted")
			}
		})
	}
}
