package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccNetworkAdoptMatchingStateUsesControllerValues(t *testing.T) {
	var mu sync.Mutex
	exists := true
	posts, deletes := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		var result any
		switch {
		case r.URL.Path == "/api/info":
			result = map[string]any{"omadacId": "controller", "controllerVer": "6.1.0.19", "apiVer": "3"}
		case r.URL.Path == "/controller/api/v2/login":
			result = map[string]any{"token": "fixture-token"}
		case r.URL.Path == "/controller/api/v2/sites/site/setting/lan/networks" && r.Method == http.MethodGet:
			data := []any{}
			if exists {
				data = append(data, map[string]any{
					"id": "existing-network", "name": "Office VLAN", "purpose": "vlan", "vlan": 20,
					"igmpSnoopEnable": true,
				})
			}
			result = map[string]any{"totalRows": len(data), "currentPage": 1, "currentSize": 100, "data": data}
		case r.URL.Path == "/controller/api/v2/sites/site/setting/lan/networks" && r.Method == http.MethodPost:
			posts++
			w.WriteHeader(http.StatusInternalServerError)
			return
		case r.URL.Path == "/controller/api/v2/sites/site/setting/lan/networks/existing-network" && r.Method == http.MethodDelete:
			deletes++
			exists = false
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "msg": "Success.", "result": result})
	}))
	defer server.Close()

	config := fmt.Sprintf(`provider "omada" {
  url      = %q
  username = "fixture"
  password = "fixture"
}

resource "omada_network" "adopted" {
  site_id = "site"
  name    = "Office VLAN"
  purpose = "vlan"
  vlan_id = 20
}
`, server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: config,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("omada_network.adopted", "id", "existing-network"),
				resource.TestCheckResourceAttr("omada_network.adopted", "igmp_snoop_enable", "true"),
			),
		}},
		CheckDestroy: func(_ *terraform.State) error {
			mu.Lock()
			defer mu.Unlock()
			if exists || posts != 0 || deletes != 1 {
				return fmt.Errorf("adoption cleanup state: exists=%t creates=%d deletes=%d", exists, posts, deletes)
			}
			return nil
		},
	})
}

func TestAccNetworkDeleteProtectsBuiltInDefault(t *testing.T) {
	var mu sync.Mutex
	name := "Default"
	exists := true
	deletes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		var result any
		switch {
		case r.URL.Path == "/api/info":
			result = map[string]any{"omadacId": "controller", "controllerVer": "6.1.0.19", "apiVer": "3"}
		case r.URL.Path == "/controller/api/v2/login":
			result = map[string]any{"token": "fixture-token"}
		case r.URL.Path == "/controller/api/v2/sites/site/setting/lan/networks" && r.Method == http.MethodGet:
			data := []any{}
			if exists {
				data = append(data, map[string]any{
					"id": "default-network", "name": name, "purpose": "interface", "vlan": 1,
					"gatewaySubnet": "192.0.2.1/24", "igmpSnoopEnable": false,
				})
			}
			result = map[string]any{"totalRows": len(data), "currentPage": 1, "currentSize": 100, "data": data}
		case r.URL.Path == "/controller/api/v2/sites/site/setting/lan/networks/default-network" && r.Method == http.MethodDelete:
			deletes++
			exists = false
		case r.URL.Path == "/controller/api/v2/sites/site/setting/lan/networks/default-network" && r.Method == http.MethodPatch:
			var update struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
				t.Errorf("decoding network update: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			name = update.Name
			result = map[string]any{
				"id": "default-network", "name": name, "purpose": "interface", "vlan": 1,
				"gatewaySubnet": "192.0.2.1/24", "igmpSnoopEnable": false,
			}
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "msg": "Success.", "result": result})
	}))
	defer server.Close()

	config := func(networkName string) string {
		return fmt.Sprintf(`provider "omada" {
  url      = %q
  username = "fixture"
  password = "fixture"
}

resource "omada_network" "default" {
  site_id        = "site"
  name           = %q
  purpose        = "interface"
  vlan_id        = 1
  gateway_subnet = "192.0.2.1/24"
}
`, server.URL, networkName)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config("Default"), Check: resource.TestCheckResourceAttr("omada_network.default", "id", "default-network")},
			{Config: config("Default"), Destroy: true, ExpectError: regexp.MustCompile("Protected default network")},
			{Config: config("Cleanup Default")},
		},
		CheckDestroy: func(_ *terraform.State) error {
			mu.Lock()
			defer mu.Unlock()
			if exists || deletes != 1 {
				return fmt.Errorf("cleanup state: exists=%t deletes=%d", exists, deletes)
			}
			return nil
		},
	})
}
