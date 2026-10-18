package resources

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/terraform-provider-tplink-omada/internal/client"
)

func TestWirelessNetworkGroupIdentityPlan(t *testing.T) {
	ctx := context.Background()
	r := &WirelessNetworkResource{}
	var s resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &s)
	known := types.StringValue("group-a")
	for _, tc := range []struct {
		name                     string
		config, plan, want       types.String
		create, destroy, replace bool
	}{
		{name: "unchanged", config: known, plan: known, want: known},
		{name: "changed", config: types.StringValue("group-b"), plan: types.StringValue("group-b"), want: types.StringValue("group-b"), replace: true},
		{name: "omitted retains import", config: types.StringNull(), plan: types.StringUnknown(), want: known},
		{name: "omitted already known", config: types.StringNull(), plan: known, want: known},
		{name: "explicit unknown", config: types.StringUnknown(), plan: types.StringUnknown(), want: types.StringUnknown(), replace: true},
		{name: "initial explicit", config: known, plan: known, want: known, create: true},
		{name: "initial omitted", config: types.StringNull(), plan: types.StringUnknown(), want: types.StringUnknown(), create: true},
		{name: "initial unknown", config: types.StringUnknown(), plan: types.StringUnknown(), want: types.StringUnknown(), create: true},
		{name: "destroy", config: types.StringNull(), plan: types.StringNull(), want: types.StringNull(), destroy: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := WirelessNetworkResourceModel{ID: types.StringValue("ssid"), WlanGroupID: known}
			state := tfsdk.State{Schema: s.Schema}
			plan := tfsdk.Plan{Schema: s.Schema}
			if d := state.Set(ctx, &model); d.HasError() {
				t.Fatal(d)
			}
			model.WlanGroupID = tc.plan
			if d := plan.Set(ctx, &model); d.HasError() {
				t.Fatal(d)
			}
			if tc.create {
				state.Raw = tftypes.NewValue(s.Schema.Type().TerraformType(ctx), nil)
			}
			if tc.destroy {
				plan.Raw = tftypes.NewValue(s.Schema.Type().TerraformType(ctx), nil)
			}
			req := planmodifier.StringRequest{Path: path.Root("wlan_group_id"), State: state, Plan: plan, StateValue: known, ConfigValue: tc.config, PlanValue: tc.plan}
			resp := planmodifier.StringResponse{PlanValue: tc.plan}
			for _, modifier := range s.Schema.Attributes["wlan_group_id"].(schema.StringAttribute).PlanModifiers {
				req.PlanValue = resp.PlanValue
				modifier.PlanModifyString(ctx, req, &resp)
			}
			if resp.Diagnostics.HasError() || !resp.PlanValue.Equal(tc.want) || resp.RequiresReplace != tc.replace {
				t.Fatalf("plan=%v replace=%v diagnostics=%v; want %v replace=%v", resp.PlanValue, resp.RequiresReplace, resp.Diagnostics, tc.want, tc.replace)
			}
		})
	}
}

func TestWirelessNetworkGroupIdentityUpdateGuard(t *testing.T) {
	for _, group := range []types.String{types.StringValue("group-b"), types.StringUnknown(), types.StringNull()} {
		t.Run(group.String(), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/info" {
					fmt.Fprint(w, `{"errorCode":0,"result":{"omadacId":"controller"}}`)
					return
				}
				if r.URL.Path == "/controller/api/v2/login" {
					fmt.Fprint(w, `{"errorCode":0,"result":{"token":"token"}}`)
					return
				}
				requests.Add(1)
				http.Error(w, "unexpected controller request", http.StatusInternalServerError)
			}))
			defer server.Close()
			c, err := client.NewClient(server.URL, "fixture", "fixture", false)
			if err != nil {
				t.Fatal(err)
			}
			r := &WirelessNetworkResource{client: c}
			ctx := context.Background()
			var s resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &s)
			old := WirelessNetworkResourceModel{ID: types.StringValue("ssid"), SiteID: types.StringValue("site"), WlanGroupID: types.StringValue("group-a"), Name: types.StringValue("Original")}
			state := tfsdk.State{Schema: s.Schema}
			plan := tfsdk.Plan{Schema: s.Schema}
			if d := state.Set(ctx, &old); d.HasError() {
				t.Fatal(d)
			}
			planned := old
			planned.WlanGroupID = group
			if d := plan.Set(ctx, &planned); d.HasError() {
				t.Fatal(d)
			}
			resp := resource.UpdateResponse{State: tfsdk.State{Schema: s.Schema, Raw: plan.Raw}}
			r.Update(ctx, resource.UpdateRequest{State: state, Plan: plan}, &resp)
			if !resp.Diagnostics.HasError() || requests.Load() != 0 || !resp.State.Raw.Equal(state.Raw) {
				t.Fatalf("expected diagnostic and unchanged state before any request; diagnostics=%v requests=%d state=%v", resp.Diagnostics, requests.Load(), resp.State.Raw)
			}
		})
	}
}

func TestWirelessNetworkGroupIdentityRecreationPlan(t *testing.T) {
	for _, tc := range []struct {
		name       string
		security   types.Int64
		passphrase types.String
		wantError  bool
	}{
		{"secured without key", types.Int64Value(3), types.StringNull(), true},
		{"omitted security defaults to secured", types.Int64Null(), types.StringNull(), true},
		{"open with key", types.Int64Value(0), types.StringValue("fixture-password"), true},
		{"unsupported security", types.Int64Value(2), types.StringNull(), true},
		{"unsupported security with unknown key", types.Int64Value(2), types.StringUnknown(), true},
		{"unknown security deferred", types.Int64Unknown(), types.StringNull(), false},
		{"unknown key deferred", types.Int64Value(3), types.StringUnknown(), false},
		{"secured with key", types.Int64Value(3), types.StringValue("fixture-password"), false},
		{"open without key", types.Int64Value(0), types.StringNull(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			r := &WirelessNetworkResource{}
			var s resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &s)
			model := WirelessNetworkResourceModel{Security: tc.security, Passphrase: tc.passphrase}
			plan := tfsdk.Plan{Schema: s.Schema}
			if d := plan.Set(ctx, &model); d.HasError() {
				t.Fatal(d)
			}
			resp := resource.ModifyPlanResponse{Plan: plan}
			r.ModifyPlan(ctx, resource.ModifyPlanRequest{Plan: plan, Config: tfsdk.Config{Schema: s.Schema, Raw: plan.Raw}, State: tfsdk.State{Schema: s.Schema, Raw: tftypes.NewValue(s.Schema.Type().TerraformType(ctx), nil)}}, &resp)
			if resp.Diagnostics.HasError() != tc.wantError {
				t.Fatalf("diagnostics=%v; want error=%v", resp.Diagnostics, tc.wantError)
			}
		})
	}
}
