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

func TestWirelessNetworkPreservation(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		edit                      func(*WirelessNetworkResourceModel, *WirelessNetworkResourceModel)
		mutate                    func(map[string]any)
		fail, response, wantError string
		noWrite, array            bool
		vlan                      int
	}{
		{name: "rename"},
		{name: "direct array", array: true},
		{name: "partial PATCH result", response: "partial"},
		{name: "full PATCH result", response: "full"},
		{name: "absent optional fields and explicit zero", mutate: func(raw map[string]any) {
			delete(raw, "mloEnable")
			raw["multiCastSetting"].(map[string]any)["channelUtil"] = float64(0)
		}},
		{name: "incomplete beacon settings", mutate: func(raw map[string]any) {
			delete(raw["rateAndBeaconCtrl"].(map[string]any), "lowerDensity2g")
		}, wantError: "missing rateAndBeaconCtrl.lowerDensity2g", noWrite: true},
		{name: "empty filter association", mutate: func(raw map[string]any) { raw["macFilterId"] = "" }, wantError: "missing SSID.macFilterId", noWrite: true},
		{name: "incomplete rate limits", mutate: func(raw map[string]any) {
			limits := raw["rateLimit"].(map[string]any)
			delete(limits, "rateLimitId")
			delete(limits, "downLimit")
		}, wantError: "missing rateLimit.downLimit", noWrite: true},
		{name: "incomplete VLAN association", mutate: func(raw map[string]any) {
			raw["vlanSetting"].(map[string]any)["customConfig"].(map[string]any)["lanNetworkVlanIds"] = map[string]any{}
		}, wantError: "incomplete VLAN network association", noWrite: true},
		{name: "unsupported VLAN custom mode", mutate: func(raw map[string]any) {
			raw["vlanSetting"].(map[string]any)["customConfig"].(map[string]any)["customMode"] = float64(1)
		}, wantError: "unsupported VLAN custom mode", noWrite: true},
		{name: "no-op", edit: func(p, s *WirelessNetworkResourceModel) { p.Name = s.Name }, noWrite: true},
		{name: "unknown computed settings", edit: func(p, s *WirelessNetworkResourceModel) {
			p.Band = types.Int64Unknown()
			p.Security = types.Int64Unknown()
			p.Broadcast = types.BoolUnknown()
			p.Enable11r = types.BoolUnknown()
			p.PmfMode = types.Int64Unknown()
			p.VlanID = types.Int64Unknown()
		}},
		{name: "null computed settings", edit: func(p, s *WirelessNetworkResourceModel) {
			p.Band = types.Int64Null()
			p.Security = types.Int64Null()
			p.Broadcast = types.BoolNull()
			p.Enable11r = types.BoolNull()
			p.PmfMode = types.Int64Null()
		}},
		{name: "password rotation", edit: func(p, s *WirelessNetworkResourceModel) { p.Passphrase = types.StringValue("rotated-fixture-password") }},
		{name: "legacy state key is untrusted", edit: func(p, s *WirelessNetworkResourceModel) {
			p.Passphrase = types.StringNull()
			s.Passphrase = types.StringValue("********")
		}, wantError: "configure passphrase", noWrite: true},
		{name: "missing trusted key", edit: func(p, s *WirelessNetworkResourceModel) {
			p.Passphrase = types.StringNull()
			s.Passphrase = types.StringNull()
		}, wantError: "configure passphrase", noWrite: true},
		{name: "empty key", edit: func(p, s *WirelessNetworkResourceModel) { p.Passphrase = types.StringValue("") }, wantError: "configure passphrase", noWrite: true},
		{name: "unknown key", edit: func(p, s *WirelessNetworkResourceModel) { p.Passphrase = types.StringUnknown() }, wantError: "must be known", noWrite: true},
		{name: "unknown root field", mutate: func(raw map[string]any) { raw["newFeature"] = true }, wantError: "unsupported SSID field", noWrite: true},
		{name: "unknown nested field", mutate: func(raw map[string]any) { raw["pskSetting"].(map[string]any)["newFeature"] = true }, wantError: "unsupported SSID field", noWrite: true},
		{name: "incomplete snapshot", mutate: func(raw map[string]any) { delete(raw, "guestNetEnable") }, wantError: "missing SSID.guestNetEnable", noWrite: true},
		{name: "incomplete schedule", mutate: func(raw map[string]any) { delete(raw, "scheduleId") }, wantError: "missing SSID.scheduleId", noWrite: true},
		{name: "incomplete filter", mutate: func(raw map[string]any) { delete(raw, "macFilterId") }, wantError: "missing SSID.macFilterId", noWrite: true},
		{name: "incomplete rekey", mutate: func(raw map[string]any) { delete(raw["pskSetting"].(map[string]any), "rekeyPskInterval") }, wantError: "missing pskSetting.rekeyPskInterval", noWrite: true},
		{name: "incomplete multicast", mutate: func(raw map[string]any) { delete(raw["multiCastSetting"].(map[string]any), "macGroupId") }, wantError: "missing multiCastSetting.macGroupId", noWrite: true},
		{name: "malformed snapshot", mutate: func(raw map[string]any) { raw["broadcast"] = "false" }, wantError: "invalid value type", noWrite: true},
		{name: "null nested object", mutate: func(raw map[string]any) { raw["rateLimit"] = nil }, wantError: "expected an object", noWrite: true},
		{name: "unsupported existing security", mutate: func(raw map[string]any) { raw["security"] = float64(2) }, wantError: "only open", noWrite: true},
		{name: "unsupported planned security", edit: func(p, s *WirelessNetworkResourceModel) { p.Security = types.Int64Value(2) }, wantError: "only open", noWrite: true},
		{name: "open with password", edit: func(p, s *WirelessNetworkResourceModel) { p.Security = types.Int64Value(0) }, wantError: "remove passphrase", noWrite: true},
		{name: "open to secured requires new key", mutate: func(raw map[string]any) { raw["security"] = float64(0); delete(raw, "pskSetting") }, edit: func(p, s *WirelessNetworkResourceModel) { p.Passphrase = types.StringNull() }, wantError: "configure passphrase", noWrite: true},
		{name: "VLAN change", edit: func(p, s *WirelessNetworkResourceModel) { p.VlanID = types.Int64Value(30) }, vlan: 30},
		{name: "VLAN removal", edit: func(p, s *WirelessNetworkResourceModel) { p.VlanID = types.Int64Null() }, vlan: -1},
		{name: "VLAN network lookup fails", edit: func(p, s *WirelessNetworkResourceModel) { p.VlanID = types.Int64Value(30) }, fail: "networks", wantError: "looking up VLAN", noWrite: true},
		{name: "VLAN network missing", edit: func(p, s *WirelessNetworkResourceModel) { p.VlanID = types.Int64Value(40) }, wantError: "no network found", noWrite: true},
		{name: "VLAN network ambiguous", edit: func(p, s *WirelessNetworkResourceModel) { p.VlanID = types.Int64Value(30) }, fail: "duplicate", wantError: "multiple networks", noWrite: true},
		{name: "invalid VLAN", edit: func(p, s *WirelessNetworkResourceModel) { p.VlanID = types.Int64Value(0) }, wantError: "between 1 and 4094", noWrite: true},
		{name: "invalid VLAN association", mutate: func(raw map[string]any) {
			raw["vlanSetting"].(map[string]any)["customConfig"].(map[string]any)["lanNetworkVlanIds"] = "invalid"
		}, wantError: "invalid value type", noWrite: true},
		{name: "read failure", fail: "read", wantError: "fixture failure", noWrite: true},
		{name: "missing SSID", fail: "missing", wantError: "not found", noWrite: true},
		{name: "PATCH failure retains prior state", fail: "patch", wantError: "fixture failure"},
		{name: "readback failure retains prior state", fail: "readback", wantError: "fixture failure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			data, err := os.ReadFile("../testdata/wireless_network.json")
			if err != nil {
				t.Fatal(err)
			}
			var current map[string]any
			if err = json.Unmarshal(data, &current); err != nil {
				t.Fatal(err)
			}
			current["pskSetting"].(map[string]any)["securityKey"] = "********"
			current["vlanSetting"].(map[string]any)["currentVlanId"] = float64(20)
			if tc.mutate != nil {
				tc.mutate(current)
			}
			old := WirelessNetworkResourceModel{ID: types.StringValue("ssid"), SiteID: types.StringValue("site"), WlanGroupID: types.StringValue("group"), Name: types.StringValue("Original"), Band: types.Int64Value(1), Security: types.Int64Value(3), Broadcast: types.BoolValue(false), Enable11r: types.BoolValue(true), PmfMode: types.Int64Value(3), VlanID: types.Int64Value(20), Passphrase: types.StringValue("fixture-passphrase-123")}
			plan := old
			plan.Name = types.StringValue("Renamed")
			if tc.edit != nil {
				tc.edit(&plan, &old)
			}
			var mu sync.Mutex
			var sent map[string]any
			writes := 0
			const collection = "/controller/api/v2/sites/site/setting/wlans/group/ssids"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				var result any
				failure := false
				switch {
				case req.URL.Path == "/api/info":
					result = map[string]any{"omadacId": "controller"}
				case req.URL.Path == "/controller/api/v2/login":
					result = map[string]any{"token": "token"}
				case req.Method == http.MethodGet && req.URL.Path == collection:
					failure = tc.fail == "read" || (tc.fail == "readback" && writes > 0)
					items := []any{current}
					if tc.fail == "missing" {
						items = nil
					}
					result = map[string]any{"data": items, "totalRows": len(items)}
					if tc.array {
						result = items
					}
				case req.Method == http.MethodGet && req.URL.Path == "/controller/api/v2/sites/site/setting/lan/networks":
					failure = tc.fail == "networks"
					items := []any{map[string]any{"id": "network-30", "vlan": 30}}
					if tc.fail == "duplicate" {
						items = append(items, map[string]any{"id": "duplicate", "vlan": 30})
					}
					result = map[string]any{"data": items}
				case req.Method == http.MethodPatch && req.URL.Path == collection+"/ssid":
					writes++
					if err := json.NewDecoder(req.Body).Decode(&sent); err != nil {
						t.Error(err)
					}
					failure = tc.fail == "patch"
					if !failure {
						b, _ := json.Marshal(sent)
						_ = json.Unmarshal(b, &current)
						current["id"] = "ssid"
					}
					switch tc.response {
					case "partial":
						result = map[string]any{"ssidId": "ssid"}
					case "full":
						result = current
					}
				default:
					t.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
				}
				if failure {
					fmt.Fprint(w, `{"errorCode":-1001,"msg":"fixture failure"}`)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "result": result})
			}))
			defer server.Close()
			c, err := client.NewClient(server.URL, "fixture", "fixture", false)
			if err != nil {
				t.Fatal(err)
			}
			r := &WirelessNetworkResource{client: c}
			var schema resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schema)
			state := tfsdk.State{Schema: schema.Schema}
			if d := state.Set(ctx, &old); d.HasError() {
				t.Fatal(d)
			}
			planned := tfsdk.Plan{Schema: schema.Schema}
			if d := planned.Set(ctx, &plan); d.HasError() {
				t.Fatal(d)
			}
			resp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema, Raw: planned.Raw}}
			r.Update(ctx, resource.UpdateRequest{State: state, Plan: planned}, &resp)
			if resp.Diagnostics.HasError() != (tc.wantError != "") {
				t.Fatalf("diagnostics: %v", resp.Diagnostics)
			}
			if tc.wantError != "" && !strings.Contains(fmt.Sprint(resp.Diagnostics), tc.wantError) {
				t.Errorf("missing diagnostic %q: %v", tc.wantError, resp.Diagnostics)
			}
			mu.Lock()
			defer mu.Unlock()
			wantWrites := 1
			if tc.noWrite {
				wantWrites = 0
			}
			if writes != wantWrites {
				t.Errorf("writes=%d, want %d", writes, wantWrites)
			}
			var actual WirelessNetworkResourceModel
			if d := resp.State.Get(ctx, &actual); d.HasError() {
				t.Fatal(d)
			}
			if tc.wantError != "" {
				if !reflect.DeepEqual(actual, old) {
					t.Error("error replaced prior state")
				}
				return
			}
			if actual.Name.ValueString() != plan.Name.ValueString() || actual.ID != old.ID {
				t.Error("incorrect result identity/name")
			}
			if !actual.Passphrase.Equal(plan.Passphrase) {
				t.Error("controller key replaced the planned secret")
			}
			if writes == 0 {
				return
			}
			var original map[string]any
			_ = json.Unmarshal(data, &original)
			if tc.mutate != nil {
				tc.mutate(original)
			}
			for _, key := range []string{"guestNetEnable", "wlanScheduleEnable", "action", "scheduleId", "macFilterEnable", "policy", "macFilterId", "rateLimit", "ssidRateLimit", "rateAndBeaconCtrl", "multiCastSetting", "mloEnable", "prohibitWifiShare"} {
				if !reflect.DeepEqual(sent[key], original[key]) {
					t.Errorf("unrelated field changed: %s", key)
				}
			}
			for _, key := range []string{"id", "idInt", "index", "site", "resource", "portalEnable", "accessEnable"} {
				if _, ok := sent[key]; ok {
					t.Errorf("read-only field sent: %s", key)
				}
			}
			psk := sent["pskSetting"].(map[string]any)
			wantKey := plan.Passphrase
			if wantKey.IsNull() {
				wantKey = old.Passphrase
			}
			if psk["securityKey"] != wantKey.ValueString() {
				t.Error("untrusted secret in payload")
			}
			for key, value := range original["pskSetting"].(map[string]any) {
				if key != "securityKey" && !reflect.DeepEqual(psk[key], value) {
					t.Errorf("PSK field changed: %s", key)
				}
			}
			vlan := sent["vlanSetting"].(map[string]any)
			if _, ok := vlan["currentVlanId"]; ok {
				t.Error("response-only VLAN field sent")
			}
			if tc.vlan == -1 {
				if !reflect.DeepEqual(vlan, map[string]any{"mode": float64(0)}) {
					t.Error("VLAN not removed")
				}
			} else if tc.vlan == 30 {
				custom := vlan["customConfig"].(map[string]any)
				if custom["bridgeVlan"] != float64(30) || custom["lanNetworkId"] != "network-30" {
					t.Error("incorrect VLAN association")
				}
			} else if !reflect.DeepEqual(vlan, original["vlanSetting"]) {
				t.Error("VLAN association changed")
			}
		})
	}
}

func TestWirelessNetworkCreateValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*WirelessNetworkResourceModel)
		want string
	}{
		{"missing password", func(p *WirelessNetworkResourceModel) { p.Passphrase = types.StringNull() }, "configure passphrase"},
		{"empty password", func(p *WirelessNetworkResourceModel) { p.Passphrase = types.StringValue("") }, "configure passphrase"},
		{"unsupported security", func(p *WirelessNetworkResourceModel) { p.Security = types.Int64Value(2) }, "only open"},
		{"open with password", func(p *WirelessNetworkResourceModel) { p.Security = types.Int64Value(0) }, "remove passphrase"},
		{"unresolved VLAN", func(p *WirelessNetworkResourceModel) { p.VlanID = types.Int64Unknown() }, "must be known"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			plan := WirelessNetworkResourceModel{SiteID: types.StringValue("site"), WlanGroupID: types.StringValue("group"), Name: types.StringValue("Created"), Band: types.Int64Value(3), Security: types.Int64Value(3), Broadcast: types.BoolValue(true), Enable11r: types.BoolValue(false), PmfMode: types.Int64Value(2), VlanID: types.Int64Null(), Passphrase: types.StringValue("fixture-passphrase-123")}
			tc.edit(&plan)
			// No client: validation must precede any discovery or mutation.
			r := &WirelessNetworkResource{}
			var schema resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schema)
			planned := tfsdk.Plan{Schema: schema.Schema}
			if d := planned.Set(ctx, &plan); d.HasError() {
				t.Fatal(d)
			}
			resp := resource.CreateResponse{}
			r.Create(ctx, resource.CreateRequest{Plan: planned}, &resp)
			if !resp.Diagnostics.HasError() || !strings.Contains(fmt.Sprint(resp.Diagnostics), tc.want) {
				t.Errorf("diagnostics: %v", resp.Diagnostics)
			}
		})
	}
}
