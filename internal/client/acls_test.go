package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestACLCreateReadback(t *testing.T) {
	for _, mode := range []string{"empty acknowledgment", "object acknowledgment", "preexisting", "preflight failure", "readback failure", "missing", "ambiguous", "different configuration", "late page failure"} {
		t.Run(mode, func(t *testing.T) {
			input := ACLRule{Name: "Test ACL", Protocols: []int{6}, SourceType: 2, SourceIDs: []string{"group"}, DestinationType: 2, DestinationIDs: []string{"group"}, Direction: ACLDirection{LanToWan: true}, Policy: 1}
			created := input
			created.ID = "new"
			created.Index = 3
			posts := 0
			server := mockOmadaServer(t, map[string]http.HandlerFunc{"/sites/site/setting/firewall/acls": func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts++
					var payload map[string]json.RawMessage
					_ = json.NewDecoder(r.Body).Decode(&payload)
					if _, ok := payload["id"]; ok {
						t.Error("sent identity")
					}
					if _, ok := payload["index"]; ok {
						t.Error("sent index")
					}
					if mode == "object acknowledgment" {
						json.NewEncoder(w).Encode(APIResponse{Result: mustMarshal(t, created)})
					} else {
						fmt.Fprint(w, `{"errorCode":0,"msg":"Success."}`)
					}
					return
				}
				if r.Method != http.MethodGet {
					t.Errorf("unexpected method %s", r.Method)
				}
				if mode == "preflight failure" || (posts > 0 && mode == "readback failure") {
					http.Error(w, "unavailable", 503)
					return
				}
				rows := []ACLRule{}
				if mode == "preexisting" {
					rows = append(rows, created)
				}
				if posts > 0 && mode != "missing" {
					rows = append(rows, created)
					if mode == "ambiguous" {
						other := created
						other.ID = "other"
						rows = append(rows, other)
					}
					if mode == "different configuration" {
						rows[0].Policy = 0
					}
				}
				if mode == "late page failure" && posts > 0 {
					if r.URL.Query().Get("currentPage") == "2" {
						http.Error(w, "unavailable", 503)
						return
					}
					json.NewEncoder(w).Encode(APIResponse{Result: mustMarshal(t, ACLListResult{TotalRows: 2, CurrentPage: 1, CurrentSize: 1, Data: rows})})
					return
				}
				json.NewEncoder(w).Encode(APIResponse{Result: paginatedResponse(t, rows)})
			}})
			defer server.Close()
			c := newTestClient(t, server)
			got, err := c.CreateACLRule(context.Background(), "site", &input)
			success := mode == "empty acknowledgment" || mode == "object acknowledgment"
			if success {
				if err != nil || got == nil || got.ID != "new" || got.Index != 3 {
					t.Fatalf("result=%+v error=%v", got, err)
				}
			} else {
				if err == nil || got != nil {
					t.Fatalf("unsafe success: %+v %v", got, err)
				}
				if mode != "preflight failure" && !strings.Contains(err.Error(), "import") {
					t.Errorf("missing recovery guidance: %v", err)
				}
			}
			want := 1
			if mode == "preexisting" || mode == "preflight failure" {
				want = 0
			}
			if posts != want {
				t.Fatalf("posts=%d want=%d", posts, want)
			}
		})
	}
}

func TestACLUpdatePreservesUnmanagedFields(t *testing.T) {
	raw := map[string]any{"id": "rule", "siteId": "site", "type": 0, "name": "old", "index": 7, "status": false, "policy": 0, "protocols": []int{6}, "sourceType": 2, "sourceIds": []string{"g"}, "destinationType": 2, "destinationIds": []string{"g"}, "syslog": true, "stateMode": 2, "customAclPorts": []string{"port"}, "futureSetting": map[string]any{"value": true}, "direction": map[string]any{"lanToWan": false, "lanToLan": true, "wanInIds": []string{"wan"}, "futureDirection": true}}
	patches := 0
	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site/setting/firewall/acls": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{Result: paginatedResponse(t, []any{raw})})
		},
		"/sites/site/setting/firewall/acls/rule": func(w http.ResponseWriter, r *http.Request) {
			patches++
			if r.Method != http.MethodPut {
				t.Errorf("method=%s", r.Method)
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["index"] != float64(7) || payload["syslog"] != true || payload["stateMode"] != float64(2) || payload["futureSetting"] == nil || payload["customAclPorts"] == nil {
				t.Errorf("lost fields: %v", payload)
			}
			if payload["id"] != nil || payload["siteId"] != nil {
				t.Error("sent response identity fields")
			}
			direction := payload["direction"].(map[string]any)
			if direction["lanToWan"] != true || direction["lanToLan"] != false || direction["wanInIds"] == nil || direction["futureDirection"] != true {
				t.Errorf("direction=%v", direction)
			}
			raw = payload
			raw["id"] = "rule"
			fmt.Fprint(w, `{"errorCode":0}`)
		}})
	defer server.Close()
	c := newTestClient(t, server)
	got, err := c.UpdateACLRule(context.Background(), "site", "rule", &ACLRule{Name: "new", Policy: 1, Protocols: []int{17}, SourceType: 2, SourceIDs: []string{"g"}, DestinationType: 2, DestinationIDs: []string{"g"}, Direction: ACLDirection{LanToWan: true}})
	if err != nil || got.Name != "new" || got.ID != "rule" || got.Index != 7 || !got.Syslog || patches != 1 {
		t.Fatalf("result=%+v err=%v patches=%d", got, err, patches)
	}
}
