package resources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const ssidConfiguredKey = "ssid_configured_key_v1"

// Defaults belong only to creation. Schema defaults would also replace omitted
// values in an imported SSID before Update can preserve them.
func (r *WirelessNetworkResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	if req.State.Raw.IsNull() {
		for name, value := range map[string]int64{"band": 3, "security": 3} {
			var configured types.Int64
			resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(name), &configured)...)
			if configured.IsNull() {
				resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(name), types.Int64Value(value))...)
			}
		}
		for name, value := range map[string]bool{"broadcast": true, "enable_11r": false} {
			var configured types.Bool
			resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(name), &configured)...)
			if configured.IsNull() {
				resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(name), types.BoolValue(value))...)
			}
		}
	}
	// PMF is inapplicable to open SSIDs. Do not overwrite an explicit value,
	// or guess while a security dependency is still unknown.
	var configuredSecurity, configuredPMF types.Int64
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("security"), &configuredSecurity)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("pmf_mode"), &configuredPMF)...)
	var effectiveSecurity types.Int64
	resp.Diagnostics.Append(resp.Plan.GetAttribute(ctx, path.Root("security"), &effectiveSecurity)...)
	var prior WirelessNetworkResourceModel
	if !req.State.Raw.IsNull() {
		resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
		if configuredSecurity.IsNull() {
			effectiveSecurity = prior.Security
		}
	}
	if configuredPMF.IsNull() {
		if effectiveSecurity.IsUnknown() {
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("pmf_mode"), types.Int64Unknown())...)
		} else if effectiveSecurity.ValueInt64() == 0 && !effectiveSecurity.IsNull() {
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("pmf_mode"), types.Int64Value(0))...)
		} else if req.State.Raw.IsNull() || !effectiveSecurity.Equal(prior.Security) {
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("pmf_mode"), types.Int64Value(2))...)
		}
	}
	if err := validateSSIDPMF(effectiveSecurity, configuredPMF); err != nil {
		resp.Diagnostics.AddError("Invalid wireless network PMF configuration", err.Error())
	}
	if !req.State.Raw.IsNull() {
		return
	}
	// Terraform replans replacements with null prior state. Reject known
	// invalid creation settings here, before it can destroy the old SSID.
	var plan WirelessNetworkResourceModel
	resp.Diagnostics.Append(resp.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := validateSSIDCreateSecurity(plan); err != nil {
		resp.Diagnostics.AddError("Invalid wireless network creation configuration", err.Error())
	}
}

func validateSSIDCreate(plan WirelessNetworkResourceModel) error {
	if plan.Band.IsNull() || plan.Band.IsUnknown() || plan.Security.IsNull() || plan.Security.IsUnknown() ||
		plan.Broadcast.IsNull() || plan.Broadcast.IsUnknown() || plan.Enable11r.IsNull() || plan.Enable11r.IsUnknown() ||
		plan.PmfMode.IsNull() || plan.PmfMode.IsUnknown() || plan.VlanID.IsUnknown() || plan.Passphrase.IsUnknown() {
		return fmt.Errorf("SSID settings must be known before creation")
	}
	if err := validateSSIDCreateSecurity(plan); err != nil {
		return err
	}
	return validateSSIDPMF(plan.Security, plan.PmfMode)
}

func validateSSIDPMF(security, pmf types.Int64) error {
	if security.IsNull() || security.IsUnknown() || pmf.IsNull() || pmf.IsUnknown() {
		return nil
	}
	if security.ValueInt64() == 0 && pmf.ValueInt64() != 0 {
		return fmt.Errorf("open security requires pmf_mode = 0 or omission")
	}
	if security.ValueInt64() == 3 && (pmf.ValueInt64() < 1 || pmf.ValueInt64() > 3) {
		return fmt.Errorf("secured SSIDs require pmf_mode between 1 and 3")
	}
	return nil
}

func validateSSIDCreateSecurity(plan WirelessNetworkResourceModel) error {
	// Unknown configuration is resolved during apply; validateSSIDCreate also
	// requires known settings before the controller can be contacted.
	if plan.Security.IsUnknown() {
		return nil
	}
	if plan.Security.ValueInt64() != 0 && plan.Security.ValueInt64() != 3 {
		return fmt.Errorf("only open (0) and WPA-Personal (3) security modes are supported")
	}
	if plan.Passphrase.IsUnknown() {
		return nil
	}
	if plan.Security.ValueInt64() == 3 && plan.Passphrase.ValueString() == "" {
		return fmt.Errorf("configure passphrase before creating a secured SSID")
	}
	if plan.Security.ValueInt64() == 0 && !plan.Passphrase.IsNull() {
		return fmt.Errorf("remove passphrase when selecting open security")
	}
	return nil
}

