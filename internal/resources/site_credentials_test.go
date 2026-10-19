package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/terraform-provider-tplink-omada/internal/client"
)

const credentialOld = "Old-Fixture-Password-123!"
const credentialNew = "New-Fixture-Password-456!"

func credentialSnapshot(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile("../testdata/site_settings.json")
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func credentialModel(t *testing.T, raw map[string]any) SiteSettingsResourceModel {
	t.Helper()
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var remote client.SiteSettings
	if err := json.Unmarshal(data, &remote); err != nil {
		t.Fatal(err)
	}
	model := SiteSettingsResourceModel{ID: types.StringValue("site"), SiteID: types.StringValue("site"), DeviceAccountPassword: types.StringValue(credentialOld)}
	siteSettingsToState(&remote, &model)
	return model
}

func credentialAuth(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case "/api/info":
		fmt.Fprint(w, `{"errorCode":0,"result":{"omadacId":"controller"}}`)
		return true
	case "/controller/api/v2/login":
		fmt.Fprint(w, `{"errorCode":0,"result":{"token":"token"}}`)
		return true
	}
	return false
}

func TestSiteSettingsCredentials(t *testing.T) {
	for _, tc := range []struct {
		name, operation, response, failure                                                                                                                          string
		username, password, omitted, partial, partialPassword, empty, unknown, imported, unknownSettings, unknownSibling, changedSection, readOnlySibling, mismatch bool
		wantError                                                                                                                                                   bool
	}{
		{name: "password rotation", password: true},
		{name: "username rotation", username: true},
		{name: "combined rotation", username: true, password: true},
		{name: "imported null password rotation", password: true, imported: true},
		{name: "adopt with credentials", operation: "create", password: true, imported: true},
		{name: "adopt without credentials", operation: "create", omitted: true, imported: true},
		{name: "omit both on update", omitted: true},
		{name: "partial username only", partial: true, wantError: true},
		{name: "partial password only", partialPassword: true, wantError: true},
		{name: "empty password", empty: true, wantError: true},
		{name: "adopt PATCH rejected", operation: "create", password: true, imported: true, failure: "patch", wantError: true},
		{name: "adopt accepted readback fails", operation: "create", password: true, imported: true, failure: "readback", wantError: true},
		{name: "readback account missing", password: true, response: "missing-account", wantError: true},
		{name: "unresolved password", unknown: true, wantError: true},
		{name: "read fails before write", password: true, failure: "read", wantError: true},
		{name: "PATCH rejected", password: true, failure: "patch", wantError: true},
		{name: "accepted PATCH readback fails", password: true, failure: "readback", wantError: true},
		{name: "readback username mismatch", username: true, mismatch: true, wantError: true},
		{name: "PATCH response empty", password: true, response: "empty"},
		{name: "PATCH response plaintext", password: true, response: "plaintext"},
		{name: "PATCH response other mask", password: true, response: "other-mask"},
		{name: "unknown unrelated settings", unknownSettings: true},
		{name: "unrelated unknown section preserved", password: true, unknownSibling: true},
		{name: "changed section preserves siblings", password: true, changedSection: true},
		{name: "changed section rejects unknown sibling", password: true, unknownSibling: true, changedSection: true, wantError: true},
		{name: "site read-only key excluded", password: true, readOnlySibling: true},
		{name: "refresh mask", operation: "read"},
		{name: "refresh other mask", operation: "read", response: "other-mask"},
		{name: "refresh plaintext", operation: "read", response: "plaintext"},
		{name: "refresh absent password", operation: "read", response: "absent"},
		{name: "import mask", operation: "import", imported: true},
		{name: "import plaintext", operation: "import", response: "plaintext", imported: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			current := credentialSnapshot(t)
			if tc.unknownSibling {
				current["mesh"].(map[string]any)["futureControl"] = true
			}
			prior := credentialModel(t, current)
			if tc.imported {
				prior.DeviceAccountPassword = types.StringNull()
			}
			plan := prior
			if tc.password {
				plan.DeviceAccountPassword = types.StringValue(credentialNew)
			}
			if tc.username {
				plan.DeviceAccountUsername = types.StringValue("rotated-admin")
			}
			config := SiteSettingsResourceModel{SiteID: prior.SiteID, DeviceAccountUsername: plan.DeviceAccountUsername, DeviceAccountPassword: plan.DeviceAccountPassword}
			if tc.omitted {
				config.DeviceAccountUsername = types.StringNull()
				config.DeviceAccountPassword = types.StringNull()
				plan.DeviceAccountPassword = types.StringNull()
				plan.LEDEnable = types.BoolValue(true)
				config.LEDEnable = plan.LEDEnable
			}
			if tc.partial {
				config.DeviceAccountPassword = types.StringNull()
				plan.DeviceAccountPassword = types.StringNull()
			}
			if tc.partialPassword {
				config.DeviceAccountUsername = types.StringNull()
			}
			if tc.empty {
				config.DeviceAccountPassword = types.StringValue("")
				plan.DeviceAccountPassword = types.StringValue("")
			}
			if tc.unknown {
				config.DeviceAccountPassword = types.StringUnknown()
				plan.DeviceAccountPassword = types.StringUnknown()
			}
			if tc.unknownSettings {
				plan.MeshEnable = types.BoolUnknown()
				plan.MeshAutoFailover = types.BoolUnknown()
				plan.FastRoamingEnable = types.BoolUnknown()
			}
			if tc.changedSection {
				plan.MeshEnable = types.BoolValue(false)
				config.MeshEnable = plan.MeshEnable
			}
			if tc.readOnlySibling {
				plan.SiteName = types.StringValue("Renamed")
				config.SiteName = plan.SiteName
			}
			var mu sync.Mutex
			writes, reads := 0, 0
			var sent map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if credentialAuth(w, r) {
					return
				}
				mu.Lock()
				defer mu.Unlock()
				if r.URL.Path != "/controller/api/v2/sites/site/setting" {
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				if r.Method == http.MethodPatch {
					writes++
					if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
						t.Error(err)
					}
					if tc.failure == "patch" {
						fmt.Fprintf(w, `{"errorCode":-1001,"msg":"rejected %s"}`, credentialNew)
						return
					}
					for k, v := range sent {
						current[k] = v
					}
					if tc.response == "empty" || tc.failure == "readback" {
						fmt.Fprint(w, `{"errorCode":0,"result":null}`)
						return
					}
				} else if r.Method == http.MethodGet {
					reads++
					if tc.failure == "read" || (tc.failure == "readback" && writes > 0) {
						fmt.Fprint(w, `{"errorCode":-1001,"msg":"fixture read failure"}`)
						return
					}
				} else {
					t.Errorf("unexpected method %s", r.Method)
				}
				b, _ := json.Marshal(current)
				var observed map[string]any
				_ = json.Unmarshal(b, &observed)
				account := observed["deviceAccount"].(map[string]any)
				account["password"] = "***"
				switch tc.response {
				case "plaintext":
					account["password"] = "Untrusted-API-Password!"
				case "other-mask":
					account["password"] = "********"
				case "absent":
					delete(account, "password")
				}
				if tc.mismatch && writes > 0 {
					account["username"] = "admin"
				}
				if tc.response == "missing-account" && writes > 0 {
					delete(observed, "deviceAccount")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "result": observed})
			}))
			defer server.Close()
			c, err := client.NewClient(server.URL, "fixture", "fixture", false)
			if err != nil {
				t.Fatal(err)
			}
			r := &SiteSettingsResource{client: c}
			var schema resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schema)
			old := tfsdk.State{Schema: schema.Schema}
			next := tfsdk.Plan{Schema: schema.Schema}
			configured := tfsdk.State{Schema: schema.Schema}
			if d := old.Set(ctx, &prior); d.HasError() {
				t.Fatal(d)
			}
			if d := next.Set(ctx, &plan); d.HasError() {
				t.Fatal(d)
			}
			if d := configured.Set(ctx, &config); d.HasError() {
				t.Fatal(d)
			}
			var actual tfsdk.State
			var diagnostics string
			var hasError bool
			switch tc.operation {
			case "create":
				resp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema, Raw: next.Raw}}
				r.Create(ctx, resource.CreateRequest{Plan: next, Config: tfsdk.Config{Schema: schema.Schema, Raw: configured.Raw}}, &resp)
				actual, diagnostics, hasError = resp.State, fmt.Sprint(resp.Diagnostics), resp.Diagnostics.HasError()
			case "read":
				resp := resource.ReadResponse{State: old}
				r.Read(ctx, resource.ReadRequest{State: old}, &resp)
				actual, diagnostics, hasError = resp.State, fmt.Sprint(resp.Diagnostics), resp.Diagnostics.HasError()
			case "import":
				resp := resource.ImportStateResponse{State: tfsdk.State{Schema: schema.Schema}}
				r.ImportState(ctx, resource.ImportStateRequest{ID: "site"}, &resp)
				actual, diagnostics, hasError = resp.State, fmt.Sprint(resp.Diagnostics), resp.Diagnostics.HasError()
			default:
				resp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema, Raw: next.Raw}}
				r.Update(ctx, resource.UpdateRequest{State: old, Plan: next, Config: tfsdk.Config{Schema: schema.Schema, Raw: configured.Raw}}, &resp)
				actual, diagnostics, hasError = resp.State, fmt.Sprint(resp.Diagnostics), resp.Diagnostics.HasError()
			}
			if hasError != tc.wantError {
				t.Fatalf("error=%v want=%v diagnostics=%s", hasError, tc.wantError, diagnostics)
			}
			if strings.Contains(diagnostics, credentialNew) || strings.Contains(diagnostics, credentialOld) {
				t.Fatal("credential leaked through diagnostics")
			}
			mu.Lock()
			defer mu.Unlock()
			if tc.partial || tc.partialPassword || tc.empty || tc.unknown || tc.failure == "read" || (tc.changedSection && tc.unknownSibling) {
				if writes != 0 || !actual.Raw.Equal(old.Raw) {
					t.Fatalf("validation/discovery changed state or wrote: writes=%d", writes)
				}
				return
			}
			if tc.failure == "patch" {
				if tc.operation == "create" {
					if !actual.Raw.IsNull() {
						t.Fatal("rejected adopt saved planned state")
					}
				} else if !actual.Raw.Equal(old.Raw) {
					t.Fatal("rejected PATCH saved planned values")
				}
				return
			}
			var got SiteSettingsResourceModel
			if d := actual.Get(ctx, &got); d.HasError() {
				t.Fatal(d)
			}
			want := plan.DeviceAccountPassword
			if tc.operation == "read" {
				want = prior.DeviceAccountPassword
			}
			if tc.operation == "import" {
				want = types.StringNull()
			}
			if !got.DeviceAccountPassword.Equal(want) {
				t.Fatal("state did not retain the configured/accepted secret ownership")
			}
			if tc.operation == "read" || tc.operation == "import" {
				if writes != 0 {
					t.Fatal("read/import wrote settings")
				}
				return
			}
			if writes != 1 {
				t.Fatalf("PATCH count %d want 1", writes)
			}
			if tc.omitted {
				if _, exists := sent["deviceAccount"]; exists {
					t.Fatal("omitted credentials sent")
				}
			} else {
				account, _ := sent["deviceAccount"].(map[string]any)
				if account["password"] != plan.DeviceAccountPassword.ValueString() || account["username"] != plan.DeviceAccountUsername.ValueString() {
					t.Fatal("PATCH did not contain configured pair")
				}
			}
			for k := range sent {
				if k != "deviceAccount" && !(tc.omitted && k == "led") && !(tc.changedSection && k == "mesh") && !(tc.readOnlySibling && k == "site") {
					t.Errorf("unrelated section %s written", k)
				}
			}
			if tc.changedSection {
				section := sent["mesh"].(map[string]any)
				if section["meshEnable"] != false || section["autoFailoverEnable"] != true || section["defGatewayEnable"] != true || section["fullSector"] != true {
					t.Error("changed section lost writable siblings")
				}
			}
			if tc.readOnlySibling {
				if _, exists := sent["site"].(map[string]any)["key"]; exists {
					t.Error("read-only site key sent")
				}
			}
			if tc.unknownSettings && (!got.MeshEnable.ValueBool() || !got.MeshAutoFailover.ValueBool() || !got.FastRoamingEnable.ValueBool()) {
				t.Error("unknown settings were zeroed")
			}
			if tc.unknownSibling && !reflect.DeepEqual(current["mesh"], credentialSnapshotWithExtra(t)) {
				t.Error("unrelated unknown setting was lost")
			}
		})
	}
}

