# D1 — SSID update preservation

Prepared for Will on 2026-10-19. Implemented, locally verified, and committed/pushed to Will's fork as [80991a5](https://github.com/wbniv/terraform-provider-tplink-omada/commit/80991a5197674a2ee5e2d93ab8d3b571ef4ea774) in `/home/will/terraform-provider-tplink-omada-ssids`, branch `fix/ssid-update-preservation`, based on `6bc71b310bc81a90f3ad32194ba8c17947bf79d6`. Parent: [SSID repair plan](../../plans/ssid-reliability.md).

## Findings and decisions

- [x] Reproduce F04 with real Terraform import/apply against a synthetic HTTP controller. Baseline resets omitted radio defaults, disables guest/schedule/filter controls, and drops nested rate, multicast, and PSK settings.
- [x] Remove the five schema defaults; supply the same defaults only for creation with omitted configuration. Imported settings and omitted Optional+Computed fields remain controller-owned.
- [x] Preserve writable fields recursively from a fresh snapshot. Drop explicitly recognized response-only fields; fail before PATCH for unknown fields, unsupported security modes, or incomplete required settings.
- [x] Preserve PSK version, encryption, and rekey settings. Use only configured passwords, never an API-returned value, regardless of whether it looks like a mask.
- [x] Verify a private trust marker for prior configured keys. Earlier provider state may contain API-returned masks, so an unmarked old state value cannot be used when configuration removes the password.
- [x] Keep `vlan_id` ownership explicit: configured VLAN survives rename; null selects mode 0; unknown does not clear VLAN. Resolve changed VLAN associations before writes and reject failed, missing, or ambiguous lookups.
- [x] Read back after every successful PATCH, including partial response bodies. Restore prior state on discovery/PATCH/readback failure rather than leaving the framework's plan-initialized response state.
- [x] Complete container gates, baseline comparisons, authentication-A compatibility, migration documentation, and final code review.
- [ ] Validate controller/firmware compatibility on hardware.
- [x] Commit and push D1 as [80991a5](https://github.com/wbniv/terraform-provider-tplink-omada/commit/80991a5197674a2ee5e2d93ab8d3b571ef4ea774); all 11 committed file hashes match the tested files.
- [ ] Submit an upstream PR when requested.

D2/F14 (mode-aware VLAN state) and D3/F13 (group replacement) remain separate. Pagination, missing-resource semantics, default-group discovery, HTTP envelope validation, and general error redaction retain their existing tracker entries.

## Sources and compatibility boundary

TP-Link's [official Web API announcement](https://community.tp-link.com/en/business/forum/topic/590430?moduleId=582) provides the [v5.9.9 API document ZIP](https://static-community.tp-link.com/attach/12/12/2023/867825f8f0ef45d59976eb0fe7e1634c.zip). Its “Modify SSID” request documents the schedule/filter associations, nested rate limits, PSK rekey controls, beacon/rate controls, and multicast fields used here. The existing repository supplies the v6-style VLAN, MLO, Wi-Fi sharing, and management-rate fields.

This is a conservative full-object PATCH contract inferred from that reference and the existing client. The synthetic fixtures are not captured controller traffic. TP-Link warns that Web API details vary by version; v5.9's legacy VLAN fields and PMF labels differ from this repository's v6 assumptions. D1 does not reinterpret PMF numbers or claim general v5/v6 compatibility. Unknown fields stop an update rather than being silently dropped. Hardware verification remains required before claiming support for a particular controller/firmware combination.

HashiCorp's [plan modification guidance](https://developer.hashicorp.com/terraform/plugin/framework/resources/plan-modification) and [Create state requirements](https://developer.hashicorp.com/terraform/plugin/framework/resources/create) explain why creation-only defaults must respect known configuration and leave unknown expressions unresolved until apply. The framework's installed `resource/update.go` confirms that `UpdateResponse.State` starts from the plan; error paths explicitly reset it to prior state.

## Writable-field inventory

This inventory is intentionally restrictive. For optional fields, absence stays absent; explicit false, zero, and empty values are copied. Required associations cannot be absent or empty. Objects must have the expected shape; unknown root or nested fields stop updates. This prevents the provider silently discarding a field from a full-object request.

| Fields | Source | Update handling |
| --- | --- | --- |
| `name`, `band`, `security`, `broadcast`, `enable11r`, `pmfMode` | v5.9 Modify SSID; existing resource | Required snapshot; overlay known planned values, preserve omitted/unknown computed values. |
| `wlanId`, `guestNetEnable`, `greEnable` | v5.9 Modify SSID | Preserve values; guest flag required. Group identity changes remain D3. |
| `wlanScheduleEnable`, `action`, `scheduleId` | v5.9 Modify SSID | Preserve; enabled scheduling requires action and a nonempty schedule ID. |
| `macFilterEnable`, `policy`, `macFilterId` | v5.9 Modify SSID | Preserve; enabled filtering requires policy and a nonempty filter ID. |
| `rateLimit`, `ssidRateLimit`: profile ID, up/down enabled flags, rates, units | v5.9 Modify SSID | Preserve nested values; enabled directions need a profile or explicit rate/unit fields. |
| `pskSetting`: version, encryption, rekey enabled/interval/unit | v5.9 Modify SSID | Preserve on rename/rotation; enabled rekey requires interval/unit. API key is excluded and replaced only with a trusted Terraform key. |
| `rateAndBeaconCtrl`: 2g/5g/6g enabled, density, client-rate and beacon fields; `cckRatesDisable` | v5.9 Modify SSID | Preserve nested values; enabled band controls require their density/rate/beacon fields. |
| `manageRateControl2gEnable`, `manageRateControl5gEnable` | Existing v6 client model | Preserve; same completeness check for enabled controls. Hardware contract pending. |
| `multiCastSetting`: multicast/channel utilization, ARP, IPv6, filter enabled/mode/group | v5.9 Modify SSID | Preserve nested values; enabled filtering requires mode/group. |
| `vlanSetting.mode`, `customConfig` bridge VLAN/network associations | Existing v6 client model | Preserve a matching configured bridge VLAN and associations; explicit changes resolve one network. Unsupported/malformed modes stop writes. Null configuration selects mode 0. |
| `mloEnable`, `prohibitWifiShare` | Existing v6 client model | Preserve when present; do not invent false when absent. Hardware contract pending. |
| `id`, `idInt`, `index`, `site`, `resource`, `portalEnable`, `accessEnable`; VLAN `currentVlanId`/`currentVlanIds` | Existing client response model/exclusions | Excluded from outgoing payload. Legacy `vlanEnable`/`vlanId` are unsupported, not converted by guesswork. |

The exact recursive allowlist is in `internal/resources/wireless_network_update.go`. The reference ZIP SHA-256 is `f7ce197eed9219c3d10653bc7fff41ad8f69b6d765d5e482193f7988b55ededf`; the extracted document was `Omada_SDN_Controller_V5.9.9 API Document.html`. Downloaded/extracted copies are disposable, not contribution files.

## Verification

### Where Terraform ran

Terraform ran at `/usr/local/bin/terraform` inside the temporary Docker container `omada-ssid-fix`, launched by Go's acceptance-test harness. The binary was copied from `/home/will/.local/bin/terraform`. Source worktrees were mounted read-only, with disposable test copies at `/tmp/review`, `/tmp/baseline`, and `/tmp/compat`. Each acceptance test started an `httptest` Omada server inside the same container and configured Terraform to use that server's local URL. No real controller or cloud account was contacted. The container and its scratch files were removed after the logs were saved.


Go `1.25.6 linux/amd64`, Docker image `golang:1.25.6@sha256:06d1251c59a75761ce4ebc8b299030576233d7437c886a68b43464bad62d4bb1`; Terraform `1.15.8 linux_amd64`. These match earlier repair runs. No host Go installation is needed. All credentials and identifiers in fixtures are synthetic; no live network or controller was changed.

| Gate | Result | Evidence |
| --- | --- | --- |
| Baseline lifecycle | All 42 preservation/update cases fail | [Baseline lifecycle](baseline-lifecycle.log) |
| Baseline Terraform | All five import/update scenarios fail | [Baseline acceptance](baseline-acceptance.log) |
| Patched lifecycle | 42 update cases and five create-validation cases pass | [Patched lifecycle](patched-lifecycle.log) |
| Patched Terraform | Eight scenarios, 52 steps: three creation scenarios and five import/update scenarios | [Patched acceptance](patched-acceptance.log) |
| Formatting | `gofmt -l .` has no output; SSID example `terraform fmt -check -diff` exits 0 | Container check |
| Static/build | `go vet ./...` and `go build ./...` exit 0 | Container check |
| Unit suite | `go test ./... -count=1 -cover` passes; resource statement coverage 9.5% in D1 alone | [Unit suite](patched-unit-suite.log) |
| A + D1 | Authentication patch applies cleanly; full unit suite and all eight SSID Terraform scenarios pass | [Compatibility](authentication-compatibility.log) |

Lifecycle coverage includes fresh snapshot preservation, no-op writes, absent/false/zero values, unknown/null planned values, direct-array/envelope reads, incomplete/unknown fields, missing associations, legacy mask state, password validation, VLAN changes/removal/lookup failures, full/partial/empty PATCH results, and read/PATCH/readback failures. Error tests initialize response state from the plan, as the real framework does, and assert retention of prior state.

Terraform coverage includes creation defaults, explicit values, values unknown during planning, imports with nondefault radio settings, rename, explicit changes, removal of configured computed fields, plaintext/masked/absent API passwords, rotation, password removal, open/secured transitions, configured VLAN changes/removal, refusal to mutate an unmanaged secured import, and stable repeated plans. Private state is exercised through Terraform, including reusing a previously configured key during removal. The provider stops trusting that key after it leaves state. Legacy state without the marker cannot authorize fallback to a possibly masked value.

A secured import without a password can be read without a write. A subsequent change requiring PATCH fails before mutation. For older state, keep the real password configured through a successful update before removing it. Remote password drift is deliberately not inferred from API-returned strings.

The full unit suite keeps live acceptance tests disabled. Mock acceptance is run separately and is not currently part of the ordinary CI unit job. A + D1 is not a combined A + B + C + D1 or hardware test. No race run was added because D1 changes no shared client concurrency behavior.

### Reproduction commands

In a disposable checkout inside the pinned container:

- Lifecycle: `go test ./internal/resources -run TestWirelessNetwork -count=1 -v`.
- Terraform: `TF_ACC=1 TF_ACC_TERRAFORM_PATH=/usr/local/bin/terraform go test ./internal/provider -run TestAccWirelessNetwork -count=1 -v -timeout=180s`.
- Full gates: `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./... -count=1 -cover`.
- Example: `terraform fmt -check -diff examples/resources/omada_wireless_network`.

For the baseline, check out `6bc71b3` and copy D1's resource test, preservation acceptance test, and JSON fixture only. Run `go test ./internal/resources -run TestWirelessNetworkPreservation -count=1 -v` and the Terraform command with `-run TestAccWirelessNetworkPreservation`. Both fail. Baseline and patched logs use the same final test sources and fixture.

For compatibility, copy the tested D1 tree into a disposable directory and apply `git diff 6bc71b310bc81a90f3ad32194ba8c17947bf79d6 52ed681674e5a0058ae7807fcd08399c21fa021a -- internal/client/client.go internal/client/auth_test.go` with `git apply --check` followed by `git apply`. This merges A's changes while retaining D1's edits to the same client file. The generated diff's SHA-256 was `6969a2b46ca02abe5f6f632a20c5424e3f22e5f6672be05f2e80fd9f9edd86c1`. Run the full unit suite and targeted Terraform command. Do not replace D1's whole client file with A's version.

## Tested files

D1 contains 11 source, test, fixture, documentation, example, and template files. Host, container, and committed SHA-256 values match. Planning/evidence files remain on the separate documentation branch. The custom resource template preserves the migration notes during future documentation generation; its documented template functions were checked against [HashiCorp's template reference](https://github.com/hashicorp/terraform-plugin-docs/blob/main/README.md#templates).

| File | SHA-256 |
| --- | --- |
| `internal/client/client.go` | `e610ca7d70bd0e009e7dd66ef4ddf8d19f16d6f9fe04a22a1e692b94247df41d` |
| `internal/resources/wireless_network.go` | `7db3c064f57f4cc6e6d5cd7a70a3f67a54f47d35a8250c540d0a96a63ace5faa` |
| `internal/resources/wireless_network_update.go` | `86a15f9d35f4fd01fbd071426ec89e93ca60c05bd61241cd759f62233bc30a6d` |
| `internal/resources/wireless_network_update_test.go` | `f9b8278fb6a4f93831ea62a13cca2f6bab1b75c0b7bc1683df7b87a68da14230` |
| `internal/provider/wireless_network_create_test.go` | `32268d7d514defba8c00840b3bbd79d22fff46ab2125d1ad9c5f86efcf92afd5` |
| `internal/provider/wireless_network_preservation_test.go` | `69d0314f18380b90fcea2400b97706deb2d15b1fcb2175a84a3bdf1bd73b5ea1` |
| `internal/testdata/wireless_network.json` | `dae215a71bf034e8aae70a0231a889abfda259a3d0dc1895164d1e087789552a` |
| `docs/resources/wireless_network.md` | `2131333995a032f49b0e0a37ffc5311e967a993f6139547cb90b9331e239b175` |
| `examples/resources/omada_wireless_network/resource.tf` | `f6ed256c8b82ebb9ccc96bf86ef8f42c6e57fab0355013863ffe4b6834665ea6` |
| `examples/resources/omada_wireless_network/import.sh` | `3e29ae8069721120e757aa29279c95a02465c32f7bdaa9db0c45bf6708356a04` |
| `templates/resources/wireless_network.md.tmpl` | `4fe8d30b79998e91ada0323d923a23e909801bfe27e93a66ef89f4aa724e5a5a` |

## PR package and remaining gates

Suggested title: `fix!: preserve SSID settings during updates`.

Release classification is breaking: omission no longer reapplies defaults to existing SSIDs, imported/API-returned secrets are not trusted, unsupported/incomplete payloads now fail before writes, and an unknown/missing/ambiguous VLAN association cannot be guessed. Attribute names/types and the three-part import identity remain unchanged; no re-import is required. Resource documentation, its template, the example, and the corrected import example describe migration.

- [x] Review F04's payload/default/secret scope and preserve D2/D3 as separate work.
- [x] Compare the same regression tests against baseline and patch.
- [x] Complete full container gates and A compatibility.
- [x] Review read-only/error paths and verify no real credentials in fixtures or captured evidence.
- [x] Inspect contribution requirements and confirm the upstream baseline remains `6bc71b3`; no explicit AI attribution requirement was found. No tool/model metadata was guessed.
- [x] Save logs, remove disposable containers/downloads, and check local documentation links, all 11 host/container file hashes, example consistency, and worktree status. B and C remain clean; D1 is committed and pushed. Documentation records its revision separately.
- [ ] Validate with a controlled controller/AP, recording firmware/version, accepted payload fields, preserved settings, PSK/VLAN behavior, and PMF meaning. Leave the upstream template's hardware checkbox unchecked.
- [x] Commit and push D1 as [80991a5](https://github.com/wbniv/terraform-provider-tplink-omada/commit/80991a5197674a2ee5e2d93ab8d3b571ef4ea774) on `fork/fix/ssid-update-preservation`; verify all 11 committed hashes against these tested files.
- [ ] Recheck upstream heads, contribution/attribution requirements, release classification, and current CI before PR submission.
- [ ] Publish an upstream PR only when authorized; record URL, CI, reviews, and merge separately.
