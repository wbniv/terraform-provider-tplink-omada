package resources

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/terraform-provider-tplink-omada/internal/client"
)

var _ resource.Resource = &WirelessNetworkResource{}
var _ resource.ResourceWithImportState = &WirelessNetworkResource{}
var _ resource.ResourceWithModifyPlan = &WirelessNetworkResource{}

// WirelessNetworkResource manages an Omada SSID/WLAN.
type WirelessNetworkResource struct {
	client *client.Client
}

// WirelessNetworkResourceModel maps the resource schema to Go types.
type WirelessNetworkResourceModel struct {
	ID          types.String `tfsdk:"id"`
	SiteID      types.String `tfsdk:"site_id"`
	WlanGroupID types.String `tfsdk:"wlan_group_id"`
	Name        types.String `tfsdk:"name"`
	Band        types.Int64  `tfsdk:"band"`
	Security    types.Int64  `tfsdk:"security"`
	Passphrase  types.String `tfsdk:"passphrase"`
	Broadcast   types.Bool   `tfsdk:"broadcast"`
	VlanID      types.Int64  `tfsdk:"vlan_id"`
	Enable11r   types.Bool   `tfsdk:"enable_11r"`
	PmfMode     types.Int64  `tfsdk:"pmf_mode"`
}

func NewWirelessNetworkResource() resource.Resource {
	return &WirelessNetworkResource{}
}

func (r *WirelessNetworkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_wireless_network"
}

func (r *WirelessNetworkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a wireless network (SSID) on the Omada Controller.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the SSID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_id": siteIDResourceSchema(),
			"wlan_group_id": schema.StringAttribute{
				Description: "The WLAN group ID. Omission uses the default group on creation and retains the current group on updates. Changing the group requires replacement.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The SSID name (broadcast name).",
				Required:    true,
			},
			"band": schema.Int64Attribute{
				Description: "Radio band bitmask: 1=2.4GHz, 2=5GHz, 3=both. Defaults to 3 on creation; omission preserves existing settings.",
				Optional:    true,
				Computed:    true,
			},
			"security": schema.Int64Attribute{
				Description: "Security mode: 0=Open, 3=WPA2/WPA3-Personal. Defaults to 3 on creation; omission preserves existing settings. Other modes are unsupported.",
				Optional:    true,
				Computed:    true,
			},
			"passphrase": schema.StringAttribute{
				Description: "The Wi-Fi password (WPA pre-shared key). Required for secured creation and updates without a previously configured key, including imports. API-returned keys are not trusted. Removing this attribute does not clear the remote key; set security to 0 to open the network.",
				Optional:    true,
				Sensitive:   true,
			},
			"broadcast": schema.BoolAttribute{
				Description: "Whether the SSID is broadcast (visible). Set false for hidden networks. Defaults to true on creation; omission preserves existing settings.",
				Optional:    true,
				Computed:    true,
			},
			"vlan_id": schema.Int64Attribute{
				Description: "Custom VLAN ID to assign to this SSID. If not set, uses the default VLAN.",
				Optional:    true,
			},
			"enable_11r": schema.BoolAttribute{
				Description: "Enable 802.11r Fast Roaming. Defaults to false on creation; omission preserves existing settings.",
				Optional:    true,
				Computed:    true,
			},
			"pmf_mode": schema.Int64Attribute{
				Description: "Protected Management Frames mode: 1=disabled, 2=optional, 3=required. Defaults to 2 on creation; omission preserves existing settings. Verify mode meanings for your controller version.",
				Optional:    true,
				Computed:    true,
			},
		},
	}
}

func (r *WirelessNetworkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData),
		)
		return
	}
	r.client = c
}

