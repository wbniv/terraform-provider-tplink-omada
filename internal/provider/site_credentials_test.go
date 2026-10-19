package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type credentialPlan struct{ address string }

func (c credentialPlan) CheckPlan(_ context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	for _, change := range req.Plan.ResourceChanges {
		if change.Address == c.address && (change.Change.Actions.Delete() || change.Change.Actions.Replace()) {
			resp.Error = fmt.Errorf("credential operation planned destruction: %v", change.Change.Actions)
		}
	}
}

func TestAccSiteCredentials(t *testing.T) {
	for _, scenario := range []string{"masked", "other mask", "plaintext", "absent", "adopt", "unknown credentials", "rejected PATCH retry", "accepted PATCH read failure", "site ownership migration", "site bootstrap", "site unknown bootstrap", "partial credentials rejected"} {
		t.Run(scenario, func(t *testing.T) {
			data, err := os.ReadFile("../testdata/site_settings.json")
			if err != nil {
				t.Fatal(err)
			}
			var current map[string]any
			if err := json.Unmarshal(data, &current); err != nil {
				t.Fatal(err)
			}
			current["mesh"].(map[string]any)["futureControl"] = true
			var mu sync.Mutex
			patches, creates, deletes, credentialWrites := 0, 0, 0, 0
			failPatch, failReadback := false, false
			readFailures := 0
			remotePassword := "Original-Fixture-Password-123!"
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
				case r.URL.Path == "/controller/api/v2/sites" && r.Method == http.MethodPost:
					creates++
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					account, _ := body["deviceAccountSetting"].(map[string]any)
					if account["username"] != "admin" || account["password"] != "Bootstrap-Fixture-Password-123!" {
						t.Error("invalid bootstrap pair")
					}
					remotePassword = "Bootstrap-Fixture-Password-123!"
					result = map[string]any{"siteId": "site"}
				case r.URL.Path == "/controller/api/v2/sites/site" && r.Method == http.MethodDelete:
					deletes++
				case r.URL.Path == "/controller/api/v2/sites/site" && r.Method == http.MethodGet:
					s := current["site"].(map[string]any)
					result = map[string]any{"id": "site", "name": s["name"], "region": s["region"], "timeZone": s["timeZone"], "scenario": s["scenario"], "type": 0}
				case r.URL.Path == "/controller/api/v2/sites/site/setting" && r.Method == http.MethodPatch:
					patches++
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if failPatch {
						failPatch = false
						fmt.Fprint(w, `{"errorCode":-1001,"msg":"fixture rejected write"}`)
						return
					}
					for key, value := range body {
						if key != "deviceAccount" && key != "led" {
							t.Errorf("credential flow changed unrelated section %s", key)
						}
						current[key] = value
					}
					if account, ok := body["deviceAccount"].(map[string]any); ok {
						credentialWrites++
						username, uok := account["username"].(string)
						password, pok := account["password"].(string)
						if !uok || !pok || username == "" || password == "" || strings.HasPrefix(password, "*") {
							t.Error("incomplete or masked credentials sent")
						}
						remotePassword = password
					}
					if failReadback {
						failReadback = false
						readFailures = 1
					}
					// Force the provider to distinguish an accepted write from GET failure.
					result = nil
				case r.URL.Path == "/controller/api/v2/sites/site/setting" && r.Method == http.MethodGet:
					if readFailures > 0 {
						readFailures--
						fmt.Fprint(w, `{"errorCode":-1001,"msg":"fixture readback failure"}`)
						return
					}
					b, _ := json.Marshal(current)
					var snapshot map[string]any
					_ = json.Unmarshal(b, &snapshot)
					account := snapshot["deviceAccount"].(map[string]any)
					account["password"] = "***"
					switch scenario {
					case "other mask":
						account["password"] = "********"
					case "plaintext":
						account["password"] = "Untrusted-API-Password!"
					case "absent":
						delete(account, "password")
					}
					result = snapshot
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "result": result})
			}))
			defer server.Close()
			provider := fmt.Sprintf("provider \"omada\" {\n url = %q\n username = \"fixture\"\n password = \"fixture\"\n}\n", server.URL)
			settings := func(username, password, extra string) string {
				pair := ""
				if username != "" {
					pair = fmt.Sprintf("device_account_username = %q\ndevice_account_password = %q", username, password)
				}
				return provider + fmt.Sprintf("resource \"omada_site_settings\" \"test\" {\n site_id = \"site\"\n%s\n%s\n}\n", pair, extra)
			}
			site := func(pair string) string {
				return provider + fmt.Sprintf("resource \"omada_site\" \"test\" {\n name = \"Original\"\n region = \"United States\"\n time_zone = \"UTC\"\n scenario = \"Office\"\n%s\n}\n", pair)
			}
			check := func(writes, credentials int, password string) resource.TestCheckFunc {
				return func(state *terraform.State) error {
					mu.Lock()
					defer mu.Unlock()
					if patches != writes || credentialWrites != credentials || deletes != 0 {
						return fmt.Errorf("writes=%d credentials=%d deletes=%d, want writes=%d credentials=%d no deletes", patches, credentialWrites, deletes, writes, credentials)
					}
					if password != "" && remotePassword != password {
						return fmt.Errorf("remote password not updated")
					}
					if current["mesh"].(map[string]any)["meshEnable"] != true || current["mesh"].(map[string]any)["futureControl"] != true || current["roaming"].(map[string]any)["fastRoamingEnable"] != true {
						return fmt.Errorf("unrelated settings changed")
					}
					if password != "" {
						return resource.TestCheckResourceAttr("omada_site_settings.test", "device_account_password", password)(state)
					}
					return resource.TestCheckNoResourceAttr("omada_site_settings.test", "device_account_password")(state)
				}
			}
			const oldPassword = "Managed-Fixture-Password-123!"
			const newPassword = "Rotated-Fixture-Password-456!"
			plain := settings("", "", "")
			configured := settings("admin", oldPassword, "")
			rotated := settings("admin", newPassword, "")
			importBlock := "import {\n to = omada_site_settings.test\n id = \"site\"\n}\n"
			steps := []resource.TestStep{
				{Config: plain + importBlock, Check: check(0, 0, "")},
				{Config: configured, Check: check(1, 1, oldPassword)},
				{Config: rotated, ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{credentialPlan{"omada_site_settings.test"}}}, Check: check(2, 2, newPassword)},
				{Config: settings("rotated-admin", newPassword, ""), Check: check(3, 3, newPassword)},
				{Config: settings("combined-admin", oldPassword, ""), Check: check(4, 4, oldPassword)},
				{Config: settings("combined-admin", oldPassword, ""), PlanOnly: true},
				{Config: settings("combined-admin", oldPassword, "led_enable = true"), Check: check(5, 5, oldPassword)},
				{Config: settings("", "", "led_enable = true"), Check: check(5, 5, "")},
				{Config: settings("", "", "led_enable = true"), PlanOnly: true},
			}
			switch scenario {
			case "partial credentials rejected":
				partial := settings("", "", "device_account_username = \"admin\"")
				steps = []resource.TestStep{{Config: plain + importBlock, Check: check(0, 0, "")}, {Config: partial, PlanOnly: true, ExpectNonEmptyPlan: true, ExpectError: regexp.MustCompile("configure both")}, {Config: plain, Check: check(0, 0, "")}, {Config: plain, PlanOnly: true}}
			case "adopt":
				steps = []resource.TestStep{{Config: configured, Check: check(1, 1, oldPassword)}, {Config: rotated, Check: check(2, 2, newPassword)}, {Config: rotated, PlanOnly: true}}
			case "unknown credentials":
				config := settings("", "", "device_account_username = terraform_data.credentials.output.username\ndevice_account_password = terraform_data.credentials.output.password") + fmt.Sprintf("resource \"terraform_data\" \"credentials\" {\n input = { username = \"admin\", password = %q }\n}\n", newPassword)
				steps = []resource.TestStep{{Config: config, Check: check(1, 1, newPassword)}, {Config: config, PlanOnly: true}}
			case "rejected PATCH retry", "accepted PATCH read failure":
				failure := regexp.MustCompile("API error -1001")
				count := 3
				if scenario == "accepted PATCH read failure" {
					count = 2
				}
				steps = []resource.TestStep{{Config: configured, Check: check(1, 1, oldPassword)},
					{PreConfig: func() {
						mu.Lock()
						defer mu.Unlock()
						if scenario == "rejected PATCH retry" {
							failPatch = true
						} else {
							failReadback = true
						}
					}, Config: rotated, ExpectError: failure},
					{Config: rotated, Check: check(count, 2, newPassword)}, {Config: rotated, PlanOnly: true}}
			case "site ownership migration":
				imported := site("") + "import {\n to = omada_site.test\n id = \"site\"\n}\n"
				invalid := site(fmt.Sprintf("device_account_username = \"admin\"\ndevice_account_password = %q", newPassword))
				migrated := site("") + strings.TrimPrefix(rotated, provider)
				steps = []resource.TestStep{
					{Config: imported},
					{Config: invalid, PlanOnly: true, ExpectNonEmptyPlan: true, ExpectError: regexp.MustCompile("creation-only")},
					{Config: migrated, ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{credentialPlan{"omada_site.test"}}}, Check: resource.ComposeTestCheckFunc(check(1, 1, newPassword), resource.TestCheckResourceAttr("omada_site.test", "id", "site"))},
					{Config: migrated, PlanOnly: true},
				}
			case "site bootstrap", "site unknown bootstrap":
				pair := "device_account_username = \"admin\"\ndevice_account_password = \"Bootstrap-Fixture-Password-123!\""
				config := site(pair)
				if scenario == "site unknown bootstrap" {
					config = site("device_account_username = terraform_data.bootstrap.output.username\ndevice_account_password = terraform_data.bootstrap.output.password") + "resource \"terraform_data\" \"bootstrap\" {\n input = { username = \"admin\", password = \"Bootstrap-Fixture-Password-123!\" }\n}\n"
				}
				steps = []resource.TestStep{
					{Config: config, Check: resource.TestCheckResourceAttr("omada_site.test", "id", "site")},
					{Config: config, PlanOnly: true},
					{Config: site(""), Check: func(_ *terraform.State) error {
						mu.Lock()
						defer mu.Unlock()
						if creates != 1 || deletes != 0 || patches != 0 {
							return fmt.Errorf("removing bootstrap config mutated the site: create=%d delete=%d patch=%d", creates, deletes, patches)
						}
						return nil
					}},
					{Config: site(""), PlanOnly: true},
				}
			}
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: steps})
		})
	}
}
