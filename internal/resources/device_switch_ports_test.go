package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/terraform-provider-tplink-omada/internal/client"
)

func TestSwitchPortUpdates(t *testing.T) {
	// Every expected payload includes preserved writable fields and excludes
	// operational speed and arbitrary fields from the controller response.
	preserved := func(changes map[string]interface{}) map[string]interface{} {
		want := map[string]interface{}{
			"name": "Uplink", "disable": true, "profileId": "profile",
			"profileOverrideEnable": true, "nativeNetworkId": "native",
			"networkTagsSetting": 1, "tagNetworkIds": []string{"tagged"},
			"untagNetworkIds": []string{"untagged"}, "voiceNetworkEnable": true, "voiceDscpEnable": true,
		}
		for key, value := range changes {
			want[key] = value
		}
		return want
	}
	tests := []struct {
		name        string
		edit        func(*DeviceSwitchResourceModel, []SwitchPortModel) []SwitchPortModel
		want        map[string]interface{}
		wantError   bool
		failRequest bool
	}{
		{"unchanged", nil, nil, false, false},
		{"whole list unknown", func(plan *DeviceSwitchResourceModel, ports []SwitchPortModel) []SwitchPortModel {
			plan.Ports = types.ListUnknown(plan.Ports.ElementType(context.Background()))
			return nil
		}, nil, false, false},
		{"whole list omitted", func(plan *DeviceSwitchResourceModel, ports []SwitchPortModel) []SwitchPortModel {
			plan.Ports = types.ListNull(plan.Ports.ElementType(context.Background()))
			return nil
		}, nil, false, false},
		{"rename preserves unknown values", func(_ *DeviceSwitchResourceModel, ports []SwitchPortModel) []SwitchPortModel {
			ports[0] = SwitchPortModel{Port: types.Int64Value(1), Name: types.StringValue("Renamed"),
				Disable: types.BoolUnknown(), ProfileID: types.StringUnknown(), NetworkTagsSetting: types.Int64Unknown(),
				TagNetworkIDs: types.ListUnknown(types.StringType), UntagNetworkIDs: types.ListUnknown(types.StringType)}
			return ports
		}, preserved(map[string]interface{}{"name": "Renamed"}), false, false},
		{"explicit changes including false zero and empty list", func(_ *DeviceSwitchResourceModel, ports []SwitchPortModel) []SwitchPortModel {
			ports[0] = SwitchPortModel{Port: types.Int64Value(1), Name: types.StringValue(""), Disable: types.BoolValue(false),
				ProfileID: types.StringValue("new-profile"), ProfileOverrideEnable: types.BoolValue(false),
				NativeNetworkID: types.StringValue("new-native"), NetworkTagsSetting: types.Int64Value(0),
				TagNetworkIDs:      types.ListValueMust(types.StringType, []attr.Value{types.StringValue("new-tag")}),
				UntagNetworkIDs:    types.ListValueMust(types.StringType, []attr.Value{}),
				VoiceNetworkEnable: types.BoolValue(false), VoiceDscpEnable: types.BoolValue(false), Speed: types.Int64Value(3)}
			return ports
		}, preserved(map[string]interface{}{"name": "", "disable": false, "profileId": "new-profile", "profileOverrideEnable": false,
			"nativeNetworkId": "new-native", "networkTagsSetting": 0, "tagNetworkIds": []string{"new-tag"},
			"untagNetworkIds": []string{}, "voiceNetworkEnable": false, "voiceDscpEnable": false}), false, false},
		{"clear tagged list preserve omitted untagged list", func(_ *DeviceSwitchResourceModel, ports []SwitchPortModel) []SwitchPortModel {
			ports[0].TagNetworkIDs = types.ListValueMust(types.StringType, []attr.Value{})
			ports[0].UntagNetworkIDs = types.ListNull(types.StringType)
			return ports
		}, preserved(map[string]interface{}{"tagNetworkIds": []string{}}), false, false},
		{"invalid later port prevents all writes", func(_ *DeviceSwitchResourceModel, ports []SwitchPortModel) []SwitchPortModel {
			ports[0].Name = types.StringValue("Renamed")
			ports[1].Port = types.Int64Value(999)
			return ports
		}, nil, true, false},
		{"duplicate port prevents all writes", func(_ *DeviceSwitchResourceModel, ports []SwitchPortModel) []SwitchPortModel {
			ports[1].Port = ports[0].Port
			return ports
		}, nil, true, false},
		{"unknown port prevents all writes", func(_ *DeviceSwitchResourceModel, ports []SwitchPortModel) []SwitchPortModel {
			ports[0].Port = types.Int64Unknown()
			return ports
		}, nil, true, false},
		{"incomplete list prevents all writes", func(_ *DeviceSwitchResourceModel, ports []SwitchPortModel) []SwitchPortModel {
			return ports[:1]
		}, nil, true, false},
		{"unknown list element prevents all writes", func(_ *DeviceSwitchResourceModel, ports []SwitchPortModel) []SwitchPortModel {
			ports[1].TagNetworkIDs = types.ListValueMust(types.StringType, []attr.Value{types.StringUnknown()})
			return ports
		}, nil, true, false},
		{"failed port write stops subsequent ports", func(_ *DeviceSwitchResourceModel, ports []SwitchPortModel) []SwitchPortModel {
			ports[0].Name = types.StringValue("Renamed")
			ports[1].Name = types.StringValue("Other")
			return ports
		}, preserved(map[string]interface{}{"name": "Renamed"}), true, true},
		{"reordered ports preserve identity", func(_ *DeviceSwitchResourceModel, ports []SwitchPortModel) []SwitchPortModel {
			return []SwitchPortModel{ports[1], ports[0]}
		}, nil, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			cfg := client.SwitchConfig{MAC: "02-00-00-00-00-01", Name: "Switch", LEDSetting: 2,
				Ports: []client.SwitchPort{
					{Port: 1, Name: "Uplink", Disable: true, ProfileID: "profile", ProfileOverrideEnable: true,
						NativeNetworkID: "native", NetworkTagsSetting: 1, TagNetworkIDs: []string{"tagged"}, UntagNetworkIDs: []string{"untagged"},
						VoiceNetworkEnable: true, VoiceDscpEnable: true, Speed: 3},
					{Port: 2, Name: "Other", ProfileID: "other-profile", NativeNetworkID: "other-native", TagNetworkIDs: []string{}, UntagNetworkIDs: []string{}, Speed: 2},
				}}
			var old DeviceSwitchResourceModel
			switchConfigToState(&cfg, &old)
			old.SiteID = types.StringValue("site")
			plan := old
			var ports []SwitchPortModel
			if d := plan.Ports.ElementsAs(ctx, &ports, false); d.HasError() {
				t.Fatal(d)
			}
			if tc.edit != nil {
				if changed := tc.edit(&plan, ports); changed != nil {
					value, d := types.ListValueFrom(ctx, old.Ports.ElementType(ctx), changed)
					if d.HasError() {
						t.Fatal(d)
					}
					plan.Ports = value
				}
			}

			var mu sync.Mutex
			var patches []map[string]interface{}
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				var result interface{}
				switch {
				case req.URL.Path == "/api/info":
					result = map[string]interface{}{"omadacId": "controller"}
				case strings.HasSuffix(req.URL.Path, "/login"):
					result = map[string]interface{}{"token": "token"}
				case req.Method == http.MethodGet:
					data, _ := json.Marshal(cfg)
					var raw map[string]interface{}
					_ = json.Unmarshal(data, &raw)
					for _, port := range raw["ports"].([]interface{}) {
						port.(map[string]interface{})["portStatus"] = map[string]interface{}{"linkStatus": 1}
					}
					result = raw
				case req.Method == http.MethodPut:
					writes++
				case req.Method == http.MethodPatch && strings.Contains(req.URL.Path, "/ports/"):
					writes++
					var payload map[string]interface{}
					if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					patches = append(patches, payload)
					if tc.failRequest {
						fmt.Fprint(w, `{"errorCode":-1001,"msg":"rejected port update"}`)
						return
					}
					segments := strings.Split(req.URL.Path, "/")
					number, _ := strconv.Atoi(segments[len(segments)-1])
					if number != 1 {
						t.Errorf("unexpected PATCH to port %d", number)
					}
					for i := range cfg.Ports {
						if cfg.Ports[i].Port == number {
							data, _ := json.Marshal(payload)
							_ = json.Unmarshal(data, &cfg.Ports[i])
						}
					}
				default:
					t.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"errorCode": 0, "result": result})
			}))
			defer server.Close()
			c, err := client.NewClient(server.URL, "test", "test", false)
			if err != nil {
				t.Fatal(err)
			}
			r := &DeviceSwitchResource{client: c}
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
			response := resource.UpdateResponse{State: state}
			r.Update(ctx, resource.UpdateRequest{State: state, Plan: planned}, &response)
			if response.Diagnostics.HasError() != tc.wantError {
				t.Fatalf("diagnostics: %v; wantError=%t", response.Diagnostics, tc.wantError)
			}
			mu.Lock()
			defer mu.Unlock()
			if tc.wantError && !tc.failRequest && writes != 0 {
				t.Errorf("invalid configuration caused %d writes", writes)
			}
			if tc.want == nil {
				if len(patches) != 0 {
					t.Errorf("unchanged/invalid ports caused %d PATCH requests", len(patches))
				}
			} else {
				if len(patches) != 1 {
					t.Fatalf("got %d port PATCH requests, want 1", len(patches))
				}
				got, _ := json.Marshal(patches[0])
				want, _ := json.Marshal(tc.want)
				if string(got) != string(want) {
					t.Errorf("payload\n got: %s\nwant: %s", got, want)
				}
			}
			if tc.name == "reordered ports preserve identity" && !response.Diagnostics.HasError() {
				var updated DeviceSwitchResourceModel
				if d := response.State.Get(ctx, &updated); d.HasError() {
					t.Fatal(d)
				}
				if !updated.Ports.Equal(plan.Ports) {
					t.Error("readback changed the configured port order")
				}
			}
		})
	}
}