// The v5.9 Web API Modify SSID request documents these nested fields. VLAN,
// MLO, sharing and management-rate fields use the existing v6 client contract.
// Unknown fields stop updates: silently dropping a new writable field from a
// full-object PATCH could reset it. Known response-only fields are discarded.
// See docs/resources/wireless_network.md for version and migration limitations.
type ssidFields map[string]any

var ssidRateFields = ssidFields{
	"rateLimitId": "string", "downLimitEnable": "bool", "upLimitEnable": "bool",
	"downLimit": "number|string", "downLimitType": "number", "upLimit": "number|string", "upLimitType": "number",
}

var ssidWritableFields = ssidFields{
	"id": "ignore", "idInt": "ignore", "index": "ignore", "site": "ignore", "resource": "ignore",
	"portalEnable": "ignore", "accessEnable": "ignore",
	// 6.1: vlanEnable/vlanId are derived from vlanSetting. deviceType is create-only
	// in the controller UI; omitting it on PATCH preserves the existing target.
	"vlanEnable": "ignore", "vlanId": "ignore", "deviceType": "ignore",
	"wlanId": "string", "name": "string", "band": "number", "security": "number",
	"broadcast": "bool", "enable11r": "bool", "pmfMode": "number", "guestNetEnable": "bool",
	"wlanScheduleEnable": "bool", "action": "number", "scheduleId": "string",
	"macFilterEnable": "bool", "policy": "number", "macFilterId": "string",
	"mloEnable": "bool", "prohibitWifiShare": "bool", "greEnable": "bool",
	"rateLimit": ssidRateFields, "ssidRateLimit": ssidRateFields,
	"pskSetting": ssidFields{
		"securityKey": "ignore", // A response key is never a trusted password.
		"versionPsk":  "number", "encryptionPsk": "number", "gikRekeyPskEnable": "bool",
		"rekeyPskInterval": "number", "intervalPskType": "number",
	},
	"rateAndBeaconCtrl": ssidFields{
		"rate2gCtrlEnable": "bool", "rate5gCtrlEnable": "bool", "rate6gCtrlEnable": "bool",
		"manageRateControl2gEnable": "bool", "manageRateControl5gEnable": "bool",
		"lowerDensity2g": "number", "higherDensity2g": "number", "cckRatesDisable": "bool",
		"clientRatesRequire2g": "bool", "sendBeacons2g": "bool",
		"lowerDensity5g": "number", "higherDensity5g": "number", "clientRatesRequire5g": "bool", "sendBeacons5g": "bool",
		"lowerDensity6g": "number", "higherDensity6g": "number", "clientRatesRequire6g": "bool", "sendBeacons6g": "bool",
	},
	"multiCastSetting": ssidFields{
		"multiCastEnable": "bool", "channelUtil": "number", "arpCastEnable": "bool", "ipv6CastEnable": "bool",
		"filterEnable": "bool", "filterMode": "number", "macGroupId": "string",
	},
	"vlanSetting": ssidFields{
		"mode": "number", "currentVlanId": "ignore", "currentVlanIds": "ignore",
		"customConfig": ssidFields{
			"customMode": "number", "bridgeVlan": "number", "lanNetworkId": "string", "lanNetworkVlanIds": "vlan-map",
		},
	},
}

func copySSIDFields(current map[string]any, fields ssidFields, prefix string) (map[string]any, error) {
	result := make(map[string]any, len(current))
	for name, value := range current {
		spec, exists := fields[name]
		if !exists {
			return nil, fmt.Errorf("unsupported SSID field in %s; update the provider's writable-field contract before retrying", prefix)
		}
		if spec == "ignore" {
			continue
		}
		fieldPath := prefix + "." + name
		if nested, ok := spec.(ssidFields); ok {
			object, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("expected an object at %s", fieldPath)
			}
			copied, err := copySSIDFields(object, nested, fieldPath)
			if err != nil {
				return nil, err
			}
			result[name] = copied
			continue
		}
		valid := false
		switch spec {
		case "string":
			_, valid = value.(string)
		case "bool":
			_, valid = value.(bool)
		case "number":
			_, valid = value.(float64)
		case "number|string":
			switch value.(type) {
			case float64, string:
				valid = true
			}
		case "vlan-map":
			// Copy and validate dynamic network IDs without treating their names as schema fields.
			data, err := json.Marshal(value)
			var ids map[string][]int
			valid = err == nil && json.Unmarshal(data, &ids) == nil && ids != nil
			if valid {
				var copied any
				_ = json.Unmarshal(data, &copied)
				value = copied
			}
		}
		if !valid {
			return nil, fmt.Errorf("invalid value type at %s", fieldPath)
		}
		result[name] = value
	}
	return result, nil
}

