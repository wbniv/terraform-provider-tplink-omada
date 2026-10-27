package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// Exercise public methods so a missed collection or raw lookup stays visible.
func TestPaginationCollections(t *testing.T) {
	for _, endpoint := range []struct {
		name, path, key string
		list            func(context.Context, *Client) (int, error)
	}{
		{"sites", "/sites", "id", func(ctx context.Context, c *Client) (int, error) { v, e := c.ListSites(ctx); return len(v), e }},
		{"networks", "/sites/site/setting/lan/networks", "id", func(ctx context.Context, c *Client) (int, error) {
			v, e := c.ListNetworks(ctx, "site")
			return len(v), e
		}},
		{"groups", "/sites/site/setting/wlans", "id", func(ctx context.Context, c *Client) (int, error) {
			v, e := c.ListWlanGroups(ctx, "site")
			return len(v), e
		}},
		{"SSIDs", "/sites/site/setting/wlans/group/ssids", "id", func(ctx context.Context, c *Client) (int, error) {
			v, e := c.ListWirelessNetworks(ctx, "site", "group")
			return len(v), e
		}},
		{"profiles", "/sites/site/setting/lan/profiles", "id", func(ctx context.Context, c *Client) (int, error) {
			v, e := c.ListPortProfiles(ctx, "site")
			return len(v), e
		}},
		{"ACLs", "/sites/site/setting/firewall/acls", "id", func(ctx context.Context, c *Client) (int, error) {
			v, e := c.ListACLRules(ctx, "site", 2)
			return len(v), e
		}},
		{"IP groups", "/sites/site/setting/profiles/groups/1", "groupId", func(ctx context.Context, c *Client) (int, error) {
			v, e := c.ListIPGroups(ctx, "site")
			return len(v), e
		}},
		{"mDNS", "/sites/site/setting/service/mdns", "id", func(ctx context.Context, c *Client) (int, error) {
			v, e := c.ListMDNSRules(ctx, "site")
			return len(v), e
		}},
		{"IdPs", "/idps", "idpId", func(ctx context.Context, c *Client) (int, error) { v, e := c.ListSAMLIdPs(ctx); return len(v), e }},
		{"roles", "/extendUserGroups", "id", func(ctx context.Context, c *Client) (int, error) { v, e := c.ListSAMLRoles(ctx); return len(v), e }},
	} {
		for _, mode := range []string{"201 rows", "capped pages", "no metadata", "direct array", "empty", "late failure"} {
			t.Run(endpoint.name+"/"+mode, func(t *testing.T) {
				calls := 0
				server := mockOmadaServer(t, map[string]http.HandlerFunc{endpoint.path: func(w http.ResponseWriter, r *http.Request) {
					calls++
					page, _ := strconv.Atoi(r.URL.Query().Get("currentPage"))
					if page != calls || r.URL.Query().Get("currentPageSize") != "100" {
						t.Errorf("incorrect page query: %v", r.URL.Query())
					}
					if endpoint.name == "ACLs" && r.URL.Query().Get("type") != "2" {
						t.Error("lost ACL filter")
					}
					if r.URL.Query().Get("token") != "test-csrf-token" || r.Header.Get("Csrf-Token") != "test-csrf-token" {
						t.Error("lost authentication")
					}
					if mode == "late failure" && page == 2 {
						w.WriteHeader(503)
						return
					}
					total, size := 201, 100
					if mode == "empty" {
						total = 0
					}
					if mode == "capped pages" || mode == "no metadata" {
						size = 73
					}
					if mode == "direct array" || ((endpoint.name == "groups" || endpoint.name == "IP groups") && mode == "no metadata") {
						size = total
					}
					rows := []map[string]any{}
					for i := (page - 1) * size; i < min(page*size, total); i++ {
						row := map[string]any{endpoint.key: fmt.Sprintf("row-%03d", i)}
						if endpoint.name == "IP groups" {
							row["type"] = 1
							row["portList"] = []string{"80"}
							row["ipList"] = []any{map[string]any{"ip": "192.0.2.1", "mask": 32}}
						}
						rows = append(rows, row)
					}
					var result any = map[string]any{"totalRows": total, "currentPage": page, "currentSize": size, "data": rows, "extraCapability": true}
					if mode == "direct array" {
						result = rows
					}
					if mode == "no metadata" {
						result = map[string]any{"data": rows}
					}
					json.NewEncoder(w).Encode(APIResponse{Result: mustMarshal(t, result)})
				}})
				defer server.Close()
				n, err := endpoint.list(context.Background(), newTestClient(t, server))
				if mode == "late failure" {
					if err == nil || n != 0 || !strings.Contains(err.Error(), "HTTP error 503") {
						t.Fatalf("partial success: n=%d err=%v", n, err)
					}
					return
				}
				want, requests := 201, 3
				if mode == "no metadata" {
					requests = 4
				}
				if mode == "empty" {
					want, requests = 0, 1
				}
				if mode == "direct array" || ((endpoint.name == "groups" || endpoint.name == "IP groups") && mode == "no metadata") {
					requests = 1
				}
				if err != nil || n != want || calls != requests {
					t.Fatalf("n=%d calls=%d err=%v; want %d rows, %d requests", n, calls, err, want, requests)
				}
			})
		}
	}
}

