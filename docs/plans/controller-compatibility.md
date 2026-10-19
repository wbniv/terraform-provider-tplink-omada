# Controller compatibility follow-ups

Prepared for Will on 2026-10-19 after running the real Software Controller 6.1.0.19. Parent: [reliability tracker](provider-reliability-prs.md). Evidence: [software-controller lab](../investigations/software-controller-lab/README.md). These labels supplement the historical F01–F16 report; they do not rewrite its baseline.

## Order and review boundaries

### L1 — WLAN groups are an unpaginated object envelope

Collection correction implemented; live SSID follow-ups remain: [L1 evidence](../investigations/wlan-collection-fix/README.md).

- [x] Reproduce on G1: `/setting/wlans` returns the same `data` on pages 1 and 2, without pagination metadata. IDs are valid. G1 errors on the duplicate second page.
- [x] Add an explicit per-endpoint collection policy for known unpaginated envelopes. Do not treat arbitrary missing totals, short pages, or repeated rows as proof of completion.
- [x] Retain strict paginated behavior for other collections; paginated mock regressions and the real 102-network case pass.
- [ ] Test populated variants of collections where only empty real responses have been observed.
- [x] Add an envelope fixture and verify default/custom lookup; rerun D mock scenarios and real SSID probes.
- [x] Verify real secured SSID create, import, group replacement, and stable plan.
- [x] Verify live secured SSID rename, password rotation, and custom/default VLAN transitions with L7.
- [x] Verify L6 open PMF omission and open↔secured transitions on 6.1.0.19.
- [x] Rerun G1 malformed-page/failure gates and the real 102-network test.
- [x] Commit the focused L1 correction as `0730101` on `fix/wlan-group-collection`.

### L2 — Certificate wire format differs from the mocks

Locally verified on `fix/controller-certificate-protocol`, based on the verified lab stack (`c026124`); committed as `4c21666`. [L2 evidence](../investigations/certificate-protocol-fix/README.md). Five Terraform lifecycle steps, client activation/readback, Go gates, and exact TLS fingerprint after restart pass on 6.1.0.19.

- [x] Reproduce upload rejection `-30021` using the provider's multipart `data={}`.
- [x] Verify that UI-style `cerName` / `keyName` metadata succeeds and returns `cerId` / `keyId`, with no generic `id`.
- [x] Correct metadata and decode endpoint-specific identities; reject successful envelopes with missing/empty IDs.
- [x] Verify activation fields, readback, restart requirements, and actual TLS presentation. An accepted upload alone is insufficient.
- [x] Retain A's deadlock/retry/concurrency protections and F's status/body protections; rerun their gates plus real certificate lifecycle.

### L3 — Lost-session recovery

Locally verified on `fix/controller-session-recovery`, based on L2 (`4c21666`); committed as `58610fd`. [L3 evidence](../investigations/session-recovery-fix/README.md). Six live recovery cases, client race detector, Go gates, and five Terraform certificate steps pass.

- [x] Invalidate the client's local cookie/token and observe HTTP 302 on the protected request; the client returns an HTTP error rather than reauthenticating.
- [x] Capture the exact same-origin login redirect for lost cookies, invalid CSRF tokens, and logout. The corrected probe disables redirect following for every opener.
- [ ] Measure natural session expiry separately; logout is not evidence of the timeout behavior.
- [x] Specify a narrow reauthentication signal for supported controllers. Keep arbitrary/cross-origin redirects rejected, bound retries, and never send credentials or tokens to redirect destinations.
- [x] Test failure state, concurrent requests, and multipart replay. Verify against the real controller; this probe did not wait for natural session expiry.

### L4 — IP-group endpoint mismatch

Locally verified on `fix/ip-group-protocol`, based on L3 (`58610fd`); committed as `a2b1baa`. [L4 evidence](../investigations/ip-group-fix/README.md). Five live Terraform steps, three mock failure-recovery steps, Go/race gates, all 16 supported groups, and disabled ACL references pass. The schema now requires identical nonempty port lists on every entry.

- [x] Observe provider route `/setting/firewall/ipGroups` rejected with `-1600`.
- [x] Locate `/setting/profiles/groups` and `/setting/profiles/groups/0` in the bundled UI and verify successful real responses.
- [x] Inventory group types, identities, payloads, protected defaults, and pagination semantics before choosing a replacement route.
- [x] Test complete IP-group CRUD/import and disabled ACL references on 6.1.0.19; traffic enforcement and other versions remain separate gates. ACL creation response handling is tracked as L8.

### L8 — ACL creation acknowledges success without an identity

Discovered during L4 on 6.1.0.19: disabled gateway ACL creation succeeds and stores valid IP-group references, but returns no `result`. `CreateACLRule` currently decodes that empty result and fails. [L4 evidence](../investigations/ip-group-fix/README.md).

- [x] Reproduce successful raw creation, readback of both group references, and deletion on the disposable controller.
- [ ] Capture the creation response contract and choose a complete, unique readback strategy for the new rule; reject missing or ambiguous matches.
- [ ] Cover accepted writes followed by failed readback without inventing an ID or blindly repeating a mutation; make uncertain outcomes explicit and document recovery.
- [ ] Verify Terraform ACL create, stable plan, update, import, and destroy with synthetic group references; retain rule identity/order and unrelated fields.
- [ ] Keep actual traffic enforcement and other controller versions as separate compatibility gates.

