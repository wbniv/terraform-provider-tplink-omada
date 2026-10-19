package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
)

// CreateACLRule resolves an empty success acknowledgment using complete inventories.
// No existing rule is adopted by name, and an uncertain write is never repeated here.
func (c *Client) CreateACLRule(ctx context.Context, siteID string, rule *ACLRule) (*ACLRule, error) {
	if c.readOnly {
		return nil, fmt.Errorf("client is read-only")
	}
	if rule == nil || strings.TrimSpace(rule.Name) == "" {
		return nil, fmt.Errorf("ACL name is required")
	}
	before, err := c.ListACLRules(ctx, siteID, rule.Type)
	if err != nil {
		return nil, fmt.Errorf("ACL create preflight: %w", err)
	}
	ids := make(map[string]bool, len(before))
	for _, r := range before {
		ids[r.ID] = true
		if r.Name == rule.Name {
			return nil, fmt.Errorf("ACL with this name already exists; inspect and import the intended rule using siteID/aclType/ruleID before retrying")
		}
	}
	payload := *rule
	payload.ID = ""
	payload.Index = 0
	_, err = c.doSiteRequest(ctx, siteID, http.MethodPost, "/setting/firewall/acls", &payload)
	if err != nil {
		return nil, fmt.Errorf("ACL creation did not complete: %w; inspect the controller before retrying if the write outcome is uncertain", err)
	}
	after, err := c.ListACLRules(ctx, siteID, rule.Type)
	recovery := "ACL creation was accepted but its identity could not be confirmed; inspect the controller and import the intended rule using siteID/aclType/ruleID before retrying"
	if err != nil {
		return nil, fmt.Errorf("%s: %w", recovery, err)
	}
	var matches []ACLRule
	for _, r := range after {
		if !ids[r.ID] && r.Name == rule.Name {
			matches = append(matches, r)
		}
	}
	if len(matches) != 1 || !sameACLConfiguration(matches[0], *rule) {
		return nil, fmt.Errorf("%s (new same-name matches: %d)", recovery, len(matches))
	}
	return &matches[0], nil
}

func sameACLConfiguration(a, b ACLRule) bool {
	return a.Name == b.Name && a.Type == b.Type && a.Status == b.Status && a.Policy == b.Policy &&
		slices.Equal(a.Protocols, b.Protocols) && a.SourceType == b.SourceType && slices.Equal(a.SourceIDs, b.SourceIDs) &&
		a.DestinationType == b.DestinationType && slices.Equal(a.DestinationIDs, b.DestinationIDs) &&
		a.Direction.LanToWan == b.Direction.LanToWan && a.Direction.LanToLan == b.Direction.LanToLan &&
		slices.Equal(a.Direction.WanInIDs, b.Direction.WanInIDs) && slices.Equal(a.Direction.VpnInIDs, b.Direction.VpnInIDs) &&
		a.BiDirectional == b.BiDirectional && a.StateMode == b.StateMode && a.IPSec == b.IPSec && a.Syslog == b.Syslog
}

// UpdateACLRule overlays Terraform-owned fields on the current rule. Preserve
// ordering, unmodeled settings, and direction selectors outside the schema.
func (c *Client) UpdateACLRule(ctx context.Context, siteID, ruleID string, rule *ACLRule) (*ACLRule, error) {
	if c.readOnly {
		return nil, fmt.Errorf("client is read-only")
	}
	rows, err := listAll[map[string]json.RawMessage](ctx, c, c.siteURL(siteID, "/setting/firewall/acls")+fmt.Sprintf("&type=%d", rule.Type), "id")
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	for _, row := range rows {
		var id string
		_ = json.Unmarshal(row["id"], &id)
		if id == ruleID {
			raw = row
			break
		}
	}
	if raw == nil {
		return nil, fmt.Errorf("ACL rule %q not found", ruleID)
	}
	var currentType int
	if err := json.Unmarshal(raw["type"], &currentType); err != nil || currentType != rule.Type {
		return nil, fmt.Errorf("ACL type cannot change during update")
	}
	fields := map[string]any{"name": rule.Name, "type": rule.Type, "status": rule.Status, "policy": rule.Policy, "protocols": rule.Protocols, "sourceType": rule.SourceType, "sourceIds": rule.SourceIDs, "destinationType": rule.DestinationType, "destinationIds": rule.DestinationIDs, "biDirectional": rule.BiDirectional}
	for k, v := range fields {
		raw[k], err = json.Marshal(v)
		if err != nil {
			return nil, err
		}
	}
	direction := map[string]json.RawMessage{}
	if d, ok := raw["direction"]; ok {
		if err := json.Unmarshal(d, &direction); err != nil {
			return nil, fmt.Errorf("decoding ACL direction: %w", err)
		}
	}
	if direction == nil {
		direction = map[string]json.RawMessage{}
	}
	direction["lanToWan"], _ = json.Marshal(rule.Direction.LanToWan)
	direction["lanToLan"], _ = json.Marshal(rule.Direction.LanToLan)
	raw["direction"], _ = json.Marshal(direction)
	delete(raw, "id")
	delete(raw, "siteId")
	if _, err := c.doSiteRequest(ctx, siteID, http.MethodPut, fmt.Sprintf("/setting/firewall/acls/%s", ruleID), raw); err != nil {
		return nil, err
	}
	return c.GetACLRule(ctx, siteID, ruleID, rule.Type)
}
