# D2 — SSID VLAN state

Prepared for Will on 2026-10-19. Implemented, locally verified, committed, and pushed as [0bbd076](https://github.com/wbniv/terraform-provider-tplink-omada/commit/0bbd07687fe76f7203779536bc9b5a24095aa6d3) in `/home/will/terraform-provider-tplink-omada-ssid-vlan`, branch `fix/ssid-vlan-state`, based on D1 commit `80991a5197674a2ee5e2d93ab8d3b571ef4ea774`. Review D2 against that base; its write/readback validation deliberately uses D1's preservation and secret-handling changes. Parent: [SSID repair plan](../../plans/ssid-reliability.md).

## Contract and scope

- [x] Reproduce F14 on D1: refresh retains a custom VLAN after the controller selects default mode. Import and `omada_wireless_networks` also read stale VLAN fields without checking the selected mode.
- [x] Use one mapper for refresh, import, the SSID data source, and create/update readback. Mode 0 always maps to null, including responses with stale custom/effective VLAN fields.
- [x] In mode 1, prefer a valid bridge VLAN, with `currentVlanId` fallback only when no bridge VLAN is supplied. Require VLAN 1–4094. Reject unsupported custom modes and ambiguous fallback lists.
- [x] Distinguish an explicit mode 0 from absent/null mode using a pointer in the API model. Missing settings or modes are errors, not evidence that the SSID uses its default VLAN.
- [x] Return diagnostics for invalid/unsupported snapshots without replacing prior resource state or publishing partial data-source/import state.
- [x] Detect create/update VLAN readback mismatches. Save known observed state with the error; retain prior state when update readback is uninterpretable. Retain a newly created object's ID when a decoded readback object has invalid VLAN data, so Terraform can clean it up.
- [x] Complete real Terraform drift/import tests, full container gates, D1 regression and A compatibility checks, documentation, and final evidence.
- [ ] Validate the mapping and response variants on a controlled controller/AP.
- [x] Commit and push D2 as [0bbd076](https://github.com/wbniv/terraform-provider-tplink-omada/commit/0bbd07687fe76f7203779536bc9b5a24095aa6d3); committed hashes match the tested files.
- [ ] Publish the PR when requested.

This changes state interpretation only. The existing `vlan_id` configuration contract still applies: omission requests the default VLAN, and a configured custom VLAN is restored after detected drift. D3 group identity is now implemented and locally verified separately; see [D3 evidence](../ssid-group-fix/README.md). HTTP-envelope validation, pagination, and missing-resource classification retain their separate work items.

## Response decisions

| Controller response | Terraform value or result |
| --- | --- |
| `mode = 0` | Null, regardless of stale custom/current VLAN numbers. |
| `mode = 1`, bridge VLAN 1–4094 | That bridge VLAN; stale `currentVlanId` cannot override it. |
| `mode = 1`, absent/zero bridge, valid `currentVlanId` | The fallback VLAN, provided `currentVlanIds` is absent/empty or matches that single value. |
| Missing/null setting or mode | Diagnostic; preserve prior managed-resource state. |
| Other mode, unsupported custom mode, invalid VLAN, ambiguous fallback | Diagnostic; do not guess or silently clear state. |
| Create/update returns a different interpretable VLAN | Error and observed state; do not report the requested VLAN as applied. |
| Create returns a decoded SSID/ID with an uninterpretable VLAN | Error with created identity retained. |
| Update returns an uninterpretable VLAN | Error with prior state retained; subsequent refresh can recover observed state. |

The VLAN field names and mode numbers come from the existing v6 client model and D1's recorded contract. The v5.9 reference uses a different legacy VLAN shape; it is not proof of v6 firmware behavior. Hardware compatibility remains unchecked. HashiCorp's [Read guidance](https://developer.hashicorp.com/terraform/plugin/framework/resources/read) calls for reporting refreshed values and preserving prior state on errors; the [acceptance TestStep documentation](https://developer.hashicorp.com/terraform/plugin/testing/acceptance-tests/teststep) describes refresh and plan checks used here.

## Test environment

Terraform ran inside the disposable Docker container `omada-ssid-vlan-fix`, using Terraform `1.15.8` at `/usr/local/bin/terraform`, copied from `/home/will/.local/bin/terraform`. The source worktree was mounted read-only at `/src`; writable test copies were under `/tmp/baseline`, `/tmp/review`, and `/tmp/compat`. Go was `1.25.6` in `golang:1.25.6@sha256:06d1251c59a75761ce4ebc8b299030576233d7437c886a68b43464bad62d4bb1`. The container and its scratch copies were removed after saving evidence.

Go acceptance tests started local `httptest` Omada servers inside the container and configured Terraform with their URLs. All credentials and identifiers are synthetic. No real controller or cloud account was contacted.

## Verification

D2's baseline is D1 commit `80991a5`, where F14 is still present. The original review also reproduced F14 on `6bc71b3`; that historical probe/log remains unchanged. The logs below compare the same final D2 tests against D1 and the patched tree.

| Gate | Result | Evidence |
| --- | --- | --- |
| Baseline lifecycle | 59 failures and 36 passes among 95 cases | [Baseline lifecycle](baseline-lifecycle.log) |
| Baseline Terraform | All three D2 scenarios fail: stale refresh values or an unnecessary PATCH during default-mode import | [Baseline acceptance](baseline-acceptance.log) |
| Patched lifecycle | All 95 cases pass | [Patched lifecycle](patched-lifecycle.log) |
| Patched Terraform | Three D2 scenarios, 21 steps, pass | [D2 acceptance](patched-acceptance.log) |
| D1 regression + D2 Terraform | All 11 scenarios, 73 steps, pass | [All SSID acceptance](all-ssid-acceptance.log) |
| Formatting / static / build | `gofmt -l .` empty; `go vet ./...` and `go build ./...` exit 0 | Container gates |
| Unit suite | `go test ./... -count=1 -cover` passes; resource statement coverage 14.2% with D1 + D2 | [Unit suite](patched-unit-suite.log) |
| A + D1 + D2 | Authentication patch applies cleanly; full unit suite and all 11 SSID Terraform scenarios pass | [Compatibility](authentication-compatibility.log) |

The 95 direct lifecycle cases comprise 28 response shapes exercised through refresh, import, and the data source (84 cases), plus 11 create/update readback cases. They cover default mode with absent/stale/invalid inactive VLAN numbers, custom-mode precedence/fallback and range boundaries, missing/null/unsupported/malformed modes, ambiguous fallback, retained state on errors, mismatched write results, created identity, and resolving an unknown planned VLAN from observed readback.

The two drift scenarios each run nine Terraform steps: import VLAN 20; externally select mode 0; refresh without PATCH; assert a plan from null back to configured VLAN 20; apply and settle; accept another default-mode change by removing `vlan_id` without a write; introduce remote VLAN 30; refresh and plan its removal; apply default mode and settle. One scenario clears inactive fields and one leaves stale fields. The third scenario imports a default-mode SSID with stale VLAN fields, verifies zero PATCH requests and a stable plan, then verifies command-based import. Config/apply steps assert both managed-resource and data-source values. Refresh steps assert resource state and request counts; the dependent data source is deferred by the refresh workflow and checked after apply instead.

The full unit run keeps live acceptance disabled. Mock acceptance is still a separate recorded gate outside the repository's ordinary CI unit job. No race run was added because D2 changes no shared concurrency behavior. A + D1 + D2 is not a combined A–D2/hardware test; B/C were not added to this stack.

### Reproduction commands

Inside the pinned Go container with Terraform installed as described above:

- D2 lifecycle: `go test ./internal/resources -run TestWirelessNetworkVLAN -count=1 -v`.
- D2 Terraform: `TF_ACC=1 TF_ACC_TERRAFORM_PATH=/usr/local/bin/terraform go test ./internal/provider -run TestAccWirelessNetworkVLANState -count=1 -v -timeout=180s`.
- D1 + D2 Terraform: same command with `-run TestAccWirelessNetwork`.
- Full gates: `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./... -count=1 -cover`.

For baseline reproduction, check out D1 commit `80991a5197674a2ee5e2d93ab8d3b571ef4ea774` and copy only `internal/resources/wireless_network_vlan_test.go` and `internal/provider/wireless_network_vlan_test.go` from D2. The existing D1 fixture is unchanged. The two D2 test commands fail with the recorded results. The final sources, including formatting, match both baseline and patched runs.

For A compatibility, copy the tested D2 tree into a disposable directory. Generate `git diff 6bc71b310bc81a90f3ad32194ba8c17947bf79d6 52ed681674e5a0058ae7807fcd08399c21fa021a -- internal/client/client.go internal/client/auth_test.go` and run `git apply --check` followed by `git apply`. This preserves D1/D2 edits in the same client file. The patch SHA-256 is `6969a2b46ca02abe5f6f632a20c5424e3f22e5f6672be05f2e80fd9f9edd86c1`. Run the full unit suite and all SSID Terraform scenarios.

## Tested files

Nine D2 files differ from D1. Host and container hashes match. The three documentation files were reviewed with the corresponding schema/template; no example or dependency changes are required. Planning and evidence remain on the separate documentation branch.

| File | SHA-256 |
| --- | --- |
| `internal/client/client.go` | `e141930b51b6537f20a051f78b1fbbfb54de2c52ccfc7e73e396b5ddad245100` |
| `internal/resources/data_sources.go` | `c48c2b29fb7ce04c18a8041cc3819ca22b399b31a1e8356cca8a87e40ae0ec37` |
| `internal/resources/wireless_network.go` | `614576136b18c7dede81633298d19d95b4bdd28cee2a3a5a4a89401a49d73a36` |
| `internal/resources/wireless_network_vlan.go` | `80fe8fc7c8574d8797c7555cce29659d18ba95a475f7f8d40c7de3703d5d353c` |
| `internal/resources/wireless_network_vlan_test.go` | `e111b48d437d60cd73f3c4f47094cf092882abe258f207327d8581e3916d6923` |
| `internal/provider/wireless_network_vlan_test.go` | `6813fa20d467efbf4e700bd9b957acd8d5b3f9735c083ca7f4c0cd4283741a96` |
| `docs/resources/wireless_network.md` | `534a1e43ff842c387ca19377014d1a992c67ee037095926c1bd28c7dca55e707` |
| `docs/data-sources/wireless_networks.md` | `b80235931597aecca737f0354cc6589b3450412bda4ecec1d3ff7b5d61038c17` |
| `templates/resources/wireless_network.md.tmpl` | `17f9891f0946b75ee445d95b474f43bd5af87678ce81fd32b422b4f24e811f89` |

## PR package and remaining gates

Suggested title: `fix(resources): clear stale SSID VLAN state`.

Review this patch against D1 `80991a5`, not the original `main`: the branch includes D1 intentionally, and write readback relies on its validated update path. Record that dependency in an upstream PR and rebase after D1 merges if needed. Terraform attribute types, configuration ownership, and import IDs are unchanged; no state upgrade or re-import is required. Release notes must mention stricter read validation: missing/unsupported VLAN shapes now fail explicitly, and default mode no longer exposes stale/effective VLAN numbers as custom configuration.

Create recovery in this patch applies when the client returns a decoded SSID with an ID. A transport, lookup, or JSON decoding failure earlier in `CreateWirelessNetwork` still follows existing behavior and can prevent the resource from receiving the newly created ID. That separate recovery path is not covered by D2's created-ID claim and remains a follow-up item in the tracker.

- [x] Keep D2 focused on F14 and matching data-source/write-readback mapping; retain D3 separately.
- [x] Capture failing baseline and passing patch tests, including actual Terraform refresh/plan behavior.
- [x] Complete full container gates, D1 regression tests, and A + D1 + D2 compatibility.
- [x] Update resource/data-source documentation and the resource template; record stricter validation and migration behavior.
- [x] Record exact base, tested file hashes, versions, commands, and logs.
- [x] Recheck upstream `main` (`6bc71b3`) and contribution standards; no new AI-attribution requirement was found. No tool/model metadata was guessed.
- [x] Check final links, hashes, and worktree state; remove the disposable container and temporary compatibility patch after saving evidence. All 60 local links resolve and all nine tested file hashes match; unrelated worktrees remain clean.
- [ ] Validate default/custom modes, stale-field responses, fallback shapes, and failed-write behavior on a controlled controller/AP. Keep the upstream template's hardware checkbox unchecked.
- [x] Commit/push D2 as [0bbd076](https://github.com/wbniv/terraform-provider-tplink-omada/commit/0bbd07687fe76f7203779536bc9b5a24095aa6d3); all nine committed file hashes match the tested sources.
- [ ] Recheck upstream and attribution requirements before submission; publish only when authorized, then record PR URL, CI, reviews, and merge separately.
