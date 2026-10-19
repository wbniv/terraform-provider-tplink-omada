package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCertificateOperations(t *testing.T) {
	for _, redirect := range []bool{false, true} {
		t.Run(fmt.Sprintf("redirect=%v", redirect), func(t *testing.T) {
			operations := []struct {
				name, path, method, want string
				call                     func(context.Context, *Client) (string, error)
			}{
				{"certificate", "/files/controller/certificate", http.MethodPost, "uploaded", func(ctx context.Context, c *Client) (string, error) {
					return c.UploadCertificate(ctx, []byte("test PEM\n"), "test.pem")
				}},
				{"key", "/files/controller/key", http.MethodPost, "uploaded", func(ctx context.Context, c *Client) (string, error) {
					return c.UploadKey(ctx, []byte("test PEM\n"), "test.pem")
				}},
				{"activate", "/controller/setting", http.MethodPatch, "", func(ctx context.Context, c *Client) (string, error) {
					return "", c.ActivateCertificate(ctx, "cert", "key", "cert.pem", "key.pem")
				}},
				{"read", "/controller/setting", http.MethodGet, "cert/key", func(ctx context.Context, c *Client) (string, error) {
					setting, err := c.GetControllerCertificateSetting(ctx)
					if err != nil {
						return "", err
					}
					return setting.CertID + "/" + setting.KeyID, nil
				}},
			}
			for _, op := range operations {
				for _, initialToken := range []string{"valid", "", "expired"} {
					t.Run(op.name+"/token="+initialToken, func(t *testing.T) {
						var logins, requests atomic.Int32
						server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							if strings.HasSuffix(r.URL.Path, "/login") {
								logins.Add(1)
								fmt.Fprint(w, `{"errorCode":0,"result":{"token":"fresh"}}`)
								return
							}
							requests.Add(1)
							if r.URL.Path != "/controller/api/v2"+op.path || r.Method != op.method {
								t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
							}
							// Certificate endpoints use header authentication, without a token query.
							if r.URL.Query().Has("token") {
								t.Error("added a token query to a header-authenticated certificate endpoint")
							}
							if op.method == http.MethodPost {
								file, header, err := r.FormFile("file")
								if err != nil {
									t.Error(err)
									return
								}
								defer file.Close()
								defer r.MultipartForm.RemoveAll()
								data, err := io.ReadAll(file)
								if err != nil || string(data) != "test PEM\n" || header.Filename != "test.pem" || r.FormValue("data") != map[string]string{"certificate": `{"cerName":"test.pem"}`, "key": `{"keyName":"test.pem"}`}[op.name] {
									t.Errorf("multipart file or metadata changed: filename=%q data=%q err=%v", header.Filename, data, err)
								}
							}
							if op.method == http.MethodPatch {
								data, err := io.ReadAll(r.Body)
								if err != nil || string(data) != `{"certificate":{"cerName":"cert.pem","keyName":"key.pem","enable":true,"cerType":"PEM","cerId":"cert","keyId":"key"}}` {
									t.Errorf("activation body = %s, err=%v", data, err)
								}
							}
							if r.Header.Get("Csrf-Token") == "expired" {
								writeExpiryResponse(w, redirect)
								return
							}
							expectedToken := "valid"
							if initialToken != "valid" {
								expectedToken = "fresh"
							}
							if got := r.Header.Get("Csrf-Token"); got != expectedToken {
								t.Errorf("token = %q, want %q", got, expectedToken)
							}
							if op.method == http.MethodPost {
								fmt.Fprint(w, `{"errorCode":0,"result":{"cerId":"uploaded","keyId":"uploaded"}}`)
							} else {
								fmt.Fprint(w, `{"errorCode":0,"result":{"certificate":{"cerId":"cert","keyId":"key"}}}`)
							}
						}))
						defer server.Close()
						c := &Client{controllerVersion: "6.1.0.19", baseURL: server.URL, omadacID: "controller", token: initialToken, httpClient: server.Client()}
						ctx, cancel := context.WithTimeout(context.Background(), time.Second)
						defer cancel()
						type result struct {
							value string
							err   error
						}
						done := make(chan result, 1)
						go func() {
							value, err := op.call(ctx, c)
							done <- result{value, err}
						}()
						select {
						case got := <-done:
							if got.err != nil || got.value != op.want {
								t.Fatalf("operation = %q, %v; want %q, nil", got.value, got.err, op.want)
							}
						case <-ctx.Done():
							t.Fatal("certificate operation did not complete before its deadline")
						}
						wantLogins, wantRequests := int32(0), int32(1)
						if initialToken != "valid" {
							wantLogins = 1
						}
						if initialToken == "expired" {
							wantRequests = 2
						}
						if logins.Load() != wantLogins || requests.Load() != wantRequests {
							t.Errorf("logins/requests = %d/%d, want %d/%d", logins.Load(), requests.Load(), wantLogins, wantRequests)
						}
					})
				}
			}

		})
	}
}

