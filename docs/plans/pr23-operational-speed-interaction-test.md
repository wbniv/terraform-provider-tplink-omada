# Switch port operational-speed write invariant

## Goal

Add a regression guard for the switch-port write invariant that PR #23 relies on: operational `speed` is controller observation, never configured `linkSpeed`. This test is not a test of PR #23's computed-only schema change; it passes on the base branch with or without that schema change. It complements the speed-only test by exercising a real port write while operational speed changes.

These checks exercise the provider against an HTTP mock. They do not verify behavior against an Omada controller or physical switch.

## Work

1. Use an isolated worktree at PR #23 head `2ec46a4ba19cdbda4a2ee6c1546a40bb5324b594`.
2. Import a mock switch reporting port 7 name `Test-Port`, observed speed 1, and configured `linkSpeed` 0.
3. Plan a rename to `Renamed-Port`. During the port `PATCH`, change the mock's observed speed to 3 so readback differs from the planning-time observation.
4. Require an update action, inspect the exact standard-switch port endpoint, require the new name, and reject `speed` and `linkSpeed` fields in the payload. Verify readback and exactly one port update.
5. Run the focused acceptance tests and relevant Go tests, vet, build, and formatting checks in the PR worktree.

## Acceptance criteria

- The mixed name/speed change completes with an update action and exactly one expected port-name write.
- The captured port update contains `name: Renamed-Port` and contains neither `speed` nor `linkSpeed`.
- Speed changes from 1 to 3 after planning; readback records `Renamed-Port` and operational speed 3 without an inconsistent-result error.
- Existing speed-only drift coverage remains intact.
- The plan and results explain this invariant test passes with or without PR #23's schema change and distinguish mock evidence from live-controller validation.

## Status

Implementation and verification are complete. On PR #23 head plus this test, all three switch acceptance tests passed with Terraform 1.15.8. `go test ./...`, `go vet ./...`, `go build ./...`, and `gofmt -l .` passed with Go 1.25.8. The new test also passes on base `main` without PR #23's schema change, confirming it guards a shared invariant rather than the schema change itself. This is mock/local evidence, not live-controller validation; the test changes no provider behavior.

## Simulated PR diff

This exact unified diff is generated from the test-only commit against PR #23 head `2ec46a4ba19cdbda4a2ee6c1546a40bb5324b594`.

