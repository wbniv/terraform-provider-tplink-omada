package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccPagination(t *testing.T) {
	for _, mode := range []string{"import last page", "first page failure recovery", "confirmed disappearance", "data source"} {
		t.Run(mode, func(t *testing.T) {
			var mu sync.Mutex
			exists, failLater := true, false
			deletes := 0
			target := 200
			if mode == "first page failure recovery" {
				target = 0
			}
			id := fmt.Sprintf("network-%03d", target)
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
					page, _ := strconv.Atoi(r.URL.Query().Get("currentPage"))
					if failLater && page == 2 {
						w.WriteHeader(503)
						return
					}
					all := []map[string]any{}
					for i := 0; i < 201; i++ {
						if !exists && i == target {
							continue
						}
						all = append(all, map[string]any{"id": fmt.Sprintf("network-%03d", i), "name": fmt.Sprintf("Network %03d", i), "purpose": "vlan", "vlan": 20, "igmpSnoopEnable": false})
					}
					start := min((page-1)*100, len(all))
					end := min(page*100, len(all))
					result = map[string]any{"totalRows": len(all), "currentPage": page, "currentSize": 100, "data": all[start:end]}
				case r.URL.Path == "/controller/api/v2/sites/site/setting/lan/networks/"+id && r.Method == http.MethodDelete:
					deletes++
					exists = false
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(500)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "result": result})
			}))
			defer server.Close()
			provider := fmt.Sprintf("provider \"omada\" {\nurl = %q\nusername = \"fixture\"\npassword = \"fixture\"\n}\n", server.URL)
			if mode == "data source" {
				config := provider + "data \"omada_networks\" \"test\" {\nsite_id = \"site\"\n}\n"
				resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{
					{Config: config, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("data.omada_networks.test", "networks.#", "201"), resource.TestCheckResourceAttr("data.omada_networks.test", "networks.200.id", "network-200"))},
				}})
				return
			}
			config := provider + fmt.Sprintf("resource \"omada_network\" \"test\" {\nsite_id = \"site\"\nname = \"Network %03d\"\npurpose = \"vlan\"\nvlan_id = 20\n}\n", target)
			setFailure := func(fail bool) func() { return func() { mu.Lock(); defer mu.Unlock(); failLater = fail } }
			setAbsent := func() { mu.Lock(); defer mu.Unlock(); exists = false }
			check := func(s *terraform.State) error {
				mu.Lock()
				defer mu.Unlock()
				r, ok := s.RootModule().Resources["omada_network.test"]
				if !ok || r.Primary.ID != id || !exists || deletes != 0 {
					return fmt.Errorf("lost managed identity or changed remote object during refresh")
				}
				return nil
			}
			if mode == "confirmed disappearance" {
				resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{
					{Config: config + fmt.Sprintf("import {\nto = omada_network.test\nid = \"site/%s\"\n}\n", id), Check: check},
					{PreConfig: setAbsent, Config: config, PlanOnly: true, ExpectNonEmptyPlan: true},
				}})
				return
			}
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{
				{Config: config + fmt.Sprintf("import {\nto = omada_network.test\nid = \"site/%s\"\n}\n", id), Check: check},
				{Config: config, PlanOnly: true, Check: check},
				{PreConfig: setFailure(true), Config: config, PlanOnly: true, ExpectError: regexp.MustCompile("HTTP error 503")},
				{PreConfig: setFailure(false), Config: config, PlanOnly: true, Check: check},
				{Config: config, Destroy: true},
			}, CheckDestroy: func(_ *terraform.State) error {
				mu.Lock()
				defer mu.Unlock()
				if exists || deletes != 1 {
					return fmt.Errorf("managed object not deleted exactly once")
				}
				return nil
			}})
		})
	}
}