func (r *WirelessNetworkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan WirelessNetworkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := validateSSIDCreate(plan); err != nil {
		resp.Diagnostics.AddError("Invalid wireless network configuration", err.Error())
		return
	}

	siteID := plan.SiteID.ValueString()

	// Resolve WLAN group ID
	wlanGroupID := plan.WlanGroupID.ValueString()
	if wlanGroupID == "" {
		gid, err := r.client.GetDefaultWlanGroupID(ctx, siteID)
		if err != nil {
			resp.Diagnostics.AddError("Error getting default WLAN group", err.Error())
			return
		}
		wlanGroupID = gid
	}

	ssid := &client.WirelessNetwork{
		Name:               plan.Name.ValueString(),
		Band:               int(plan.Band.ValueInt64()),
		Security:           int(plan.Security.ValueInt64()),
		Broadcast:          plan.Broadcast.ValueBool(),
		Enable11r:          plan.Enable11r.ValueBool(),
		PmfMode:            int(plan.PmfMode.ValueInt64()),
		WlanScheduleEnable: false,
		MacFilterEnable:    false,
		RateLimit:          &client.RateLimit{},
		SSIDRateLimit:      &client.RateLimit{},
		RateAndBeaconCtrl:  &client.RateAndBeaconCtrl{},
		MultiCastSetting: &client.MultiCastSetting{
			MultiCastEnable: true,
			ChannelUtil:     100,
			ArpCastEnable:   true,
			Ipv6CastEnable:  true,
		},
	}

	if plan.Security.ValueInt64() == 3 {
		ssid.PSKSetting = &client.PSKSetting{
			VersionPsk:    2,
			EncryptionPsk: 3,
			SecurityKey:   plan.Passphrase.ValueString(),
		}
	}

	vlanPayload := map[string]any{}
	if err := r.applySSIDVLAN(ctx, siteID, vlanPayload, plan.VlanID); err != nil {
		resp.Diagnostics.AddError("Cannot configure wireless network VLAN", err.Error())
		return
	}
	vlanJSON, err := json.Marshal(vlanPayload["vlanSetting"])
	if err != nil {
		resp.Diagnostics.AddError("Error preparing wireless network VLAN", err.Error())
		return
	}
	if err := json.Unmarshal(vlanJSON, &ssid.VlanSetting); err != nil {
		resp.Diagnostics.AddError("Error preparing wireless network VLAN", err.Error())
		return
	}

	created, err := r.client.CreateWirelessNetwork(ctx, siteID, wlanGroupID, ssid)
	if err != nil {
		resp.Diagnostics.AddError("Error creating wireless network", err.Error())
		return
	}

	vlanID, vlanErr := wirelessNetworkVLAN(created.VlanSetting)
	if vlanErr != nil {
		resp.Diagnostics.AddError("Cannot read created wireless network VLAN", vlanErr.Error())
	} else if !plan.VlanID.Equal(vlanID) {
		resp.Diagnostics.AddError("Wireless network VLAN was not applied", "The controller returned a VLAN setting different from the requested value. Refresh and review the controller configuration before retrying.")
	}
	// Preserve the created object's ID so Terraform can clean up after an error.
	plan.VlanID = vlanID

	plan.ID = types.StringValue(created.ID)
	plan.WlanGroupID = types.StringValue(wlanGroupID)
	plan.Band = types.Int64Value(int64(created.Band))
	plan.Security = types.Int64Value(int64(created.Security))
	plan.Broadcast = types.BoolValue(created.Broadcast)
	plan.Enable11r = types.BoolValue(created.Enable11r)
	plan.PmfMode = types.Int64Value(int64(created.PmfMode))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if !resp.Diagnostics.HasError() && resp.Private != nil {
		marker := []byte("false")
		if plan.Security.ValueInt64() == 3 && !plan.Passphrase.IsNull() {
			marker = []byte("true")
		}
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, ssidConfiguredKey, marker)...)
	}
}

