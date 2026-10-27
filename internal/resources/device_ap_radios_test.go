package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/terraform-provider-tplink-omada/internal/client"
)

func TestAPRadioUpdates(t *testing.T) {
	unknown := func(p *DeviceAPResourceModel) {
		p.Radio2gEnable, p.Radio5gEnable = types.BoolUnknown(), types.BoolUnknown()
		p.Radio2gChannel, p.Radio5gChannel = types.StringUnknown(), types.StringUnknown()
		p.Radio2gChannelWidth, p.Radio5gChannelWidth = types.StringUnknown(), types.StringUnknown()
		p.Radio2gTxPower, p.Radio5gTxPower = types.Int64Unknown(), types.Int64Unknown()
		p.Radio2gTxPowerLevel, p.Radio5gTxPowerLevel = types.Int64Unknown(), types.Int64Unknown()
	}
	changeChannel := func(p *DeviceAPResourceModel) { unknown(p); p.Radio2gChannel = types.StringValue("6") }
	const changed2g = `{"radioSetting2g":{"radioEnable":true,"channelWidth":"4","channel":"6","txPower":20,"txPowerLevel":3}}`
	for _, tc := range []struct {
		name          string
		edit          func(*DeviceAPResourceModel)
		mutate        func(map[string]any)
		want          string
		wantError     string
		fail          string
		readbackFails bool
	}{
		{name: "unchanged radios"},
		{name: "unknown radios on rename", edit: unknown},
		{name: "null radios on rename", edit: func(p *DeviceAPResourceModel) {
			p.Radio2gEnable, p.Radio5gEnable = types.BoolNull(), types.BoolNull()
			p.Radio2gChannel, p.Radio5gChannel = types.StringNull(), types.StringNull()
			p.Radio2gChannelWidth, p.Radio5gChannelWidth = types.StringNull(), types.StringNull()
			p.Radio2gTxPower, p.Radio5gTxPower = types.Int64Null(), types.Int64Null()
			p.Radio2gTxPowerLevel, p.Radio5gTxPowerLevel = types.Int64Null(), types.Int64Null()
		}},
		{name: "partial 2g change preserves unknowns", edit: changeChannel, want: changed2g},
		{name: "explicit false zero and empty", edit: func(p *DeviceAPResourceModel) {
			unknown(p)
			p.Radio2gEnable = types.BoolValue(false)
			p.Radio2gTxPower = types.Int64Value(0)
			p.Radio2gTxPowerLevel = types.Int64Value(0)
			p.Radio2gChannel = types.StringValue("")
		}, want: `{"radioSetting2g":{"radioEnable":false,"channelWidth":"4","channel":"","txPower":0,"txPowerLevel":0}}`},
		{name: "both bands change", edit: func(p *DeviceAPResourceModel) { changeChannel(p); p.Radio5gEnable = types.BoolValue(false) },
			want: `{"radioSetting2g":{"radioEnable":true,"channelWidth":"4","channel":"6","txPower":20,"txPowerLevel":3},"radioSetting5g":{"radioEnable":false,"channelWidth":"5","channel":"36","txPower":18,"txPowerLevel":3}}`},
		{name: "unsupported 5g prevents all writes", edit: func(p *DeviceAPResourceModel) { changeChannel(p); p.Radio5gEnable = types.BoolValue(true) }, mutate: func(raw map[string]any) { delete(raw, "radioSetting5g") }, wantError: "does not report"},
		{name: "incomplete snapshot prevents all writes", edit: changeChannel, mutate: func(raw map[string]any) { delete(raw["radioSetting2g"].(map[string]any), "txPower") }, wantError: "missing txPower"},
		{name: "invalid snapshot prevents all writes", edit: changeChannel, mutate: func(raw map[string]any) { raw["radioSetting2g"].(map[string]any)["radioEnable"] = "invalid" }, wantError: "invalid controller radio settings"},
		{name: "failed general update stops radio writes", edit: changeChannel, fail: "general", wantError: "updating AP config"},
		{name: "radio failure records successful rename", edit: changeChannel, want: changed2g, fail: "radios", wantError: "updating AP radio config"},
		{name: "advanced failure records radio progress", edit: changeChannel, want: changed2g, fail: "advanced", wantError: "updating AP advanced config"},
		{name: "services failure records radio progress", edit: changeChannel, want: changed2g, fail: "services", wantError: "updating AP services config"},
		{name: "failed readback retains prior state", edit: changeChannel, want: changed2g, fail: "radios", wantError: "updating AP radio config", readbackFails: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			cfg := client.APConfig{MAC: "02-00-00-00-00-01", Name: "Original", LEDSetting: 2,
				RadioSetting2g: &client.APRadioSetting{RadioEnable: true, ChannelWidth: "4", Channel: "0", TxPower: 20, TxPowerLevel: 3, Freq: 2412, WirelessMode: 9},
				RadioSetting5g: &client.APRadioSetting{RadioEnable: true, ChannelWidth: "5", Channel: "36", TxPower: 18, TxPowerLevel: 3, Freq: 5180, WirelessMode: 9}}
			var old DeviceAPResourceModel
			apConfigToState(&cfg, &old)
			old.SiteID = types.StringValue("site")
			plan := old
			plan.Name = types.StringValue("Renamed")
			if tc.edit != nil {
				tc.edit(&plan)
			}
			var mu sync.Mutex
			var sent []map[string]any
			var writes []string
			failed := false
			const path = "/controller/api/v2/sites/site/eaps/02-00-00-00-00-01"
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
					if failed && tc.readbackFails {
						fmt.Fprint(w, `{"errorCode":-1001,"msg":"readback failed"}`)
						return
					}
					data, _ := json.Marshal(cfg)
					var raw map[string]any
					_ = json.Unmarshal(data, &raw)
					if tc.mutate != nil {
						tc.mutate(raw)
					}
					result = raw
				case (r.Method == http.MethodPatch && r.URL.Path == path) || (r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, path+"/config/")):
					endpoint := strings.TrimPrefix(r.URL.Path, path+"/config/")
					if r.Method == http.MethodPatch {
						endpoint = "general"
					}
					writes = append(writes, endpoint)
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if endpoint == "radios" {
						sent = append(sent, body)
					}
					if endpoint == tc.fail {
						failed = true
						fmt.Fprint(w, `{"errorCode":-1001,"msg":"write rejected"}`)
						return
					}
					if endpoint == "general" {
						if _, exists := body["radioSetting2g"]; exists {
							t.Error("radio 2g included in general PATCH")
						}
						if _, exists := body["radioSetting5g"]; exists {
							t.Error("radio 5g included in general PATCH")
						}
						if name, ok := body["name"].(string); ok {
							cfg.Name = name
						}
						result = cfg
					}
					if endpoint == "radios" {
						data, _ := json.Marshal(body)
						var next client.APRadioConfig
						if err := json.Unmarshal(data, &next); err != nil {
							t.Error(err)
						}
						if next.RadioSetting2g != nil {
							cfg.RadioSetting2g = next.RadioSetting2g
						}
						if next.RadioSetting5g != nil {
							cfg.RadioSetting5g = next.RadioSetting5g
						}
					}
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "result": result})
			}))
			defer srv.Close()
			c, err := client.NewClient(srv.URL, "test", "test", false)
			if err != nil {
				t.Fatal(err)
			}
			r := &DeviceAPResource{client: c}
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
			config := tfsdk.Config{Schema: schema.Schema, Raw: planned.Raw}
			resp := resource.UpdateResponse{State: state}
			r.Update(ctx, resource.UpdateRequest{Config: config, State: state, Plan: planned}, &resp)
			if resp.Diagnostics.HasError() != (tc.wantError != "") {
				t.Fatalf("diagnostics: %v", resp.Diagnostics)
			}
			if tc.wantError != "" && !strings.Contains(fmt.Sprint(resp.Diagnostics), tc.wantError) {
				t.Errorf("missing %q: %v", tc.wantError, resp.Diagnostics)
			}
			mu.Lock()
			defer mu.Unlock()
			if tc.want == "" {
				if len(sent) != 0 {
					t.Errorf("unexpected radio payloads: %v", sent)
				}
			} else {
				var want map[string]any
				if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
					t.Fatal(err)
				}
				if len(sent) != 1 || !reflect.DeepEqual(sent[0], want) {
					t.Errorf("payloads = %v, want %v", sent, want)
				}
			}
			if tc.wantError != "" && tc.fail == "" && len(writes) != 0 {
				t.Errorf("invalid radio config caused writes: %v", writes)
			}
			if tc.fail != "" && (len(writes) == 0 || writes[len(writes)-1] != tc.fail) {
				t.Errorf("writes did not stop at %s: %v", tc.fail, writes)
			}
			var actual DeviceAPResourceModel
			if d := resp.State.Get(ctx, &actual); d.HasError() {
				t.Fatal(d)
			}
			if tc.readbackFails {
				if !reflect.DeepEqual(actual, old) {
					t.Errorf("failed readback replaced prior state")
				}
				if resp.Diagnostics.WarningsCount() != 1 {
					t.Errorf("missing readback warning: %v", resp.Diagnostics)
				}
			} else if tc.mutate == nil {
				if actual.Name.ValueString() != cfg.Name || actual.Radio2gChannel.ValueString() != cfg.RadioSetting2g.Channel || actual.Radio2gTxPower.ValueInt64() != int64(cfg.RadioSetting2g.TxPower) {
					t.Errorf("response state does not reflect controller: %+v", actual)
				}
			}
		})
	}
}