func TestPaginationInvalid(t *testing.T) {
	for _, tc := range []struct{ name, first, second string }{
		{"null result", `null`, ""},
		{"missing data", `{"totalRows":0}`, ""},
		{"null data", `{"totalRows":0,"data":null}`, ""},
		{"object data", `{"totalRows":0,"data":{}}`, ""},
		{"null total", `{"totalRows":null,"data":[]}`, ""},
		{"negative total", `{"totalRows":-1,"data":[]}`, ""},
		{"fractional total", `{"totalRows":1.5,"data":[]}`, ""},
		{"string total", `{"totalRows":"private-secret","data":[]}`, ""},
		{"empty before total", `{"totalRows":1,"data":[]}`, ""},
		{"over total", `{"totalRows":0,"data":[{"id":"one"}]}`, ""},
		{"wrong page", `{"totalRows":1,"currentPage":2,"data":[{"id":"one"}]}`, ""},
		{"null page", `{"totalRows":0,"currentPage":null,"data":[]}`, ""},
		{"negative size", `{"totalRows":0,"currentSize":-1,"data":[]}`, ""},
		{"null size", `{"totalRows":0,"currentSize":null,"data":[]}`, ""},
		{"duplicate metadata", `{"totalRows":2,"totalRows":1,"data":[{"id":"one"}]}`, ""},
		{"null row", `{"totalRows":1,"data":[null]}`, ""},
		{"missing identity", `{"totalRows":1,"data":[{"name":"private-secret"}]}`, ""},
		{"invalid identity", `{"totalRows":1,"data":[{"id":42}]}`, ""},
		{"invalid typed field", `{"totalRows":1,"data":[{"id":"one","vlan":"private-secret"}]}`, ""},
		{"duplicate within page", `{"totalRows":2,"data":[{"id":"one"},{"id":"one"}]}`, ""},
		{"duplicate array", `[{"id":"one"},{"id":"one"}]`, ""},
		{"repeated page", `{"totalRows":2,"data":[{"id":"one"}]}`, `{"totalRows":2,"data":[{"id":"one"}]}`},
		{"changed total", `{"totalRows":2,"data":[{"id":"one"}]}`, `{"totalRows":3,"data":[{"id":"two"}]}`},
		{"missing total later", `{"totalRows":2,"data":[{"id":"one"}]}`, `{"data":[{"id":"two"}]}`},
		{"new total later", `{"data":[{"id":"one"}]}`, `{"totalRows":2,"data":[{"id":"two"}]}`},
		{"empty later", `{"totalRows":2,"data":[{"id":"one"}]}`, `{"totalRows":2,"data":[]}`},
		{"array later", `{"totalRows":2,"data":[{"id":"one"}]}`, `[{"id":"two"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := mockOmadaServer(t, map[string]http.HandlerFunc{"/sites/site/setting/lan/networks": func(w http.ResponseWriter, r *http.Request) {
				calls++
				body := tc.first
				if calls > 1 {
					body = tc.second
				}
				if calls > 2 {
					t.Error("unbounded pagination")
					w.WriteHeader(500)
					return
				}
				fmt.Fprintf(w, `{"errorCode":0,"result":%s}`, body)
			}})
			defer server.Close()
			rows, err := newTestClient(t, server).ListNetworks(context.Background(), "site")
			if err == nil || rows != nil {
				t.Fatalf("unsafe success/partial collection: rows=%v err=%v", rows, err)
			}
			if strings.Contains(err.Error(), "private-secret") {
				t.Fatalf("server content in diagnostic: %v", err)
			}
		})
	}
}

func TestPaginationLookupAndRaw(t *testing.T) {
	for _, mode := range []string{"typed network", "raw SSID", "raw direct array", "site resolution"} {
		t.Run(mode, func(t *testing.T) {
			path := "/sites/site/setting/lan/networks"
			if strings.HasPrefix(mode, "raw") {
				path = "/sites/site/setting/wlans/group/ssids"
			}
			if mode == "site resolution" {
				path = "/sites"
			}
			server := mockOmadaServer(t, map[string]http.HandlerFunc{path: func(w http.ResponseWriter, r *http.Request) {
				page, _ := strconv.Atoi(r.URL.Query().Get("currentPage"))
				rows := []map[string]any{}
				start, end := (page-1)*100, min(page*100, 101)
				if mode == "raw direct array" {
					start, end = 0, 101
				}
				for i := start; i < end; i++ {
					rows = append(rows, map[string]any{"id": fmt.Sprintf("row-%03d", i), "name": fmt.Sprintf("name-%03d", i), "unknown": map[string]any{"preserve": true}})
				}
				var result any = map[string]any{"totalRows": 101, "data": rows}
				if mode == "raw direct array" {
					result = rows
				}
				json.NewEncoder(w).Encode(APIResponse{Result: mustMarshal(t, result)})
			}})
			defer server.Close()
			c := newTestClient(t, server)
			ctx := context.Background()
			switch mode {
			case "typed network":
				n, e := c.GetNetwork(ctx, "site", "row-100")
				if e != nil || n.ID != "row-100" {
					t.Fatalf("lookup: %v %v", n, e)
				}
			case "site resolution":
				id, e := c.ResolveSiteID(ctx, "name-100")
				if e != nil || id != "row-100" {
					t.Fatalf("resolve: %s %v", id, e)
				}
			default:
				n, e := c.GetWirelessNetworkRaw(ctx, "site", "group", "row-100")
				if e != nil {
					t.Fatal(e)
				}
				if n["unknown"].(map[string]any)["preserve"] != true {
					t.Fatal("lost raw attributes")
				}
			}
		})
	}
}

func TestPaginationCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := mockOmadaServer(t, map[string]http.HandlerFunc{"/sites/site/setting/lan/networks": func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"errorCode":0,"result":{"totalRows":2,"data":[{"id":"one"}]}}`)
		cancel()
	}})
	defer server.Close()
	rows, err := newTestClient(t, server).ListNetworks(ctx, "site")
	if !errors.Is(err, context.Canceled) || rows != nil {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
}