### L5 — Empty device inventory becomes Terraform null

- [x] Observe the real devices endpoint returning `[]` and successful client decoding to zero devices.
- [x] Locate the uninitialized nil slice in the data source and the failing existing acceptance assertion.
- [ ] Initialize empty collections consistently and test `length(data.omada_devices.example.devices) == 0` on a controller without devices.
- [ ] Audit other collection data sources for the same nil-to-null behavior; retain populated-list coverage.

### L6 — Open SSID PMF defaults disagree with the controller

Implementation and validation: [L6 investigation](../investigations/ssid-pmf-fix/README.md).

- [x] After L1 unblocked discovery, reproduce an inconsistent apply: `security = 0` with the provider default `pmf_mode = 2` is read back as `0` by 6.1.0.19.
- [x] Inspect pinned 6.1 UI: open requests omit PMF; secured values are 1 required, 2 capable/optional, 3 disabled. Real open creation returns PMF 0 in Terraform; the raw response may omit the field.
- [x] Pass the nine-step live PMF matrix: open create/rename/stable plan, open→secured, secured modes 1/2/3, secured→open, final stable plan.
- [x] Define open mode 0 and omission on the wire, secured default 2 on create/transition, preservation on other secured updates, unknown-security deferral, and diagnostics for incompatible explicit values. Document the corrected enum descriptions without remapping stored numbers.
- [x] Add PMF validation and unknown-security creation coverage; retain import/replacement and preservation gates. Live PMF and existing secured lifecycle scenarios pass.

An explicit `pmf_mode = 0` create was rejected with API `-1001`. The L1 lifecycle used a secured SSID with mode 2. L6 now omits PMF for open requests and accepts absent open-response metadata; the complete live matrix passes.

### L7 — Real SSID fields exceed the D1 writable contract

Implementation and validation: [L7 investigation](../investigations/ssid-controller-fields/README.md).

- [x] A secured SSID is created, but rename fails with “unsupported SSID field in SSID”; the fail-closed D1 check prevents the PATCH.
- [x] Capture default/custom VLAN response fixtures and compare the bundled controller UI serializers.
- [x] Exclude response summaries `vlanEnable`/top-level `vlanId` and create-only `deviceType` from PATCH; keep `vlanSetting` and existing unknown-field rejection.
- [x] Add default/custom real-response fixtures and pass secured rename, password rotation, VLAN assignment/clearing, import, replacement, and stable-plan checks on 6.1.0.19.
- [x] Verify open/secured transitions with L6. Physical-device validation remains separate.

## Review screens and flow

The first terminal screen below records the baseline; current L1 results are in the linked investigation.

```text
$ dev/controller-lab/run.sh up
Controller: 6.1.0.19       Target: https://127.0.0.1:18043
Account: local lab        Adopted devices: 0
Archive: verified        Application JARs: 533 matched

$ dev/controller-lab/run.sh test
Go vet / unit / build: PASS
Terraform: 9 PASS, 3 FAIL
Open: WLAN discovery; empty-device assertion; length(null)
Action: fix L1 first; retain failures as regression evidence
```

```text
L1 review after the proposed correction
  WLAN group discovery      PASS: one complete unpaginated response
  SSID lifecycle            must pass on the real controller
  Network inventory         PASS required: 102 rows; last ID found
  Repeated paged identities  ERROR required: no partial success

Review action: inspect diff + logs, then prepare the focused commit
Next action: G2 only after complete discovery is verified
```

| Check | Current review result | Required result |
| --- | --- | --- |
| WLAN data-source plan | Discovery fixed; empty-list assertion now exposed | Read complete known-unpaginated group list, then list SSIDs |
| Certificate apply | L2 verified: nested PEM activation, stored JKS IDs, explicit restart, exact TLS match | Nonempty certificate/key IDs; verified activation and TLS presentation |
| Refresh after lost session | L3 verified for 6.1.0.19: one shared refresh, bounded replay, no redirect following | One bounded, verified reauthentication attempt or an explicit unsupported result |
| IP-group plan | API `-1600` | Correct typed group inventory from a verified route |
| Empty device count | Null list in Terraform | Empty list, count zero |

```mermaid
flowchart TD
    A[Reproduce against pinned controller] --> B[Capture sanitized response contract]
    B --> C[Add regression fixture and focused fix]
    C --> D[Run Go and Terraform mock gates]
    D --> E[Run real-controller lifecycle and stable plan]
    E --> F{All claimed operations pass?}
    F -->|No| B
    F -->|Yes| G[Update evidence and prepare focused commit]
    G --> H[L1 complete: proceed to G2 disappearance]
```

## Publication gates

- [ ] Keep each compatibility correction separately reviewable; the lab/vendoring baseline records failures; `fix/wlan-group-collection` contains the L1 production correction.
- [ ] Recheck upstream heads and contribution requirements before opening PRs.
- [ ] Declare exact controller/version/test coverage. Physical AP/switch behavior remains unverified.
- [ ] Publish upstream only when authorized; fork commits and pushes are already authorized for this work.
