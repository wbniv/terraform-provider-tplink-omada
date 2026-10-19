package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/terraform-provider-tplink-omada/internal/client"
)

var _ resource.ResourceWithModifyPlan = &SiteResource{}
var _ resource.ResourceWithModifyPlan = &SiteSettingsResource{}

func credentialPair(username, password types.String, required, applying bool) error {
	if username.IsNull() && password.IsNull() && !required {
		return nil
	}
	if username.IsNull() || password.IsNull() {
		return fmt.Errorf("configure both device_account_username and device_account_password, or omit both on an existing site/settings resource")
	}
	if applying && (username.IsUnknown() || password.IsUnknown()) {
		return fmt.Errorf("device account credentials must be known before applying")
	}
	if (!username.IsUnknown() && username.ValueString() == "") || (!password.IsUnknown() && password.ValueString() == "") {
		return fmt.Errorf("device account credentials must not be empty")
	}
	return nil
}

func siteCredentialChange(plan, prior SiteResourceModel, applying bool) error {
	if err := credentialPair(plan.DeviceAccountUsername, plan.DeviceAccountPassword, false, applying); err != nil {
		return err
	}
	if plan.DeviceAccountUsername.IsNull() && plan.DeviceAccountPassword.IsNull() {
		return nil
	}
	if (!plan.DeviceAccountUsername.IsUnknown() && !plan.DeviceAccountUsername.Equal(prior.DeviceAccountUsername)) || (!plan.DeviceAccountPassword.IsUnknown() && !plan.DeviceAccountPassword.Equal(prior.DeviceAccountPassword)) {
		return fmt.Errorf("site credentials are creation-only; remove both device_account inputs from omada_site and configure both on one omada_site_settings resource to rotate credentials without replacing the site")
	}
	return nil
}

func (r *SiteResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var plan SiteResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var err error
	if req.State.Raw.IsNull() {
		err = credentialPair(plan.DeviceAccountUsername, plan.DeviceAccountPassword, true, false)
	} else {
		var prior SiteResourceModel
		resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
		if resp.Diagnostics.HasError() {
			return
		}
		err = siteCredentialChange(plan, prior, false)
	}
	if err != nil {
		resp.Diagnostics.AddError("Invalid site device credentials", err.Error())
	}
}

func (r *SiteSettingsResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var config SiteSettingsResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := credentialPair(config.DeviceAccountUsername, config.DeviceAccountPassword, false, false); err != nil {
		resp.Diagnostics.AddError("Invalid site settings credentials", err.Error())
	}
}

func credentialError(err error, passwords ...types.String) string {
	message := err.Error()
	for _, password := range passwords {
		if value := password.ValueString(); value != "" {
			message = strings.ReplaceAll(message, value, "[redacted]")
		}
	}
	return message
}

