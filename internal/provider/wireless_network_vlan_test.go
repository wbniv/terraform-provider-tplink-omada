package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type ssidVLANPlan struct{ before, after string }

func (check ssidVLANPlan) CheckPlan(_ context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	for _, change := range req.Plan.ResourceChanges {
		if change.Address != "omada_wireless_network.test" {
			continue
		}
		before, _ := change.Change.Before.(map[string]any)
		after, _ := change.Change.After.(map[string]any)
		if !change.Change.Actions.Update() || fmt.Sprint(before["vlan_id"]) != check.before || fmt.Sprint(after["vlan_id"]) != check.after {
			resp.Error = fmt.Errorf("expected VLAN update %s -> %s, got %v -> %v (%v)", check.before, check.after, before["vlan_id"], after["vlan_id"], change.Change.Actions)
		}
		return
	}
	resp.Error = fmt.Errorf("SSID missing from plan")
}

func TestAccWirelessNetworkVLANState(t *testing.T) {
	for _, scenario := range []string{"default clears fields", "default retains stale fields", "import default"} {
		t.Run(scenario, func(t *testing.T) {
			data, err := os.ReadFile("../testdata/wireless_network.json")
			if err != nil {
				t.Fatal(err)
			}
			var current map[string]any
			if err = json.Unmarshal(data, &current); err != nil {
				t.Fatal(err)
			}
			current["security"] = float64(0)
			delete(current, "pskSetting")
			var mu sync.Mutex
			writes := 0
			setDefault := func() {
				mu.Lock()
				defer mu.Unlock()
				setting := map[string]any{"mode": float64(0)}
				if scenario != "default clears fields" {
					setting["customConfig"] = map[string]any{"customMode": float64(0), "bridgeVlan": float64(20)}
					setting["currentVlanId"] = float64(20)
					setting["currentVlanIds"] = "20"
				}
				current["vlanSetting"] = setting
			}
			setCustom30 := func() {
				mu.Lock()
				defer mu.Unlock()
				current["vlanSetting"] = map[string]any{"mode": float64(1), "customConfig": map[string]any{"customMode": float64(0), "bridgeVlan": float64(30), "lanNetworkId": "network-30", "lanNetworkVlanIds": map[string]any{"network-30": []any{float64(30)}}}}
			}
			const collection = "/controller/api/v2/sites/site/setting/wlans/group/ssids"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				var result any
				switch {
				case r.URL.Path == "/api/info":
					result = map[string]any{"omadacId": "controller"}
				case r.URL.Path == "/controller/api/v2/login":
					result = map[string]any{"token": "token"}
				case r.Method == http.MethodGet && r.URL.Path == collection:
					result = map[string]any{"data": []any{current}, "totalRows": 1}
				case r.Method == http.MethodGet && r.URL.Path == "/controller/api/v2/sites/site/setting/lan/networks":
					result = map[string]any{"data": []any{map[string]any{"id": "network-20", "vlan": 20}}}
				case r.Method == http.MethodPatch && r.URL.Path == collection+"/ssid":
					writes++
					var next map[string]any
					if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
						t.Error(err)
					}
					next["id"] = "ssid"
					next["wlanId"] = "group"
					current = next
				case r.Method == http.MethodDelete && r.URL.Path == collection+"/ssid":
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "result": result})
			}))
			defer server.Close()
			config := func(vlan string) string {
				return fmt.Sprintf(`
provider "omada" {
  url = %q
  username = "fixture"
  password = "fixture"
}
resource "omada_wireless_network" "test" {
  site_id = "site"
  wlan_group_id = "group"
  name = "Original"
  %s
}
data "omada_wireless_networks" "test" {
  site_id = "site"
  wlan_group_id = "group"
  depends_on = [omada_wireless_network.test]
}
`, server.URL, vlan)
			}
			check := func(count int, vlan string, includeData bool) resource.TestCheckFunc {
				checks := []resource.TestCheckFunc{func(_ *terraform.State) error {
					mu.Lock()
					defer mu.Unlock()
					if writes != count {
						return fmt.Errorf("PATCH count = %d, want %d", writes, count)
					}
					return nil
				}}
				for _, attr := range []struct{ address, name string }{{"omada_wireless_network.test", "vlan_id"}, {"data.omada_wireless_networks.test", "wireless_networks.0.vlan_id"}} {
					if !includeData && attr.address == "data.omada_wireless_networks.test" {
						continue
					}
					if vlan == "" {
						checks = append(checks, resource.TestCheckNoResourceAttr(attr.address, attr.name))
					} else {
						checks = append(checks, resource.TestCheckResourceAttr(attr.address, attr.name, vlan))
					}
				}
				return resource.ComposeTestCheckFunc(checks...)
			}
			const importBlock = `
import {
  to = omada_wireless_network.test
  id = "site/group/ssid"
}`
			custom, defaultVLAN := config("vlan_id = 20"), config("")
			steps := []resource.TestStep{
				{Config: custom + importBlock, Check: check(0, "20", true)},
				{PreConfig: setDefault, RefreshState: true, ExpectNonEmptyPlan: true, Check: check(0, "", false), RefreshPlanChecks: resource.RefreshPlanChecks{PostRefresh: []plancheck.PlanCheck{ssidVLANPlan{"<nil>", "20"}}}},
				{Config: custom, ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{ssidVLANPlan{"<nil>", "20"}}}, Check: check(1, "20", true)},
				{Config: custom, PlanOnly: true, ExpectNonEmptyPlan: false},
				{PreConfig: setDefault, Config: defaultVLAN, Check: check(1, "", true)},
				{Config: defaultVLAN, PlanOnly: true, ExpectNonEmptyPlan: false},
				{PreConfig: setCustom30, RefreshState: true, ExpectNonEmptyPlan: true, Check: check(1, "30", false), RefreshPlanChecks: resource.RefreshPlanChecks{PostRefresh: []plancheck.PlanCheck{ssidVLANPlan{"30", "<nil>"}}}},
				{Config: defaultVLAN, Check: check(2, "", true)},
				{Config: defaultVLAN, PlanOnly: true, ExpectNonEmptyPlan: false},
			}
			if scenario == "import default" {
				setDefault()
				steps = []resource.TestStep{
					{Config: defaultVLAN + importBlock, Check: check(0, "", true)},
					{Config: defaultVLAN, PlanOnly: true, ExpectNonEmptyPlan: false},
					{ResourceName: "omada_wireless_network.test", ImportState: true, ImportStateId: "site/group/ssid", ImportStateVerify: true},
				}
			}
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: steps})
		})
	}
}
