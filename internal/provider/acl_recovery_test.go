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

func TestAccACLCreateReadbackFailure(t *testing.T) {
	var mu sync.Mutex
	var rule map[string]any
	posts := 0
	failRead := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var result any
		switch r.URL.Path {
		case "/api/info":
			result = map[string]any{"omadacId": "controller"}
		case "/controller/api/v2/login":
			result = map[string]any{"token": "fixture"}
		case "/controller/api/v2/sites/site/setting/firewall/acls":
			if r.Method == "POST" {
				posts++
				if posts > 1 {
					t.Error("duplicate ACL creation")
				}
				_ = json.NewDecoder(r.Body).Decode(&rule)
				rule["id"] = "rule"
				rule["index"] = 1
				failRead = true
			} else {
				if failRead {
					failRead = false
					w.WriteHeader(503)
					return
				}
				data := []any{}
				if rule != nil {
					data = append(data, rule)
				}
				result = map[string]any{"data": data, "totalRows": len(data), "currentPage": 1, "currentSize": 100}
			}
		case "/controller/api/v2/sites/site/setting/firewall/acls/rule":
			if r.Method != "DELETE" {
				t.Errorf("unexpected method %s", r.Method)
			}
			rule = nil
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
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
resource "omada_firewall_acl" "test" {
 site_id = "site"
 name = "Recovery"
 type = 0
 status = false
 policy = 1
 protocols = [6]
 source_type = 2
 source_ids = ["group"]
 destination_type = 2
 destination_ids = ["group"]
}`, server.URL)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			mu.Lock()
			defer mu.Unlock()
			if rule != nil {
				return fmt.Errorf("ACL survived destroy")
			}
			if posts != 1 {
				return fmt.Errorf("POST count=%d", posts)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: config, ExpectError: regexp.MustCompile("identity could not be confirmed")},
			{Config: config, ExpectError: regexp.MustCompile("already exists")},
			{Config: config, ResourceName: "omada_firewall_acl.test", ImportState: true, ImportStateId: "site/0/rule", ImportStatePersist: true},
			{Config: config, PlanOnly: true, Check: resource.TestCheckResourceAttr("omada_firewall_acl.test", "id", "rule")},
		}})
}
