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

func TestAccDeviceAPSettingsPreservation(t *testing.T) {
	var mu sync.Mutex
	const apPath = "/controller/api/v2/sites/site/eaps/02-00-00-00-00-01"
	current := map[string]any{
		"mac": "02-00-00-00-00-01", "name": "Original", "wlanId": "group", "model": "EAP670",
		"ip": "192.0.2.20", "firmwareVersion": "1.0.0", "ledSetting": 0,
		"ipSetting":      map[string]any{"mode": "static", "fallback": false, "useFixedAddr": true},
		"radioSetting2g": map[string]any{"radioEnable": true, "channelWidth": "4", "channel": "0", "txPower": 20, "txPowerLevel": 3},
		"radioSetting5g": map[string]any{"radioEnable": true, "channelWidth": "5", "channel": "36", "txPower": 18, "txPowerLevel": 3},
		"ofdmaEnable2g":  false, "ofdmaEnable5g": true, "loopbackDetectEnable": false,
		"mvlanEnable": false, "lbSetting2g": map[string]any{"lbEnable": true, "maxClients": 23},
		"lbSetting5g":   map[string]any{"lbEnable": false, "maxClients": 19},
		"rssiSetting2g": map[string]any{"rssiEnable": true, "threshold": -70},
		"rssiSetting5g": map[string]any{"rssiEnable": false, "threshold": -75},
		"lldpEnable":    1, "l3AccessSetting": map[string]any{"enable": true},
	}
	var generalBodies []map[string]any
	var advancedBodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		var result any
		switch {
		case r.URL.Path == "/api/info":
			result = map[string]any{"omadacId": "controller", "controllerVer": "6.1.0.19", "apiVer": "3"}
		case r.URL.Path == "/controller/api/v2/login":
			result = map[string]any{"token": "fixture-token"}
		case r.Method == http.MethodGet && r.URL.Path == apPath:
			result = current
		case r.Method == http.MethodPatch && r.URL.Path == apPath:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode AP PATCH: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			generalBodies = append(generalBodies, body)
			for key, value := range body {
				current[key] = value
			}
			result = current
		case r.Method == http.MethodPut && r.URL.Path == apPath+"/config/advanced":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode AP advanced PUT: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			advancedBodies = append(advancedBodies, body)
			for key, value := range body {
				current[key] = value
			}
		case r.Method == http.MethodPut && r.URL.Path == apPath+"/config/services":
		case r.Method == http.MethodPut && r.URL.Path == apPath+"/config/radios":
			t.Errorf("unexpected radio update during AP settings preservation test")
			w.WriteHeader(http.StatusInternalServerError)
			return
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "msg": "Success.", "result": result})
	}))
	defer server.Close()

	config := func(name, extra string) string {
		return fmt.Sprintf(`provider "omada" {
  url      = %q
  username = "fixture"
  password = "fixture"
}

resource "omada_device_ap" "test" {
  name = %q
  %s
}
`, server.URL, name, extra)
	}
	assertRemote := func(_ *terraform.State) error {
		mu.Lock()
		defer mu.Unlock()
		ip := current["ipSetting"].(map[string]any)
		if current["ledSetting"] != float64(0) && current["ledSetting"] != 0 {
			return fmt.Errorf("LED setting changed: %v", current["ledSetting"])
		}
		if ip["mode"] != "static" {
			return fmt.Errorf("unmanaged static IP mode changed: %v", ip["mode"])
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("Original", "") + `import {
  to = omada_device_ap.test
  id = "site/02-00-00-00-00-01"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_device_ap.test", "led_setting", "0"),
					resource.TestCheckResourceAttr("omada_device_ap.test", "ip_setting_mode", "static"),
					resource.TestCheckResourceAttr("omada_device_ap.test", "lb_2g_max_clients", "23"),
					resource.TestCheckResourceAttr("omada_device_ap.test", "rssi_5g_threshold", "-75"),
				),
			},
			{
				Config: config("Renamed", ""),
				Check: resource.ComposeAggregateTestCheckFunc(assertRemote,
					resource.TestCheckResourceAttr("omada_device_ap.test", "led_setting", "0"),
					resource.TestCheckResourceAttr("omada_device_ap.test", "ip_setting_mode", "static")),
			},
			{
				Config: config("Renamed", "lb_2g_enable = false"),
				Check: resource.ComposeAggregateTestCheckFunc(assertRemote, func(_ *terraform.State) error {
					mu.Lock()
					defer mu.Unlock()
					group := current["lbSetting2g"].(map[string]any)
					if group["lbEnable"] != false || group["maxClients"] != float64(23) && group["maxClients"] != 23 {
						return fmt.Errorf("load balancing sibling changed: %#v", group)
					}
					if len(advancedBodies) == 0 {
						return fmt.Errorf("missing advanced settings PUT")
					}
					bodyGroup := advancedBodies[len(advancedBodies)-1]["lbSetting2g"].(map[string]any)
					if bodyGroup["lbEnable"] != false || bodyGroup["maxClients"] != float64(23) {
						return fmt.Errorf("unexpected load balancing payload: %#v", bodyGroup)
					}
					return nil
				}),
			},
			{
				Config: config("Renamed", "lb_2g_enable = false\nrssi_5g_threshold = -81"),
				Check: resource.ComposeAggregateTestCheckFunc(assertRemote, func(_ *terraform.State) error {
					mu.Lock()
					defer mu.Unlock()
					group := current["rssiSetting5g"].(map[string]any)
					if group["rssiEnable"] != false || group["threshold"] != float64(-81) && group["threshold"] != -81 {
						return fmt.Errorf("unexpected RSSI state: %#v", group)
					}
					bodyGroup := advancedBodies[len(advancedBodies)-1]["rssiSetting5g"].(map[string]any)
					if bodyGroup["rssiEnable"] != false || bodyGroup["threshold"] != float64(-81) {
						return fmt.Errorf("unexpected RSSI payload: %#v", bodyGroup)
					}
					return nil
				}),
			},
			{
				Config:      config("Static Rejected", "ip_setting_mode = \"static\""),
				ExpectError: regexp.MustCompile("Unsupported AP IP setting mode"),
			},
		},
	})
}
