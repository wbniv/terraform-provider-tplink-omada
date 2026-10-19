# Real Software Controller 6.1.0.19 validation

Investigation for Will, 2026-10-19. Implementation: `test/software-controller-lab`, based on G1 `9343503` (including A/E/F). Terraform runs additionally integrate D1–D3 using the recorded patches. [Lab plan](../../plans/software-controller-lab.md); [focused follow-up plans](../../plans/controller-compatibility.md).

## Result

The real controller runs without adopted devices and supports useful API/CRUD validation. The first existing acceptance run passes **8 of 10 scenarios**, including network create/update/import/destroy and port-profile create/update/destroy. A separate four-step Terraform site/credential scenario passes bootstrap, settings ownership, two accepted credential writes, preservation of selected unrelated settings, and a stable final plan. A client probe creates **101 temporary VLAN networks**, lists **102 total networks**, resolves the final fixture, and restores the original inventory.

The suite also exposes real compatibility failures. G1 is **not ready for an upstream compatibility claim**: WLAN groups are returned in an unpaginated `data` envelope. Certificate upload, session recovery, IP-group routing, and empty-device state have separate failures. The lab branch retains failing probes; it does not claim to fix these production paths.

## Reproduction and isolation

Harness: [lab guide](https://github.com/wbniv/terraform-provider-tplink-omada/blob/test/software-controller-lab/dev/controller-lab/README.md). From that checkout:

```sh
git lfs pull
dev/controller-lab/run.sh up
LAB_SSID_INTEGRATION=1 \
LAB_EVIDENCE_DIR=/home/will/terraform-provider-tplink-omada/docs/investigations \
dev/controller-lab/run.sh test
dev/controller-lab/run.sh down
```

Controller/relay project: `omada-provider-lab`. Controller: internal Docker bridge only; relay: fixed destination `controller:8043`, exposed solely on `127.0.0.1:18043`. No discovery/adoption ports, cloud account, production controller, or physical device is used. Local credentials and state are private ignored runtime data.

Final harness compilation and execution use the internal network, checked-in Go dependencies, `GOPROXY=off`, `GOSUMDB=off`, and `GOTOOLCHAIN=local`. Go is **1.25.6**; Terraform is **1.15.8**. The source checkout is read-only; the writable build lives in the disposable runner. Initial exploratory compilation downloaded modules before disconnecting the runner's normal bridge; distinguish it from the finished vendored harness.

Combined source recipe: G1 plus D's diff from `6bc71b3` to `ec2d949`, excluding `internal/client/client.go`; then `pagination-fix/ssid-client-integration.patch`, `http-response-fix/ssid-diagnostic-assertions.patch`, and `pagination-fix/ssid-pagination-fixtures.patch`. The raw SSID decoder is G1's implementation. Original D worktrees remain unchanged.

## Controller provenance and vendoring

| Artifact | Identity |
| --- | --- |
| Official distribution | [TP-Link Linux x64 archive](https://static.tp-link.com/upload/software/2026/202601/20260121/Omada_Network_Application_v6.1.0.19_linux_x64_20260117100056.tar.gz), 307,699,260 bytes |
| Archive SHA-256 | `885eb2aeaed16b300e618297af2132d4e015c7f9bc5edce2ba81b8cdb181e126` |
| Controller image | `mbentley/omada-controller:6.1.0.19@sha256:94571a6544a5db9419681d3cecdaa7dccf9042e586cae9a1c34c3f22994e6b38` |
| Packaging | [Community image source](https://github.com/mbentley/docker-omada-controller), not an official TP-Link container |
| Application comparison | All **533 JARs** exactly match the official distribution; no missing, extra, or different JARs |
| Runtime | OpenJDK **17.0.20.1**, MongoDB **8.0.32** |
| Go image | `golang:1.25.6@sha256:06d1251c59a75761ce4ebc8b299030576233d7437c886a68b43464bad62d4bb1` |
| Relay image | `python:3.14-slim@sha256:a2b82f3c48559aa0a8446d9af49826b6e2b2016f4cd2afabfe6013ec53729170` |

Will explicitly selected **Git LFS** for the controller binary. The unmodified official archive is stored under `third_party/omada/`, with `.gitattributes`, a manifest, and a verification script. Go dependencies are separately vendored under `vendor/` without changing dependency versions. Both the Git branch and LFS object are uploaded. A fresh download into an empty LFS cache matches the archive size and SHA-256 exactly. Runtime images remain pinned external artifacts; an offline machine needs those cached/exported images and Terraform as documented in the vendoring guide.

## Findings

| Follow-up | Evidence | Consequence / next check |
| --- | --- | --- |
| **L1: unpaginated WLAN-group envelope** | Pages 1 and 2 return the same valid group ID under `data`, without `totalRows/currentPage/currentSize`; G1 errors on duplicates | Introduce explicit endpoint pagination policy; retain rejection of repeated pages on genuinely paged routes. D's live SSID validation is blocked at group discovery |
| **L2: certificate upload protocol** | Public upload sends `data={}` and gets `-30021`; UI-style metadata `cerName` / `keyName` succeeds, returning `cerId` / `keyId`, with no generic `id` | Repair metadata and identity decoding; then verify activation/readback/TLS. A's deadlock repair remains a valid fix but does not establish certificate functionality |
| **L3: lost-session recovery** | Clearing the local cookie jar and invalidating its token makes the protected request return HTTP 302; client returns an error | Investigate exact login redirect/session signals without permitting arbitrary redirects. Natural timeout was not exercised |
| **L4: IP-group route** | `/setting/firewall/ipGroups` returns `-1600`; UI routes `/setting/profiles/groups` and `/setting/profiles/groups/0` succeed | Audit complete group contract/types/CRUD before replacing the route |
| **L5: empty devices become null** | API returns `[]`; client lists zero devices; data source leaves its Go slice nil and the existing Terraform assertion fails | Initialize empty collections and test count/iteration with zero devices. This is a provider state issue, not a requirement to buy hardware |

The initial commentary described the WLAN error as an identity-field problem. Inspection corrected that: the IDs are valid; the duplicate second page is the problem. The initial empty-device explanation was also too broad: the nil slice produces null rather than an empty Terraform list.

## Response observations and limits

- Networks and profiles return pagination metadata. A real 102-network inventory completes successfully. The small-page probe shows profiles honoring page 2. Some empty or out-of-range requests clamp `currentPage` to 1; G1 stops at the stated total and therefore does not request those pages during normal complete reads.
- WLAN groups ignore page number and return their complete `data` envelope with capability fields. SSID empty responses have explicit zero totals; populated SSID behavior still needs live validation after L1.
- Devices use a direct array. Empty ACL/mDNS/IdP/role responses succeed, but empty lists do not establish all populated CRUD behaviors or page limits.
- Certificate diagnostic uploads were removed. No successful provider certificate activation/TLS claim is made.
- Initial synthetic passwords were rejected by site creation with `-30105`; the wizard's accepted generated credential succeeds. Those initial failures are fixture diagnostics, not a provider-create defect. A name-only raw site PATCH is also rejected; the provider normally sends the full site fields. Do not generalize that exploratory request into a resource failure.
- Controller acceptance of stored device credentials does not prove that a physical AP/switch received them.
- The complete site lifecycle passes when the update supplies its full site fields. After successful deletion, both a direct GET and repeated DELETE return API `-1505`. This is measured evidence for this endpoint/version, not a global not-found mapping.

## Final vendored run

- [x] `go vet -mod=vendor ./...`, `go test -mod=vendor ./...`, provider build, and both test-binary builds pass with external network access disabled. [Build log and exact source hashes](build.log).
- [x] Live Terraform: **9 passed / 3 failed** of 12 scenarios. Failures: WLAN discovery, existing empty-device assertion, and the new `length(null)` reproduction. Credential rotation and stable planning pass. [Acceptance log](acceptance.log).
- [x] Live client: **3 passed / 3 failed** top-level probes. Passing: 102-network listing/cleanup, full site lifecycle, and the diagnostic UI-format certificate uploads. Failing: collection parent (WLAN/IP groups), session recovery, and provider certificate upload. Collection children: 8 pass / 2 fail; both certificate diagnostic children pass. [Client log](client-probes.log).
- [x] Keep nonzero suite exit statuses visible: [test status](test-status.txt). These are recorded compatibility failures, not waived assertions.
- [x] Preserve [page metadata/UI-route comparison](collection-probe-pages.jsonl), [package/JAR provenance](provenance.json), and [harness source hashes](source.sha256). No credentials, private keys, cookies, raw settings bodies, or Terraform state are included.

## Repository migration

GitHub rejected new LFS objects in the public fork. Will then requested recreating the **whole repository** as a standalone repository so code and binaries can live together. The original fork is preserved under `wbniv/terraform-provider-tplink-omada-upstream-fork`, and `wbniv/terraform-provider-tplink-omada` has been recreated as a standalone public repository. [Original refs](repository-before.refs) contain 14 branches and nine tags. All original refs match exactly; see [migration verification](repository-migration.json). Git history used the existing authenticated SSH identity because the HTTPS OAuth credential lacked workflow scope. An optional LFS-lock check encountered a temporary DNS timeout; it was disabled only for the historical mirror push, whose refs contain no LFS files. The Omada LFS upload subsequently completed successfully; a fresh remote retrieval verifies all 307,699,260 bytes and its SHA-256. Actions were disabled during historical ref restoration and are enabled again. No separate artifact repository was created.

## Evidence and completion

- [x] Preserve a repeatable harness with explicit opt-in mutation guards and cleanup.
- [x] Verify the vendored official archive and all controller application JARs.
- [x] Record independent findings and a plan for each before production corrections.
- [x] Retain final vendored-build/test logs and exact counts above.
- [x] Stop disposable services and remove owned volumes, credentials, and exploratory files after preserving evidence; verify no lab containers/volumes remain.
- [x] Commit/push the lab and vendor artifacts, with Git and LFS verified separately. This documentation is published on `docs/provider-reliability-review`.
- [ ] Complete L1–L5 and then resume G2. Physical-device and upstream-submission gates remain open.

Local implementation commits: Go vendor `5555332a65a4b52761c81b67d7b767e377f70fd3`; lab and LFS archive pointer `0d26e723ce833a44c2043b6be23e69282b189a35`. Production source and `go.mod`/`go.sum` are unchanged from G1. The workspace Markdown-history hook adds `.history/README.md` sidecars; these are retained separately from upstream module contents.

Diagnostic follow-up `c953da6ed94021d7774e26e76e55bb1ef2d7b8fc` excludes missing identity fields from page-overlap counts and reports identified rows. The retained page log predates this correction: its IP-group `repeated_ids` values collapse absent `id`/`idpId` fields and **must not be used as evidence of repeated IP-group identities**. Route success and row counts remain observations; L4 still needs the actual identity/type contract. The WLAN records have valid IDs and are unaffected. `source.sha256` records the corrected harness; the full Go/Terraform test source hashes remain in `build.log`.

## L1 follow-up

The [WLAN collection correction](../wlan-collection-fix/README.md) verifies real discovery, 102-network pagination, and secured SSID create/import/group replacement/stable plan. Rename remains blocked by the D1 writable-field contract (L7); open-network PMF behavior is tracked as L6. The baseline failures above remain a historical record.
