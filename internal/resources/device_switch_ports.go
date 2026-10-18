package resources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/terraform-provider-tplink-omada/internal/client"
)

type switchPortUpdate struct {
	port   int
	config map[string]interface{}
}

// planSwitchPortUpdates validates the whole port list before any writes. The
// controller snapshot supplies omitted/unknown values; explicit false, zero,
// empty strings, and empty VLAN lists remain intentional configuration.
func planSwitchPortUpdates(ctx context.Context, raw map[string]interface{}, plan *DeviceSwitchResourceModel, diags *diag.Diagnostics) []switchPortUpdate {
	if plan.Ports.IsNull() || plan.Ports.IsUnknown() {
		return nil
	}
	var ports []SwitchPortModel
	diags.Append(plan.Ports.ElementsAs(ctx, &ports, false)...)
	if diags.HasError() {
		return nil
	}

	data, err := json.Marshal(raw["ports"])
	if err != nil {
		diags.AddError("Error reading switch ports", err.Error())
		return nil
	}
	var currentPorts []map[string]interface{}
	if err := json.Unmarshal(data, &currentPorts); err != nil {
		diags.AddError("Error reading switch ports", err.Error())
		return nil
	}
	if len(ports) != len(currentPorts) {
		diags.AddError("Incomplete switch port configuration", "When configuring ports, include every controller port.")
		return nil
	}
	byNumber := make(map[int]map[string]interface{}, len(currentPorts))
	for _, current := range currentPorts {
		number, ok := current["port"].(float64)
		if !ok || number <= 0 || number != float64(int(number)) {
			diags.AddError("Invalid controller port", "The controller returned a port without a valid positive port number.")
			return nil
		}
		if _, exists := byNumber[int(number)]; exists {
			diags.AddError("Duplicate controller port", fmt.Sprintf("The controller returned port %d more than once.", int(number)))
			return nil
		}
		byNumber[int(number)] = current
	}

	var updates []switchPortUpdate
	seen := make(map[int]bool, len(ports))
	for _, port := range ports {
		if port.Port.IsNull() || port.Port.IsUnknown() || port.Port.ValueInt64() < 1 {
			diags.AddError("Invalid switch port", "Each configured port must have a known positive port number before updating the switch.")
			return nil
		}
		number := int(port.Port.ValueInt64())
		if int64(number) != port.Port.ValueInt64() {
			diags.AddError("Invalid switch port", "The configured port number is outside the supported integer range.")
			return nil
		}
		if seen[number] {
			diags.AddError("Duplicate switch port", fmt.Sprintf("Port %d appears more than once in the configuration.", number))
			return nil
		}
		seen[number] = true
		current, exists := byNumber[number]
		if !exists {
			diags.AddError("Unknown switch port", fmt.Sprintf("The controller does not report port %d on this switch.", number))
			return nil
		}

		// Only schema-backed writable fields may enter a PATCH. In particular,
		// observed speed and other operational/read-only API fields are excluded.
		fields := map[string]attr.Value{
			"name":                  port.Name,
			"disable":               port.Disable,
			"profileId":             port.ProfileID,
			"profileOverrideEnable": port.ProfileOverrideEnable,
			"nativeNetworkId":       port.NativeNetworkID,
			"networkTagsSetting":    port.NetworkTagsSetting,
			"tagNetworkIds":         port.TagNetworkIDs,
			"untagNetworkIds":       port.UntagNetworkIDs,
			"voiceNetworkEnable":    port.VoiceNetworkEnable,
			"voiceDscpEnable":       port.VoiceDscpEnable,
		}
		before := make(map[string]interface{}, len(fields))
		after := make(map[string]interface{}, len(fields))
		for key, value := range fields {
			if previous, present := current[key]; present && previous != nil {
				before[key] = previous
				after[key] = previous
			}
			if value.IsNull() || value.IsUnknown() {
				continue
			}
			switch value := value.(type) {
			case types.String:
				after[key] = value.ValueString()
			case types.Bool:
				after[key] = value.ValueBool()
			case types.Int64:
				after[key] = value.ValueInt64()
			case types.List:
				ids := []string{}
				diags.Append(value.ElementsAs(ctx, &ids, false)...)
				if diags.HasError() {
					return nil
				}
				after[key] = ids
			}
		}
		// JSON comparison normalizes the controller's numeric/list types against
		// the corresponding Go values without treating unchanged ports as writes.
		beforeJSON, err := json.Marshal(before)
		if err != nil {
			diags.AddError("Error comparing switch port", err.Error())
			return nil
		}
		afterJSON, err := json.Marshal(after)
		if err != nil {
			diags.AddError("Error preparing switch port", err.Error())
			return nil
		}
		if !bytes.Equal(beforeJSON, afterJSON) {
			updates = append(updates, switchPortUpdate{port: number, config: after})
		}
	}
	return updates
}

// Preserve the configured/prior list order by port identity. Import starts with
// no prior order; newly discovered ports retain their controller ordering.
func switchPortsInStateOrder(ports []client.SwitchPort, prior types.List) []client.SwitchPort {
	order := make(map[int]int)
	for i, element := range prior.Elements() {
		object, ok := element.(types.Object)
		if !ok || object.IsUnknown() || object.IsNull() {
			continue
		}
		number, ok := object.Attributes()["port"].(types.Int64)
		if ok && !number.IsNull() && !number.IsUnknown() {
			order[int(number.ValueInt64())] = i
		}
	}
	ordered := append([]client.SwitchPort(nil), ports...)
	sort.SliceStable(ordered, func(i, j int) bool {
		iOrder, iKnown := order[ordered[i].Port]
		jOrder, jKnown := order[ordered[j].Port]
		if iKnown && jKnown {
			return iOrder < jOrder
		}
		return iKnown && !jKnown
	})
	return ordered
}