func requireSSIDFields(object map[string]any, prefix, names string) error {
	for _, name := range strings.Fields(names) {
		if value, ok := object[name]; !ok || value == "" {
			return fmt.Errorf("incomplete SSID snapshot: missing %s.%s", prefix, name)
		}
	}
	return nil
}

func (r *WirelessNetworkResource) prepareSSIDUpdate(ctx context.Context, siteID string, current map[string]any, plan, state WirelessNetworkResourceModel, trustedPrior bool) (map[string]any, bool, error) {
	payload, err := copySSIDFields(current, ssidWritableFields, "SSID")
	if err != nil {
		return nil, false, err
	}
	if err := requireSSIDFields(payload, "SSID", "name band security broadcast enable11r guestNetEnable wlanScheduleEnable macFilterEnable vlanSetting rateLimit ssidRateLimit rateAndBeaconCtrl multiCastSetting"); err != nil {
		return nil, false, err
	}
	for _, enabled := range []struct{ name, required string }{{"wlanScheduleEnable", "action scheduleId"}, {"macFilterEnable", "policy macFilterId"}} {
		if payload[enabled.name] == true {
			if err := requireSSIDFields(payload, "SSID", enabled.required); err != nil {
				return nil, false, err
			}
		}
	}
	for _, name := range []string{"rateLimit", "ssidRateLimit"} {
		limits := payload[name].(map[string]any)
		if err := requireSSIDFields(limits, name, "downLimitEnable upLimitEnable"); err != nil {
			return nil, false, err
		}
		if limits["rateLimitId"] == nil || limits["rateLimitId"] == "" {
			for _, direction := range []string{"down", "up"} {
				if limits[direction+"LimitEnable"] == true {
					if err := requireSSIDFields(limits, name, direction+"Limit "+direction+"LimitType"); err != nil {
						return nil, false, err
					}
				}
			}
		}
	}
	beacon := payload["rateAndBeaconCtrl"].(map[string]any)
	for _, band := range []string{"2g", "5g", "6g"} {
		if beacon["rate"+band+"CtrlEnable"] == true || beacon["manageRateControl"+band+"Enable"] == true {
			if err := requireSSIDFields(beacon, "rateAndBeaconCtrl", "lowerDensity"+band+" higherDensity"+band+" clientRatesRequire"+band+" sendBeacons"+band); err != nil {
				return nil, false, err
			}
		}
	}
	multicast := payload["multiCastSetting"].(map[string]any)
	if multicast["filterEnable"] == true {
		if err := requireSSIDFields(multicast, "multiCastSetting", "filterMode macGroupId"); err != nil {
			return nil, false, err
		}
	}
	vlan := payload["vlanSetting"].(map[string]any)
	if vlan["mode"] != float64(0) && vlan["mode"] != float64(1) {
		return nil, false, fmt.Errorf("unsupported or missing VLAN mode in SSID snapshot")
	}
	if vlan["mode"] == float64(1) {
		custom, _ := vlan["customConfig"].(map[string]any)
		if err := requireSSIDFields(custom, "vlanSetting.customConfig", "customMode bridgeVlan lanNetworkId lanNetworkVlanIds"); err != nil {
			return nil, false, err
		}
		if custom["customMode"] != float64(0) {
			return nil, false, fmt.Errorf("unsupported VLAN custom mode in SSID snapshot")
		}
		associations, _ := custom["lanNetworkVlanIds"].(map[string]any)
		ids, _ := associations[custom["lanNetworkId"].(string)].([]any)
		matched := false
		for _, id := range ids {
			if id == custom["bridgeVlan"] {
				matched = true
			}
		}
		if !matched {
			return nil, false, fmt.Errorf("incomplete VLAN network association in SSID snapshot")
		}
	}
	oldSecurity := payload["security"].(float64)
	if oldSecurity == 3 {
		if err := requireSSIDFields(payload, "SSID", "pmfMode"); err != nil {
			return nil, false, err
		}
	}
	if oldSecurity != 0 && oldSecurity != 3 {
		return nil, false, fmt.Errorf("only open (0) and WPA-Personal (3) SSIDs can be safely updated")
	}
	// Compare the writable form, excluding inapplicable open-network PMF.
	if oldSecurity == 0 {
		delete(payload, "pmfMode")
	}
	before, _ := json.Marshal(payload)
	if !plan.Name.IsNull() && !plan.Name.IsUnknown() {
		payload["name"] = plan.Name.ValueString()
	}
	for name, value := range map[string]types.Int64{"band": plan.Band, "security": plan.Security, "pmfMode": plan.PmfMode} {
		if !value.IsNull() && !value.IsUnknown() {
			payload[name] = float64(value.ValueInt64())
		}
	}
	for name, value := range map[string]types.Bool{"broadcast": plan.Broadcast, "enable11r": plan.Enable11r} {
		if !value.IsNull() && !value.IsUnknown() {
			payload[name] = value.ValueBool()
		}
	}
	security := payload["security"].(float64)
	if security != 0 && security != 3 {
		return nil, false, fmt.Errorf("only open (0) and WPA-Personal (3) security modes are supported")
	}
	if plan.Passphrase.IsUnknown() {
		return nil, false, fmt.Errorf("passphrase must be known before updating a secured SSID")
	}
	if security == 0 {
		if !plan.Passphrase.IsNull() {
			return nil, false, fmt.Errorf("remove passphrase when selecting open security")
		}
		if err := validateSSIDPMF(types.Int64Value(0), plan.PmfMode); err != nil {
			return nil, false, err
		}
		delete(payload, "pmfMode")
		delete(payload, "pskSetting")
	} else if oldSecurity == 3 {
		psk, _ := payload["pskSetting"].(map[string]any)
		if err := requireSSIDFields(psk, "pskSetting", "versionPsk encryptionPsk gikRekeyPskEnable"); err != nil {
			return nil, false, err
		}
		if psk["gikRekeyPskEnable"] == true {
			if err := requireSSIDFields(psk, "pskSetting", "rekeyPskInterval intervalPskType"); err != nil {
				return nil, false, err
			}
		}
	} else {
		payload["pskSetting"] = map[string]any{"versionPsk": float64(2), "encryptionPsk": float64(3), "gikRekeyPskEnable": false}
	}
	if security == 3 {
		if oldSecurity == 0 && (plan.PmfMode.IsNull() || plan.PmfMode.IsUnknown()) {
			payload["pmfMode"] = float64(2)
		}
		pmf, ok := payload["pmfMode"].(float64)
		if !ok {
			return nil, false, fmt.Errorf("missing secured SSID PMF mode")
		}
		if err := validateSSIDPMF(types.Int64Value(3), types.Int64Value(int64(pmf))); err != nil {
			return nil, false, err
		}
	}
	if err := r.applySSIDVLAN(ctx, siteID, payload, plan.VlanID); err != nil {
		return nil, false, err
	}
	after, _ := json.Marshal(payload)
	secretChanged := security == 3 && !plan.Passphrase.IsNull() && !plan.Passphrase.Equal(state.Passphrase)
	changed := !bytes.Equal(before, after) || secretChanged
	if changed && security == 3 {
		key := plan.Passphrase
		if key.IsNull() && oldSecurity == 3 && trustedPrior {
			key = state.Passphrase
		}
		if key.IsNull() || key.IsUnknown() || key.ValueString() == "" {
			return nil, false, fmt.Errorf("configure passphrase before updating this secured SSID; no trusted Terraform key is available and controller-returned keys may be masked")
		}
		payload["pskSetting"].(map[string]any)["securityKey"] = key.ValueString()
	}
	return payload, changed, nil
}

