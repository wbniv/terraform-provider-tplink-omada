# Provider reliability: PR tracker

Started 2026-10-18 for Will. Baseline: `6bc71b310bc81a90f3ad32194ba8c17947bf79d6`.

This is the working record for fixing the [code-review findings](../investigations/2026-10-18-code-review.md). The investigation remains a historical record of the baseline. Update this tracker as patches and evidence change; an implemented fix is not automatically validated or merged.

## Current work

- [x] Implement and locally verify D1, SSID update preservation (F04), on `fix/ssid-update-preservation` in `/home/will/terraform-provider-tplink-omada-ssids`, based on `6bc71b3`. Committed and pushed as [80991a5](https://github.com/wbniv/terraform-provider-tplink-omada/commit/80991a5197674a2ee5e2d93ab8d3b571ef4ea774); [evidence and PR notes](../investigations/ssid-fix/README.md).
- [x] Implement and locally verify D2/F14, mode-aware SSID VLAN state, on `fix/ssid-vlan-state` in `/home/will/terraform-provider-tplink-omada-ssid-vlan`, stacked on D1 `80991a5`. Committed and pushed as [0bbd076](https://github.com/wbniv/terraform-provider-tplink-omada/commit/0bbd07687fe76f7203779536bc9b5a24095aa6d3); [D2 evidence and PR notes](../investigations/ssid-vlan-fix/README.md).
- [x] Implement and locally verify D3/F13, WLAN-group replacement, on `fix/ssid-group-identity` in `/home/will/terraform-provider-tplink-omada-ssid-group`, stacked on D2 `0bbd076`. Committed and pushed as [ec2d949](https://github.com/wbniv/terraform-provider-tplink-omada/commit/ec2d949699274c3a6dea6cf3d272f22bde13d12e); [D3 evidence and PR notes](../investigations/ssid-group-fix/README.md).
- [x] Implement and locally verify E, credential rotation/state and settings preservation (F05/F06/F16), on `fix/site-credentials` in `/home/will/terraform-provider-tplink-omada-credentials`, based on A `52ed681`. E is committed and pushed as [8f24c37](https://github.com/wbniv/terraform-provider-tplink-omada/commit/8f24c37c643a968e51cc77aeb63d3ffe2f0a96e5); [plan](site-credentials.md) and [evidence/PR notes](../investigations/site-credentials-fix/README.md) record 48 direct cases, 12 Terraform scenarios, and combined A + D1–D3 + E verification.
- [x] Implement and locally verify F/F12 on `fix/http-responses` in `/home/will/terraform-provider-tplink-omada-http-responses`, based on E `8f24c37` (including A). F is committed and pushed as [c210dec](https://github.com/wbniv/terraform-provider-tplink-omada/commit/c210dec9a4bb10a15881f15a250b7c1843543d31); [plan](http-responses.md) and [evidence](../investigations/http-response-fix/README.md) record 223 client cases, full Go/race gates, and 41 combined Terraform scenarios/205 steps.
- [x] G1/F09 complete pagination is implemented and locally verified on `fix/complete-pagination`, based on F `c210dec`; pushed as [9343503](https://github.com/wbniv/terraform-provider-tplink-omada/commit/9343503d13a5428e91d121e0104ce7c36cd93cd6). [Evidence](../investigations/pagination-fix/README.md): 94 client cases, full Go/race gates, three new Terraform scenarios, and 44 combined scenarios/216 steps pass. See the [G1/G2 plan](pagination-and-disappearance.md). Real-controller testing exposed an unpaginated WLAN-group regression; L1 is corrected and pushed as `0730101`. The successful mock evidence remains valid for its recorded fixtures.
- [x] Implement and locally verify C, AP radio preservation (F03), in `/home/will/terraform-provider-tplink-omada-ap-radios` on `fix/ap-radio-updates`, based on `6bc71b3`. [C evidence and review notes](../investigations/ap-radio-fix/README.md).
- [ ] Finish C's controlled-AP validation and PR-submission review. A, B, and C are committed and pushed to Will's fork; no upstream PR has been opened.
- [x] Implement and locally verify PR B in worktree `/home/will/terraform-provider-tplink-omada-switch-ports`, branch `fix/switch-port-updates`, based directly on `6bc71b3`. A remains isolated on `fix/client-authentication`. [B evidence and review notes](../investigations/switch-port-fix/README.md).
- [x] Complete controlled-switch validation for B/F15 on the ES216G hardware revision 1.20 and firmware `1.0.1 Build 20260329 Rel.14489`, using Omada Controller 6.1.0.19: import, repeated empty plans with `ports` omitted, reversible port 8 name change/restore, and state-only destroy all pass. [Live acceptance record](es216g-terraform-acceptance.md).
- [ ] Finish PR B/F15 PR-submission review; the passing live check is limited to this switch revision, firmware, and controller version.
- [x] Implement and locally verify PR A, authentication and certificates, on `fix/client-authentication`; see [evidence and review notes](../investigations/authentication-fix/README.md).
- [ ] Finish PR A's PR-submission review and controller compatibility declaration. Local verification and a fork push do not mark a PR submitted or merged.
- [x] Preserve baseline reproductions and logs under `docs/investigations/`.
- [x] Inspect repository contribution standards: `.github/PULL_REQUEST_TEMPLATE.md` requires conventional commits, formatting/vet/tests, documentation where applicable, and a real-controller test declaration.
- [x] Check project AI attribution policy: no explicit policy found in tracked contribution files at the reviewed baseline. Will's attribution preference still applies if attribution is included or required later; verify tool/model/reasoning metadata before writing it.
- [x] Run the first real-controller round on Software Controller 6.1.0.19 with zero adopted devices; see the [lab report](../investigations/software-controller-lab/README.md). Network/profile CRUD, site credential rotation, and a 102-network inventory pass.
- [x] Locally verify the [controller compatibility follow-ups](controller-compatibility.md), including L5 empty collections and L8 ACL creation/PUT updates. [L5 evidence](../investigations/empty-collections-fix/README.md), [L8 evidence](../investigations/acl-create-fix/README.md). L7 is pushed as `ce80af6`; its secured SSID fields, 24 mock scenarios, and both live scenarios pass; [evidence](../investigations/ssid-controller-fields/README.md). Physical provisioning, radios, and switch ports remain separate gates.
- [x] Complete the lab/README/plan updates and local commits: Go vendoring `5555332`, lab/Omada LFS manifest `0d26e72`. Go vet/unit/build pass without external network access. Live failures remain visible.
- [x] Publish the lab/vendoring commits to the recreated standalone repository and verify the Omada LFS object with a fresh download/checksum. All original branches/tags are preserved; Actions are enabled again.
- [ ] Publish PRs after the local changes and review packages are ready and publication is authorized. No GitHub review/comment/PR has been posted by this work.

- [x] Locally verify L6 on `fix/ssid-pmf-handling`: 26 mock scenarios, three live scenarios, Go gates, and client discovery pass. [L6 evidence](../investigations/ssid-pmf-fix/README.md). Committed as `a046a36`, with lab coverage in `c026124`; upstream publication remains pending.

- [x] Locally verify L2 on `fix/controller-certificate-protocol`: uploads, nested activation, stored identity readback, five Terraform steps, Go gates, and TLS fingerprint after restart pass. [L2 evidence](../investigations/certificate-protocol-fix/README.md). Committed as `4c21666`; upstream publication remains pending.

- [x] Locally verify L3 on `fix/controller-session-recovery`: six live recovery cases, client race detector, Go gates, and five Terraform certificate steps pass. [L3 evidence](../investigations/session-recovery-fix/README.md). Committed as `58610fd`; upstream publication remains pending; natural expiry and other controller versions remain verification limits.

- [x] Locally verify L4 on `fix/ip-group-protocol`: IP/Port CRUD/import/destroy, complete 16-group inventory, ACL references, and readback-failure recovery pass; Go/race/build gates pass. [L4 evidence](../investigations/ip-group-fix/README.md). Committed as `a2b1baa`; upstream publication remains pending. Port lists are now required and shared across entries.
- [x] Locally verify L8 on `fix/acl-create-readback`: empty creation acknowledgment, duplicate-safe import recovery, PUT updates, order/syslog preservation, five live Terraform steps, four mock steps, and Go/race/build gates pass. [L8 evidence](../investigations/acl-create-fix/README.md). Committed as `5b1b14f`; upstream publication remains pending.
- [x] Locally verify L5 on `fix/empty-data-source-collections`: all eight collection data sources, seven mock Terraform steps, live empty/populated inventories with a stable plan, the existing IP/Port-group lifecycle, and Go/race/build gates pass. [L5 evidence](../investigations/empty-collections-fix/README.md). Committed as `6d5a721`; upstream publication remains pending; G2 is next.

## Commits and fork branches — 2026-10-19

Will subsequently requested a standalone repository so the code and Omada LFS binary can live together. The original fork is preserved as `wbniv/terraform-provider-tplink-omada-upstream-fork`; `wbniv/terraform-provider-tplink-omada` is now standalone. All 14 original branches and nine tags match their recorded pre-migration commits exactly. New lab branch: `test/software-controller-lab`; Go vendor commit `5555332a65a4b52761c81b67d7b767e377f70fd3`, lab commit `0d26e723ce833a44c2043b6be23e69282b189a35`, diagnostic identity correction `c953da6ed94021d7774e26e76e55bb1ef2d7b8fc`. Historical fork-publication records below remain intact.

Will authorized commit and push. Upstream does not grant this account push access, so the branches are published to [Will's fork](https://github.com/wbniv/terraform-provider-tplink-omada). All nine code commits retain the validated file hashes. A–D1 are based directly on `6bc71b3`; D2 stacks on D1, D3 stacks on D2, E stacks on A, F stacks on E, and G1 stacks on F.

| Work | Branch | Commit | Scope |
| --- | --- | --- | --- |
| A | `fix/client-authentication` | [52ed681](https://github.com/wbniv/terraform-provider-tplink-omada/commit/52ed681674e5a0058ae7807fcd08399c21fa021a) | Two client files |
| B | `fix/switch-port-updates` | [1c7f535](https://github.com/wbniv/terraform-provider-tplink-omada/commit/1c7f535e86a4db9de53645c2c844d49429ee5694) | Five switch source/test/documentation files |
| C | `fix/ap-radio-updates` | [9f4b4f8](https://github.com/wbniv/terraform-provider-tplink-omada/commit/9f4b4f8c3ccd1c61f10dd2622f2ae2b3bcb9679b) | Seven AP source/test/documentation/example files |
| D1 | `fix/ssid-update-preservation` | [80991a5](https://github.com/wbniv/terraform-provider-tplink-omada/commit/80991a5197674a2ee5e2d93ab8d3b571ef4ea774) | Eleven SSID source/test/fixture/documentation/example/template files |
| D2 | `fix/ssid-vlan-state` | [0bbd076](https://github.com/wbniv/terraform-provider-tplink-omada/commit/0bbd07687fe76f7203779536bc9b5a24095aa6d3) | Nine VLAN source/test/documentation/template files; based on D1 |
| D3 | `fix/ssid-group-identity` | [ec2d949](https://github.com/wbniv/terraform-provider-tplink-omada/commit/ec2d949699274c3a6dea6cf3d272f22bde13d12e) | Seven group-identity source/test/documentation/template/example files; based on D2 |
| E | `fix/site-credentials` | [8f24c37](https://github.com/wbniv/terraform-provider-tplink-omada/commit/8f24c37c643a968e51cc77aeb63d3ffe2f0a96e5) | Fifteen credential/settings source/test/fixture/documentation/template/example files; based on A |
| F | `fix/http-responses` | [c210dec](https://github.com/wbniv/terraform-provider-tplink-omada/commit/c210dec9a4bb10a15881f15a250b7c1843543d31) | Nine HTTP source/test/guide files plus generated README history; based on E |
| G1 | `fix/complete-pagination` | [9343503](https://github.com/wbniv/terraform-provider-tplink-omada/commit/9343503d13a5428e91d121e0104ce7c36cd93cd6) | Seven pagination source/test/guide files plus generated README history; based on F |

D2 is committed and pushed as [0bbd076](https://github.com/wbniv/terraform-provider-tplink-omada/commit/0bbd07687fe76f7203779536bc9b5a24095aa6d3) on `fork/fix/ssid-vlan-state`, based on D1 `80991a5`. Its nine committed file hashes match the recorded tested sources.

Plans, investigations, and evidence are maintained separately on `docs/provider-reliability-review` in the original checkout, `/home/will/terraform-provider-tplink-omada`. A is retained as a local and remote branch; B, C, D1, D2, and D3 retain their dedicated worktrees. This documentation branch contains baseline production code. Use a fix branch to inspect or run that fix.

E is committed and pushed as [8f24c37](https://github.com/wbniv/terraform-provider-tplink-omada/commit/8f24c37c643a968e51cc77aeb63d3ffe2f0a96e5) on `fork/fix/site-credentials`, based on A `52ed681`. Its 15 committed file hashes match the tested sources; the remote branch was verified.

- [x] Recheck upstream `main`, open PR heads/checks, and contribution requirements before committing. No explicit AI-attribution requirement was found; no tool/model metadata was guessed.
- [x] Commit A, B, and C with conventional titles and push those three branches atomically to `fork`.
- [x] Commit D1 with its breaking-change title/footer and push `fix/ssid-update-preservation` to `fork`; verify every committed file against the tested hashes.
- [x] Commit/push D2 as `0bbd076` and documentation as `2b31fb1`; verify matching remote refs and all nine committed code/doc hashes.
- [x] Commit/push D3 as [ec2d949](https://github.com/wbniv/terraform-provider-tplink-omada/commit/ec2d949699274c3a6dea6cf3d272f22bde13d12e) with its breaking-change note; all seven committed hashes match the tested files.
- [ ] Open upstream PRs, then record their URLs and CI results. The existing CI workflow triggers on pushes to `main` and pull requests targeting `main`; these fork feature-branch pushes do not trigger that workflow.

## Proposed PR boundaries

These are local work labels, not GitHub issue or PR numbers. Keep each patch reviewable and avoid bundling dependency upgrades into behavior fixes.

| Work | Findings | Scope | Dependency | State |
| --- | --- | --- | --- | --- |
| A | F01, F07, F08 | Certificate deadlock, consistent session tokens, concurrent refresh, cancellation, multipart retry | None | Implemented and locally verified; controller test pending |
| B | F02 | Preserve switch-port VLAN lists and honor supported writable fields | Stored #23 compatibility verified | Implemented and locally verified; controller test pending |
| C | F03 | Preserve omitted/unknown AP radio settings through Terraform updates | A compatibility verified | Implemented and locally verified; controller test pending |
| D | F04, F13, F14 | SSID update preservation, WLAN-group identity, VLAN drift | D1 → D2 → D3; A + D1 + D2 + D3 verified; controller/device validation pending | D1, D2, and D3 pushed |
| E | F05, F06, F16 | Site and site-settings credential ownership/state; preserve unrelated settings | Stacked on A; A + D1–D3 + E verified | Implemented, locally verified, and pushed; controller/device validation pending |
| F | F12 | HTTP status/envelope validation, typed errors, bounded diagnostics, redirect protection | Stacked on E, includes A; D compatibility verified with recorded test adjustment | Implemented and locally verified; pushed as [c210dec](https://github.com/wbniv/terraform-provider-tplink-omada/commit/c210dec9a4bb10a15881f15a250b7c1843543d31); controller validation pending |
| G1 | F09 | Complete pagination across ten collections and raw SSID lookup | Stacked on F; D integration recorded | Implemented and locally verified; pushed as [9343503](https://github.com/wbniv/terraform-provider-tplink-omada/commit/9343503d13a5428e91d121e0104ce7c36cd93cd6) |
| G2 | F10 | Trustworthy missing-resource handling and state removal | G1 complete discovery; F response classification | Implemented, mock/live validated, pushed. Reconciled with L4/L8 on `fix/confirmed-disappearance-l4` (`0cf3c8a`); combined tree passed Go 1.25.8 `go test -mod=vendor ./...`, `go vet -mod=vendor ./...`, and provider build on 2026-10-27. Focused combined live IP-group disappearance/recreation acceptance passed on Controller 6.1.0.19 on 2026-10-28. [PR draft, branch reconciliation, and dependency review](../investigations/confirmed-disappearance-fix/README.md); [combined live record](../investigations/confirmed-disappearance-fix/combined-l4-live-acceptance.md). Still stacked on earlier work, so resolve the upstream base before submission. |
| H | F11 | Explicit handling of same-name network adoption and actual state | G2 + L4; `fix/confirmed-disappearance-l4` | Implemented, regression-tested, and pushed through [`307424c`](https://github.com/wbniv/terraform-provider-tplink-omada/commit/307424c); hardware compatibility remains separate |
| I | F15, found during B | Persistent switch update plan when the entire ports attribute is omitted after import | B; `fix/switch-omitted-ports-plan` | Implemented, acceptance-tested, and pushed through [`286a9a2`](https://github.com/wbniv/terraform-provider-tplink-omada/commit/286a9a2) |

Prioritize B–E after A because they can affect live configuration or credential expectations. F–H remain substantive correctness work. If a boundary grows too large, split it while preserving these finding IDs and dependencies.

## Acceptance criteria and compatibility notes

### A — Authentication and certificates

Suggested title: `fix(client): prevent certificate deadlocks and refresh sessions safely`

- [x] Certificate upload, key upload, activation, and read complete successfully against a mock API with an existing token.
- [x] Those paths authenticate when the token is absent; login cancellation and waiting-caller cancellation/recovery pass without hanging.
- [x] An expired session retries once with matching refreshed query/header tokens; preserve additional query parameters.
- [x] Multipart upload retries replay the same filename, metadata, and bytes.
- [x] Concurrent expiry causes one refresh for the failed session; waiting callers can cancel.
- [x] Authentication failure, repeated expiry, and empty-token login responses fail clearly.
- [x] Race detector passes with concurrent mock request/refresh traffic, including URL builders and UpdateSite.
- [x] Existing read-only certificate protections still hold.
- [x] Run formatting, vet, build, full unit suite, and targeted race tests using Go 1.25.6, the repository's pinned version, in Docker.
- [x] Record precise commands/results, file hashes, and local diff scope in the [patch evidence](../investigations/authentication-fix/README.md).

Compatibility: retain public resource schema and state format, endpoint paths, multipart form names, and bounded retry count. Controller compatibility still requires a controlled test of upload/activation; activation can change TLS behavior. Unit tests establish client behavior, not compatibility with every firmware release.

### B — Switch ports

Suggested title: `fix!: preserve imported switch port settings during updates`. Treat removed implicit false defaults as a documented behavior change; the state shape remains compatible.

- [x] Isolate work from PR A in a separate branch/worktree based on the original main commit.
- [x] Inspect #23's stored head (`2ec46a4`): its change concerns the speed schema/documentation and separate tests. B will keep speed out of update payloads and verify compatibility without duplicating that schema change.
- [x] Identify a related preservation requirement: four nested port booleans default to false. Omitted settings on imported ports must retain controller values, so remove those defaults and document explicit `false` configuration when desired.
- [x] Implement and verify port updates from current writable controller fields plus known planned values, validate before writes, skip unchanged ports, and distinguish explicit empty VLAN lists from unknown/omitted lists.
- [x] Update switch resource documentation with omitted-value semantics, full-list validation, ordering by port identity, and the four removed boolean defaults.
- [x] Run the new lifecycle probes and the real Terraform mock acceptance sequence for both standard and Agile Series routes; the same tests fail on baseline and pass with B.
- [x] Initial 13 lifecycle probes pass: preservation, explicit clearing/false/zero, unchanged ports, invalid input before writes, failed-request handling, and port identity ordering.
- [x] Isolate the import-planning failure: the same omitted-ports case fails on unchanged `6bc71b3` and B. Record it as F15, retain the probe/logs, and document the limitation. B's acceptance sequence uses the existing complete-port-list contract; it does not claim to fix F15.
- [x] Reproduce nonempty tagged/untagged VLANs, native network, disable, and voice settings in request assertions; baseline fails and the patch passes.
- [x] Limit PATCH payloads to supported writable fields, preserve omitted values from the controller snapshot, and skip unchanged ports.
- [x] Run the eight-step Terraform import/update sequence on mock standard and Agile Series routes, including name-only edits, explicit VLAN/boolean changes, empty-list clearing, stable plans, and port reordering; both pass.
- [x] Apply stored #23 head `2ec46a4` on top of B in a disposable checkout. The patch applies cleanly, and port preservation plus both speed acceptance tests pass. The speed schema change remains outside B.
- [x] Run formatting, vet, build, and full unit tests with Go 1.25.6 in Docker; all pass. Preserve commands, logs, tested file hashes, and limitations in the evidence directory.
- [ ] Validate payloads against the relevant switch model/controller version; record both.

### C — AP radios

Suggested title: `fix!: preserve imported AP radio settings during updates`. Removing implicit `true`/`4` radio defaults changes omission semantics and needs an explicit migration note.

Starting evidence: [F03](../investigations/2026-10-18-code-review.md#f03--an-ap-rename-sends-destructive-radio-defaults), [retained Terraform probe](../investigations/reproductions/provider_probe_test.go.txt), and [baseline acceptance log](../investigations/evidence/ap-probe.log). The recorded failure proves the 2.4 GHz case; add explicit 5 GHz assertions before claiming both are verified.

- [x] Isolate C in `/home/will/terraform-provider-tplink-omada-ap-radios`, branch `fix/ap-radio-updates`, based on `6bc71b310bc81a90f3ad32194ba8c17947bf79d6`. Keep A's client changes separate and check their combination in a disposable checkout.
- [x] Promote the retained reproduction into `TestAccDeviceAPRadioPreservation` and show all three cases failing on baseline: dual-band defaults, dual-band nondefaults, and a 2.4 GHz-only AP. The baseline also sends a bogus 5 GHz payload for the single-band case.
- [x] Audit radio defaults: remove implicit `true` from `radio_2g_enable` and `4` from `radio_2g_tx_power_level` so omission preserves imported values. Default and nondefault import scenarios pass.
- [x] Preserve 2.4 GHz and 5 GHz radio settings when omitted or unknown in the plan.
- [x] Distinguish explicit false/zero from unknown values; test intentional radio changes, one changed field with other fields omitted, and a 2.4 GHz-only AP with no 5 GHz settings.
- [x] Skip unchanged radios, validate both bands before writes, and verify state/readback after partial failures. Fourteen lifecycle cases pass; thirteen fail on baseline.
- [x] Verify import, name-only changes, intentional radio changes, resulting state, removal of configuration, and repeated empty plans through Terraform. Ten-step dual-band default/nondefault sequences and the seven-step single-band sequence pass.
- [x] Document radio ownership, removed defaults, complete-snapshot requirements, and partial-failure behavior. Update resource examples to use the schema's radio-width enum codes and site/MAC import ID.
- [x] Run formatting, vet, build, the full unit suite, and targeted mock acceptance in Go 1.25.6; retain failing baseline and passing patch logs with tested file hashes. The AP Terraform example passes formatting checks.
- [x] Combine A and C in a disposable checkout: full unit suite and all three AP acceptance sequences pass. A's client code remains outside C's patch.
- [ ] Record controlled-AP validation separately, including controller version, AP model/firmware, operations, and readback. Mock tests do not satisfy the upstream template's hardware-test declaration.

Verification and remaining submission gates: [C evidence](../investigations/ap-radio-fix/README.md), completed locally on 2026-10-19.

### D — Wireless networks

Detailed plan: [SSID preservation, identity, and VLAN state](ssid-reliability.md), prepared 2026-10-19. It defines D1 preservation, D2 VLAN readback, and D3 group identity, with ownership/API decisions, review screens, regression cases, and PR gates. D1 is implemented and locally verified; [evidence and PR notes](../investigations/ssid-fix/README.md) record baseline failures, 47 passing lifecycle/validation cases, eight Terraform scenarios, A compatibility, migration decisions, and tested file hashes. D1 is committed and pushed as [80991a5](https://github.com/wbniv/terraform-provider-tplink-omada/commit/80991a5197674a2ee5e2d93ab8d3b571ef4ea774). D2 is locally verified and committed/pushed as [0bbd076](https://github.com/wbniv/terraform-provider-tplink-omada/commit/0bbd07687fe76f7203779536bc9b5a24095aa6d3) on D1; [D2 evidence](../investigations/ssid-vlan-fix/README.md) records 95 lifecycle cases, three new Terraform scenarios, full D1 regression, and A compatibility. D3 is locally verified and committed/pushed as [ec2d949](https://github.com/wbniv/terraform-provider-tplink-omada/commit/ec2d949699274c3a6dea6cf3d272f22bde13d12e) on D2; [D3 evidence](../investigations/ssid-group-fix/README.md) records 21 direct cases, 13 new Terraform scenarios, full D1/D2 regression, A compatibility, and recreation risks.

- [x] Preserve guest controls, MAC filtering, schedules, rate limits, multicast, and PSK mode during unrelated changes.
- [x] Inventory writable fields from the official v5.9 reference and existing v6 model, copy them recursively, exclude known read-only fields, and reject unknown/incomplete snapshots.
- [ ] Verify the inferred full-object PATCH contract and PMF meanings against a controlled controller; mock tests do not establish firmware compatibility.
- [x] Require WLAN-group replacement in D3, with a migration/recreation note.
- [x] Clear stale custom VLAN state when the controller returns default VLAN mode; share interpretation across refresh/import/data-source/write-readback paths and reject incomplete/unsupported snapshots.
- [x] Cover import, rename, defaults/unknown values, masked/absent API passphrases, old untrusted state, configured security changes, VLAN writes, failure state, and repeated plans.
- [x] Cover default/custom VLAN drift, stale fields, import, observed readback, restoration/removal plans, and stable repeated plans in D2.
- [x] Cover D3 group replacement planning, old/new identities, unchanged/omitted/unknown values, import, site replacement, both lifecycle orders, failure/retry, and stable plans. Known invalid recreation credentials fail planning before deletion.
- [ ] Review SSID create recovery when POST succeeds but the following transport/lookup/JSON decode fails before the resource receives the new ID. D2's created-ID preservation covers a returned decoded object; earlier client failures need separate evidence and a repair.
- [x] Split D1 preservation, D2 VLAN drift, and D3 group identity.

### E — Credentials

Suggested title: `fix(resources)!: make site credential rotation explicit and preserve settings`. [Detailed plan](site-credentials.md); [evidence and migration](../investigations/site-credentials-fix/README.md).

- [x] Reject changed existing-site credentials with migration instructions to settings; validate bootstrap pairs and allow removal of both site inputs without an API write.
- [x] Keep the accepted planned secret after settings rotation, including successful PATCH followed by failed readback. Never import/refresh API passwords into state.
- [x] Require both explicitly configured settings inputs; test username-only/password-only/combined changes, imported/null secrets, omitted/partial config, unknown references, and failures/retries.
- [x] Use settings as the rotation owner, without replacing a site. Document one settings resource per site; cross-resource uniqueness is not enforced.
- [x] Reproduce and fix F16's unknown unrelated settings being serialized as zero values. Preserve known writable siblings, omit unchanged sections, and reject unknown fields in changed sections before writing.
- [x] Pass 48 direct cases, 12 Terraform scenarios/65 steps, formatting/vet/build/full unit gates, and combined A + D1–D3 + E full unit/36-scenario acceptance in Docker.
- [x] Update schemas, docs/templates/examples, and import IDs; record breaking behavior and existing-default caveats. All fixtures and evidence credentials are synthetic.
- [x] Commit/push E as [8f24c37](https://github.com/wbniv/terraform-provider-tplink-omada/commit/8f24c37c643a968e51cc77aeb63d3ffe2f0a96e5); retain evidence on `docs/provider-reliability-review`. All 15 committed hashes and the code remote ref are verified.
- [ ] Complete controlled-controller validation and upstream submission separately.

### F — Response handling

Detailed plan: [HTTP response validation](http-responses.md), including status/envelope policy, empty-body decisions, redirect protection, error types, diagnostic limits, Terraform delete recovery, and compatibility gates.

- [x] Check HTTP status before accepting an Omada success envelope.
- [x] Distinguish explicit success from missing `errorCode`; define endpoint-specific empty-success behavior.
- [x] Add typed transport/API/HTTP/protocol errors; expose HTTP 404 without inferring resource absence. Endpoint-specific API not-found mapping remains G/F10 after complete pagination.
- [x] Test JSON and non-JSON 4xx/5xx responses, malformed bodies, normal success, and session expiry.
- [x] Redact tokens and bound response diagnostics; review changes to existing error-message tests.

- [x] Verify 214 shared behavioral cases plus nine typed-error/read-limit checks; five Terraform delete/retry scenarios, all E scenarios, full Go/race gates, and combined D1–D3 acceptance pass.
- [x] Commit/push F as [c210dec](https://github.com/wbniv/terraform-provider-tplink-omada/commit/c210dec9a4bb10a15881f15a250b7c1843543d31); verify all nine committed hashes and the fork ref.
- [ ] Include the [six-assertion SSID diagnostic adjustment](../investigations/http-response-fix/ssid-diagnostic-assertions.patch) when integrating D with F. Hardware/upstream gates remain open.

### G — Pagination and disappearance

Detailed plan: [G1 complete pagination and G2 confirmed disappearance](pagination-and-disappearance.md). G1 is pushed. The WLAN-group collection correction is implemented and passes real discovery and the 102-network regression on `fix/wlan-group-collection`; [L1 evidence](../investigations/wlan-collection-fix/README.md) records the original SSID gates; [L7 evidence](../investigations/ssid-controller-fields/README.md) now verifies secured updates, and [L6 evidence](../investigations/ssid-pmf-fix/README.md) verifies open/secured PMF handling.

- [x] Iterate all pages for every affected collection; retain direct-array compatibility.
- [x] Test more than 100 objects, empty collections, later-page errors, duplicate/stalled pages, and metadata variations.
- [x] Never treat a partial or failed list as proof that a resource is absent.
- [x] Remove confirmed absent collection-backed resources from state during Read; retain state on transient/authentication errors and unconfirmed direct-endpoint failures.
- [x] Make already-absent deletes idempotent for the supported collection-backed resources.
- [x] Add Terraform refresh/recreation acceptance coverage and focused real-controller deletion/recreation validation.

### H — Network adoption

- [x] Replace silent same-name adoption with an import conflict when purpose, VLAN, or explicitly configured settings differ; reject ambiguous duplicate names.
- [x] Preserve adoption only for a unique matching network and populate resource state from the controller response.
- [x] Protect the built-in `Default` interface/VLAN 1 network from deletion; treat an already absent network as an idempotent delete.
- [x] Document ownership, import, and destroy behavior in the network resource docs.
- [x] Cover same-name/different-VLAN and explicitly configured mismatches, matching adoption with controller-derived state, protected-default deletion, and cleanup in client/provider regressions.
- [x] Run `TF_ACC=1` mock Terraform acceptance for matching adoption and protected-default deletion; full vendored Go tests, vet, and provider build pass with Go 1.25.8.
- [ ] Validate network behavior against a live controller before claiming controller compatibility.

## Additional investigations to retain

These were static concerns, not part of the 14 reproduced defects. Establish behavior and scope before making claims in a PR.

- [x] AP static-IP mode: the resource lacked writable address/mask/gateway attributes. Explicit `static` configuration now returns a clear error; controller-reported static mode remains readable and is preserved when unmanaged.
- [x] AP settings beyond radios: removed the implicit LED default and merge omitted load-balancing/RSSI siblings from a fresh controller snapshot before a grouped write. If the current group is unavailable, the update stops before writing. Pushed on `fix/ap-settings-preservation` as [`ed49a99`](https://github.com/wbniv/terraform-provider-tplink-omada/commit/ed49a99).
- [x] Run Terraform 1.15.8 mock acceptance for imported LED `0`, unmanaged static-IP retention, explicit static-mode rejection, partial load-balancing/RSSI sibling preservation, and dual-band/single-band radio preservation. Full vendored Go tests, vet, and provider build pass with Go 1.25.8. Committed and pushed on `fix/ap-settings-preservation` as [`bd4762b`](https://github.com/wbniv/terraform-provider-tplink-omada/commit/bd4762b).
- [ ] Port-profile updates: capture preservation of nondefault controller settings during a rename.
- [ ] SAML IdP certificates: verify PEM/base64 import, remote certificate changes, and empty-field clearing.
- [ ] TLS defaults: decide a compatibility-aware trust/CA strategy; keep any default change separate from bug fixes.
- [ ] Input validation: audit ranges, enumerations, and related fields before API side effects.
- [ ] Default WLAN selection: verify selection uses controller semantics rather than array position.
- [ ] Provider startup cancellation: propagate context through controller discovery/client construction; PR A fixes login request cancellation but does not change startup signatures.
- [ ] Partial failure/state: audit operations that make multiple writes before returning an error.
- [ ] Site create recovery: retain the allocated ID when POST succeeds but the following GET/decoding fails. E validates before creation but does not repair this earlier client failure path; reproduce it separately, as with SSID create recovery.
- [ ] Switch service settings: `buildSwitchServiceConfig` converts an unknown loopback setting to false; audit this separately from B's port payload changes.
- [ ] CI: run local mock acceptance tests and targeted race coverage; preserve a separate real-controller compatibility declaration.
- [ ] Correct the existing README license reference: `[LICENSE](LICENSE)` points to a missing file at the E baseline. Confirm the intended license with the maintainer; F leaves this unrelated reference unchanged.

### F15 — Persistent plan after importing a switch without ports configured

- [x] Reproduce a nonempty update plan immediately after a successful import with only the switch name configured, on both original main and B.
- [x] Preserve [the original acceptance probe](../investigations/switch-port-fix/omitted-ports-probe.go.txt), [baseline failure](../investigations/switch-port-fix/omitted-ports-baseline.log), and [patch failure](../investigations/switch-port-fix/omitted-ports-patch.log).
- [x] Determine the planning cause: after import, framework planning marked the omitted Optional+Computed `ports` list and other unconfigured computed fields unknown, creating an update plan.
- [x] When the entire `ports` list is omitted, carry forward known prior values for null-config planned attributes. Configured or unknown `ports` expressions remain under Terraform's plan.
- [x] Run the import, repeated empty-plan, configured-list removal, and unrelated-switch-change mock acceptance cases on standard and Agile series. Both pass with Terraform 1.15.8; full vendored Go tests, vet, and provider build pass with Go 1.25.8.
- [x] Run `go test -mod=vendor ./...` on the F15 worktree `fix/switch-omitted-ports-plan` after live acceptance; client, provider, and resource packages pass with Go 1.25.8.
- [x] Run live ES216G acceptance on the F15 head `286a9a2`: import with `ports` omitted, two stable empty plans, full 16-port configuration, reversible name-only update on physically empty port 8, readback, restore, stable empty plan, and destroy that leaves the adopted switch intact. Controller 6.1.0.19; ES216G v1.20 firmware `1.0.1 Build 20260329 Rel.14489`. [Live record](es216g-terraform-acceptance.md).

Priority: P2. The provider exposes `ports` as Optional+Computed. B preserves omitted individual attributes; I carries forward imported values when configuration omits the whole list. The original baseline reproduction and patch failure remain historical evidence; I's standard and Agile mock acceptance cases pass.

### F16 — Site settings updates zero unknown unrelated values

- [x] Reproduce on A, whose site/settings resource sources match `6bc71b3`: a credential update emits false mesh/failover/roaming values for unknown unconfigured plan attributes.
- [x] Fix under E by resolving unconfigured unknown values from a fresh snapshot and patching changed sections with preserved writable siblings.
- [x] Retain [baseline/patch evidence](../investigations/site-credentials-fix/README.md#new-finding-f16-unknown-site-settings-become-destructive-zero-values), including a direct case that keeps the old password unchanged to isolate preservation from F06.
- [x] Validate stored credential rotation and selected unrelated-settings preservation on Software Controller 6.1.0.19; see the [lab evidence](../investigations/software-controller-lab/README.md).
- [ ] Validate resulting device behavior on dedicated hardware.

Priority: P1. The incorrect request is reproduced; connectivity effects depend on firmware acceptance. This additional finding is fixed in the pushed E commit and does not change the original 14-finding baseline record.

## Existing upstream PRs

Preserve the earlier [open-PR plan](open-pull-requests.md) and its recorded checks. Metadata was refreshed on 2026-10-19: upstream `main` is still `6bc71b3`; #23 remains `2ec46a4` with no posted checks; #20 at `e54b759` has successful Lint/Build/Unit Tests; #18 at `272ef99` still reports failed Lint/Build and skipped Unit Tests. These are reported check states, not new CI executions.

- [ ] #18: align `.go-version` with the dependency upgrade's Go minimum and inspect fresh CI before merging. Do not attribute its old failed checks to an unverified cause.
- [ ] #20: verify setup-go compatibility and the release path independently of ordinary CI.
- [x] #23: verify B against stored head `2ec46a4`; the combined patch applies cleanly and port-preservation plus both operational-speed acceptance tests pass. Keep the speed-schema change and migration note in #23's scope.
- [ ] Repair invalid Dependabot labels in a separate maintenance change or maintainer task.
- [ ] Recheck current upstream heads, overlaps, and CI immediately before publishing/rebasing each fix.

## Review package required for each PR

- [ ] Small diff with a specific conventional-commit title and explicit scope.
- [ ] A failing regression on the recorded base and the same regression passing on the patch.
- [ ] Explain the triggering configuration, root cause, changed behavior, and why the test exercises it.
- [ ] Record exact base/head revisions when committed, Go/Terraform versions, container image, commands, and results.
- [ ] Include schema/state/import compatibility, migration notes, and generated documentation changes where needed.
- [ ] Separate mock evidence from actual controller validation; leave the template's real-controller checkbox unchecked until performed.
- [ ] Complete formatting, vet, build, unit tests, and relevant acceptance/race gates without changing unrelated files.
- [ ] Review error/cancellation/read-only paths and unintended side effects.
- [ ] Verify attribution requirements again at submission time; use actual session metadata if attribution is needed, never guessed versions.
- [ ] Link real upstream issue numbers only after verifying them. F01–F16 are local investigation IDs, not GitHub issues.
- [ ] Record PR URL, CI run, review requests, follow-up comments, and merge commit when those events occur.

## Validation ledger

| Work | Base/head | Verification | Result / limitation |
| --- | --- | --- | --- |
| Investigation | Base `6bc71b3` | Go 1.25.8 containers, Terraform 1.15.8 mock acceptance, race detector | 14 reproduced defects; baseline suite passes; no live controller |
| A | Base `6bc71b3`; head `52ed681`; `fix/client-authentication`, pushed to fork | Go 1.25.6: formatting, vet, build, full unit suite, targeted race regressions | Pass; committed file hashes match tested files; same regressions fail on baseline; no live controller; [logs and scope](../investigations/authentication-fix/README.md) |
| B | Base `6bc71b3`; head `1c7f535`; `fix/switch-port-updates`, pushed to fork | Go 1.25.6 full gates; 13 lifecycle cases; Terraform 1.15.8 standard/ES acceptance; combined #23 acceptance; live ES216G acceptance on the F15 stack | Pass; committed file hashes match tested files; live read/write acceptance is limited to ES216G v1.20 firmware `1.0.1 Build 20260329 Rel.14489` on Controller 6.1.0.19; [mock evidence](../investigations/switch-port-fix/README.md) and [live acceptance](es216g-terraform-acceptance.md) |
| C | Base `6bc71b3`; head `9f4b4f8`; `fix/ap-radio-updates`, pushed to fork | Go 1.25.6 full gates; 14 lifecycle cases; Terraform 1.15.8 dual/default, dual/nondefault, and single-band acceptance; A + C full unit/acceptance checks | Pass on 2026-10-19; committed file hashes match tested files; no live AP; removed defaults documented; [evidence](../investigations/ap-radio-fix/README.md) |
| D1 | Base `6bc71b3`; head `80991a5`; `fix/ssid-update-preservation`, pushed to fork | Go 1.25.6 full gates; 47 lifecycle/validation cases; eight Terraform 1.15.8 scenarios; A + D1 full unit/acceptance checks | Pass on 2026-10-19; matching host/container hashes; baseline 42 update cases and five import scenarios fail; no hardware; [evidence](../investigations/ssid-fix/README.md) |
| D2 | Base D1 `80991a5`; `fix/ssid-vlan-state`, `0bbd076` | Go 1.25.6 full gates; 95 lifecycle cases; three D2 Terraform scenarios plus all eight D1 scenarios; A + D1 + D2 unit/acceptance checks | Pass on 2026-10-19; host/container hashes match; D1 baseline has 59 lifecycle failures and all three new Terraform failures; no hardware; [evidence](../investigations/ssid-vlan-fix/README.md) |
| D3 | Base D2 `0bbd076`; `fix/ssid-group-identity`, `ec2d949`, pushed to fork | Go 1.25.6 full gates; 21 direct cases; 13 D3 Terraform scenarios plus all 11 D1/D2 scenarios; A + D1 + D2 + D3 unit/acceptance checks | Pass on 2026-10-19; seven host/container hashes match; D2 baseline has 10 direct-case and nine new Terraform failures; no hardware; [evidence](../investigations/ssid-group-fix/README.md) |
| E | Base A `52ed681`; `fix/site-credentials`, `8f24c37`, pushed to fork | Go 1.25.6 full gates; 48 direct cases; 12 Terraform 1.15.8 scenarios/65 steps; A + D1–D3 + E full unit/36 scenarios/185 steps | Pass on 2026-10-19; 15 host/container hashes match; A baseline has 44 direct-case and all 12 Terraform-scenario failures; no hardware; [evidence](../investigations/site-credentials-fix/README.md) |
| F | Base E `8f24c37`; `fix/http-responses`, `c210dec`, pushed to fork | Go 1.25.6 full/race gates; 214 behavior + nine typed/read-bound cases; Terraform 1.15.8 five F + 12 E + 24 D scenarios/205 steps | Pass on 2026-10-19; nine host/container hashes match; baseline has 149/214 behavioral failures and all five Terraform failures; combined D tests need the recorded six diagnostic assertion changes; no hardware; [evidence](../investigations/http-response-fix/README.md) |
| G1 | Base F `c210dec`; `fix/complete-pagination`, pushed as [9343503](https://github.com/wbniv/terraform-provider-tplink-omada/commit/9343503d13a5428e91d121e0104ce7c36cd93cd6) | Go 1.25.6 full/race gates; 94 client cases; three Terraform scenarios/11 steps; combined 44 scenarios/216 steps | Pass on 2026-10-19; seven host/container hashes match; baseline 69 client failures and all three Terraform failures; recorded D client integration and fixture patches; no real controller; [evidence](../investigations/pagination-fix/README.md) |

Keep the baseline evidence unchanged. Add patch evidence separately, and distinguish implemented, locally verified, controller-verified, submitted, and merged states.