```diff
diff --git a/internal/provider/device_switch_test.go b/internal/provider/device_switch_test.go
index 3368afb..1922d82 100644
--- a/internal/provider/device_switch_test.go
+++ b/internal/provider/device_switch_test.go
@@ -97,6 +97,124 @@ import {
 	})
 }
 
+// This mock-controller regression test guards the invariant that operational
+// speed is never sent as configured port linkSpeed during an intentional update.
+// It passes with or without the computed-only schema change in PR #23.
+func TestAccDeviceSwitchOperationalSpeedDuringPortUpdate(t *testing.T) {
+	var speed atomic.Int64
+	speed.Store(1)
+	var renamed atomic.Bool
+	var portUpdateCount atomic.Int64
+	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		w.Header().Set("Content-Type", "application/json")
+		var result any
+		switch {
+		case r.Method == http.MethodGet && r.URL.Path == "/api/info":
+			result = map[string]any{"omadacId": "controller"}
+		case r.Method == http.MethodPost && r.URL.Path == "/controller/api/v2/login":
+			result = map[string]any{"token": "test-token"}
+		case r.Method == http.MethodGet && r.URL.Path == "/controller/api/v2/sites/site/switches/02-00-00-00-00-01":
+			portName := "Test-Port"
+			if renamed.Load() {
+				portName = "Renamed-Port"
+			}
+			result = map[string]any{
+				"mac": "02-00-00-00-00-01", "name": "Test-Switch", "ledSetting": 2,
+				"ipSetting": map[string]any{"mode": "dhcp"},
+				"ports": []map[string]any{{
+					"port": 7, "name": portName, "speed": speed.Load(), "linkSpeed": 0,
+					"portStatus": map[string]any{"linkStatus": 1, "linkSpeed": speed.Load()},
+				}},
+			}
+		case r.Method == http.MethodPut && r.URL.Path == "/controller/api/v2/sites/site/switches/02-00-00-00-00-01/config/general":
+			// The resource updates switch-level settings before the port PATCH.
+			result = map[string]any{}
+		case r.Method == http.MethodPut && r.URL.Path == "/controller/api/v2/sites/site/switches/02-00-00-00-00-01/config/service":
+			result = map[string]any{}
+		case r.Method == http.MethodPatch && r.URL.Path == "/controller/api/v2/sites/site/switches/02-00-00-00-00-01/ports/7":
+			var portConfig map[string]any
+			if err := json.NewDecoder(r.Body).Decode(&portConfig); err != nil {
+				t.Errorf("decoding port update: %v", err)
+				w.WriteHeader(http.StatusBadRequest)
+				return
+			}
+			if got := portConfig["name"]; got != "Renamed-Port" {
+				t.Errorf("port update name = %v, want Renamed-Port", got)
+			}
+			for _, field := range []string{"speed", "linkSpeed"} {
+				if got, ok := portConfig[field]; ok {
+					t.Errorf("port update unexpectedly contains %s=%v; operational speed must not be configured", field, got)
+				}
+			}
+			portUpdateCount.Add(1)
+			renamed.Store(true)
+			// Change operational speed after planning, during the write itself.
+			speed.Store(3)
+			result = map[string]any{}
+		default:
+			t.Errorf("unexpected controller request: %s %s", r.Method, r.URL.Path)
+			w.WriteHeader(http.StatusMethodNotAllowed)
+			return
+		}
+		if err := json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "result": result}); err != nil {
+			t.Errorf("encoding controller response: %v", err)
+		}
+	}))
+	defer server.Close()
+
+	config := fmt.Sprintf(`
+provider "omada" {
+  url      = %q
+  username = "test"
+  password = "test"
+}
+
+resource "omada_device_switch" "test" {
+  name = "Test-Switch"
+  ports = [{ port = 7, name = "%%s" }]
+}
+`, server.URL)
+
+	resource.Test(t, resource.TestCase{
+		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
+		Steps: []resource.TestStep{
+			{
+				Config: fmt.Sprintf(config, "Test-Port") + `
+import {
+  to = omada_device_switch.test
+  id = "site/02-00-00-00-00-01"
+}
+`,
+				Check: resource.ComposeAggregateTestCheckFunc(
+					resource.TestCheckResourceAttr("omada_device_switch.test", "ports.0.name", "Test-Port"),
+					resource.TestCheckResourceAttr("omada_device_switch.test", "ports.0.speed", "1"),
+				),
+			},
+			{
+				Config: fmt.Sprintf(config, "Renamed-Port"),
+				ConfigPlanChecks: resource.ConfigPlanChecks{
+					PreApply: []plancheck.PlanCheck{
+						plancheck.ExpectResourceAction("omada_device_switch.test", plancheck.ResourceActionUpdate),
+					},
+				},
+				Check: resource.ComposeAggregateTestCheckFunc(
+					resource.TestCheckResourceAttr("omada_device_switch.test", "ports.0.name", "Renamed-Port"),
+					resource.TestCheckResourceAttr("omada_device_switch.test", "ports.0.speed", "3"),
+				),
+			},
+			{
+				Config: fmt.Sprintf(config, "Renamed-Port"),
+				ConfigPlanChecks: resource.ConfigPlanChecks{
+					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
+				},
+			},
+		},
+	})
+	if got := portUpdateCount.Load(); got != 1 {
+		t.Errorf("mock controller observed %d port updates, want exactly 1", got)
+	}
+}
+
 func TestAccDeviceSwitchRejectConfiguredSpeed(t *testing.T) {
 	resource.Test(t, resource.TestCase{
 		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
```
