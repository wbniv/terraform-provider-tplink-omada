package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Rejected DELETE must leave both the remote object and its Terraform identity
// intact. Import avoids mixing this regression with network adoption semantics.
func TestAccHTTPResponseDelete(t *testing.T) {
	for _, tc := range []struct {
		name             string
		status           int
		body, diagnostic string
	}{
		{"403 JSON", 403, `{"message":"Forbidden"}`, "HTTP error 403"},
		{"500 apparent success", 500, `{"errorCode":0}`, "HTTP error 500"},
		{"200 missing code", 200, `{"message":"not acknowledged"}`, "invalid API response"},
		{"200 null envelope", 200, `null`, "invalid API response"},
		{"502 HTML", 502, `<html>proxy failure</html>`, "HTTP error 502"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			exists, reject := true, false
			deletes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				var result any
				switch {
				case r.URL.Path == "/api/info":
					result = map[string]any{"omadacId": "controller"}
				case r.URL.Path == "/controller/api/v2/login":
					result = map[string]any{"token": "fixture-token"}
				case r.URL.Path == "/controller/api/v2/sites/site/setting/lan/networks" && r.Method == http.MethodGet:
					list := []any{}
					if exists {
						list = append(list, map[string]any{"id": "network", "name": "Fixture VLAN", "purpose": "vlan", "vlan": 20, "igmpSnoopEnable": false})
					}
					result = map[string]any{"totalRows": len(list), "data": list}
				case r.URL.Path == "/controller/api/v2/sites/site/setting/lan/networks/network" && r.Method == http.MethodDelete:
					deletes++
					if reject {
						reject = false
						w.WriteHeader(tc.status)
						fmt.Fprint(w, tc.body)
						return
					}
					exists = false
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(500)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "result": result})
			}))
			defer server.Close()
			config := fmt.Sprintf("provider \"omada\" {\nurl = %q\nusername = \"fixture\"\npassword = \"fixture\"\n}\nresource \"omada_network\" \"test\" {\nsite_id = \"site\"\nname = \"Fixture VLAN\"\npurpose = \"vlan\"\nvlan_id = 20\n}\n", server.URL)
			check := func(wantDeletes int) resource.TestCheckFunc {
				return func(s *terraform.State) error {
					mu.Lock()
					defer mu.Unlock()
					r, ok := s.RootModule().Resources["omada_network.test"]
					if !ok || r.Primary.ID != "network" || !exists || deletes != wantDeletes {
						return fmt.Errorf("lost managed identity or remote object; deletes=%d want=%d", deletes, wantDeletes)
					}
					return nil
				}
			}
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{Config: config + "import {\nto = omada_network.test\nid = \"site/network\"\n}\n", Check: check(0)},
					{PreConfig: func() { mu.Lock(); defer mu.Unlock(); reject = true }, Config: config, Destroy: true, ExpectError: regexp.MustCompile(tc.diagnostic)},
					{Config: config, PlanOnly: true, Check: check(1)},
					{Config: config, Destroy: true},
				},
				CheckDestroy: func(s *terraform.State) error {
					mu.Lock()
					defer mu.Unlock()
					if exists || deletes != 2 {
						return fmt.Errorf("destroy not acknowledged remotely; exists=%v deletes=%d", exists, deletes)
					}
					// CheckDestroy receives the pre-cleanup state; verify remote deletion here.
					// Terraform and the explicit destroy step verify state removal.
					return nil
				},
			})
		})
	}
}