func credentialSnapshotWithExtra(t *testing.T) any {
	v := credentialSnapshot(t)["mesh"].(map[string]any)
	v["futureControl"] = true
	return v
}

func TestSiteCredentialOwnership(t *testing.T) {
	for _, mode := range []string{"password", "username", "both", "imported", "unknown", "partial removal", "remove both", "metadata"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			old := SiteResourceModel{ID: types.StringValue("site"), Name: types.StringValue("Original"), Region: types.StringValue("United States"), TimeZone: types.StringValue("UTC"), Scenario: types.StringValue("Office"), Type: types.Int64Value(0), DeviceAccountUsername: types.StringValue("admin"), DeviceAccountPassword: types.StringValue(credentialOld)}
			plan := old
			switch mode {
			case "password":
				plan.DeviceAccountPassword = types.StringValue(credentialNew)
			case "username":
				plan.DeviceAccountUsername = types.StringValue("rotated-admin")
			case "both":
				plan.DeviceAccountUsername = types.StringValue("rotated-admin")
				plan.DeviceAccountPassword = types.StringValue(credentialNew)
			case "imported":
				old.DeviceAccountUsername = types.StringNull()
				old.DeviceAccountPassword = types.StringNull()
			case "unknown":
				plan.DeviceAccountPassword = types.StringUnknown()
			case "partial removal":
				plan.DeviceAccountPassword = types.StringNull()
			case "remove both":
				plan.DeviceAccountUsername = types.StringNull()
				plan.DeviceAccountPassword = types.StringNull()
			case "metadata":
				plan.Name = types.StringValue("Renamed")
			}
			var mu sync.Mutex
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if credentialAuth(w, r) {
					return
				}
				mu.Lock()
				defer mu.Unlock()
				if r.Method == http.MethodPatch {
					writes++
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "result": map[string]any{"id": "site", "name": plan.Name.ValueString(), "region": "United States", "timeZone": "UTC", "scenario": "Office", "type": 0}})
			}))
			defer server.Close()
			c, err := client.NewClient(server.URL, "fixture", "fixture", false)
			if err != nil {
				t.Fatal(err)
			}
			r := &SiteResource{client: c}
			var schema resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schema)
			state := tfsdk.State{Schema: schema.Schema}
			planned := tfsdk.Plan{Schema: schema.Schema}
			if d := state.Set(ctx, &old); d.HasError() {
				t.Fatal(d)
			}
			if d := planned.Set(ctx, &plan); d.HasError() {
				t.Fatal(d)
			}
			resp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema, Raw: planned.Raw}}
			r.Update(ctx, resource.UpdateRequest{State: state, Plan: planned}, &resp)
			mu.Lock()
			defer mu.Unlock()
			if mode != "metadata" && mode != "remove both" {
				if !resp.Diagnostics.HasError() || writes != 0 || !resp.State.Raw.Equal(state.Raw) {
					t.Fatalf("credential change must fail before writes with prior state, got writes=%d diagnostics=%v", writes, resp.Diagnostics)
				}
				return
			}
			wantWrites := 0
			if mode == "metadata" {
				wantWrites = 1
			}
			if resp.Diagnostics.HasError() || writes != wantWrites {
				t.Fatalf("writes=%d diagnostics=%v", writes, resp.Diagnostics)
			}
			var got SiteResourceModel
			if d := resp.State.Get(ctx, &got); d.HasError() {
				t.Fatal(d)
			}
			if !got.DeviceAccountPassword.Equal(plan.DeviceAccountPassword) {
				t.Fatal("wrong retained/removed credential state")
			}
		})
	}
}