func TestPaginationBounds(t *testing.T) {
	for _, mode := range []string{"pages", "bytes"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			padding := ""
			if mode == "bytes" {
				padding = strings.Repeat("x", 2<<20)
			}
			c := &Client{baseURL: "http://fixture.invalid", omadacID: "controller", token: "fixture", httpClient: &http.Client{Transport: responseRoundTripper(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls > 10001 {
					t.Fatal("unbounded listing")
				}
				body := fmt.Sprintf(`{"errorCode":0,"result":{"data":[{"id":"row-%d","name":"%s"}]}}`, calls, padding)
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}}
			rows, err := c.ListNetworks(context.Background(), "site")
			if err == nil || rows != nil {
				t.Fatalf("expected bounded failure: rows=%d err=%v", len(rows), err)
			}
			wantCalls, diagnostic := 10000, "10000 page limit"
			if mode == "bytes" {
				wantCalls, diagnostic = 32, "64 MiB limit"
			}
			if calls != wantCalls || !strings.Contains(err.Error(), diagnostic) {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}


func TestWlanGroupUnpaginatedEnvelope(t *testing.T) {
	for _, body := range []string{
		`{"data":[{"id":"default","name":"Default"},{"id":"custom","name":"Custom"}],"maxWlanCount":8}`,
		`{"data":[]}`,
		`{"data":[{"id":"same"},{"id":"same"}]}`,
		`{"data":[{"name":"missing"}]}`,
		`{"data":null}`,
	} {
		t.Run(body, func(t *testing.T) {
			calls := 0
			server := mockOmadaServer(t, map[string]http.HandlerFunc{"/sites/site/setting/wlans": func(w http.ResponseWriter, r *http.Request) {
				calls++
				fmt.Fprintf(w, `{"errorCode":0,"result":%s}`, body)
			}})
			defer server.Close()
			c := newTestClient(t, server)
			rows, err := c.ListWlanGroups(context.Background(), "site")
			valid := strings.Contains(body, `"maxWlanCount"`) || body == `{"data":[]}`
			if (err == nil) != valid || calls != 1 || (!valid && rows != nil) {
				t.Fatalf("rows=%v calls=%d err=%v", rows, calls, err)
			}
			if len(rows) == 2 {
				id, err := c.GetDefaultWlanGroupID(context.Background(), "site")
				if err != nil || id != "default" {
					t.Fatalf("default: %q %v", id, err)
				}
				group, err := c.GetWlanGroup(context.Background(), "site", "custom")
				if err != nil || group.Name != "Custom" {
					t.Fatalf("custom: %v %v", group, err)
				}
			}
		})
	}
}

func TestGetNetworkConfirmedAbsence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		result   any
		wantMiss bool
	}{
		{
			name:     "complete empty inventory",
			result:   PaginatedResult{TotalRows: 0, CurrentPage: 1, CurrentSize: 100, Data: mustMarshal(t, []Network{})},
			wantMiss: true,
		},
		{
			name:   "incomplete inventory",
			result: PaginatedResult{TotalRows: 1, CurrentPage: 1, CurrentSize: 100, Data: mustMarshal(t, []Network{})},
		},
		{
			name:   "server failure",
			status: http.StatusServiceUnavailable,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := mockOmadaServer(t, map[string]http.HandlerFunc{
				"/sites/site/setting/lan/networks": func(w http.ResponseWriter, r *http.Request) {
					if tc.status != 0 {
						w.WriteHeader(tc.status)
						return
					}
					json.NewEncoder(w).Encode(APIResponse{ErrorCode: 0, Result: mustMarshal(t, tc.result)})
				},
			})
			defer server.Close()

			_, err := newTestClient(t, server).GetNetwork(context.Background(), "site", "missing")
			if err == nil {
				t.Fatal("expected lookup error")
			}
			if got := errors.Is(err, ErrNotFound); got != tc.wantMiss {
				t.Fatalf("errors.Is(err, ErrNotFound) = %v, want %v; err=%v", got, tc.wantMiss, err)
			}
		})
	}
}
