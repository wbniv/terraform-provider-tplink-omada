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

func TestAccIPGroupCreateReadbackFailure(t *testing.T) {
	var mu sync.Mutex
	exists, failRead := false, false
	creates, deletes := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var result any
		switch r.URL.Path {
		case "/api/info":
			result = map[string]any{"omadacId": "controller"}
		case "/controller/api/v2/login":
			result = map[string]any{"token": "fixture"}
		case "/controller/api/v2/sites/site/setting/profiles/groups":
			if r.Method != "POST" {
				t.Errorf("unexpected method %s", r.Method)
			}
			if exists {
				t.Error("retried creation without cleaning up the recorded group")
				w.WriteHeader(409)
				return
			}
			creates++
			exists = true
			failRead = creates == 1
			result = "group-1"
		case "/controller/api/v2/sites/site/setting/profiles/groups/1":
			if failRead {
				failRead = false
				w.WriteHeader(503)
				return
			}
			data := []any{}
			if exists {
				data = append(data, map[string]any{"groupId": "group-1", "name": "Recovery", "type": 1, "portType": 0, "portList": []string{"80"}, "ipList": []any{map[string]any{"ip": "192.0.2.1", "mask": 32}}})
			}
			result = map[string]any{"data": data}
		case "/controller/api/v2/sites/site/setting/profiles/groups/1/group-1":
			if r.Method != "DELETE" {
				t.Errorf("unexpected method %s", r.Method)
			}
			deletes++
			exists = false
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "result": result})
	}))
	defer server.Close()
	config := fmt.Sprintf(`provider "omada" {
 url = %q
 username = "fixture"
 password = "fixture"
}
resource "omada_ip_group" "test" {
 site_id = "site"
 name = "Recovery"
 ip_list = [{ip = "192.0.2.1", port_list = ["80"]}]
}`, server.URL)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			mu.Lock()
			defer mu.Unlock()
			if exists {
				return fmt.Errorf("orphan group survived destroy")
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: config, ExpectError: regexp.MustCompile("readback failed")},
			{Config: config, Check: func(s *terraform.State) error {
				mu.Lock()
				defer mu.Unlock()
				if creates != 2 || deletes != 1 {
					return fmt.Errorf("recovery did not replace the recorded failed create: creates=%d deletes=%d", creates, deletes)
				}
				return resource.TestCheckResourceAttr("omada_ip_group.test", "id", "group-1")(s)
			}},
			{Config: config, PlanOnly: true},
		}})
}
