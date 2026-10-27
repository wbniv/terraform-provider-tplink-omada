package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Real Terraform planning against a local mock, including omitted computed
// fields on import and update. No live controller credentials are required.
func TestAccDeviceSwitchPortPreservation(t *testing.T) {
	for _, isES := range []bool{false, true} {
		t.Run(fmt.Sprintf("agile_series=%t", isES), func(t *testing.T) {
			const mac = "02-00-00-00-00-01"
			const base = "/controller/api/v2/sites/site/switches/"
			var mu sync.Mutex
			patches := 0
			ports := []map[string]interface{}{
				{"port": 1, "name": "Uplink", "disable": true, "profileId": "profile", "profileOverrideEnable": true,
					"nativeNetworkId": "native", "networkTagsSetting": 1, "tagNetworkIds": []string{"tagged"},
					"untagNetworkIds": []string{"untagged"}, "voiceNetworkEnable": true, "voiceDscpEnable": true,
					"speed": 3, "linkSpeed": 0, "portStatus": map[string]interface{}{"linkStatus": 1}},
				{"port": 2, "name": "Other", "disable": false, "profileId": "other-profile", "profileOverrideEnable": false,
					"nativeNetworkId": "other-native", "networkTagsSetting": 0, "tagNetworkIds": []string{},
					"untagNetworkIds": []string{}, "voiceNetworkEnable": false, "voiceDscpEnable": false, "speed": 2},
			}
			config := map[string]interface{}{"mac": mac, "name": "Original", "ledSetting": 2, "es": isES,
				"ipSetting":            map[string]interface{}{"mode": "dhcp", "fallback": false},
				"loopbackDetectEnable": false, "stp": 0, "ports": ports}
			seriesPath := base + mac
			if isES {
				seriesPath = base + "es/" + mac
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				var result interface{}
				switch {
				case r.URL.Path == "/api/info":
					result = map[string]interface{}{"omadacId": "controller"}
				case r.URL.Path == "/controller/api/v2/login":
					result = map[string]interface{}{"token": "test-token"}
				case isES && r.Method == http.MethodGet && r.URL.Path == base+mac:
					fmt.Fprint(w, `{"errorCode":-39742,"msg":"Agile Series endpoint required"}`)
					return
				case r.Method == http.MethodGet && r.URL.Path == seriesPath:
					result = config
				case r.Method == http.MethodPut && (r.URL.Path == seriesPath+"/config/general" || r.URL.Path == seriesPath+"/config/service"):
					var update map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
						t.Error(err)
					}
					for key, value := range update {
						config[key] = value
					}
				case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, base+mac+"/ports/"):
					patches++
					number, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, base+mac+"/ports/"))
					if err != nil || number != 1 {
						t.Errorf("unexpected write to unchanged port: %s", r.URL.Path)
					}
					var update map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
						t.Error(err)
					}
					allowed := map[string]bool{"name": true, "disable": true, "profileId": true, "profileOverrideEnable": true,
						"nativeNetworkId": true, "networkTagsSetting": true, "tagNetworkIds": true, "untagNetworkIds": true,
						"voiceNetworkEnable": true, "voiceDscpEnable": true}
					for key, value := range update {
						if !allowed[key] {
							t.Errorf("PATCH included read-only or unsupported field %q", key)
						}
						if value == nil {
							t.Errorf("PATCH included null value for %q", key)
						}
						if number > 0 && number <= len(ports) {
							ports[number-1][key] = value
						}
					}
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				if err := json.NewEncoder(w).Encode(map[string]interface{}{"errorCode": 0, "result": result}); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			hcl := func(name, portConfig string) string {
				return fmt.Sprintf(`
provider "omada" {
  url = %q
  username = "test"
  password = "test"
}
resource "omada_device_switch" "test" {
  name = %q
  %s
}
`, server.URL, name, portConfig)
			}
			const minimal = `ports = [{ port = 1, name = "Uplink" }, { port = 2, name = "Other" }]`
			const renamed = `ports = [{ port = 1, name = "Renamed Uplink" }, { port = 2, name = "Other" }]`
			const configuredPort = `{
  port = 1
  name = "Renamed Uplink"
  disable = false
  profile_id = "new-profile"
  profile_override_enable = true
  native_network_id = "new-native"
  network_tags_setting = 1
  tag_network_ids = ["new-tag"]
  untag_network_ids = []
  voice_network_enable = false
  voice_dscp_enable = false
}`
			clearedPort := strings.ReplaceAll(configuredPort, `tag_network_ids = ["new-tag"]`, `tag_network_ids = []`)
			clearedPort = strings.ReplaceAll(clearedPort, "profile_override_enable = true", "profile_override_enable = false")
			clearedPort = strings.ReplaceAll(clearedPort, "network_tags_setting = 1", "network_tags_setting = 0")
			configured := "ports = [" + configuredPort + `, { port = 2, name = "Other" }]`
			cleared := "ports = [" + clearedPort + `, { port = 2, name = "Other" }]`
			reordered := `ports = [{ port = 2, name = "Other" }, ` + clearedPort + "]"
			checkWrites := func(want int) resource.TestCheckFunc {
				return func(_ *terraform.State) error {
					mu.Lock()
					defer mu.Unlock()
					if patches != want {
						return fmt.Errorf("port PATCH count = %d, want %d", patches, want)
					}
					return nil
				}
			}
			preserved := func(wantWrites int) resource.TestCheckFunc {
				return resource.ComposeAggregateTestCheckFunc(checkWrites(wantWrites),
					resource.TestCheckResourceAttr("omada_device_switch.test", "ports.0.disable", "true"),
					resource.TestCheckResourceAttr("omada_device_switch.test", "ports.0.profile_override_enable", "true"),
					resource.TestCheckResourceAttr("omada_device_switch.test", "ports.0.voice_network_enable", "true"),
					resource.TestCheckResourceAttr("omada_device_switch.test", "ports.0.voice_dscp_enable", "true"),
					resource.TestCheckResourceAttr("omada_device_switch.test", "ports.0.tag_network_ids.0", "tagged"),
					resource.TestCheckResourceAttr("omada_device_switch.test", "ports.0.untag_network_ids.0", "untagged"))
			}
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{Config: hcl("Original", "") + `
import {
  to = omada_device_switch.test
  id = "site/02-00-00-00-00-01"
}
`, Check: preserved(0)},
					{Config: hcl("Original", ""), ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
					}, Check: preserved(0)},
					{Config: hcl("Renamed Switch", minimal), Check: preserved(0)},
					{Config: hcl("Renamed Switch", minimal), ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
					}, Check: preserved(0)},
					{Config: hcl("Renamed Switch", renamed), Check: preserved(1)},
					{Config: hcl("Renamed Switch", configured), Check: resource.ComposeAggregateTestCheckFunc(checkWrites(2),
						resource.TestCheckResourceAttr("omada_device_switch.test", "ports.0.disable", "false"),
						resource.TestCheckResourceAttr("omada_device_switch.test", "ports.0.profile_id", "new-profile"),
						resource.TestCheckResourceAttr("omada_device_switch.test", "ports.0.native_network_id", "new-native"),
						resource.TestCheckResourceAttr("omada_device_switch.test", "ports.0.tag_network_ids.0", "new-tag"),
						resource.TestCheckResourceAttr("omada_device_switch.test", "ports.0.untag_network_ids.#", "0"),
						resource.TestCheckResourceAttr("omada_device_switch.test", "ports.0.voice_network_enable", "false"),
						resource.TestCheckResourceAttr("omada_device_switch.test", "ports.0.voice_dscp_enable", "false"))},
					{Config: hcl("Renamed Switch", cleared), Check: resource.ComposeAggregateTestCheckFunc(checkWrites(3),
						resource.TestCheckResourceAttr("omada_device_switch.test", "ports.0.profile_override_enable", "false"),
						resource.TestCheckResourceAttr("omada_device_switch.test", "ports.0.network_tags_setting", "0"),
						resource.TestCheckResourceAttr("omada_device_switch.test", "ports.0.tag_network_ids.#", "0"))},
					{Config: hcl("Renamed Switch", reordered), Check: resource.ComposeAggregateTestCheckFunc(checkWrites(3),
						resource.TestCheckResourceAttr("omada_device_switch.test", "ports.0.port", "2"),
						resource.TestCheckResourceAttr("omada_device_switch.test", "ports.1.port", "1"))},
					{Config: hcl("Another Switch Name", reordered), Check: checkWrites(3)},
					{Config: hcl("Another Switch Name", ""), Check: checkWrites(3)},
					{Config: hcl("Another Switch Name", ""), ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
					}, Check: checkWrites(3)},
				},
			})
		})
	}
}