func TestConcurrentSessionExpiry(t *testing.T) {
	for _, redirect := range []bool{false, true} {
		t.Run(fmt.Sprintf("redirect=%v", redirect), func(t *testing.T) {
			const count = 12
			const freshToken = "fresh+token&with=punctuation"
			var oldRequests, logins atomic.Int32
			allArrived, firstRetry := make(chan struct{}), make(chan struct{})
			var retryOnce sync.Once
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/login") {
					logins.Add(1)
					fmt.Fprintf(w, `{"errorCode":0,"result":{"token":%q}}`, freshToken)
					return
				}
				if r.URL.Query().Get("currentPage") != "2" || r.URL.Query().Get("name") != "a&b" {
					t.Errorf("additional query parameters changed: %s", r.URL.RawQuery)
				}
				if r.Header.Get("Csrf-Token") == "expired" {
					ordinal := oldRequests.Add(1)
					if ordinal == count {
						close(allArrived)
					}
					select {
					case <-allArrived:
					case <-r.Context().Done():
						return
					}
					// Delay one stale response until a refreshed request has succeeded.
					if ordinal == count {
						select {
						case <-firstRetry:
						case <-r.Context().Done():
							return
						}
					}
					writeExpiryResponse(w, redirect)
					return
				}
				if r.Header.Get("Csrf-Token") != freshToken || r.URL.Query().Get("token") != freshToken {
					t.Errorf("header/query token mismatch: %q / %q", r.Header.Get("Csrf-Token"), r.URL.Query().Get("token"))
				}
				retryOnce.Do(func() { close(firstRetry) })
				fmt.Fprint(w, `{"errorCode":0,"result":{}}`)
			}))
			defer server.Close()
			c := &Client{controllerVersion: "6.1.0.19", baseURL: server.URL, omadacID: "controller", token: "expired", httpClient: server.Client()}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var wg sync.WaitGroup
			for i := 0; i < count; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, err := c.doSiteRequestWithParams(ctx, "site", http.MethodGet, "/probe", "&currentPage=2&name=a%26b", nil)
					if err != nil {
						t.Error(err)
					}
				}()
			}
			wg.Wait()
			if got := logins.Load(); got != 1 {
				t.Errorf("concurrent expiry caused %d logins, want 1", got)
			}

		})
	}
}

func TestConcurrentRequestsAndRefresh(t *testing.T) {
	var sequence atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/login") {
			fmt.Fprintf(w, `{"errorCode":0,"result":{"token":"token-%d"}}`, sequence.Add(1))
			return
		}
		if token := r.URL.Query().Get("token"); token == "" || token != r.Header.Get("Csrf-Token") {
			t.Errorf("query/header tokens disagree: %q / %q", token, r.Header.Get("Csrf-Token"))
		}
		fmt.Fprint(w, `{"errorCode":0,"result":{}}`)
	}))
	defer server.Close()
	c := &Client{baseURL: server.URL, omadacID: "controller", token: "initial", httpClient: server.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 30; i++ {
			if err := c.reAuth(ctx); err != nil {
				t.Error(err)
			}
		}
	}()
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				_, err := c.doRequest(ctx, http.MethodGet, c.globalURL("/probe"), nil)
				if err != nil {
					t.Error(err)
				}
				if err := c.UpdateSite(ctx, "site", &SiteSettingFields{Name: "test"}); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
}

func TestAuthenticationCancellation(t *testing.T) {
	for _, cancelLeader := range []bool{true, false} {
		t.Run(fmt.Sprintf("cancelLeader=%t", cancelLeader), func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			var startOnce, releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				startOnce.Do(func() { close(started) })
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				fmt.Fprint(w, `{"errorCode":0,"result":{"token":"fresh"}}`)
			}))
			defer server.Close()
			defer unblock()
			c := &Client{baseURL: server.URL, omadacID: "controller", httpClient: server.Client()}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			leaderCtx, cancelLogin := context.WithCancel(ctx)
			defer cancelLogin()
			leaderDone := make(chan error, 1)
			go func() { leaderDone <- c.ensureAuth(leaderCtx) }()
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("login did not start")
			}
			var done <-chan error = leaderDone
			wantError := context.Canceled
			if cancelLeader {
				cancelLogin()
			} else {
				waiterCtx, cancelWaiter := context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancelWaiter()
				waiterDone := make(chan error, 1)
				go func() { waiterDone <- c.ensureAuth(waiterCtx) }()
				done = waiterDone
				wantError = context.DeadlineExceeded
			}
			select {
			case err := <-done:
				if !errors.Is(err, wantError) {
					t.Errorf("canceled authentication returned %v", err)
				}
			case <-time.After(time.Second):
				t.Error("cancellation blocked behind an active login")
			}
			unblock()
			if !cancelLeader {
				select {
				case err := <-leaderDone:
					if err != nil {
						t.Errorf("canceling a waiter interrupted the login: %v", err)
					}
				case <-ctx.Done():
					t.Fatal("login did not finish after releasing the server")
				}
			}
			if err := c.ensureAuth(ctx); err != nil {
				t.Errorf("authentication did not recover after cancellation: %v", err)
			}
		})
	}
}

