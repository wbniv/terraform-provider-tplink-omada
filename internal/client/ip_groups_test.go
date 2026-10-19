package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

const ipGroupFixture = `{"groupId":"group-1","name":"Fixture","type":1,"resource":0,"description":"keep group note","ipList":[{"ip":"192.0.2.0","mask":24,"description":"keep subnet note"},{"ip":"198.51.100.10","mask":32}],"portType":0,"portList":["80","7000-7100"],"portMaskList":[],"count":4}`

func fixtureGroup() *IPGroup {
	return &IPGroup{Name: "Fixture", Type: 1, IPList: []IPGroupEntry{{IP: "192.0.2.0/24", PortList: []string{"80", "7000-7100"}}, {IP: "198.51.100.10", PortList: []string{"80", "7000-7100"}}}}
}
func TestIPGroupLifecycle(t *testing.T) {
	deleted := false
	patches := 0
	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site/setting/profiles/groups/1": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Errorf("unexpected method %s", r.Method)
			}
			if deleted {
				fmt.Fprint(w, `{"errorCode":0,"result":{"data":[]}}`)
			} else {
				fmt.Fprintf(w, `{"errorCode":0,"result":{"data":[%s]}}`, ipGroupFixture)
			}
		},
		"/sites/site/setting/profiles/groups": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("unexpected method %s", r.Method)
			}
			checkGroupPayload(t, r, false)
			fmt.Fprint(w, `{"errorCode":0,"result":"group-1"}`)
		},
		"/sites/site/setting/profiles/groups/1/group-1": func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case "PATCH":
				patches++
				checkGroupPayload(t, r, true)
			case "DELETE":
				deleted = true
			default:
				t.Errorf("unexpected method %s", r.Method)
			}
			fmt.Fprint(w, `{"errorCode":0}`)
		},
	})
	defer server.Close()
	c := newTestClient(t, server)
	ctx := context.Background()
	group, err := c.CreateIPGroup(ctx, "site", fixtureGroup())
	if err != nil {
		t.Fatal(err)
	}
	if group.ID != "group-1" || group.IPList[0].IP != "192.0.2.0/24" || group.IPList[1].IP != "198.51.100.10" || len(group.IPList[1].PortList) != 2 {
		t.Fatalf("incorrect group mapping: %+v", group)
	}
	if _, err = c.UpdateIPGroup(ctx, "site", group.ID, fixtureGroup()); err != nil {
		t.Fatal(err)
	}
	if patches != 1 {
		t.Fatal("missing update")
	}
	if err = c.DeleteIPGroup(ctx, "site", group.ID); err != nil {
		t.Fatal(err)
	}
	groups, err := c.ListIPGroups(ctx, "site")
	if err != nil || len(groups) != 0 || groups == nil {
		t.Fatalf("empty collection: %v %v", groups, err)
	}
	if _, err = c.GetIPGroup(ctx, "site", group.ID); err == nil {
		t.Fatal("deleted group found")
	}
}
func checkGroupPayload(t *testing.T, r *http.Request, update bool) {
	t.Helper()
	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if string(body["type"]) != "1" || string(body["portType"]) != "0" || string(body["portList"]) != `["80","7000-7100"]` {
		t.Error("wrong group-wide ports/type")
	}
	for _, k := range []string{"groupId", "id", "buildIn", "resource", "count", "site"} {
		if _, ok := body[k]; ok {
			t.Errorf("sent response-only field %s", k)
		}
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(body["ipList"], &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || string(rows[0]["ip"]) != `"192.0.2.0"` || string(rows[0]["mask"]) != "24" || string(rows[1]["mask"]) != "32" {
		t.Fatal("wrong subnet encoding")
	}
	for _, row := range rows {
		if _, ok := row["portList"]; ok {
			t.Error("sent per-entry ports")
		}
	}
	if update && (string(body["description"]) != `"keep group note"` || string(rows[0]["description"]) != `"keep subnet note"`) {
		t.Error("lost descriptions on update")
	}
}
func TestIPGroupValidation(t *testing.T) {
	for _, mode := range []string{"empty IPs", "empty ports", "different ports", "bad IP", "IPv6", "descending ports", "out of range", "wrong type"} {
		t.Run(mode, func(t *testing.T) {
			g := fixtureGroup()
			switch mode {
			case "empty IPs":
				g.IPList = nil
			case "empty ports":
				g.IPList[0].PortList = nil
			case "different ports":
				g.IPList[1].PortList = []string{"443"}
			case "bad IP":
				g.IPList[0].IP = "192.0.2.0/99"
			case "IPv6":
				g.IPList[0].IP = "::1"
			case "descending ports":
				g.IPList[0].PortList = []string{"90-80"}
			case "out of range":
				g.IPList[0].PortList = []string{"65536"}
			case "wrong type":
				g.Type = 0
			}
			c := &Client{}
			if _, err := c.CreateIPGroup(context.Background(), "site", g); err == nil {
				t.Fatal("invalid input reached network")
			}
			if _, err := c.UpdateIPGroup(context.Background(), "site", "group", g); err == nil {
				t.Fatal("invalid update reached network")
			}
		})
	}
}
func TestIPGroupCreateIdentity(t *testing.T) {
	for _, result := range []string{`null`, `{}`, `""`, `" "`, `7`, `{"groupId":"group-1"}`} {
		t.Run(result, func(t *testing.T) {
			server := mockOmadaServer(t, map[string]http.HandlerFunc{"/sites/site/setting/profiles/groups": func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, `{"errorCode":0,"result":%s}`, result) }})
			defer server.Close()
			if _, err := newTestClient(t, server).CreateIPGroup(context.Background(), "site", fixtureGroup()); err == nil {
				t.Fatal("invalid created identity accepted")
			}
		})
	}
}
func TestIPGroupProtectedAndUnsupported(t *testing.T) {
	for _, mode := range []string{"built-in", "port masks", "wrong type", "missing subnet mask"} {
		t.Run(mode, func(t *testing.T) {
			fixture := ipGroupFixture
			switch mode {
			case "built-in":
				fixture = strings.Replace(fixture, `"resource":0`, `"resource":0,"buildIn":true`, 1)
			case "port masks":
				fixture = strings.Replace(fixture, `"portType":0`, `"portType":1`, 1)
			case "wrong type":
				fixture = strings.Replace(fixture, `"type":1`, `"type":0`, 1)
			case "missing subnet mask":
				fixture = strings.Replace(fixture, `"mask":24,`, "", 1)
			}
			server := mockOmadaServer(t, map[string]http.HandlerFunc{"/sites/site/setting/profiles/groups/1": func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{"errorCode":0,"result":{"data":[%s]}}`, fixture)
			}})
			defer server.Close()
			c := newTestClient(t, server)
			if _, err := c.UpdateIPGroup(context.Background(), "site", "group-1", fixtureGroup()); err == nil {
				t.Fatal("unsupported group update accepted")
			}
			if err := c.DeleteIPGroup(context.Background(), "site", "group-1"); err == nil {
				t.Fatal("unsupported group delete accepted")
			}
		})
	}
}
