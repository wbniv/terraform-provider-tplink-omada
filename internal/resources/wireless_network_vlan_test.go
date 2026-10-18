package resources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/terraform-provider-tplink-omada/internal/client"
)

func TestWirelessNetworkVLANState(t *testing.T) {
	for _, tc := range []struct {
		name, setting string
		want          int64
		wantError     bool
	}{
		{name: "default", setting: `{"mode":0}`},
		{name: "default ignores stale fields", setting: `{"mode":0,"customConfig":{"customMode":0,"bridgeVlan":20},"currentVlanId":30,"currentVlanIds":"20,30"}`},
		{name: "default ignores invalid stale VLAN", setting: `{"mode":0,"customConfig":{"customMode":1,"bridgeVlan":4095},"currentVlanId":-1}`},
		{name: "custom bridge", setting: `{"mode":1,"customConfig":{"customMode":0,"bridgeVlan":20}}`, want: 20},
		{name: "bridge precedes stale current", setting: `{"mode":1,"customConfig":{"customMode":0,"bridgeVlan":20},"currentVlanId":30}`, want: 20},
		{name: "custom fallback", setting: `{"mode":1,"currentVlanId":30}`, want: 30},
		{name: "zero bridge fallback", setting: `{"mode":1,"customConfig":{"customMode":0,"bridgeVlan":0},"currentVlanId":30}`, want: 30},
		{name: "matching fallback list", setting: `{"mode":1,"currentVlanId":30,"currentVlanIds":"30"}`, want: 30},
		{name: "lowest VLAN", setting: `{"mode":1,"customConfig":{"customMode":0,"bridgeVlan":1}}`, want: 1},
		{name: "highest VLAN", setting: `{"mode":1,"customConfig":{"customMode":0,"bridgeVlan":4094}}`, want: 4094},
		{name: "missing setting", wantError: true},
		{name: "null setting", setting: `null`, wantError: true},
		{name: "missing mode", setting: `{}`, wantError: true},
		{name: "missing mode with VLAN", setting: `{"customConfig":{"bridgeVlan":20}}`, wantError: true},
		{name: "null mode", setting: `{"mode":null}`, wantError: true},
		{name: "unsupported mode", setting: `{"mode":2,"currentVlanId":30}`, wantError: true},
		{name: "negative mode", setting: `{"mode":-1}`, wantError: true},
		{name: "custom without VLAN", setting: `{"mode":1}`, wantError: true},
		{name: "null custom without fallback", setting: `{"mode":1,"customConfig":null}`, wantError: true},
		{name: "negative bridge", setting: `{"mode":1,"customConfig":{"bridgeVlan":-1},"currentVlanId":30}`, wantError: true},
		{name: "out of range bridge", setting: `{"mode":1,"customConfig":{"bridgeVlan":4095}}`, wantError: true},
		{name: "out of range fallback", setting: `{"mode":1,"currentVlanId":4095}`, wantError: true},
		{name: "unsupported custom mode", setting: `{"mode":1,"customConfig":{"customMode":1,"bridgeVlan":20}}`, wantError: true},
		{name: "multiple fallback VLANs", setting: `{"mode":1,"currentVlanId":20,"currentVlanIds":"20,30"}`, wantError: true},
		{name: "conflicting fallback VLAN", setting: `{"mode":1,"currentVlanId":20,"currentVlanIds":"30"}`, wantError: true},
		{name: "malformed mode", setting: `{"mode":"0"}`, wantError: true},
		{name: "fractional mode", setting: `{"mode":0.5}`, wantError: true},
		{name: "malformed bridge", setting: `{"mode":1,"customConfig":{"bridgeVlan":"20"}}`, wantError: true},
	} {
		for _, operation := range []string{"refresh", "import", "data source"} {
			t.Run(tc.name+"/"+operation, func(t *testing.T) {
				ctx := context.Background()
				ssid := map[string]any{"id": "ssid", "wlanId": "group", "name": "Observed", "band": 3, "security": 0, "broadcast": true, "enable11r": false, "pmfMode": 2}
				if tc.setting != "" {
					ssid["vlanSetting"] = json.RawMessage(tc.setting)
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					var result any
					switch {
					case req.URL.Path == "/api/info":
						result = map[string]any{"omadacId": "controller"}
					case req.URL.Path == "/controller/api/v2/login":
						result = map[string]any{"token": "token"}
					case req.Method == http.MethodGet && req.URL.Path == "/controller/api/v2/sites/site/setting/wlans/group/ssids":
						result = map[string]any{"data": []any{ssid}}
					default:
						t.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "result": result})
				}))
				defer server.Close()
				c, err := client.NewClient(server.URL, "fixture", "fixture", false)
				if err != nil {
					t.Fatal(err)
				}
				want := types.Int64Null()
				if tc.want != 0 {
					want = types.Int64Value(tc.want)
				}
				if operation == "data source" {
					d := &WirelessNetworksDataSource{client: c}
					var schema datasource.SchemaResponse
					d.Schema(ctx, datasource.SchemaRequest{}, &schema)
					configuration := WirelessNetworksDataSourceModel{SiteID: types.StringValue("site"), WlanGroupID: types.StringValue("group")}
					configState := tfsdk.State{Schema: schema.Schema}
					if ds := configState.Set(ctx, &configuration); ds.HasError() {
						t.Fatal(ds)
					}
					resp := datasource.ReadResponse{State: tfsdk.State{Schema: schema.Schema}}
					d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schema.Schema, Raw: configState.Raw}}, &resp)
					if resp.Diagnostics.HasError() != tc.wantError {
						t.Fatalf("diagnostics: %v", resp.Diagnostics)
					}
					if tc.wantError {
						if !resp.State.Raw.IsNull() {
							t.Error("failed read published partial data")
						}
						return
					}
					var actual WirelessNetworksDataSourceModel
					if ds := resp.State.Get(ctx, &actual); ds.HasError() {
						t.Fatal(ds)
					}
					if len(actual.WirelessNetworks) != 1 || !actual.WirelessNetworks[0].VlanID.Equal(want) {
						t.Errorf("VLAN state = %+v, want %s", actual.WirelessNetworks, want)
					}
					return
				}
				r := &WirelessNetworkResource{client: c}
				var schema resource.SchemaResponse
				r.Schema(ctx, resource.SchemaRequest{}, &schema)
				old := WirelessNetworkResourceModel{ID: types.StringValue("ssid"), SiteID: types.StringValue("site"), WlanGroupID: types.StringValue("group"), Name: types.StringValue("Original"), Band: types.Int64Value(3), Security: types.Int64Value(0), Broadcast: types.BoolValue(true), Enable11r: types.BoolValue(false), PmfMode: types.Int64Value(2), VlanID: types.Int64Value(20), Passphrase: types.StringNull()}
				state := tfsdk.State{Schema: schema.Schema}
				if ds := state.Set(ctx, &old); ds.HasError() {
					t.Fatal(ds)
				}
				var actualState tfsdk.State
				if operation == "refresh" {
					resp := resource.ReadResponse{State: state}
					r.Read(ctx, resource.ReadRequest{State: state}, &resp)
					if resp.Diagnostics.HasError() != tc.wantError {
						t.Fatalf("diagnostics: %v", resp.Diagnostics)
					}
					actualState = resp.State
				} else {
					resp := resource.ImportStateResponse{State: tfsdk.State{Schema: schema.Schema}}
					r.ImportState(ctx, resource.ImportStateRequest{ID: "site/group/ssid"}, &resp)
					if resp.Diagnostics.HasError() != tc.wantError {
						t.Fatalf("diagnostics: %v", resp.Diagnostics)
					}
					if tc.wantError {
						if !resp.State.Raw.IsNull() {
							t.Error("invalid import published state")
						}
						return
					}
					actualState = resp.State
				}
				var actual WirelessNetworkResourceModel
				if ds := actualState.Get(ctx, &actual); ds.HasError() {
					t.Fatal(ds)
				}
				if tc.wantError {
					if !reflect.DeepEqual(actual, old) {
						t.Error("invalid refresh replaced prior state")
					}
					return
				}
				if !actual.VlanID.Equal(want) {
					t.Errorf("VLAN state = %s, want %s", actual.VlanID, want)
				}
				if actual.ID != old.ID || actual.SiteID != old.SiteID || actual.WlanGroupID != old.WlanGroupID || actual.Name.ValueString() != "Observed" {
					t.Error("read lost identity or other observed fields")
				}
			})
		}
	}
}

