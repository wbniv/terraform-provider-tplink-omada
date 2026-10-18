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
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Exercise Terraform's actual Optional+Computed planning, including import of
// nondefault settings. A helper-only test cannot reproduce the unknown values.
func TestAccDeviceAPRadioPreservation(t *testing.T) {
	for _, scenario := range []struct {
		name          string
		dual, enabled bool
		level         int
	}{
		{"dual_default", true, true, 4},
		{"dual_nondefault", true, false, 3},
		{"single_band", false, false, 0},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var mu sync.Mutex
			radio := func(enabled bool, width, channel string, power, level int) map[string]any {
				return map[string]any{"radioEnable": enabled, "channelWidth": width, "channel": channel, "txPower": float64(power), "txPowerLevel": float64(level)}
			}
			original2g := radio(scenario.enabled, "4", "0", 20, scenario.level)
			original5g := radio(true, "5", "36", 18, 3)
			current := map[string]any{
				"mac": "02-00-00-00-00-01", "name": "Original", "wlanId": "group", "ledSetting": 2,
				"ipSetting": map[string]any{"mode": "dhcp"}, "radioSetting2g": original2g,
			}
			if scenario.dual {
				current["radioSetting5g"] = original5g
			}
			var expected map[string]map[string]any
			writes := 0
			const path = "/controller/api/v2/sites/site/eaps/02-00-00-00-00-01"
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
				case r.Method == http.MethodGet && r.URL.Path == path:
					result = current
				case r.Method == http.MethodPatch && r.URL.Path == path:
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if name, ok := body["name"].(string); ok {
						current["name"] = name
					}
					// The controller ignores radio fields on this endpoint.
					result = current
				case r.Method == http.MethodPut && r.URL.Path == path+"/config/radios":
					var body map[string]map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					writes++
					if !reflect.DeepEqual(body, expected) {
						t.Errorf("unexpected radio write: got %#v, want %#v", body, expected)
						_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": -1001, "msg": "unexpected radio payload"})
						return
					}
					for band, settings := range body {
						current[band] = settings
					}
				case r.Method == http.MethodPut && (r.URL.Path == path+"/config/advanced" || r.URL.Path == path+"/config/services"):
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "result": result})
			}))
			defer server.Close()
			config := func(name, settings string) string {
				return fmt.Sprintf(`
provider "omada" {
  url = %q
  username = "test"
  password = "test"
}
resource "omada_device_ap" "test" {
  name = %q
  %s
}
`, server.URL, name, settings)
			}
			prepare := func(payload map[string]map[string]any) func() {
				return func() { mu.Lock(); defer mu.Unlock(); expected = payload }
			}
			check := func(name string, count int, want2g, want5g map[string]any) resource.TestCheckFunc {
				checks := []resource.TestCheckFunc{
					resource.TestCheckResourceAttr("omada_device_ap.test", "name", name),
					func(_ *terraform.State) error {
						mu.Lock()
						defer mu.Unlock()
						if writes != count {
							return fmt.Errorf("radio PUT count = %d, want %d", writes, count)
						}
						if !reflect.DeepEqual(current["radioSetting2g"], want2g) {
							return fmt.Errorf("unexpected 2g settings: %v", current["radioSetting2g"])
						}
						if scenario.dual && !reflect.DeepEqual(current["radioSetting5g"], want5g) {
							return fmt.Errorf("unexpected 5g settings: %v", current["radioSetting5g"])
						}
						if !scenario.dual && current["radioSetting5g"] != nil {
							return fmt.Errorf("created unsupported 5g radio")
						}
						return nil
					},
				}
				for _, band := range []struct {
					prefix string
					values map[string]any
				}{{"radio_2g_", want2g}, {"radio_5g_", want5g}} {
					for _, field := range []struct{ api, attr string }{{"radioEnable", "enable"}, {"channel", "channel"}, {"channelWidth", "channel_width"}, {"txPower", "tx_power"}, {"txPowerLevel", "tx_power_level"}} {
						if band.values == nil {
							checks = append(checks, resource.TestCheckNoResourceAttr("omada_device_ap.test", band.prefix+field.attr))
						} else {
							checks = append(checks, resource.TestCheckResourceAttr("omada_device_ap.test", band.prefix+field.attr, fmt.Sprint(band.values[field.api])))
						}
					}
				}
				return resource.ComposeTestCheckFunc(checks...)
			}
			var want5g map[string]any
			if scenario.dual {
				want5g = original5g
			}
			changed2g := radio(scenario.enabled, "4", "6", 20, scenario.level)
			zero2g := radio(false, "4", "6", 0, 0)
			zeroConfig := "radio_2g_channel = \"6\"\nradio_2g_enable = false\nradio_2g_tx_power = 0\nradio_2g_tx_power_level = 0"
			steps := []resource.TestStep{
				{Config: config("Original", "") + `import {
  to = omada_device_ap.test
  id = "site/02-00-00-00-00-01"
}`, Check: check("Original", 0, original2g, want5g)},
				{Config: config("Renamed", ""), Check: check("Renamed", 0, original2g, want5g)},
				{Config: config("Renamed", ""), PlanOnly: true, ExpectNonEmptyPlan: false},
				{Config: config("Renamed", "radio_2g_channel = \"6\""), PreConfig: prepare(map[string]map[string]any{"radioSetting2g": changed2g}), Check: check("Renamed", 1, changed2g, want5g)},
				{Config: config("Renamed", zeroConfig), PreConfig: prepare(map[string]map[string]any{"radioSetting2g": zero2g}), Check: check("Renamed", 2, zero2g, want5g)},
				{Config: config("Again", ""), PreConfig: prepare(nil), Check: check("Again", 2, zero2g, want5g)},
				{Config: config("Again", ""), PlanOnly: true, ExpectNonEmptyPlan: false},
			}
			if scenario.dual {
				changed5g := radio(false, "6", "0", 0, 0)
				steps = append(steps,
					resource.TestStep{Config: config("Again", "radio_5g_enable = false\nradio_5g_channel_width = \"6\"\nradio_5g_channel = \"0\"\nradio_5g_tx_power = 0\nradio_5g_tx_power_level = 0"), PreConfig: prepare(map[string]map[string]any{"radioSetting5g": changed5g}), Check: check("Again", 3, zero2g, changed5g)},
					resource.TestStep{Config: config("Final", ""), PreConfig: prepare(nil), Check: check("Final", 3, zero2g, changed5g)},
					resource.TestStep{Config: config("Final", ""), PlanOnly: true, ExpectNonEmptyPlan: false},
				)
			}
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: steps})
		})
	}
}
