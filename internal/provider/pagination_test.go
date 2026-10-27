package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/terraform-provider-tplink-omada/internal/client"
)

func TestAccPagination(t *testing.T) {
	for _, mode := range []string{"import last page", "first page failure recovery", "confirmed disappearance", "already absent at delete", "data source"} {
		t.Run(mode, func(t *testing.T) {
			var mu sync.Mutex
			exists, failLater := true, false
			disappearAfterList := false
			postDisappearanceLists := 0
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
					if !exists && mode == "already absent at delete" {
						postDisappearanceLists++
					}
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
					if disappearAfterList && page == (len(all)+99)/100 {
						exists = false
						disappearAfterList = false
					}
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
			setDisappearAfterRefresh := func() { mu.Lock(); defer mu.Unlock(); disappearAfterList = true }
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
			if mode == "already absent at delete" {
				resource.Test(t, resource.TestCase{
					ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
					Steps: []resource.TestStep{
						{Config: config + fmt.Sprintf("import {\nto = omada_network.test\nid = \"site/%s\"\n}\n", id), Check: check},
						{PreConfig: setDisappearAfterRefresh, Config: config, Destroy: true},
					},
					CheckDestroy: func(_ *terraform.State) error {
						mu.Lock()
						defer mu.Unlock()
						if exists || deletes != 0 || postDisappearanceLists != 2 {
							return fmt.Errorf("already-absent delete was not confirmed safely: exists=%v deletes=%d follow-up lists=%d", exists, deletes, postDisappearanceLists)
						}
						return nil
					},
				})
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

func TestControllerLabNetworkDisappearance(t *testing.T) {
	if os.Getenv("OMADA_CONTROLLER_LAB") != "1" {
		t.Skip("disposable controller lab only")
	}
	if os.Getenv("OMADA_URL") != "https://controller:8043" {
		t.Fatal("refusing a target outside the isolated controller lab")
	}
	siteID := os.Getenv("OMADA_TEST_SITE_ID")
	if siteID == "" {
		t.Fatal("missing isolated lab site ID")
	}
	controller, err := client.NewClient(os.Getenv("OMADA_URL"), os.Getenv("OMADA_USERNAME"), os.Getenv("OMADA_PASSWORD"), true)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	name := fmt.Sprintf("G2 disappearance %d", time.Now().UnixNano())
	vlanID := 100 + time.Now().UnixNano()%3900
	config := fmt.Sprintf(`
resource "omada_network" "g2" {
  site_id = %q
  name = %q
  purpose = "vlan"
  vlan_id = %d
}
`, siteID, name, vlanID)
	var originalID string
	checkID := func(state *terraform.State) error {
		managed, ok := state.RootModule().Resources["omada_network.g2"]
		if !ok || managed.Primary.ID == "" {
			return fmt.Errorf("network is missing from Terraform state")
		}
		return nil
	}
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) }, ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config, Check: func(state *terraform.State) error {
				if err := checkID(state); err != nil {
					return err
				}
				originalID = state.RootModule().Resources["omada_network.g2"].Primary.ID
				return nil
			}},
			{PreConfig: func() {
				if originalID == "" {
					t.Fatal("initial network ID was not recorded")
				}
				if err := controller.DeleteNetwork(ctx, siteID, originalID); err != nil {
					t.Fatalf("out-of-band delete: %v", err)
				}
			}, Config: config, PlanOnly: true, ExpectNonEmptyPlan: true},
			{Config: config, Check: func(state *terraform.State) error {
				if err := checkID(state); err != nil {
					return err
				}
				newID := state.RootModule().Resources["omada_network.g2"].Primary.ID
				if newID == originalID {
					return fmt.Errorf("recreated network reused deleted ID %q", originalID)
				}
				network, err := controller.GetNetwork(ctx, siteID, newID)
				if err != nil {
					return fmt.Errorf("read recreated network: %w", err)
				}
				if network.Name != name || network.Vlan != int(vlanID) {
					return fmt.Errorf("recreated network differs from configuration: %#v", network)
				}
				return nil
			}},
			{Config: config, PlanOnly: true},
		},
	})
}
