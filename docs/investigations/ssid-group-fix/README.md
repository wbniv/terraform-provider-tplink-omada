# D3 — SSID WLAN-group identity

Prepared for Will on 2026-10-19. Implemented, locally verified, committed, and pushed as [ec2d949](https://github.com/wbniv/terraform-provider-tplink-omada/commit/ec2d949699274c3a6dea6cf3d272f22bde13d12e) on `fix/ssid-group-identity` in `/home/will/terraform-provider-tplink-omada-ssid-group`. Base: D2 `0bbd07687fe76f7203779536bc9b5a24095aa6d3`, which includes D1 `80991a5`. Parent: [SSID repair plan](../../plans/ssid-reliability.md).

## Scope and decisions

- [x] Reproduce F13 on D2: group changes plan an in-place update, while Update uses and records the old group.
- [x] Require replacement for a changed WLAN group; retain omitted imported/current groups by running `UseStateForUnknown` before `RequiresReplace`.
- [x] Retain explicit unknown group values during planning. Terraform conservatively plans replacement even when the value eventually resolves to the old group; resolving the reference first avoids that uncertainty.
- [x] Guard Update against different, null, or unknown planned group identities before lifecycle API requests, preserving prior state with a diagnostic.
- [x] Reject known invalid security/password combinations while planning creation/recreation, before destruction. Keep unknown values deferred until apply; preserve secured imports without a configured password until a write/replacement is requested.
- [x] Verify old/new paths, new IDs, unchanged three-part import, existing site replacement semantics, ordinary updates, both replacement orders, and failure/retry behavior through real Terraform.
- [x] Record baseline/patch evidence, full Go gates, and D1/D2 regression coverage.
- [x] Pass A + D1 + D2 + D3 full unit and all 24 SSID Terraform scenarios.
- [x] Complete final documentation checks and cleanup.
- [x] Commit and push D3 as [ec2d949](https://github.com/wbniv/terraform-provider-tplink-omada/commit/ec2d949699274c3a6dea6cf3d272f22bde13d12e); all seven committed file hashes match the tested files.
- [ ] Validate against a controlled controller/AP and submit when requested.

This is deliberately stacked on D2. Replacement depends on D1's creation defaults and secret ownership, and the lifecycle tests use D2's consistent import/default-VLAN state. Review D3 against D2, then rebase after the earlier changes merge. D3 changes two production files; API move support is not assumed. Existing operations address an SSID through its site/group/SSID path.

HashiCorp documents [replacement plan modifiers and unknown-state handling](https://developer.hashicorp.com/terraform/plugin/framework/resources/plan-modification) and [lifecycle ordering](https://developer.hashicorp.com/terraform/language/meta-arguments/lifecycle). The installed framework source confirms modifiers execute in order; explicit unknown configuration is not filled from old state. These support Terraform behavior, not controller compatibility.

## Observed behavior and migration

| Case | Verified result |
| --- | --- |
| Existing/imported group omitted or explicitly unchanged | Retain group and ID; no default-group lookup or replacement. Unrelated rename PATCHes the existing identity. |
| Known group change | Plan actions `[delete create]`, replacement path `wlan_group_id`, new ID unknown. DELETE old identity, POST new group, save new ID, stable follow-up plan. |
| `create_before_destroy` | Plan `[create delete]`; create in new group before deleting old identity. |
| Explicit unknown group | Value stays unknown; plan replacement. Apply uses resolved group, including when it resolves to the same group. |
| Initial creation with omitted/unknown group | Omission uses existing default discovery; explicit reference resolves before POST without default discovery. |
| Site change | Existing site replacement remains intact; DELETE old site, POST requested site. |
| Rejected DELETE | Old object/state retained; retry deletes old identity then creates requested group. |
| Rejected POST after destroy | Old object already gone; retry plans creation and saves the new identity. |
| Duplicate-name rejection with create-before-destroy | Old object remains managed. A changed name permits retry in the synthetic controller. |
| Secured replacement without configured password | Planning error, zero SSID writes, old object/state retained. Supplying the password allows replacement using that configured key. |

A group change now recreates the SSID and may interrupt connectivity. Unmanaged guest/filter/schedule/rate settings are lost during recreation; configure modeled values and record controller-only settings for restoration. Omitted radio/security attributes use creation defaults. Secured recreation requires `passphrase`; prior imported/controller keys are not reused. No schema type/version change, state migration, or re-import is required.

The new planning validation also catches known invalid initial-creation configurations earlier. Settings still unknown at plan time can only be checked once resolved during apply; this cannot guarantee that all errors are caught before Terraform deletes an old object. Controller errors can still occur during creation. Create-before-destroy requires simultaneous objects to be accepted; duplicate-name/capacity restrictions and uninterrupted service require hardware checks.

Default-group discovery still selects the first returned group, pagination/missing-resource semantics retain their separate work items, and earlier POST-success/readback-failure identity recovery remains separately tracked. D3 does not change those paths.

## Environment

Go `1.25.6` and Terraform `1.15.8` (`linux_amd64`) ran in disposable Docker container `omada-ssid-group-fix`, image `golang:1.25.6@sha256:06d1251c59a75761ce4ebc8b299030576233d7437c886a68b43464bad62d4bb1`. Terraform was copied from `/home/will/.local/bin/terraform` to `/usr/local/bin/terraform` in the container. The source worktree was mounted read-only at `/src`; writable baseline/patched/compatibility copies were under `/tmp`. Local `httptest` servers simulated Omada responses. All credentials, SSIDs, and identifiers are synthetic. No real controller or cloud account was used. The container, scratch copies, and temporary host compatibility patch/log were removed after saving evidence.

## Verification

The same final test files run on the D2 baseline and patched sources. The baseline failures are expected regression evidence.

| Gate | Result | Evidence |
| --- | --- | --- |
| D2 baseline: direct tests | 10 failures, 11 passes among 21 cases | [Baseline unit](baseline-unit.log) |
| D2 baseline: Terraform | Nine failures, four passes among 13 D3 scenarios | [Baseline acceptance](baseline-acceptance.log) |
| Patched direct tests | All 21 pass: nine group-plan cases, three rejected update identities, nine creation-plan validations | [Patched unit](patched-unit.log) |
| Patched Terraform | All 13 D3 scenarios, 47 steps, pass | [All SSID acceptance](all-ssid-acceptance.log) |
| D1/D2 regression + D3 | All 24 SSID scenarios, 120 steps, pass | [All SSID acceptance](all-ssid-acceptance.log) |
| Full unit suite | All packages pass; resource statement coverage 15.2% | [Unit suite](full-unit.log) |
| Formatting / static / build | `gofmt -l .` empty, `go vet ./...`, `go build ./...`, and changed Terraform example formatting check pass | Container gates |
| A + D1 + D2 + D3 | Authentication patch applies cleanly; full unit suite and all 24 SSID Terraform scenarios pass | [Compatibility](authentication-compatibility.log) |

The 13 Terraform scenarios cover import/omission/same group; replacement with default and explicit settings; create-before-destroy; unknown changed/same group; site replacement; create/delete failures and retries; duplicate-name rejection; omitted/unknown group during creation; and password validation before secured replacement. Assertions inspect plan actions, replacement paths, unknown IDs/values, ordered HTTP mutations, remote identity, state identity, and stable subsequent plans. The two replacement-settings cases also verify creation defaults/explicit values and absence of preserved unmanaged controls.

Baseline plan checks stop most incorrect applies. The duplicate-name case reaches the old Update method and records Terraform's inconsistent group result. In the baseline delete-failure scenario, the plan assertion stops before the injected rejection is consumed; the mock cleanup then also reports that rejection. All fixtures are in-memory servers and are closed after their test. No live resource is left by that baseline warning.

The ordinary unit run leaves live acceptance disabled. Mock Terraform acceptance is an explicit separate gate; the current CI unit workflow does not run these provider scenarios. No race test was added because D3 introduces no shared production concurrency.

### Reproduction commands

Inside the pinned container, with Terraform installed at the recorded path:

- Direct: `go test ./internal/resources -run TestWirelessNetworkGroupIdentity -count=1 -v`.
- D3 Terraform: `TF_ACC=1 TF_ACC_TERRAFORM_PATH=/usr/local/bin/terraform go test ./internal/provider -run TestAccWirelessNetworkGroupIdentity -count=1 -v -timeout=180s`.
- All SSID Terraform: same command with `-run TestAccWirelessNetwork -timeout=240s`.
- Full checks: `test -z "$(gofmt -l .)"`, `go vet ./...`, `go build ./...`, `go test ./... -count=1 -cover`.
- Example: `terraform fmt -check examples/resources/omada_wireless_network/resource.tf`.

For baseline reproduction, check out D2 `0bbd07687fe76f7203779536bc9b5a24095aa6d3` and copy only the two new `wireless_network_group_test.go` files into their matching resource/provider directories. Existing fixtures are unchanged.

For compatibility, copy the patched source into a disposable directory without its host worktree `.git` pointer. Generate `git diff 6bc71b310bc81a90f3ad32194ba8c17947bf79d6 52ed681674e5a0058ae7807fcd08399c21fa021a -- internal/client/client.go internal/client/auth_test.go`, then `git apply --check` and `git apply` it. The patch SHA-256 is `6969a2b46ca02abe5f6f632a20c5424e3f22e5f6672be05f2e80fd9f9edd86c1`. Run the full unit suite and all SSID Terraform scenarios. This checks A + D1 + D2 + D3; it does not include B/C or hardware.

## Tested files

All seven host/container hashes match. Plans and evidence remain on the separate documentation branch.

| File | SHA-256 |
| --- | --- |
| `internal/resources/wireless_network.go` | `fd66e2b83cf4039354461fa9271c941a31d74879c930b8d2cd2feda40aa7c8b4` |
| `internal/resources/wireless_network_update.go` | `268d9cd002992da949a2f6a519cc2e7847f4adf2b48327d7845be6728e715980` |
| `internal/resources/wireless_network_group_test.go` | `0d40adacde76d592a51ec266bab9b516af888b7a9e8019b1ca5c6888ee6d85b8` |
| `internal/provider/wireless_network_group_test.go` | `bca061faab22f95866cc9a9c4feb1de2dc010c1795164ec7c47842edeefcc274` |
| `docs/resources/wireless_network.md` | `fbffe0e6a3337318757d9f25ade634a5a91e10b31f693db14d17314b7f751c15` |
| `templates/resources/wireless_network.md.tmpl` | `bcc2e201c2bf5895878b667d5a462a0666074406d955c143be7a8b2c7bd2b9e8` |
| `examples/resources/omada_wireless_network/resource.tf` | `91aec035bc6005720b102ed557c95e2f35f72a78ec41f75df4b90e259b00e6a3` |

## PR package and remaining gates

Suggested title: `fix(resources)!: require replacement for SSID WLAN group changes`.

Suggested breaking-change note: changing `wlan_group_id` now recreates the SSID. Review the replacement plan, connectivity interruption, creation settings/password, and unmanaged controller settings before applying. Unknown group references may conservatively require replacement. State/import formats are unchanged.

- [x] Record the D2 base and stack dependency, source hashes, versions, baseline/patch logs, and breaking behavior.
- [x] Update schema description, resource documentation, its generation template, and example source together.
- [x] Recheck upstream `main` (`6bc71b3`) and contribution standards at start and before committing. Conventional commits and fmt/vet/test/docs/hardware declarations remain expected; no explicit AI-attribution rule was found, and no session metadata was guessed.
- [x] Finish compatibility, validate all 74 relative documentation links and seven tested file hashes, and remove the disposable container and temporary patch/log after saving evidence. B/C/D1/D2 worktrees remain clean.
- [ ] Validate group replacement order, duplicate-name/capacity constraints, recreation payloads, controller failures, AP propagation, and connectivity interruption on controlled hardware. Keep the PR hardware checkbox unchecked.
- [x] Commit/push D3 as [ec2d949](https://github.com/wbniv/terraform-provider-tplink-omada/commit/ec2d949699274c3a6dea6cf3d272f22bde13d12e); verify all seven committed hashes and the remote branch.
- [ ] Recheck upstream and attribution standards before publication; record upstream PR, CI, reviews, and merge separately. No upstream PR has been posted.