func TestSiteCredentialBootstrapValidation(t *testing.T) {
	for _, tc := range []struct {
		name               string
		username, password types.String
	}{
		{"both omitted", types.StringNull(), types.StringNull()},
		{"username omitted", types.StringNull(), types.StringValue(credentialNew)},
		{"password omitted", types.StringValue("admin"), types.StringNull()},
		{"empty username", types.StringValue(""), types.StringValue(credentialNew)},
		{"empty password", types.StringValue("admin"), types.StringValue("")},
		{"unknown username", types.StringUnknown(), types.StringValue(credentialNew)},
		{"unknown password", types.StringValue("admin"), types.StringUnknown()},
		{"both unknown", types.StringUnknown(), types.StringUnknown()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			var mu sync.Mutex
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if credentialAuth(w, r) {
					return
				}
				mu.Lock()
				defer mu.Unlock()
				if r.Method == http.MethodPost {
					writes++
					fmt.Fprint(w, `{"errorCode":0,"result":{"siteId":"site"}}`)
					return
				}
				fmt.Fprint(w, `{"errorCode":0,"result":{"id":"site","name":"Original","region":"United States","timeZone":"UTC","scenario":"Office","type":0}}`)
			}))
			defer server.Close()
			c, err := client.NewClient(server.URL, "fixture", "fixture", false)
			if err != nil {
				t.Fatal(err)
			}
			r := &SiteResource{client: c}
			var schema resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schema)
			model := SiteResourceModel{ID: types.StringUnknown(), Name: types.StringValue("Original"), Region: types.StringValue("United States"), TimeZone: types.StringValue("UTC"), Scenario: types.StringValue("Office"), Type: types.Int64Value(0), DeviceAccountUsername: tc.username, DeviceAccountPassword: tc.password}
			plan := tfsdk.Plan{Schema: schema.Schema}
			if d := plan.Set(ctx, &model); d.HasError() {
				t.Fatal(d)
			}
			resp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema, Raw: plan.Raw}}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
			mu.Lock()
			defer mu.Unlock()
			if !resp.Diagnostics.HasError() || writes != 0 || !resp.State.Raw.IsNull() {
				t.Fatalf("invalid bootstrap must fail without a write/state; writes=%d diagnostics=%v", writes, resp.Diagnostics)
			}
		})
	}
}
