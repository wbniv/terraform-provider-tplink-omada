package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccWirelessNetworkPreservation(t *testing.T) {
	for _, mode := range []string{"plaintext", "masked", "absent", "open", "unmanaged"} {
		t.Run(mode, func(t *testing.T) {
			data, err := os.ReadFile("../testdata/wireless_network.json")
			if err != nil {
				t.Fatal(err)
			}
			var current map[string]any
			if err := json.Unmarshal(data, &current); err != nil {
				t.Fatal(err)
			}
			if mode == "open" {
				current["security"] = float64(0)
				delete(current, "pskSetting")
			}
			protected := map[string]any{}
			for _, key := range []string{"guestNetEnable", "wlanScheduleEnable", "action", "scheduleId", "macFilterEnable", "policy", "macFilterId", "rateLimit", "ssidRateLimit", "rateAndBeaconCtrl", "multiCastSetting", "mloEnable", "prohibitWifiShare"} {
				protected[key] = current[key]
			}
			var mu sync.Mutex
			writes := 0
			wantPassword := "fixture-passphrase-123"
			const collection = "/controller/api/v2/sites/site/setting/wlans/group/ssids"
			snapshot := func() map[string]any {
				b, _ := json.Marshal(current)
				var v map[string]any
				_ = json.Unmarshal(b, &v)
				if psk, ok := v["pskSetting"].(map[string]any); ok {
					switch mode {
					case "masked":
						psk["securityKey"] = "********"
					case "absent":
						delete(psk, "securityKey")
					default:
						psk["securityKey"] = "untrusted-api-value"
					}
				}
				return v
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
				case r.Method == http.MethodGet && r.URL.Path == collection:
					result = map[string]any{"data": []any{snapshot()}, "totalRows": 1}
				case r.Method == http.MethodGet && r.URL.Path == "/controller/api/v2/sites/site/setting/lan/networks":
					result = map[string]any{"data": []any{map[string]any{"id": "network-30", "vlan": 30}}}
				case r.Method == http.MethodPatch && r.URL.Path == collection+"/ssid":
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					writes++
					for key, want := range protected {
						if !reflect.DeepEqual(body[key], want) {
							t.Errorf("unrelated field %s changed: %v", key, body[key])
						}
					}
					for _, key := range []string{"id", "idInt", "index", "site", "resource", "portalEnable", "accessEnable"} {
						if _, exists := body[key]; exists {
							t.Errorf("read-only field %s in PATCH", key)
						}
					}
					if body["security"] == float64(3) {
						psk, _ := body["pskSetting"].(map[string]any)
						if psk["securityKey"] != wantPassword {
							t.Error("PATCH did not use the configured secret")
						}
						if previous, ok := current["pskSetting"].(map[string]any); ok {
							for key, value := range previous {
								if key != "securityKey" && !reflect.DeepEqual(psk[key], value) {
									t.Errorf("PSK field %s changed", key)
								}
							}
						}
					} else if _, exists := body["pskSetting"]; exists {
						t.Error("open network payload includes PSK")
					}
					// Model a full-object PATCH: omitted writable settings would be lost.
					body["id"], body["wlanId"] = "ssid", "group"
					current = body
					// Exercise empty-success readback on every update.
				case r.Method == http.MethodDelete && r.URL.Path == collection+"/ssid":
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "result": result})
			}))
			defer server.Close()
			secret := "passphrase = \"fixture-passphrase-123\""
			if mode == "open" || mode == "unmanaged" {
				secret = ""
			}
			config := func(name, extra string, vlan bool) string {
				vlanConfig := ""
				if vlan {
					vlanConfig = "vlan_id = 20"
				}
				return fmt.Sprintf(`
provider "omada" {
  url = %q
  username = "fixture"
  password = "fixture"
}
resource "omada_wireless_network" "test" {
  site_id = "site"
  wlan_group_id = "group"
  name = %q
  %s
  %s
}
`, server.URL, name, extra, vlanConfig)
			}
			check := func(count int, values map[string]string) resource.TestCheckFunc {
				checks := []resource.TestCheckFunc{func(_ *terraform.State) error {
					mu.Lock()
					defer mu.Unlock()
					if writes != count {
						return fmt.Errorf("PATCH count=%d, want %d", writes, count)
					}
					return nil
				}}
				for key, value := range values {
					checks = append(checks, resource.TestCheckResourceAttr("omada_wireless_network.test", key, value))
				}
				return resource.ComposeTestCheckFunc(checks...)
			}
			// Secured imports establish the trusted configured key; open imports need no write.
			initial := 1
			if mode == "open" || mode == "unmanaged" {
				initial = 0
			}
			changes := secret + "\nband = 3\nbroadcast = true\nenable_11r = false\npmf_mode = 2"
			steps := []resource.TestStep{
				{Config: config("Original", secret, true) + `import {
  to = omada_wireless_network.test
  id = "site/group/ssid"
}`, Check: check(initial, map[string]string{"band": "1", "broadcast": "false", "enable_11r": "true", "pmf_mode": "3", "vlan_id": "20"})},
				{Config: config("Renamed", secret, true), Check: check(initial+1, map[string]string{"name": "Renamed", "band": "1", "broadcast": "false", "enable_11r": "true", "pmf_mode": "3"})},
				{Config: config("Renamed", secret, true), PlanOnly: true, ExpectNonEmptyPlan: false},
				{Config: config("Renamed", changes, true), Check: check(initial+2, map[string]string{"band": "3", "broadcast": "true", "enable_11r": "false", "pmf_mode": "2"})},
				{Config: config("Again", secret, true), Check: check(initial+3, map[string]string{"band": "3", "broadcast": "true", "enable_11r": "false", "pmf_mode": "2"})},
				{Config: config("Again", secret, true), PlanOnly: true, ExpectNonEmptyPlan: false},
			}
			if mode != "open" && mode != "unmanaged" {
				rotated := "passphrase = \"rotated-fixture-secret\""
				steps = append(steps,
					resource.TestStep{PreConfig: func() { mu.Lock(); defer mu.Unlock(); wantPassword = "rotated-fixture-secret" }, Config: config("Again", rotated, true), Check: check(initial+4, map[string]string{"passphrase": "rotated-fixture-secret"})},
					resource.TestStep{Config: config("Unmanaged key", "", true), Check: check(initial+5, map[string]string{"name": "Unmanaged key"})},
					resource.TestStep{Config: config("Unmanaged key", "", true), PlanOnly: true, ExpectNonEmptyPlan: false},
					resource.TestStep{Config: config("Open", "security = 0", true), Check: check(initial+6, map[string]string{"security": "0"})},
					resource.TestStep{Config: config("Secured", rotated+"\nsecurity = 3", true), Check: check(initial+7, map[string]string{"security": "3", "passphrase": "rotated-fixture-secret"})},
				)
			}
			if mode == "open" {
				defaultVLAN := config("Again", "", false)
				customVLAN := strings.Replace(config("Again", "", true), "vlan_id = 20", "vlan_id = 30", 1)
				steps = append(steps,
					resource.TestStep{Config: defaultVLAN, Check: resource.ComposeTestCheckFunc(check(initial+4, map[string]string{"name": "Again"}), resource.TestCheckNoResourceAttr("omada_wireless_network.test", "vlan_id"))},
					resource.TestStep{Config: defaultVLAN, PlanOnly: true, ExpectNonEmptyPlan: false},
					resource.TestStep{Config: customVLAN, Check: check(initial+5, map[string]string{"vlan_id": "30"})},
					resource.TestStep{Config: customVLAN, PlanOnly: true, ExpectNonEmptyPlan: false},
				)
			}
			if mode == "unmanaged" {
				steps = []resource.TestStep{steps[0],
					{Config: config("Renamed", "", true), ExpectError: regexp.MustCompile("configure passphrase")},
					{Config: config("Original", "", true), PlanOnly: true, ExpectNonEmptyPlan: false, Check: check(0, map[string]string{"name": "Original"})},
				}
			}
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: steps})
		})
	}
}
