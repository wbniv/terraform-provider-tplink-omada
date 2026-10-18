# Code review: correctness and operational risks

Date: 2026-10-18. Prepared for Will.

Reviewed `main` at `6bc71b310bc81a90f3ad32194ba8c17947bf79d6`. The review covers the API client, resource lifecycle implementations, schemas, and test/CI coverage. Findings below concern this commit; they are not findings against every open PR.

**There are 14 reproduced defects, including five high-priority findings.** The most serious problems are a deterministic certificate deadlock, update requests that overwrite unrelated network settings, and credential changes that Terraform records without performing. The existing test suite passes despite these failures.

The baseline reproductions use local HTTP servers. Request contents and returned Terraform state are directly observed. Hardware consequences are inferred from those requests; no live Omada controller, switch, or AP was modified. Within the original investigation, only the AP reproduction runs through Terraform's complete planning/apply flow. Other baseline resource probes invoke lifecycle methods with framework state and plan objects.

## Repair progress — updated 2026-10-19

The detailed findings and original logs below describe the reviewed baseline. Three separate patches now pass their regression tests and are committed and pushed to Will's fork. Real-controller validation and upstream PR submission remain pending. The [PR tracker](../plans/provider-reliability-prs.md#commits-and-fork-branches--2026-10-19) records the commits and remaining gates.