func TestSessionRetryFailures(t *testing.T) {
	for _, redirect := range []bool{false, true} {
		t.Run(fmt.Sprintf("redirect=%v", redirect), func(t *testing.T) {
			for _, tc := range []struct {
				name, loginResponse string
				wantRequests        int32
			}{
				{"login denied", `{"errorCode":-30109,"msg":"denied"}`, 1},
				{"empty token", `{"errorCode":0,"result":{"token":""}}`, 1},
				{"expired again", `{"errorCode":0,"result":{"token":"fresh"}}`, 2},
			} {
				t.Run(tc.name, func(t *testing.T) {
					var logins, requests atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if strings.HasSuffix(r.URL.Path, "/login") {
							logins.Add(1)
							fmt.Fprint(w, tc.loginResponse)
							return
						}
						requests.Add(1)
						writeExpiryResponse(w, redirect)
					}))
					defer server.Close()
					c := &Client{controllerVersion: "6.1.0.19", baseURL: server.URL, omadacID: "controller", token: "expired", httpClient: server.Client()}
					_, err := c.doSiteRequest(context.Background(), "site", http.MethodGet, "/probe", nil)
					if err == nil || logins.Load() != 1 || requests.Load() != tc.wantRequests {
						t.Errorf("err=%v logins=%d requests=%d; want error, 1 login, %d requests", err, logins.Load(), requests.Load(), tc.wantRequests)
					}
				})
			}

		})
	}
}

func TestCertificateWritesReadOnly(t *testing.T) {
	c := &Client{readOnly: true}
	if _, err := c.UploadCertificate(context.Background(), []byte("pem"), "cert.pem"); !errors.Is(err, ErrReadOnly) {
		t.Errorf("certificate upload: %v", err)
	}
	if _, err := c.UploadKey(context.Background(), []byte("pem"), "key.pem"); !errors.Is(err, ErrReadOnly) {
		t.Errorf("key upload: %v", err)
	}
	if err := c.ActivateCertificate(context.Background(), "cert", "key", "cert.pem", "key.pem"); !errors.Is(err, ErrReadOnly) {
		t.Errorf("activation: %v", err)
	}
}

func TestUploadIdentityValidation(t *testing.T) {
	for _, kind := range []string{"certificate", "key"} {
		wrongField := `{"keyId":"wrong-endpoint"}`
		if kind == "key" {
			wrongField = `{"cerId":"wrong-endpoint"}`
		}
		for _, result := range []string{wrongField, `{}`, `null`, `{"id":"generic"}`, `{"cerId":"","keyId":""}`, `{"cerId":" ","keyId":" "}`, `{"cerId":7,"keyId":7}`} {
			t.Run(kind+result, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprintf(w, `{"errorCode":0,"result":%s}`, result)
				}))
				defer server.Close()
				c := &Client{baseURL: server.URL, omadacID: "controller", token: "valid", httpClient: server.Client()}
				id, err := c.uploadControllerFile(context.Background(), kind, []byte("pem"), "file.pem")
				if err == nil || id != "" {
					t.Fatalf("invalid upload identity accepted: %q, %v", id, err)
				}
			})
		}
	}
}

func TestCertificateReadMissingSection(t *testing.T) {
	for _, result := range []string{`{}`, `null`, `{"certificate":null}`, `{"cerId":"flat","keyId":"flat"}`} {
		t.Run(result, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{"errorCode":0,"result":%s}`, result)
			}))
			defer server.Close()
			c := &Client{baseURL: server.URL, omadacID: "controller", token: "valid", httpClient: server.Client()}
			if _, err := c.GetControllerCertificateSetting(context.Background()); err == nil {
				t.Fatal("missing certificate section accepted")
			}
		})
	}
}

func TestCertificateActivationRequiresIDs(t *testing.T) {
	c := &Client{}
	for _, ids := range [][2]string{{"", "key"}, {"cert", ""}, {" ", "key"}} {
		if err := c.ActivateCertificate(context.Background(), ids[0], ids[1], "cert.pem", "key.pem"); err == nil {
			t.Fatal("activation accepted empty identity")
		}
	}
}

func writeExpiryResponse(w http.ResponseWriter, redirect bool) {
	if redirect {
		w.Header().Set("Location", "/controller/login")
		w.WriteHeader(http.StatusFound)
		return
	}
	fmt.Fprint(w, `{"errorCode":-1,"msg":"expired"}`)
}