func (r *WirelessNetworkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state WirelessNetworkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := state.SiteID.ValueString()
	wlanGroupID := state.WlanGroupID.ValueString()

	ssid, err := r.client.GetWirelessNetwork(ctx, siteID, wlanGroupID, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading wireless network", err.Error())
		return
	}

	state.Name = types.StringValue(ssid.Name)
	state.Band = types.Int64Value(int64(ssid.Band))
	state.Security = types.Int64Value(int64(ssid.Security))
	state.Broadcast = types.BoolValue(ssid.Broadcast)
	state.Enable11r = types.BoolValue(ssid.Enable11r)
	state.PmfMode = types.Int64Value(int64(ssid.PmfMode))

	vlanID, err := wirelessNetworkVLAN(ssid.VlanSetting)
	if err != nil {
		resp.Diagnostics.AddError("Cannot read wireless network VLAN", err.Error())
		return
	}
	state.VlanID = vlanID

	// Keep only the key supplied through Terraform. Controller responses may be
	// masked, omitted, or plaintext with no reliable discriminator.

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *WirelessNetworkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// The framework initializes the response from the plan, not prior state.
	// Failed discovery, PATCH, or readback must not commit an unapplied plan.
	resp.State = req.State
	var plan WirelessNetworkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state WirelessNetworkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Group membership is part of the API path, not an editable SSID field.
	// Reject stale/invalid update plans before contacting the controller.
	if !plan.WlanGroupID.Equal(state.WlanGroupID) {
		resp.Diagnostics.AddError("Wireless network group change requires replacement", "The planned WLAN group must match the existing group for an update. Create a new Terraform plan to replace the SSID in the requested group.")
		return
	}

	siteID := state.SiteID.ValueString()
	wlanGroupID := state.WlanGroupID.ValueString()
	ssidID := state.ID.ValueString()

	current, err := r.client.GetWirelessNetworkRaw(ctx, siteID, wlanGroupID, ssidID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading wireless network before update", err.Error())
		return
	}
	marker, diagnostics := req.Private.GetKey(ctx, ssidConfiguredKey)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	payload, changed, err := r.prepareSSIDUpdate(ctx, siteID, current, plan, state, string(marker) == "true")
	if err != nil {
		resp.Diagnostics.AddError("Cannot safely update wireless network", err.Error())
		return
	}

	var updated *client.WirelessNetwork
	if changed {
		updated, err = r.client.UpdateWirelessNetwork(ctx, siteID, wlanGroupID, ssidID, payload)
	} else {
		updated, err = r.client.GetWirelessNetwork(ctx, siteID, wlanGroupID, ssidID)
	}
	if err != nil {
		resp.Diagnostics.AddError("Error updating or reading back wireless network", err.Error())
		return
	}

	vlanID, vlanErr := wirelessNetworkVLAN(updated.VlanSetting)
	if vlanErr != nil {
		resp.Diagnostics.AddError("Cannot read updated wireless network VLAN", vlanErr.Error())
		return
	}
	if !plan.VlanID.IsUnknown() && !plan.VlanID.Equal(vlanID) {
		resp.Diagnostics.AddError("Wireless network VLAN was not applied", "The controller returned a VLAN setting different from the requested value. Observed state has been saved; refresh and review the controller configuration before retrying.")
	}
	plan.VlanID = vlanID

	plan.Name = types.StringValue(updated.Name)
	plan.ID = state.ID
	plan.SiteID = state.SiteID
	plan.WlanGroupID = state.WlanGroupID
	plan.Band = types.Int64Value(int64(updated.Band))
	plan.Security = types.Int64Value(int64(updated.Security))
	plan.Broadcast = types.BoolValue(updated.Broadcast)
	plan.Enable11r = types.BoolValue(updated.Enable11r)
	plan.PmfMode = types.Int64Value(int64(updated.PmfMode))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if !resp.Diagnostics.HasError() && resp.Private != nil {
		marker := []byte("false")
		if plan.Security.ValueInt64() == 3 && !plan.Passphrase.IsNull() {
			marker = []byte("true")
		}
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, ssidConfiguredKey, marker)...)
	}
}

func (r *WirelessNetworkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state WirelessNetworkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteWirelessNetwork(ctx, state.SiteID.ValueString(), state.WlanGroupID.ValueString(), state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error deleting wireless network", err.Error())
		return
	}
}

func (r *WirelessNetworkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import ID format: "siteID/wlanGroupID/ssidID"
	siteID, wlanGroupID, ssidID, ok := parseImportID3(req.ID)
	if !ok {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Import ID must be in the format 'siteID/wlanGroupID/ssidID'.",
		)
		return
	}

	ssid, err := r.client.GetWirelessNetwork(ctx, siteID, wlanGroupID, ssidID)
	if err != nil {
		resp.Diagnostics.AddError("Error importing wireless network", err.Error())
		return
	}

	state := WirelessNetworkResourceModel{
		ID:          types.StringValue(ssid.ID),
		SiteID:      types.StringValue(siteID),
		WlanGroupID: types.StringValue(wlanGroupID),
		Name:        types.StringValue(ssid.Name),
		Band:        types.Int64Value(int64(ssid.Band)),
		Security:    types.Int64Value(int64(ssid.Security)),
		Broadcast:   types.BoolValue(ssid.Broadcast),
		Enable11r:   types.BoolValue(ssid.Enable11r),
		PmfMode:     types.Int64Value(int64(ssid.PmfMode)),
	}

	vlanID, err := wirelessNetworkVLAN(ssid.VlanSetting)
	if err != nil {
		resp.Diagnostics.AddError("Cannot import wireless network VLAN", err.Error())
		return
	}
	state.VlanID = vlanID

	// Import never turns an API-returned key or mask into a trusted secret.
	state.Passphrase = types.StringNull()

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
