package resources

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/terraform-provider-tplink-omada/internal/client"
)

// wirelessNetworkVLAN maps the controller's selected mode, not stale fields left
// behind by another mode. Missing data is not evidence of the default VLAN.
func wirelessNetworkVLAN(setting *client.VlanSetting) (types.Int64, error) {
	if setting == nil || setting.Mode == nil {
		return types.Int64Null(), fmt.Errorf("controller response is missing vlanSetting.mode")
	}
	switch *setting.Mode {
	case 0:
		return types.Int64Null(), nil
	case 1:
		vlan := 0
		if setting.CustomConfig != nil {
			if setting.CustomConfig.CustomMode != 0 {
				return types.Int64Null(), fmt.Errorf("unsupported SSID VLAN custom mode; only a single bridge VLAN can be represented")
			}
			vlan = setting.CustomConfig.BridgeVlan
		}
		if vlan == 0 {
			vlan = setting.CurrentVlanId
			// A multi-VLAN response cannot be represented by the scalar vlan_id.
			if ids := strings.TrimSpace(setting.CurrentVlanIds); ids != "" && ids != strconv.Itoa(vlan) {
				return types.Int64Null(), fmt.Errorf("ambiguous SSID VLAN fallback; controller does not report one matching VLAN")
			}
		}
		if vlan < 1 || vlan > 4094 {
			return types.Int64Null(), fmt.Errorf("custom SSID VLAN mode requires a VLAN ID between 1 and 4094")
		}
		return types.Int64Value(int64(vlan)), nil
	default:
		return types.Int64Null(), fmt.Errorf("unsupported SSID VLAN mode; only default (0) and custom (1) modes are supported")
	}
}
