package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCollectionEmptyState(t *testing.T) {
	var mu sync.Mutex
	mode := "empty"
	cases := []struct {
		source, attribute, route, config string
		row                              map[string]any
		nested                           []string
	}{
		{"sites", "sites", "/sites", "", map[string]any{"id": "site", "name": "fixture"}, nil},
		{"networks", "networks", "/sites/site/setting/lan/networks", "site_id = \"site\"", map[string]any{"id": "network", "name": "fixture", "purpose": "vlan", "vlan": 20}, nil},
		{"wireless_networks", "wireless_networks", "/sites/site/setting/wlans/group/ssids", "site_id = \"site\"\nwlan_group_id = \"group\"", map[string]any{"id": "ssid", "name": "fixture"}, nil},
		{"port_profiles", "port_profiles", "/sites/site/setting/lan/profiles", "site_id = \"site\"", map[string]any{"id": "profile", "name": "fixture"}, []string{"tag_network_ids"}},
		{"devices", "devices", "/sites/site/devices", "site_id = \"site\"", map[string]any{"mac": "00-11-22-33-44-55", "name": "fixture", "type": "ap"}, nil},
		{"firewall_acls", "acl_rules", "/sites/site/setting/firewall/acls", "site_id = \"site\"\ntype = 0", map[string]any{"id": "acl", "name": "fixture", "type": 0}, []string{"protocols", "source_ids", "destination_ids"}},
		{"ip_groups", "ip_groups", "/sites/site/setting/profiles/groups/1", "site_id = \"site\"", map[string]any{"groupId": "group", "name": "fixture", "type": 1, "portType": 0, "portList": []string{"443"}, "ipList": []any{map[string]any{"ip": "192.0.2.1", "mask": 32}}}, nil},
		{"mdns_reflectors", "rules", "/sites/site/setting/service/mdns", "site_id = \"site\"", map[string]any{"id": "mdns", "name": "fixture", "ap": map[string]any{}}, []string{"profile_ids", "service_networks", "client_networks"}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var result any
		switch r.URL.Path {
		case "/api/info":
			result = map[string]any{"omadacId": "controller"}
		case "/controller/api/v2/login":
			result = map[string]any{"token": "fixture"}
		default:
			if r.Method != "GET" {
				t.Errorf("unexpected mutation %s", r.Method)
				w.WriteHeader(500)
				return
			}
			found := false
			for _, tc := range cases {
				if r.URL.Path != "/controller/api/v2"+tc.route {
					continue
				}
				found = true
				if mode == "failure" {
					w.WriteHeader(503)
					return
				}
				if mode == "malformed" {
					result = nil
					break
				}
				rows := []any{}
				if mode == "populated" || mode == "nested populated" {
					row := map[string]any{}
					for k, v := range tc.row {
						row[k] = v
					}
					if mode == "nested populated" {
						switch tc.source {
						case "port_profiles":
							row["tagNetworkIds"] = []string{"network"}
						case "firewall_acls":
							row["protocols"] = []int{6}
							row["sourceIds"] = []string{"network"}
							row["destinationIds"] = []string{"network"}
						case "mdns_reflectors":
							delete(row, "ap")
							row["osg"] = map[string]any{"profileIds": []string{"service"}, "serviceNetworks": []string{"network"}, "clientNetworks": []string{"network"}}
						}
					}
					rows = append(rows, row)
				}
				if tc.source == "devices" {
					result = rows
				} else {
					result = map[string]any{"data": rows, "totalRows": len(rows), "currentPage": 1, "currentSize": 100}
				}
				break
			}
			if !found {
				t.Errorf("unexpected path %s", r.URL.Path)
				w.WriteHeader(404)
				return
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "result": result})
	}))
	defer server.Close()
	var config strings.Builder
	fmt.Fprintf(&config, "provider \"omada\" {\nurl = %q\nusername = \"fixture\"\npassword = \"fixture\"\n}\n", server.URL)
	checks := func(count, nested string) resource.TestCheckFunc {
		var result []resource.TestCheckFunc
		for _, tc := range cases {
			result = append(result, resource.TestCheckOutput(tc.source+"_count", count), resource.TestCheckOutput(tc.source+"_iteration", count), resource.TestCheckResourceAttr("data.omada_"+tc.source+".test", tc.attribute+".#", count))
			for _, field := range tc.nested {
				result = append(result, resource.TestCheckOutput(tc.source+"_"+field, nested))
			}
		}
		return resource.ComposeAggregateTestCheckFunc(result...)
	}
	for _, tc := range cases {
		ref := "data.omada_" + tc.source + ".test." + tc.attribute
		fmt.Fprintf(&config, "data \"omada_%s\" \"test\" {\n%s\n}\noutput \"%s_count\" { value = length(%s) }\noutput \"%s_iteration\" { value = length([for row in %s : row.name]) }\n", tc.source, tc.config, tc.source, ref, tc.source, ref)
		for _, field := range tc.nested {
			fmt.Fprintf(&config, "output \"%s_%s\" { value = sum(concat([0], [for row in %s : length(row.%s)])) }\n", tc.source, field, ref, field)
		}
	}
	setMode := func(next string) func() { return func() { mu.Lock(); defer mu.Unlock(); mode = next } }
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{
		{Config: config.String(), Check: checks("0", "0")},
		{PreConfig: setMode("populated"), Config: config.String(), Check: checks("1", "0")},
		{PreConfig: setMode("nested populated"), Config: config.String(), Check: checks("1", "1")},
		{PreConfig: setMode("empty"), Config: config.String(), Check: checks("0", "0")},
		{PreConfig: setMode("failure"), Config: config.String(), ExpectError: regexp.MustCompile("HTTP error 503")},
		{PreConfig: setMode("malformed"), Config: config.String(), ExpectError: regexp.MustCompile("Error listing")},
		{PreConfig: setMode("empty"), Config: config.String(), Check: checks("0", "0")},
	}})
}
