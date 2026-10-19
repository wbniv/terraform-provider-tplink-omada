package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// IPGroupEntry retains the Terraform representation. The controller stores
// subnet masks separately and one common port list for the whole group.
type IPGroupEntry struct {
	IP          string   `json:"ip"`
	PortList    []string `json:"portList,omitempty"`
	Description string   `json:"-"`
}
type IPGroup struct {
	ID          string         `json:"id,omitempty"`
	Name        string         `json:"name"`
	Type        int            `json:"type"`
	IPList      []IPGroupEntry `json:"ipList"`
	Description string         `json:"-"`
	BuiltIn     bool           `json:"-"`
}
type ipSubnetWire struct {
	IP          string `json:"ip"`
	Mask        *int   `json:"mask"`
	Description string `json:"description,omitempty"`
}
type ipGroupWire struct {
	ID           string            `json:"groupId,omitempty"`
	Name         string            `json:"name"`
	Type         int               `json:"type"`
	IPList       []ipSubnetWire    `json:"ipList"`
	PortType     int               `json:"portType"`
	PortList     []string          `json:"portList"`
	PortMaskList []json.RawMessage `json:"portMaskList,omitempty"`
	Description  string            `json:"description,omitempty"`
	BuiltIn      bool              `json:"buildIn,omitempty"`
}

const ipGroupsRoute = "/setting/profiles/groups"

func ipPrefix(value string) (netip.Prefix, error) {
	if strings.Contains(value, "/") {
		p, err := netip.ParsePrefix(value)
		if err != nil || !p.Addr().Is4() {
			return netip.Prefix{}, fmt.Errorf("IP group entries must be IPv4 addresses or CIDR subnets")
		}
		return p, nil
	}
	ip, err := netip.ParseAddr(value)
	if err != nil || !ip.Is4() {
		return netip.Prefix{}, fmt.Errorf("IP group entries must be IPv4 addresses or CIDR subnets")
	}
	return netip.PrefixFrom(ip, 32), nil
}
func groupToWire(group *IPGroup) (*ipGroupWire, error) {
	if group == nil || group.Type != 1 {
		return nil, fmt.Errorf("only IPv4 IP/Port groups (type 1) are supported")
	}
	if strings.TrimSpace(group.Name) == "" || len(group.IPList) == 0 {
		return nil, fmt.Errorf("IP/Port group requires a name and at least one IP entry")
	}
	ports := group.IPList[0].PortList
	if len(ports) == 0 {
		return nil, fmt.Errorf("IP/Port groups require a nonempty port_list on every IP entry")
	}
	for _, port := range ports {
		parts := strings.Split(port, "-")
		if len(parts) > 2 {
			return nil, fmt.Errorf("invalid IP group port range")
		}
		last := -1
		for _, part := range parts {
			n, err := strconv.Atoi(part)
			if err != nil || n < 0 || n > 65535 || n < last {
				return nil, fmt.Errorf("IP group ports must be numbers or ascending ranges from 0 to 65535")
			}
			last = n
		}
	}
	result := &ipGroupWire{Name: group.Name, Type: 1, PortList: slices.Clone(ports), IPList: make([]ipSubnetWire, 0, len(group.IPList)), Description: group.Description}
	for _, entry := range group.IPList {
		if !slices.Equal(ports, entry.PortList) {
			return nil, fmt.Errorf("Omada applies ports to the whole group: every IP entry must have the same port_list")
		}
		prefix, err := ipPrefix(entry.IP)
		if err != nil {
			return nil, err
		}
		bits := prefix.Bits()
		result.IPList = append(result.IPList, ipSubnetWire{IP: prefix.Addr().String(), Mask: &bits, Description: entry.Description})
	}
	return result, nil
}
func groupFromWire(w ipGroupWire) (*IPGroup, error) {
	if w.ID == "" || w.Type != 1 || w.PortType != 0 || len(w.PortMaskList) > 0 || len(w.IPList) == 0 || len(w.PortList) == 0 {
		return nil, &ProtocolError{reason: "unsupported IP/Port group identity, type, empty entries/ports or port-mask mode"}
	}
	group := &IPGroup{ID: w.ID, Name: w.Name, Type: w.Type, IPList: make([]IPGroupEntry, 0, len(w.IPList)), Description: w.Description, BuiltIn: w.BuiltIn}
	for _, entry := range w.IPList {
		ip, err := netip.ParseAddr(entry.IP)
		if err != nil || !ip.Is4() || entry.Mask == nil || *entry.Mask < 0 || *entry.Mask > 32 {
			return nil, &ProtocolError{reason: "invalid IPv4 group subnet"}
		}
		value := ip.String()
		if *entry.Mask != 32 {
			value = netip.PrefixFrom(ip, *entry.Mask).String()
		}
		group.IPList = append(group.IPList, IPGroupEntry{IP: value, PortList: slices.Clone(w.PortList), Description: entry.Description})
	}
	return group, nil
}

