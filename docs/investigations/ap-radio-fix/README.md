# AP radio patch evidence

Date: 2026-10-19. Tracker: [work C](../../plans/provider-reliability-prs.md#c--ap-radios).

Scope: F03, preserving omitted AP radio settings during Terraform updates. Worktree: `/home/will/terraform-provider-tplink-omada-ap-radios`; branch `fix/ap-radio-updates`; base `6bc71b310bc81a90f3ad32194ba8c17947bf79d6`. Committed and pushed to Will's fork on 2026-10-19 as [9f4b4f8](https://github.com/wbniv/terraform-provider-tplink-omada/commit/9f4b4f8c3ccd1c61f10dd2622f2ae2b3bcb9679b). No upstream PR has been opened. A and B remain separate.

**Status:** implemented and locally verified on 2026-10-19. At that time no live controller or AP had been accessed. As of 2026-10-28, the ES216G controller and switch checks are complete, but C's radio changes and the separate `fix/ap-settings-preservation` change still have no controlled-AP validation. AP hardware validation and upstream submission review remain pending; see the [current tracker](../../plans/provider-reliability-prs.md).

## Implementation

- Prepare both radio bands from the fresh controller snapshot before any writes. Overlay known planned values on the five schema-backed writable fields; preserve null/unknown values.
- Skip unchanged bands and omit an empty radio PUT entirely. Exclude radio settings from the general PATCH and exclude unmodeled GET fields from the radio payload.
- Reject a configured radio that the controller does not report, and reject incomplete/malformed snapshots before any writes. This avoids inventing values for missing fields.
- Remove implicit `true`/`4` defaults from `radio_2g_enable` and `radio_2g_tx_power_level` so imported disabled radios and custom power settings survive unrelated updates.
- After a failed update, attempt readback and save observed partial progress. If readback also fails, preserve prior state and add a warning. Clear missing 2.4 GHz state consistently with the existing 5 GHz behavior.
- Document omission semantics, the default change, controller requirements, and partial-failure behavior. Align radio width examples with the schema's existing enum codes and correct the import example to include the site ID.

The preservation scope is the five radio fields for each band. General, advanced, and services builders retain their existing behavior. The tracker retains separate investigations for IP settings, other defaults, and preservation in grouped advanced settings.

## Verification

All Go checks used Docker with `go1.25.6 linux/amd64`, matching `.go-version`. Image: `golang:1.25.6@sha256:06d1251c59a75761ce4ebc8b299030576233d7437c886a68b43464bad62d4bb1`. Terraform: `1.15.8 linux_amd64`. The mock servers use synthetic credentials and identifiers.

| Gate | Result | Evidence |
| --- | --- | --- |
| Baseline lifecycle regressions | 13 of 14 cases fail; the general-write failure stop case passes | [Baseline lifecycle log](baseline-lifecycle.log) |
| Patched lifecycle regressions | All 14 cases pass | [Patched lifecycle log](patched-lifecycle.log) |
| Baseline Terraform acceptance | All three scenarios fail, during rename or import/apply | [Baseline acceptance log](baseline-acceptance.log) |
| Patched Terraform acceptance | Ten steps each for dual-band default/nondefault settings; seven steps for a single-band AP | [Patched acceptance log](patched-acceptance.log) |
| `gofmt -l .` | No output | Checked in container |
| `go vet ./...` and `go build ./...` | Exit 0, no output | Checked in container |
| `go test ./... -count=1 -cover` | Pass; resource statement coverage 8.6% in this C-only run | [Unit-suite log](patched-unit-suite.log) |
| A + C: full unit suite and AP Terraform acceptance | Pass with A's recorded client/test files copied over C in a disposable checkout | [Compatibility log](authentication-compatibility.log) |
| AP example `terraform fmt -check -diff` | Exit 0, no diff | Checked with the same Terraform version |

Lifecycle cases cover unchanged/null/unknown radios, partial updates, explicit false/zero/empty values, simultaneous band changes, absent bands and malformed/incomplete snapshots before writes, stopping on failed endpoints, saving partial progress, and retaining prior state if readback fails. Request assertions exclude unmodeled radio fields and prevent radio configuration appearing in the general PATCH.

The Terraform sequences verify import with omitted radio attributes, name-only updates, empty plans, a single-field change, explicit false/zero values, removal of configured radio values, further name changes, and 5 GHz changes where supported. Both controller payloads and Terraform state are asserted. Empty strings and zero values in mocks establish serialization behavior; controller validation of particular values still requires hardware testing.

The full unit run leaves live acceptance tests gated off. Targeted mock acceptance runs separately and is not currently exercised by the repository's ordinary unit-test CI job. A mock acceptance CI gate remains tracked work. C + A compatibility does not constitute a combined A + B + C or live-controller test.

### Reproduction commands

Inside a disposable checkout in the pinned Go container:

- Lifecycle: `go test ./internal/resources -run TestAPRadioUpdates -count=1 -v`.
- Terraform acceptance: `TF_ACC=1 TF_ACC_TERRAFORM_PATH=/usr/local/bin/terraform go test ./internal/provider -run TestAccDeviceAPRadioPreservation -count=1 -v -timeout=180s`.
- Full gates: `gofmt -l .`, `go vet ./...`, `go build ./...`, and `go test ./... -count=1 -cover`.
- Example formatting: `terraform fmt -check -diff examples/resources/omada_device_ap`.

For baseline reproduction, check out `6bc71b3` and copy only C's two new test files. Both targeted test commands fail. For the patched comparison, also copy C's two production files. Host Go is not required. The baseline logs were recorded before gofmt expanded the tests; formatting is the only difference from the final test sources.

For compatibility, start from C commit `9f4b4f8c3ccd1c61f10dd2622f2ae2b3bcb9679b` and restore `internal/client/client.go` and `internal/client/auth_test.go` from A commit `52ed681674e5a0058ae7807fcd08399c21fa021a`, using the hashes in [A's evidence](../authentication-fix/README.md). Run `go test ./... -count=1` and the targeted AP acceptance command. A's client code remains outside C's branch and upstream patch scope.

## Review scope and tested files

The patch comprises seven files in `/home/will/terraform-provider-tplink-omada-ap-radios`. The four Go files on the host match the tested container files by SHA-256. A and B's previously recorded file hashes remain unchanged.

| File | SHA-256 |
| --- | --- |
| `internal/resources/device_ap.go` | `8a959b3fdfcfa2a6d27f8a6f37a41d6ff10fb0777c95d39a0b5b750bd0fa3c66` |
| `internal/resources/device_ap_radios.go` | `549dff71bf410fd77c20b05aad8d3dc1f83b86b025b2cfe5732eabb5f1666547` |
| `internal/resources/device_ap_radios_test.go` | `3cf7d68a1cd81a97c9b766722f83d42a2f8765ac2622475c99e16dda8d460171` |
| `internal/provider/device_ap_radios_test.go` | `37ea9bf6b54a5c9a3a220bc27da37c33173bd1177fe460503509533af4a1a2f3` |
| `docs/resources/device_ap.md` | `2219b2be615247d3f303aa81a8c270438ae3b3ea37f4f1542da5eccb996294b5` |
| `examples/resources/omada_device_ap/resource.tf` | `dc7e1489182be10de4b23e9a9790701fd6379d7d2d1371b031e4acae2fc00339` |
| `examples/resources/omada_device_ap/import.sh` | `3b3460afa498466b3659628759836dedb46faab843d5d04807b03db91f5bcaa0` |

Production changes are in [the AP resource](https://github.com/wbniv/terraform-provider-tplink-omada/blob/9f4b4f8c3ccd1c61f10dd2622f2ae2b3bcb9679b/internal/resources/device_ap.go) and [the radio builder](https://github.com/wbniv/terraform-provider-tplink-omada/blob/9f4b4f8c3ccd1c61f10dd2622f2ae2b3bcb9679b/internal/resources/device_ap_radios.go). Maintained regressions are in [resources](https://github.com/wbniv/terraform-provider-tplink-omada/blob/9f4b4f8c3ccd1c61f10dd2622f2ae2b3bcb9679b/internal/resources/device_ap_radios_test.go) and [provider](https://github.com/wbniv/terraform-provider-tplink-omada/blob/9f4b4f8c3ccd1c61f10dd2622f2ae2b3bcb9679b/internal/provider/device_ap_radios_test.go). [Resource documentation](https://github.com/wbniv/terraform-provider-tplink-omada/blob/9f4b4f8c3ccd1c61f10dd2622f2ae2b3bcb9679b/docs/resources/device_ap.md) includes the migration note. Local plans, baseline investigations, and captured logs remain review support; select their inclusion in an upstream PR deliberately.

## Compatibility and submission checklist

Suggested conventional title: `fix!: preserve imported AP radio settings during updates`.

The removed `true`/`4` defaults change configuration semantics: users who require enabled 2.4 GHz radios and automatic power must set those values explicitly. Schema names/types, state shape, and import IDs remain compatible; no re-import is required. Record the default change in release classification even though it repairs unsafe behavior.

- [x] Run the same Terraform test on the original baseline: all three scenarios fail, showing zeroed radio payloads and a nonexistent 5 GHz band on a single-band AP.
- [x] Run the same lifecycle tests on baseline: 13 of 14 cases fail; the existing general-write failure stop case passes.
- [x] Run patched lifecycle tests: all 14 cases pass.
- [x] Run patched Terraform acceptance: dual-band default and nondefault scenarios each pass ten steps; the single-band scenario passes seven steps.
- [x] Complete formatting, vet, build, and the full unit suite.
- [x] Test C together with A in a disposable checkout.
- [x] Record final commands, logs, file hashes, and compatibility limitations.
- [x] Remove the disposable test container; verify documentation links/anchors, whitespace, all seven file hashes, and the resource example's match with the documentation. Preserve the worktrees and evidence.
- [ ] Validate on a controlled AP/controller, recording model, firmware/controller versions, accepted payload fields, single/dual-band behavior, radio changes, and readback. Keep the upstream template's hardware-test checkbox unchecked until performed.
- [ ] Validate the separate AP-settings preservation fix (`fix/ap-settings-preservation`, `bd4762b`) on a controlled AP, including readback of LED and grouped load-balancing/RSSI settings. It currently has mock acceptance only.
- [x] Recheck upstream heads/CI and contribution requirements on 2026-10-19; commit the seven selected files as `9f4b4f8` and push to `fork/fix/ap-radio-updates`. Tested hashes match the committed files.
- [ ] Prepare the upstream PR for authorized publication; verify any required attribution from actual session metadata and recheck upstream heads/CI then.
- [ ] Record PR URL, CI, review, and merge revision in the tracker.
