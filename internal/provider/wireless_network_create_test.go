package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccWirelessNetworkCreate(t *testing.T) {
	for _, mode := range []string{"defaults", "explicit", "unknown at plan"} {
		t.Run(mode, func(t *testing.T) {
			var mu sync.Mutex
			var current map[string]any
			const collection = "/controller/api/v2/sites/site/setting/wlans/group/ssids"
			want := map[string]any{"band": float64(3), "security": float64(3), "broadcast": true, "enable11r": false, "pmfMode": float64(2)}
			if mode != "defaults" {
				want = map[string]any{"band": float64(1), "security": float64(0), "broadcast": false, "enable11r": true, "pmfMode": float64(3)}
			}
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
				case r.Method == http.MethodPost && r.URL.Path == collection:
					if err := json.NewDecoder(r.Body).Decode(&current); err != nil {
						t.Error(err)
					}
					for key, value := range want {
						if !reflect.DeepEqual(current[key], value) {
							t.Errorf("create %s=%v, want %v", key, current[key], value)
						}
					}
					if mode == "defaults" {
						psk, _ := current["pskSetting"].(map[string]any)
						if psk["securityKey"] != "fixture-passphrase-123" {
							t.Error("missing configured password")
						}
					} else if _, ok := current["pskSetting"]; ok {
						t.Error("open create includes PSK")
					}
					current["id"] = "ssid"
					result = map[string]any{"ssidId": "ssid"}
				case r.Method == http.MethodGet && r.URL.Path == collection:
					items := []any{}
					if current != nil {
						items = append(items, current)
					}
					result = map[string]any{"data": items}
				case r.Method == http.MethodDelete && r.URL.Path == collection+"/ssid":
					current = nil
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "result": result})
			}))
			defer server.Close()
			extra := `passphrase = "fixture-passphrase-123"`
			dependency := ""
			if mode == "explicit" {
				extra = "band = 1\nsecurity = 0\nbroadcast = false\nenable_11r = true\npmf_mode = 3"
			}
			if mode == "unknown at plan" {
				dependency = `resource "terraform_data" "settings" {
  input = { band = 1, security = 0, broadcast = false, enable_11r = true, pmf_mode = 3 }
}`
				extra = ""
				for _, name := range []string{"band", "security", "broadcast", "enable_11r", "pmf_mode"} {
					extra += fmt.Sprintf("%s = terraform_data.settings.output.%s\n", name, name)
				}
			}
			config := fmt.Sprintf(`
provider "omada" {
  url = %q
  username = "fixture"
  password = "fixture"
}
%s
resource "omada_wireless_network" "test" {
  site_id = "site"
  wlan_group_id = "group"
  name = "Created"
  %s
}
`, server.URL, dependency, extra)
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{
				{Config: config, Check: resource.TestCheckResourceAttr("omada_wireless_network.test", "id", "ssid")},
				{Config: config, PlanOnly: true, ExpectNonEmptyPlan: false},
			}})
		})
	}
}