// ListIPGroups lists type-1 IPv4 IP/Port groups; other profile types are separate APIs.
func (c *Client) ListIPGroups(ctx context.Context, siteID string) ([]IPGroup, error) {
	rows, err := listCollection[ipGroupWire](ctx, c, c.siteURL(siteID, ipGroupsRoute+"/1"), "groupId", completeDataCollection)
	if err != nil {
		return nil, err
	}
	groups := make([]IPGroup, 0, len(rows))
	for _, row := range rows {
		group, err := groupFromWire(row)
		if err != nil {
			return nil, err
		}
		groups = append(groups, *group)
	}
	return groups, nil
}
func (c *Client) GetIPGroup(ctx context.Context, siteID, groupID string) (*IPGroup, error) {
	groups, err := c.ListIPGroups(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		if group.ID == groupID {
			return &group, nil
		}
	}
	return nil, fmt.Errorf("IP group %q not found", groupID)
}
func (c *Client) CreateIPGroup(ctx context.Context, siteID string, group *IPGroup) (*IPGroup, error) {
	if c.readOnly {
		return nil, ErrReadOnly
	}
	payload, err := groupToWire(group)
	if err != nil {
		return nil, err
	}
	resp, err := c.doSiteRequest(ctx, siteID, http.MethodPost, ipGroupsRoute, payload)
	if err != nil {
		return nil, err
	}
	var id string
	if json.Unmarshal(resp.Result, &id) != nil || strings.TrimSpace(id) == "" {
		return nil, &ProtocolError{reason: "created IP group response requires a nonempty string identity"}
	}
	created, err := c.GetIPGroup(ctx, siteID, id)
	if err != nil {
		return &IPGroup{ID: id}, fmt.Errorf("IP group was created but readback failed: %w", err)
	}
	return created, nil
}
func (c *Client) UpdateIPGroup(ctx context.Context, siteID, groupID string, group *IPGroup) (*IPGroup, error) {
	if c.readOnly {
		return nil, ErrReadOnly
	}
	payload, err := groupToWire(group)
	if err != nil {
		return nil, err
	}
	existing, err := c.GetIPGroup(ctx, siteID, groupID)
	if err != nil {
		return nil, err
	}
	if existing.BuiltIn {
		return nil, fmt.Errorf("built-in IP groups cannot be modified")
	}
	payload.Description = existing.Description
	for i := range payload.IPList {
		for _, entry := range existing.IPList {
			old, _ := ipPrefix(entry.IP)
			if old.Addr().String() == payload.IPList[i].IP && old.Bits() == *payload.IPList[i].Mask {
				payload.IPList[i].Description = entry.Description
				break
			}
		}
	}
	_, err = c.doSiteRequest(ctx, siteID, http.MethodPatch, ipGroupsRoute+"/1/"+url.PathEscape(groupID), payload)
	if err != nil {
		return nil, err
	}
	return c.GetIPGroup(ctx, siteID, groupID)
}
func (c *Client) DeleteIPGroup(ctx context.Context, siteID, groupID string) error {
	if c.readOnly {
		return ErrReadOnly
	}
	existing, err := c.GetIPGroup(ctx, siteID, groupID)
	if err != nil {
		return err
	}
	if existing.BuiltIn {
		return fmt.Errorf("built-in IP groups cannot be deleted")
	}
	_, err = c.doSiteRequest(ctx, siteID, http.MethodDelete, ipGroupsRoute+"/1/"+url.PathEscape(groupID), nil)
	return err
}