// applySettings returns a non-nil model only when observed or acknowledged state
// can be saved. Rejected requests leave the caller's prior state intact.
func (r *SiteSettingsResource) applySettings(ctx context.Context, plan, config SiteSettingsResourceModel) (*SiteSettingsResourceModel, error) {
	if err := credentialPair(config.DeviceAccountUsername, config.DeviceAccountPassword, false, true); err != nil {
		return nil, err
	}
	manage := !config.DeviceAccountUsername.IsNull()
	if manage {
		if err := credentialPair(plan.DeviceAccountUsername, plan.DeviceAccountPassword, true, true); err != nil {
			return nil, err
		}
	}
	siteID := plan.SiteID.ValueString()
	raw, err := r.client.GetSiteSettingsRaw(ctx, siteID)
	if err != nil {
		return nil, fmt.Errorf("reading site settings before update: %w", err)
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var current client.SiteSettings
	if err := json.Unmarshal(data, &current); err != nil {
		return nil, fmt.Errorf("decoding site settings: %w", err)
	}
	observed := SiteSettingsResourceModel{ID: types.StringValue(siteID), SiteID: types.StringValue(siteID)}
	siteSettingsToState(&current, &observed)
	plan.ID = types.StringValue(siteID)
	patch, err := siteSettingsChanges(&plan, config, observed, raw)
	if err != nil {
		return nil, err
	}
	if manage {
		patch["deviceAccount"] = map[string]any{"username": plan.DeviceAccountUsername.ValueString(), "password": plan.DeviceAccountPassword.ValueString()}
	} else {
		plan.DeviceAccountPassword = types.StringNull()
		if plan.DeviceAccountUsername.IsUnknown() {
			plan.DeviceAccountUsername = observed.DeviceAccountUsername
		}
	}
	if len(patch) == 0 {
		return &plan, nil
	}
	if err := r.client.PatchSiteSettings(ctx, siteID, patch); err != nil {
		return nil, fmt.Errorf("updating site settings: %w", err)
	}
	// At this point the controller acknowledged the requested values. Preserve
	// the accepted secret even if the following read fails or returns a mask.
	readback, err := r.client.GetSiteSettingsRaw(ctx, siteID)
	if err != nil {
		return &plan, fmt.Errorf("settings update was accepted but readback failed: %w", err)
	}
	for section := range patch {
		if value, ok := readback[section].(map[string]any); !ok || value == nil {
			return &plan, fmt.Errorf("settings update was accepted but readback omitted the %s section", section)
		}
	}
	readbackJSON, err := json.Marshal(readback)
	if err != nil {
		return &plan, err
	}
	var updated client.SiteSettings
	if err := json.Unmarshal(readbackJSON, &updated); err != nil {
		return &plan, fmt.Errorf("settings update was accepted but readback was invalid: %w", err)
	}
	actual := plan
	siteSettingsToState(&updated, &actual)
	if manage && !actual.DeviceAccountUsername.Equal(plan.DeviceAccountUsername) {
		return &actual, fmt.Errorf("controller returned a different device account username after update")
	}
	for field := range siteSettingPaths {
		wanted := reflect.ValueOf(plan).FieldByName(field).Interface().(attr.Value)
		got := reflect.ValueOf(actual).FieldByName(field).Interface().(attr.Value)
		if !wanted.IsNull() && !wanted.Equal(got) {
			fieldInfo, _ := reflect.TypeOf(plan).FieldByName(field)
			return &actual, fmt.Errorf("controller returned a different %s setting after update", fieldInfo.Tag.Get("tfsdk"))
		}
	}
	return &actual, nil
}

// Paths describe the writable siblings understood by this version. Only
// changed sections are sent; an unknown sibling in a changed section aborts.
type siteSettingPath struct{ section, field string }

func siteSettingsChanges(plan *SiteSettingsResourceModel, config, observed SiteSettingsResourceModel, raw map[string]any) (map[string]any, error) {
	patch := map[string]any{}
	planned, configured, current := reflect.ValueOf(plan).Elem(), reflect.ValueOf(config), reflect.ValueOf(observed)
	for name, path := range siteSettingPaths {
		field := planned.FieldByName(name)
		value := field.Interface().(attr.Value)
		prior := current.FieldByName(name).Interface().(attr.Value)
		if value.IsUnknown() {
			if !configured.FieldByName(name).Interface().(attr.Value).IsNull() {
				return nil, fmt.Errorf("configured setting %s must be known before applying", name)
			}
			field.Set(current.FieldByName(name))
			value = prior
		}
		if value.IsNull() || value.Equal(prior) {
			continue
		}
		section, ok := patch[path.section].(map[string]any)
		if !ok {
			section = map[string]any{}
			if existing, exists := raw[path.section]; exists && existing != nil {
				values, ok := existing.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid %s section", path.section)
				}
				for key, v := range values {
					if path.section == "site" && key == "key" {
						continue
					}
					known := false
					for _, p := range siteSettingPaths {
						if p.section == path.section && p.field == key {
							known = true
							break
						}
					}
					if !known {
						return nil, fmt.Errorf("cannot safely update %s: unrecognized field %s", path.section, key)
					}
					section[key] = v
				}
			}
			patch[path.section] = section
		}
		switch value := value.(type) {
		case types.Bool:
			section[path.field] = value.ValueBool()
		case types.Int64:
			section[path.field] = value.ValueInt64()
		case types.String:
			section[path.field] = value.ValueString()
		default:
			return nil, fmt.Errorf("unsupported setting type for %s", name)
		}
	}
	return patch, nil
}