func (r *WirelessNetworkResource) applySSIDVLAN(ctx context.Context, siteID string, payload map[string]any, planned types.Int64) error {
	if planned.IsUnknown() {
		return nil
	}
	if planned.IsNull() {
		payload["vlanSetting"] = map[string]any{"mode": float64(0)}
		return nil
	}
	vlanID := int(planned.ValueInt64())
	if vlanID < 1 || vlanID > 4094 {
		return fmt.Errorf("vlan_id must be between 1 and 4094")
	}
	setting, _ := payload["vlanSetting"].(map[string]any)
	custom, _ := setting["customConfig"].(map[string]any)
	if setting["mode"] == float64(1) && custom["bridgeVlan"] == float64(vlanID) {
		if err := requireSSIDFields(custom, "vlanSetting.customConfig", "customMode lanNetworkId lanNetworkVlanIds"); err != nil {
			return err
		}
		return nil
	}
	networks, err := r.client.ListNetworks(ctx, siteID)
	if err != nil {
		return fmt.Errorf("looking up VLAN network: %w", err)
	}
	networkID := ""
	for _, network := range networks {
		if network.Vlan == vlanID {
			if network.ID == "" {
				return fmt.Errorf("matching VLAN network has no ID; cannot select a network safely")
			}
			if networkID != "" {
				return fmt.Errorf("multiple networks match vlan_id; cannot select a network safely")
			}
			networkID = network.ID
		}
	}
	if networkID == "" {
		return fmt.Errorf("no network found for vlan_id; configure the network before updating the SSID")
	}
	payload["vlanSetting"] = map[string]any{"mode": float64(1), "customConfig": map[string]any{
		"customMode": float64(0), "bridgeVlan": float64(vlanID), "lanNetworkId": networkID,
		"lanNetworkVlanIds": map[string]any{networkID: []any{float64(vlanID)}},
	}}
	return nil
}
