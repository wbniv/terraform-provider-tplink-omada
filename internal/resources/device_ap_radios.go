package resources

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/terraform-provider-tplink-omada/internal/client"
)

// buildAPRadioConfig merges known planned settings into a fresh controller
// snapshot. A nil band means no radio write is needed for that band.
func buildAPRadioConfig(raw map[string]interface{}, plan *DeviceAPResourceModel) (*client.APRadioConfig, error) {
	config := &client.APRadioConfig{}
	for _, band := range []struct {
		key    string
		values []attr.Value
		target **client.APRadioSetting
	}{
		{"radioSetting2g", []attr.Value{plan.Radio2gEnable, plan.Radio2gChannelWidth, plan.Radio2gChannel, plan.Radio2gTxPower, plan.Radio2gTxPowerLevel}, &config.RadioSetting2g},
		{"radioSetting5g", []attr.Value{plan.Radio5gEnable, plan.Radio5gChannelWidth, plan.Radio5gChannel, plan.Radio5gTxPower, plan.Radio5gTxPowerLevel}, &config.RadioSetting5g},
	} {
		settings, err := mergeAPRadioSettings(raw[band.key], band.values)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", band.key, err)
		}
		*band.target = settings
	}
	return config, nil
}

func mergeAPRadioSettings(raw interface{}, values []attr.Value) (*client.APRadioSetting, error) {
	known := false
	for _, value := range values {
		known = known || (!value.IsNull() && !value.IsUnknown())
	}
	if !known {
		return nil, nil
	}
	current, ok := raw.(map[string]interface{})
	if !ok || current == nil {
		return nil, fmt.Errorf("cannot configure a radio that the controller does not report")
	}

	// Whitelist the five schema-backed writable fields. In particular, do not
	// copy observed frequency, wireless mode, or arbitrary GET response fields.
	keys := []string{"radioEnable", "channelWidth", "channel", "txPower", "txPowerLevel"}
	before := make(map[string]interface{}, len(keys))
	after := make(map[string]interface{}, len(keys))
	for i, key := range keys {
		previous, present := current[key]
		if !present || previous == nil {
			return nil, fmt.Errorf("controller radio snapshot is missing %s", key)
		}
		before[key], after[key] = previous, previous
		value := values[i]
		if value.IsNull() || value.IsUnknown() {
			continue
		}
		switch value := value.(type) {
		case types.Bool:
			after[key] = value.ValueBool()
		case types.String:
			after[key] = value.ValueString()
		case types.Int64:
			after[key] = value.ValueInt64()
		}
	}
	decode := func(fields map[string]interface{}) (client.APRadioSetting, error) {
		var setting client.APRadioSetting
		data, err := json.Marshal(fields)
		if err != nil {
			return setting, err
		}
		err = json.Unmarshal(data, &setting)
		return setting, err
	}
	previous, err := decode(before)
	if err != nil {
		return nil, fmt.Errorf("invalid controller radio settings: %w", err)
	}
	next, err := decode(after)
	if err != nil {
		return nil, fmt.Errorf("invalid planned radio settings: %w", err)
	}
	if next == previous {
		return nil, nil
	}
	return &next, nil
}