| Work | Findings | Current evidence and status |
| --- | --- | --- |
| A — Authentication | F01, F07, F08 | Implemented and locally verified on `fix/client-authentication`; certificate, cancellation, retry, concurrency, and race tests pass. [Evidence](authentication-fix/README.md). |
| B — Switch ports | F02 | Implemented and locally verified in the separate `fix/switch-port-updates` worktree; 13 lifecycle cases and Terraform acceptance on mock standard/ES routes pass, including compatibility with stored PR #23. [Evidence](switch-port-fix/README.md). |
| C — AP radios | F03 | Implemented and locally verified on `fix/ap-radio-updates`; 14 lifecycle cases and Terraform sequences for default/nondefault dual-band and single-band APs pass, including combined A compatibility. [Evidence](ap-radio-fix/README.md). |
| D–H | F04–F06, F09–F14 | Open; D, SSID preservation/identity/drift, is next. Scope and dependencies are recorded in the tracker. |
| I — Switch planning | F15 | New P2 finding discovered during B: importing with the entire `ports` list omitted leaves a persistent update plan. Reproduced on the baseline and B; root cause and fix remain open. [Probe and logs](switch-port-fix/README.md#new-finding-f15-omitted-whole-ports-list-does-not-settle-after-import). |

F15 is additional to the original 14 findings. B documents the complete-port-list workaround; it does not resolve F15. Formatting, vet, build, and full unit tests pass for A, B, and C in Go 1.25.6 containers. These results establish local behavior, with hardware compatibility still to be checked.

## Findings at a glance

P1 means fix before relying on the affected operation: potential connectivity/security damage or an operation that cannot complete. P2 means a substantive correctness defect with a narrower trigger. All findings were open at the reviewed baseline. Current repair status is recorded in the [PR tracker](../plans/provider-reliability-prs.md).

| ID | Priority | Defect | Reproduction |
| --- | --- | --- | --- |
| F01 | P1 | Certificate operations deadlock on the authentication mutex | Four operations remain blocked after cancellation |
| F02 | P1 | Switch-port updates empty VLAN selections and omit writable fields | Captured PATCH payload |
| F03 | P1 | Renaming an AP sends zero/empty radio configuration | Real Terraform import and rename against mock API |
| F04 | P1 | SSID updates reset settings absent from the Terraform schema | Captured rename PATCH payload |
| F05 | P1 | Site credential changes are recorded without being sent | Captured PATCH and response state |
| F06 | P2 | Site-settings password rotation restores the old password in state | Captured new password and stale response state |
| F07 | P2 | Authentication retry retains the expired URL token | Captured query/header token pairs |
| F08 | P2 | Concurrent requests race with token refresh | Go race detector |
| F09 | P2 | Collection reads stop at the first 100 objects | Mock collection with 101 networks |
| F10 | P2 | Refresh of a deleted network fails instead of removing it from state | Empty successful collection response |
| F11 | P2 | Network creation silently adopts an incompatible existing network | VLAN 10 remotely, VLAN 20 in returned state |
| F12 | P2 | HTTP failures can be treated as successful operations | HTTP 403 accepted as a successful DELETE |
| F13 | P2 | SSID WLAN-group changes are ignored | Old group in request path and response state |
| F14 | P2 | SSID refresh retains a VLAN removed in the controller | Controller mode 0, stale VLAN 20 in state |

## Detailed findings

### F01 — Certificate operations always deadlock

**Source:** [UploadCertificate](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/client/client.go#L2115), [UploadKey](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/client/client.go#L2189), [ActivateCertificate](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/client/client.go#L2274), [GetControllerCertificateSetting](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/client/client.go#L2306), [ensureAuth](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/client/client.go#L380).

Each certificate operation acquires `c.mu` and then calls `ensureAuth`, which acquires the same non-reentrant mutex. This blocks even when a valid token already exists. The goroutine never reaches the HTTP request, so neither the HTTP timeout nor context cancellation releases it. Certificate creation, updates, reads, and imports can hang; holding the shared mutex also blocks other operations that need authentication.

**Evidence:** `TestInvestigationCertificateDeadlock` exercises all four methods. Each remains blocked beyond its context deadline, with `ensureAuth` on a blocked goroutine's stack. See [behavior log](evidence/behavior-probes.log).

**Repair:** Give authentication locking one owner. Avoid holding the token mutex around HTTP requests or calling methods that reacquire it. Cover both an existing token and a refresh, including cancellation. Coordinate this repair with F08.

### F02 — Switch-port writes discard VLAN selections and ignore configuration

**Source:** [updateSwitchPorts](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/resources/device_switch.go#L422), [switch Update](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/resources/device_switch.go#L477).

The update payload always contains empty `tagNetworkIds` and `untagNetworkIds`, even when the plan contains nonempty selections. It also omits configurable `disable`, `nativeNetworkId`, `voiceNetworkEnable`, and `voiceDscpEnable`. The resource calls this helper for each port in a known ports plan, so an unrelated switch change can also write these incorrect VLAN selections.

**Evidence:** `TestInvestigationSwitchPortPayload` supplies a port with tagged/untagged networks and the four additional fields. The actual PATCH empties both lists and omits all four fields. Controller acceptance and the resulting forwarding behavior were not tested. The emitted request can remove VLAN membership if accepted.

**Repair:** Map every supported writable field deliberately, preserve unknown values, and avoid writing unchanged ports. Add regression cases for nonempty VLAN lists, disabling a port, changing the native network, and a switch-name-only edit with configured ports. PR #23's operational-speed change does not repair this helper.

### F03 — An AP rename sends destructive radio defaults

**Source:** [buildAPRadioConfig](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/resources/device_ap.go#L374), [APRadioSetting](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/client/client.go#L1119).

On an update, omitted optional/computed radio attributes can become unknown. The builder treats unknown values as present because its `has2g`/`has5g` checks only test for null. It then skips assigning unknown fields into a fresh struct. The struct serializes its zero values, including empty channel strings, zero power, and potentially `radioEnable=false`.

**Evidence:** `TestAccInvestigationAPRenamePreservesRadios` imports an AP with working radio settings and changes only its name using Terraform 1.15.8. The provider sends `radioSetting2g` with empty `channel` and `channelWidth` and `txPower=0`, replacing the mock's channel `0`, width `4`, and power `20`. The mock rejects that payload and Terraform apply fails. See [AP acceptance log](evidence/ap-probe.log). The captured failure establishes the 2.4 GHz case; the same zero-initialization pattern exists for 5 GHz.

**Repair:** Preserve current writable settings for omitted/unknown fields, distinguish them from explicit zero/false values, and only update radios when needed. Keep the import-and-rename acceptance case. A helper-only test would miss the planning behavior that triggers this defect.

### F04 — SSID updates reset unmodeled controller settings

**Source:** [wireless-network Update](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/resources/wireless_network.go#L278), [wireless API model](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/client/client.go#L153).

Update constructs a fresh object using creation defaults. It explicitly disables MAC filtering and scheduling, supplies empty rate-limit structures, replaces multicast settings, and serializes `guestNetEnable=false`. When a passphrase is present it also forces `versionPsk=2` and `encryptionPsk=3`. Several of these controls are absent from the resource schema, so an imported SSID cannot express their existing values in Terraform.

**Evidence:** `TestInvestigationWirelessUpdateResetsSettings` changes only the SSID name. The PATCH contains the reset values without reading the current settings during Update. This proves the overwrite request; the exact effect of each field depends on controller support. Disabling an existing guest setting or MAC filter would change access policy.

**Repair:** Build updates from a defined set of writable fields and preserve existing unmodeled values when a complete payload is required. Expose settings that Terraform should own. Do not blindly echo the raw GET response: it can contain read-only fields. Test an imported SSID with guest controls, filtering, scheduling, and a different PSK mode, then rename it.

### F05 — Site device credentials silently fail to change

**Source:** [site credential schema](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/resources/site.go#L79), [site Update](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/resources/site.go#L175).

The site resource accepts changes to `device_account_username` and `device_account_password`, but Update sends only name, region, time zone, and scenario. It subsequently stores the plan, including the new credentials. Terraform can therefore report a completed password change while the controller still has the old credentials. Create handles these fields, making the missing update behavior particularly misleading.

**Evidence:** `TestInvestigationSiteCredentialsUpdate` changes the password, captures a metadata-only PATCH, and reads the new password from the returned state. No credential update is sent.

**Repair:** Implement rotation through the appropriate settings operation, or reject unsupported changes with a clear migration path to `omada_site_settings`. Avoid making a credential change replace the whole site. Test username-only, password-only, and combined changes, including the behavior of masked API responses.

### F06 — Site-settings rotation writes the new password but saves the old one

**Source:** [site-settings Update](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/resources/site_settings.go#L569), especially the unconditional restoration at line 580.

After a successful update, the code copies the prior state's password over the planned password whenever the old value is non-null. This preserves a secret during ordinary refresh-like updates but also discards an intentional rotation. The remote password changes while Terraform receives the old value.

**Evidence:** `TestInvestigationPasswordRotation` verifies that the outgoing API request contains the new password, the API responds with a masked password, and response state contains the old password. This is a direct lifecycle probe. Terraform is expected to reject that state as inconsistent with a known planned value, based on the framework's [Update state requirements](https://developer.hashicorp.com/terraform/plugin/framework/resources/update#caveats).

**Repair:** Retain the successfully applied planned password. Preserve the prior value only when no new password was supplied. Cover both an existing managed credential and an imported resource whose password is initially unknown or null.

### F07 — Authentication retry keeps the expired query token

**Source:** [doRequestWithRetry](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/client/client.go#L427), especially line 467.

After reauthentication, the code replaces `"&token=" + c.token` with exactly the same string. It has already lost the old token, and generated URLs commonly begin the token parameter with `?token=`. The retry consequently has a fresh `Csrf-Token` header and the expired token in its URL. Controllers that validate the query token reject the retry.

**Evidence:** `TestInvestigationRetryRefreshesQueryToken` records `old-token/old-token` followed by `old-token/new-token` for query/header pairs. The second request fails despite successful login.

**Repair:** Parse the URL and set its `token` query parameter from a synchronized token snapshot. Use that same snapshot for the header. Test token replacement with additional query parameters and preserve the bounded retry behavior.

### F08 — Token refresh races with concurrent requests

**Source:** [login/reAuth](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/client/client.go#L375), [URL construction](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/client/client.go#L401), [request header](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/client/client.go#L443).

Token writers use a mutex, but URL builders and header construction read `c.token` without that mutex. A provider client is shared across resources, so ordinary concurrent resource operations can overlap a refresh. Besides a Go data race, request construction can mix tokens from different authentication sessions.

**Evidence:** `go test -race` on `TestInvestigationTokenRace` reports conflicting accesses between `siteURL` and both `reAuth` and `login`. See [race log](evidence/race-probe.log).

**Repair:** Synchronize all token access and use one immutable snapshot per request. Coalesce concurrent refreshes where appropriate. Retain a race-enabled test with overlapping resource requests; do not solve this by keeping a mutex locked across nested authentication calls, which would preserve F01.

### F09 — Pagination silently truncates collections

**Source:** [decodePaginatedData](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/client/client.go#L479), [ListNetworks/GetNetwork](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/client/client.go#L588).

List methods request page 1 with a size of 100, then decode only `data`, discarding pagination metadata. Lookup methods search that partial collection. The same pattern appears in sites, WLAN groups, SSIDs, port profiles, ACL rules, IP groups, mDNS rules, and SAML collections. Which limits are reachable depends on the controller and collection type.

**Evidence:** `TestInvestigationPagination` supplies 101 networks with `totalRows=101`. ListNetworks returns 100, requests only page 1, and GetNetwork reports that network 101 does not exist. This can truncate data sources or prevent import/refresh of an existing object.

**Repair:** Implement shared pagination that reads all pages and retains compatibility with direct-array responses. Test multiple pages, an empty page, and a failure on a later page. A partial result must not be interpreted as confirmed absence; fix this before changing missing-resource handling in F10.

### F10 — Refresh cannot recover from a manually deleted network

**Source:** [network Read](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/resources/network.go#L183).

If the controller no longer contains the network, Read returns an error and keeps the old state. Subsequent planning fails instead of recognizing that Terraform needs to recreate it. There is no `RemoveResource` call in the reviewed resource implementations, so comparable missing-object behavior deserves an audit across resources; the concrete reproduction here is a network.

**Evidence:** `TestInvestigationMissingNetworkRefresh` returns a successful empty network collection. Read emits an error for the deleted ID and retains the prior object.

**Repair:** Represent confirmed absence separately from transport, authentication, and server failures. During Read, remove a genuinely missing object from state, as described in the framework's [Read guidance](https://developer.hashicorp.com/terraform/plugin/framework/resources/read). Keep state on transient errors. Also make deletion of an already absent object idempotent where the API allows it. Complete pagination first (F09).

### F11 — Network creation adopts an incompatible object and reports false state

**Source:** [CreateNetwork](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/client/client.go#L616), [network Create state mapping](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/resources/network.go#L149).

CreateNetwork returns an existing network with the same name without reconciling its settings. The resource then copies selected response fields but leaves the planned VLAN ID in state. This silently claims ownership of an existing object and reports configuration that was never applied. A later refresh reveals drift, and later deletion can target the adopted object.

**Evidence:** `TestInvestigationNetworkAdoption` supplies an existing network named LAN with VLAN 10 and requests VLAN 20. No write occurs, but Create succeeds and returns VLAN 20 in state using the existing object's ID.

**Repair:** Require explicit import or return a conflict for an existing incompatible object. If automatic adoption is an intentional feature, specify its ownership semantics and reconcile/validate all managed fields before reporting success. Treat protected/default networks explicitly. Add a same-name/different-VLAN regression case.

### F12 — HTTP errors can be accepted as success

**Source:** [HTTP response handling](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/client/client.go#L445).

The common request handler checks the decoded Omada `errorCode` but ignores the HTTP status. A valid JSON error body without `errorCode` decodes to Go's default integer zero and passes the success check. A reverse proxy or other intermediary can return such a body even when normal controller errors use an Omada envelope.

**Evidence:** `TestInvestigationHTTPError` returns HTTP 403 with `{"message":"Forbidden"}`. DeleteNetwork returns nil. A resource Delete that trusts this result can allow Terraform to forget an object that still exists.

**Repair:** Validate HTTP status before treating a response as success, and distinguish a missing error code from an explicit success code when an envelope is required. Handle endpoint-specific empty successes deliberately. Add JSON and non-JSON failure cases. Bound and redact diagnostics rather than exposing complete response bodies or token-bearing URLs.

### F13 — Changing an SSID's WLAN group cannot work as planned

**Source:** [wlan_group_id schema](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/resources/wireless_network.go#L60), [Update path selection](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/resources/wireless_network.go#L274), [state overwrite](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/resources/wireless_network.go#L353).

The schema allows a changed WLAN group without requiring replacement. Update always addresses the old group's endpoint and then copies the old group into response state. It never moves the SSID to the planned group. Returning the old known value also violates Terraform's [Update state requirements](https://developer.hashicorp.com/terraform/plugin/framework/resources/update#caveats).

**Evidence:** `TestInvestigationWirelessGroupChange` plans `new-group`, observes a PATCH under `old-group`, and receives `old-group` in state. The lifecycle probe establishes the mismatch; it does not separately run a complete Terraform apply for this case.

**Repair:** Implement a supported move operation, or use `RequiresReplace` for WLAN-group changes if movement requires recreation. Add an acceptance test that changes only this attribute and verifies the resulting location and Terraform plan.

### F14 — SSID refresh retains a custom VLAN that no longer exists

**Source:** [wireless-network Read](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/resources/wireless_network.go#L246).

Read sets `vlan_id` when the API reports a nonzero custom VLAN but has no branch that clears an old value. If the controller changes the SSID back to its default VLAN behavior, Terraform retains the previous custom VLAN. That masks drift and can cause a later unrelated update to reapply the stale VLAN.

**Evidence:** `TestInvestigationWirelessVLANRefresh` begins with VLAN 20 in state and returns `VlanSetting.Mode=0` without custom configuration. The refreshed state still says VLAN 20.

**Repair:** Map the VLAN mode explicitly and set `vlan_id` to null when custom VLAN configuration is absent. Share this mapping between Read and Import. Test custom-to-default, default-to-custom, and custom-to-different-custom transitions.

## Broader code critique

The strongest recurring problem is the conversion between three different kinds of data: Terraform's known/null/unknown values, API fields that may be omitted, and Go zero values. Several builders collapse these distinctions. Creation defaults are also reused during updates, causing the provider to overwrite settings outside its schema. Centralize conversion rules, use separate create/update request types where their semantics differ, and test preservation of unrelated settings.

Authentication, pagination, and API error handling belong in one well-tested client boundary. Currently defects in that boundary propagate to many resources. The resource layer also needs consistent rules for confirmed absence, import, masked secrets, immutable identifiers, and state after partial failure. Focus the first refactoring on these contracts rather than cosmetic reorganization.

The following additional concerns were identified statically. They are **not included in the 14 reproduced findings**:

| Area | Concern | Next verification |
| --- | --- | --- |
| [AP IP configuration](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/resources/device_ap.go#L346) | The update branch implements DHCP, while the schema describes a static mode; a static-mode change does not receive corresponding implementation. | Specify supported static fields and exercise a DHCP-to-static update. |
| [Port-profile Update](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/resources/port_profile.go#L236) | Rebuilding a complete profile with defaults resembles F04 and can reset controller settings not represented in the plan. | Capture a rename update on an imported profile with nondefault settings. |
| [SAML IdP import/state mapping](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/resources/saml_idp.go#L196) | Import stores the API's base64 certificate where the schema promises PEM; the comment that the original cannot be recovered from base64 is incorrect. Read does not reconcile certificate changes. | Add PEM/base64 round-trip and remote certificate rotation cases. |
| [TLS configuration](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/internal/provider/provider.go#L61) | Certificate verification defaults to disabled. This is an explicit design choice, but it leaves controller identity unverified unless the user opts in. | Consider verified TLS by default with documented private-CA configuration and an explicit exception for self-signed deployments. |

## Verification and reproducibility

- [x] Reviewed client and resource implementations, schemas, and CI configuration.
- [x] Ran the existing suite in a container; it passes.
- [x] Reproduced 12 defects with targeted client/resource probes.
- [x] Reproduced the token race with Go's race detector.
- [x] Reproduced AP radio corruption through Terraform's actual planning/apply flow.
- [x] Preserved probe sources and execution logs in this directory.
- [ ] Validate controller-version-specific request handling on a controlled Omada installation after fixes.
- [x] Implement and locally verify A (F01/F07/F08), B (F02), and C (F03), with maintained regression tests and separate patch evidence.
- [ ] Implement the remaining fixes and promote their reproductions to maintained regression tests.

Environment: Linux amd64, Go 1.25.8 in Docker, Terraform CLI 1.15.8. Go image digest: `golang@sha256:3ac2864710f25e84381bf5d4272261c7ba73ada0339d62034df4de20dabb33ca`. The repository's `.go-version` is 1.25.6; these investigation results use the explicitly stated 1.25.8 toolchain. No host Go installation was needed.

| Run | Command inside disposable checkout | Result | Evidence |
| --- | --- | --- | --- |
| Existing tests | `go test ./... -skip Investigation -count=1 -cover` | Pass; live-controller acceptance tests remain gated off | [Baseline log](evidence/baseline-coverage.log) |
| Behavior probes | `go test ./internal/client ./internal/resources -run TestInvestigation -count=1 -v -timeout=90s` | 12 failing top-level cases demonstrate the defects; race probe passes without the detector | [Behavior log](evidence/behavior-probes.log) |
| Concurrency | `go test -race ./internal/client -run TestInvestigationTokenRace -count=1 -v -timeout=90s` | Fails with detected races | [Race log](evidence/race-probe.log) |
| Terraform planning | `TF_ACC=1 TF_ACC_TERRAFORM_PATH=/usr/local/bin/terraform go test ./internal/provider -run TestAccInvestigation -count=1 -v -timeout=90s` | Fails when the mock rejects an incorrect radio payload | [AP log](evidence/ap-probe.log) |

The unchanged suite's measured statement coverage is **26.4% for the client, 0.4% for resources, and 2.9% for the provider**. These figures describe this non-live run. The main branch has only helper tests in the resources package. [CI's unit-test step](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/blob/6bc71b310bc81a90f3ad32194ba8c17947bf79d6/.github/workflows/ci.yml#L66) runs client and resource packages, so it does not exercise the provider acceptance package. Passing that gate currently gives little evidence that resource updates preserve configuration.

The retained [client probes](reproductions/client_probe_test.go.txt), [resource probes](reproductions/resources_probe_test.go.txt), and [provider acceptance probe](reproductions/provider_probe_test.go.txt) are stored as `.go.txt` so they do not alter the normal suite. They assert expected correct behavior and intentionally fail the reviewed commit. Some logs include dependency downloads and host sandbox messages; the Go test results follow those messages. All test credentials and identifiers are synthetic.

Run `bash docs/investigations/reproduce.sh` from this checkout to reproduce in a disposable container. The script checks out the reviewed commit, copies in the probes, and runs all four groups. It requires Docker, network access for image/modules, and a Linux Terraform binary available through `TF_ACC_TERRAFORM_PATH` or `PATH` (1.15.8 was used here). It mounts the repository read-only and removes the container when finished. Its nonzero exit status is expected while the defects remain. It prints results to standard output without modifying existing evidence logs.

## Repair order and current status

- [x] Implement and locally verify F01 and the F07/F08 authentication design together, with deadline and race coverage (A).
- [x] Implement and locally verify F02 switch-port preservation, including real Terraform planning against mock controllers (B).
- [x] Implement and locally verify F03 AP radio preservation, including default migration and partial-failure state recovery (C).
- [ ] Next: fix F04 SSID preservation alongside F13/F14 identity and VLAN drift (D); retain tests through Terraform where unknown planning values matter.
- [ ] Fix both credential paths, F05/F06, with explicit rotation tests.
- [ ] Fix F12's HTTP classification, F09's pagination, then F10's missing-resource handling in that order so absence is trustworthy.
- [ ] Fix F11, F13, and F14; add import, identity-change, and remote-drift acceptance cases.
- [ ] Investigate and fix F15's persistent plan when the whole switch ports list is omitted (I).
- [ ] Complete controlled-hardware validation and submission review for A, B, and C; record publication and merge separately.
- [ ] Add local mock acceptance tests to CI alongside a targeted race job. Keep live-controller compatibility testing as a separate gate for payload/API changes.

The original investigation did not change production code. Subsequent A, B, and C fixes are isolated in the local branches recorded above; their evidence is stored separately from the baseline logs. No review comments or PRs have been posted to GitHub by this work.