// A PATCH success does not prove that the controller applied the requested VLAN.
// Exercise readback with the framework's plan-initialized response state.
func TestWirelessNetworkVLANWriteReadback(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		for _, tc := range []struct {
			name, setting                      string
			wantError, plannedDefault, unknown bool
			want                               int64
		}{
			{name: "applied", setting: `{"mode":1,"customConfig":{"customMode":0,"bridgeVlan":20}}`, want: 20},
			{name: "rejected", setting: `{"mode":0,"customConfig":{"bridgeVlan":20},"currentVlanId":20}`, wantError: true},
			{name: "unreadable", setting: `{"mode":1}`, wantError: true},
			{name: "default applied", setting: `{"mode":0,"customConfig":{"bridgeVlan":20}}`, plannedDefault: true},
			{name: "default rejected", setting: `{"mode":1,"customConfig":{"bridgeVlan":20}}`, plannedDefault: true, wantError: true, want: 20},
			{name: "unknown resolves from readback", setting: `{"mode":1,"currentVlanId":30}`, unknown: true, want: 30},
		} {
			if operation == "create" && tc.unknown {
				continue
			}
			t.Run(operation+"/"+tc.name, func(t *testing.T) {
				ctx := context.Background()
				var writes atomic.Int32
				// Complete pre-update snapshot required by D1's preservation contract.
				current := json.RawMessage(`{"id":"ssid","name":"Original","band":3,"security":0,"broadcast":true,"enable11r":false,"pmfMode":2,"guestNetEnable":false,"wlanScheduleEnable":false,"macFilterEnable":false,"rateLimit":{"downLimitEnable":false,"upLimitEnable":false},"ssidRateLimit":{"downLimitEnable":false,"upLimitEnable":false},"rateAndBeaconCtrl":{},"multiCastSetting":{},"vlanSetting":{"mode":0}}`)
				const collection = "/controller/api/v2/sites/site/setting/wlans/group/ssids"
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					var result any
					switch {
					case req.URL.Path == "/api/info":
						result = map[string]any{"omadacId": "controller"}
					case req.URL.Path == "/controller/api/v2/login":
						result = map[string]any{"token": "token"}
					case req.Method == http.MethodGet && req.URL.Path == collection:
						result = map[string]any{"data": []any{current}}
					case req.Method == http.MethodGet && req.URL.Path == "/controller/api/v2/sites/site/setting/lan/networks":
						result = map[string]any{"data": []any{map[string]any{"id": "network-20", "vlan": 20}}}
					case (req.Method == http.MethodPost && req.URL.Path == collection) || (req.Method == http.MethodPatch && req.URL.Path == collection+"/ssid"):
						writes.Add(1)
						var raw map[string]any
						_ = json.Unmarshal(current, &raw)
						raw["name"] = "Changed"
						raw["vlanSetting"] = json.RawMessage(tc.setting)
						b, _ := json.Marshal(raw)
						current = b
						if operation == "create" {
							result = map[string]any{"ssidId": "ssid"}
						}
					default:
						t.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
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
				old := WirelessNetworkResourceModel{ID: types.StringValue("ssid"), SiteID: types.StringValue("site"), WlanGroupID: types.StringValue("group"), Name: types.StringValue("Original"), Band: types.Int64Value(3), Security: types.Int64Value(0), Broadcast: types.BoolValue(true), Enable11r: types.BoolValue(false), PmfMode: types.Int64Value(2), VlanID: types.Int64Null(), Passphrase: types.StringNull()}
				plan := old
				plan.Name = types.StringValue("Changed")
				plan.VlanID = types.Int64Value(20)
				if tc.plannedDefault {
					plan.VlanID = types.Int64Null()
				}
				if tc.unknown {
					plan.VlanID = types.Int64Unknown()
				}
				if operation == "create" {
					plan.ID = types.StringUnknown()
				}
				state := tfsdk.State{Schema: schema.Schema}
				if d := state.Set(ctx, &old); d.HasError() {
					t.Fatal(d)
				}
				planned := tfsdk.Plan{Schema: schema.Schema}
				if d := planned.Set(ctx, &plan); d.HasError() {
					t.Fatal(d)
				}
				var actualState tfsdk.State
				if operation == "update" {
					resp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema, Raw: planned.Raw}}
					r.Update(ctx, resource.UpdateRequest{State: state, Plan: planned}, &resp)
					if resp.Diagnostics.HasError() != tc.wantError {
						t.Fatalf("diagnostics: %v", resp.Diagnostics)
					}
					actualState = resp.State
				} else {
					resp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema, Raw: planned.Raw}}
					r.Create(ctx, resource.CreateRequest{Plan: planned}, &resp)
					if resp.Diagnostics.HasError() != tc.wantError {
						t.Fatalf("diagnostics: %v", resp.Diagnostics)
					}
					actualState = resp.State
				}
				if writes.Load() != 1 {
					t.Fatalf("writes = %d, want 1", writes.Load())
				}
				var actual WirelessNetworkResourceModel
				if d := actualState.Get(ctx, &actual); d.HasError() {
					t.Fatal(d)
				}
				if !tc.wantError || tc.name != "unreadable" {
					want := types.Int64Null()
					if tc.want != 0 {
						want = types.Int64Value(tc.want)
					}
					if !actual.VlanID.Equal(want) {
						t.Errorf("observed VLAN = %s, want %s", actual.VlanID, want)
					}
				}
				if !tc.wantError {
					return
				}
				if operation == "create" && actual.ID.ValueString() != "ssid" {
					t.Error("readback error lost the created object's identity")
				}
				if tc.name == "rejected" && !actual.VlanID.IsNull() {
					t.Error("readback mismatch recorded the unapplied VLAN")
				}
				if operation == "update" && tc.name == "unreadable" && !reflect.DeepEqual(actual, old) {
					t.Error("unreadable update replaced prior state")
				}
			})
		}
	}
}
