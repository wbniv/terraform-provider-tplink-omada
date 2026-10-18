package provider

import (
	"context"
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
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Assert the actual Terraform plan, including the replacement reason and new
// identity being unknown. This catches modifier-order regressions on omission.
type ssidGroupPlan struct {
	actions, attribute, after string
	unknown                   bool
}

func (check ssidGroupPlan) CheckPlan(_ context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	for _, change := range req.Plan.ResourceChanges {
		if change.Address != "omada_wireless_network.test" {
			continue
		}
		if fmt.Sprint(change.Change.Actions) != check.actions {
			resp.Error = fmt.Errorf("SSID actions %v, want %s", change.Change.Actions, check.actions)
			return
		}
		after, _ := change.Change.After.(map[string]any)
		unknown, _ := change.Change.AfterUnknown.(map[string]any)
		if check.unknown {
			if unknown[check.attribute] != true {
				resp.Error = fmt.Errorf("%s must remain unknown in plan", check.attribute)
			}
		} else if check.attribute != "" && after[check.attribute] != check.after {
			resp.Error = fmt.Errorf("%s=%v, want %s", check.attribute, after[check.attribute], check.after)
		}
		if strings.Contains(check.actions, "delete") && strings.Contains(check.actions, "create") {
			found := false
			for _, path := range change.Change.ReplacePaths {
				if elements, ok := path.([]any); ok && len(elements) == 1 && elements[0] == check.attribute {
					found = true
				}
			}
			if !found || unknown["id"] != true {
				resp.Error = fmt.Errorf("replacement must cite %s and plan a new unknown ID; paths=%v unknown=%v", check.attribute, change.Change.ReplacePaths, unknown)
			}
		}
		return
	}
	resp.Error = fmt.Errorf("SSID missing from plan")
}

func TestAccWirelessNetworkGroupIdentity(t *testing.T) {
	for _, scenario := range []string{
		"import retains group", "replace with defaults", "replace with explicit settings",
		"create before destroy", "unknown changed group", "unknown same group", "site replacement",
		"create failure retry", "delete failure retry", "duplicate name retry",
		"create omitted group", "create unknown group", "secured replacement requires password",
	} {
		t.Run(scenario, func(t *testing.T) {
			data, err := os.ReadFile("../testdata/wireless_network.json")
			if err != nil {
				t.Fatal(err)
			}
			var original map[string]any
			if err := json.Unmarshal(data, &original); err != nil {
				t.Fatal(err)
			}
			original["id"], original["wlanId"], original["security"] = "ssid-old", "group-a", float64(0)
			original["vlanSetting"] = map[string]any{"mode": 0}
			if scenario == "secured replacement requires password" {
				original["security"] = float64(3)
			} else {
				delete(original, "pskSetting")
			}
			objects := map[string]map[string]any{"site/group-a/ssid-old": original}
			if strings.HasPrefix(scenario, "create omitted") || strings.HasPrefix(scenario, "create unknown") {
				objects = map[string]map[string]any{}
			}
			var mu sync.Mutex
			var events []string
			serial, discoveries := 0, 0
			failCreate, failDelete, rejectDuplicate := false, false, scenario == "duplicate name retry"
			pathPattern := regexp.MustCompile(`^/controller/api/v2/sites/([^/]+)/setting/wlans/([^/]+)/ssids(?:/([^/]+))?$`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				var result any
				switch r.URL.Path {
				case "/api/info":
					result = map[string]any{"omadacId": "controller"}
				case "/controller/api/v2/login":
					result = map[string]any{"token": "token"}
				case "/controller/api/v2/sites/site/setting/wlans":
					discoveries++
					result = []any{map[string]any{"id": "group-a", "name": "Default"}, map[string]any{"id": "group-b", "name": "Other"}}
				default:
					parts := pathPattern.FindStringSubmatch(r.URL.Path)
					if parts == nil {
						t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
						http.Error(w, "unexpected path", 500)
						return
					}
					collection, key := parts[1]+"/"+parts[2], parts[1]+"/"+parts[2]+"/"+parts[3]
					if r.Method != http.MethodGet {
						events = append(events, r.Method+" "+strings.TrimSuffix(key, "/"))
					}
					switch r.Method {
					case http.MethodGet:
						items := []any{}
						for id, object := range objects {
							if strings.HasPrefix(id, collection+"/") {
								items = append(items, object)
							}
						}
						result = map[string]any{"data": items, "totalRows": len(items)}
					case http.MethodPost:
						var body map[string]any
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Error(err)
						}
						if scenario == "secured replacement requires password" {
							psk, _ := body["pskSetting"].(map[string]any)
							if body["security"] != float64(3) || psk["securityKey"] != "fixture-new-password" {
								t.Error("replacement did not use configured security/password")
							}
						}
						if failCreate {
							failCreate = false
							fmt.Fprint(w, `{"errorCode":-1001,"msg":"fixture create failure"}`)
							return
						}
						if rejectDuplicate {
							for _, object := range objects {
								if object["name"] == body["name"] {
									fmt.Fprint(w, `{"errorCode":-1001,"msg":"fixture duplicate name"}`)
									return
								}
							}
						}
						serial++
						id := fmt.Sprintf("ssid-new%d", serial)
						body["id"], body["wlanId"] = id, parts[2]
						objects[collection+"/"+id] = body
						result = map[string]any{"ssidId": id}
					case http.MethodDelete:
						if _, exists := objects[key]; !exists {
							t.Errorf("DELETE used absent identity %s", key)
						}
						if failDelete {
							failDelete = false
							fmt.Fprint(w, `{"errorCode":-1001,"msg":"fixture delete failure"}`)
							return
						}
						delete(objects, key)
					case http.MethodPatch:
						if _, exists := objects[key]; !exists {
							t.Errorf("PATCH used absent identity %s", key)
						}
						var body map[string]any
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Error(err)
						}
						body["id"], body["wlanId"] = parts[3], parts[2]
						objects[key] = body
					default:
						t.Errorf("unexpected method %s", r.Method)
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "result": result})
			}))
			defer server.Close()
			config := func(site, group, name, extra, dependency string) string {
				return fmt.Sprintf(`
provider "omada" {
  url = %q
  username = "fixture"
  password = "fixture"
}
%s
resource "omada_wireless_network" "test" {
  site_id = %q
  %s
  name = %q
  security = 0
  %s
}
`, server.URL, dependency, site, group, name, extra)
			}
			groupA, groupB := `wlan_group_id = "group-a"`, `wlan_group_id = "group-b"`
			initial := config("site", groupA, "Original", "", "")
			if scenario == "secured replacement requires password" {
				initial = strings.Replace(initial, "security = 0", "security = 3", 1)
			}
			imported := initial + `
import {
  to = omada_wireless_network.test
  id = "site/group-a/ssid-old"
}`
			check := func(identity string, wantEvents []string, wantDiscoveries int) resource.TestCheckFunc {
				return func(state *terraform.State) error {
					mu.Lock()
					defer mu.Unlock()
					if !reflect.DeepEqual(events, wantEvents) || discoveries != wantDiscoveries {
						return fmt.Errorf("events=%v discoveries=%d; want %v discoveries=%d", events, discoveries, wantEvents, wantDiscoveries)
					}
					if len(objects) != 1 || objects[identity] == nil {
						return fmt.Errorf("expected sole remote identity %s; got %v", identity, objects)
					}
					parts := strings.Split(identity, "/")
					return resource.ComposeTestCheckFunc(resource.TestCheckResourceAttr("omada_wireless_network.test", "site_id", parts[0]), resource.TestCheckResourceAttr("omada_wireless_network.test", "wlan_group_id", parts[1]), resource.TestCheckResourceAttr("omada_wireless_network.test", "id", parts[2]))(state)
				}
			}
			checks := func(actions, attribute, after string, unknown bool) resource.ConfigPlanChecks {
				return resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{ssidGroupPlan{actions: actions, attribute: attribute, after: after, unknown: unknown}}}
			}
			steps := []resource.TestStep{{Config: imported, Check: check("site/group-a/ssid-old", nil, 0)}}
			final := config("site", groupB, "Original", "", "")
			wantEvents := []string{"DELETE site/group-a/ssid-old", "POST site/group-b"}
			finalIdentity := "site/group-b/ssid-new1"
			actions, attr, after, unknown := "[delete create]", "wlan_group_id", "group-b", false
			switch scenario {
			case "import retains group":
				omitted := config("site", "", "Original", "", "")
				final = config("site", "", "Renamed", "", "")
				steps = append(steps,
					resource.TestStep{Config: omitted, ConfigPlanChecks: checks("[no-op]", "wlan_group_id", "group-a", false), Check: check("site/group-a/ssid-old", nil, 0)},
					resource.TestStep{Config: initial, ConfigPlanChecks: checks("[no-op]", "wlan_group_id", "group-a", false), Check: check("site/group-a/ssid-old", nil, 0)},
					resource.TestStep{ResourceName: "omada_wireless_network.test", ImportState: true, ImportStateId: "site/group-a/ssid-old", ImportStateVerify: true},
				)
				wantEvents, finalIdentity = []string{"PATCH site/group-a/ssid-old"}, "site/group-a/ssid-old"
				actions, after = "[update]", "group-a"
			case "replace with explicit settings":
				final = config("site", groupB, "Original", "band = 1\nbroadcast = false\nenable_11r = true\npmf_mode = 3", "")
			case "secured replacement requires password":
				final = strings.Replace(final, "security = 0", "security = 3", 1)
				steps = append(steps,
					resource.TestStep{Config: final, PlanOnly: true, ExpectNonEmptyPlan: true, ExpectError: regexp.MustCompile("configure passphrase before creating")},
					resource.TestStep{Config: initial, Check: check("site/group-a/ssid-old", nil, 0)},
				)
				final = strings.Replace(final, "security = 3", "security = 3\npassphrase = \"fixture-new-password\"", 1)
			case "create before destroy", "duplicate name retry":
				final = config("site", groupB, "Original", "lifecycle { create_before_destroy = true }", "")
				actions = "[create delete]"
				wantEvents = []string{"POST site/group-b", "DELETE site/group-a/ssid-old"}
				if scenario == "duplicate name retry" {
					steps = append(steps, resource.TestStep{Config: final, ExpectError: regexp.MustCompile("fixture duplicate name")}, resource.TestStep{Config: initial, PlanOnly: true, Check: check("site/group-a/ssid-old", []string{"POST site/group-b"}, 0)})
					final = config("site", groupB, "Renamed", "lifecycle { create_before_destroy = true }", "")
					wantEvents = []string{"POST site/group-b", "POST site/group-b", "DELETE site/group-a/ssid-old"}
				}
			case "unknown changed group", "unknown same group":
				value := "group-b"
				if scenario == "unknown same group" {
					value = "group-a"
					finalIdentity = "site/group-a/ssid-new1"
					wantEvents[1] = "POST site/group-a"
				}
				final = config("site", "wlan_group_id = terraform_data.group.output", "Original", "", fmt.Sprintf("resource \"terraform_data\" \"group\" { input = %q }", value))
				unknown = true
			case "site replacement":
				final = config("other-site", groupA, "Original", "", "")
				attr, after = "site_id", "other-site"
				finalIdentity, wantEvents[1] = "other-site/group-a/ssid-new1", "POST other-site/group-a"
			case "create failure retry":
				steps = append(steps, resource.TestStep{PreConfig: func() { mu.Lock(); defer mu.Unlock(); failCreate = true }, Config: final, ConfigPlanChecks: checks(actions, attr, after, unknown), ExpectError: regexp.MustCompile("fixture create failure")})
				wantEvents = append(wantEvents, "POST site/group-b")
				actions = "[create]"
			case "delete failure retry":
				steps = append(steps, resource.TestStep{PreConfig: func() { mu.Lock(); defer mu.Unlock(); failDelete = true }, Config: final, ConfigPlanChecks: checks(actions, attr, after, unknown), ExpectError: regexp.MustCompile("fixture delete failure")}, resource.TestStep{Config: initial, PlanOnly: true, Check: check("site/group-a/ssid-old", []string{"DELETE site/group-a/ssid-old"}, 0)})
				wantEvents = append([]string{"DELETE site/group-a/ssid-old"}, wantEvents...)
			case "create omitted group", "create unknown group":
				steps = nil
				actions, unknown = "[create]", true
				if scenario == "create omitted group" {
					final = config("site", "", "Created", "", "")
					finalIdentity, wantEvents = "site/group-a/ssid-new1", []string{"POST site/group-a"}
				} else {
					final = config("site", "wlan_group_id = terraform_data.group.output", "Created", "", `resource "terraform_data" "group" { input = "group-b" }`)
					wantEvents = []string{"POST site/group-b"}
				}
			}
			wantDiscoveries := 0
			if scenario == "create omitted group" {
				wantDiscoveries = 1
			}
			finalCheck := check(finalIdentity, wantEvents, wantDiscoveries)
			if scenario == "replace with defaults" || scenario == "replace with explicit settings" {
				values := map[string]string{"band": "3", "broadcast": "true", "enable_11r": "false", "pmf_mode": "2"}
				if scenario == "replace with explicit settings" {
					values = map[string]string{"band": "1", "broadcast": "false", "enable_11r": "true", "pmf_mode": "3"}
				}
				all := []resource.TestCheckFunc{finalCheck}
				for name, value := range values {
					all = append(all, resource.TestCheckResourceAttr("omada_wireless_network.test", name, value))
				}
				all = append(all, func(_ *terraform.State) error {
					mu.Lock()
					defer mu.Unlock()
					if objects[finalIdentity]["guestNetEnable"] == true || objects[finalIdentity]["wlanScheduleEnable"] == true {
						return fmt.Errorf("replacement unexpectedly preserved unmanaged settings")
					}
					return nil
				})
				finalCheck = resource.ComposeTestCheckFunc(all...)
			}
			steps = append(steps, resource.TestStep{Config: final, ConfigPlanChecks: checks(actions, attr, after, unknown), Check: finalCheck}, resource.TestStep{Config: final, PlanOnly: true, ExpectNonEmptyPlan: false, Check: check(finalIdentity, wantEvents, wantDiscoveries)})
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: steps, CheckDestroy: func(_ *terraform.State) error {
				mu.Lock()
				defer mu.Unlock()
				if len(objects) != 0 {
					return fmt.Errorf("objects left after destroy: %v", objects)
				}
				return nil
			}})
		})
	}
}