var siteSettingPaths = map[string]siteSettingPath{
	"SiteName":                        {"site", "name"},
	"Region":                          {"site", "region"},
	"TimeZone":                        {"site", "timeZone"},
	"Scenario":                        {"site", "scenario"},
	"AutoUpgradeEnable":               {"autoUpgrade", "enable"},
	"MeshEnable":                      {"mesh", "meshEnable"},
	"MeshAutoFailover":                {"mesh", "autoFailoverEnable"},
	"MeshDefGateway":                  {"mesh", "defGatewayEnable"},
	"MeshFullSector":                  {"mesh", "fullSector"},
	"LEDEnable":                       {"led", "enable"},
	"FastRoamingEnable":               {"roaming", "fastRoamingEnable"},
	"AiRoamingEnable":                 {"roaming", "aiRoamingEnable"},
	"DualBand11kReportEnable":         {"roaming", "dualBand11kReportEnable"},
	"ForceDisassociationEnable":       {"roaming", "forceDisassociationEnable"},
	"NonStickRoamingEnable":           {"roaming", "nonStickRoamingEnable"},
	"NonPingPongRoamingEnable":        {"roaming", "nonPingPongRoamingEnable"},
	"BandSteeringEnable":              {"bandSteering", "enable"},
	"BandSteeringConnectionThreshold": {"bandSteering", "connectionThreshold"},
	"BandSteeringDifferenceThreshold": {"bandSteering", "differenceThreshold"},
	"BandSteeringMaxFailures":         {"bandSteering", "maxFailures"},
	"BandSteeringMultiBandMode":       {"bandSteeringForMultiBand", "mode"},
	"AirtimeFairness2g":               {"airtimeFairness", "enable2g"},
	"AirtimeFairness5g":               {"airtimeFairness", "enable5g"},
	"AirtimeFairness6g":               {"airtimeFairness", "enable6g"},
	"BeaconIntvMode2g":                {"beaconControl", "beaconIntvMode2g"},
	"DtimPeriod2g":                    {"beaconControl", "dtimPeriod2g"},
	"RtsThreshold2g":                  {"beaconControl", "rtsThreshold2g"},
	"FragThreshold2g":                 {"beaconControl", "fragmentationThreshold2g"},
	"BeaconIntvMode5g":                {"beaconControl", "beaconIntvMode5g"},
	"DtimPeriod5g":                    {"beaconControl", "dtimPeriod5g"},
	"RtsThreshold5g":                  {"beaconControl", "rtsThreshold5g"},
	"FragThreshold5g":                 {"beaconControl", "fragmentationThreshold5g"},
	"BeaconInterval6g":                {"beaconControl", "beaconInterval6g"},
	"BeaconIntvMode6g":                {"beaconControl", "beaconIntvMode6g"},
	"DtimPeriod6g":                    {"beaconControl", "dtimPeriod6g"},
	"RtsThreshold6g":                  {"beaconControl", "rtsThreshold6g"},
	"FragThreshold6g":                 {"beaconControl", "fragmentationThreshold6g"},
	"LLDPEnable":                      {"lldp", "enable"},
	"AdvancedFeatureEnable":           {"advancedFeature", "enable"},
	"SpeedTestEnable":                 {"speedTest", "enable"},
	"SpeedTestInterval":               {"speedTest", "interval"},
	"AlertEnable":                     {"alert", "enable"},
	"AlertDelayEnable":                {"alert", "delayEnable"},
	"AlertDelay":                      {"alert", "delay"},
	"RemoteLogEnable":                 {"remoteLog", "enable"},
	"RemoteLogServer":                 {"remoteLog", "server"},
	"RemoteLogPort":                   {"remoteLog", "port"},
	"RemoteLogMoreClientLog":          {"remoteLog", "moreClientLog"},
	"RememberDeviceEnable":            {"rememberDevice", "enable"},
}
