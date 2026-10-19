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
- [ ] Finish open-network PMF compatibility under L6.
- [x] Rerun G1 malformed-page/failure gates and the real 102-network test.
- [x] Commit the focused L1 correction as `0730101` on `fix/wlan-group-collection`.

### L2 — Certificate wire format differs from the mocks

- [x] Reproduce upload rejection `-30021` using the provider's multipart `data={}`.
- [x] Verify that UI-style `cerName` / `keyName` metadata succeeds and returns `cerId` / `keyId`, with no generic `id`.
- [ ] Correct metadata and decode endpoint-specific identities; reject successful envelopes with missing/empty IDs.
- [ ] Verify activation fields, readback, restart requirements, and actual TLS presentation. An accepted upload alone is insufficient.
- [ ] Retain A's deadlock/retry/concurrency protections and F's status/body protections; rerun their gates plus real certificate lifecycle.

### L3 — Lost-session recovery

- [x] Invalidate the client's local cookie/token and observe HTTP 302 on the protected request; the client returns an HTTP error rather than reauthenticating.
- [ ] Capture the redirect's exact same-origin target and compare lost-cookie, invalid-CSRF-token, logout, and naturally expired-session responses.
- [ ] Specify a narrow reauthentication signal for supported controllers. Keep arbitrary/cross-origin redirects rejected, bound retries, and never send credentials or tokens to redirect destinations.
- [ ] Test failure state, concurrent requests, and multipart replay. Verify against the real controller; this probe did not wait for natural session expiry.

### L4 — IP-group endpoint mismatch

- [x] Observe provider route `/setting/firewall/ipGroups` rejected with `-1600`.
- [x] Locate `/setting/profiles/groups` and `/setting/profiles/groups/0` in the bundled UI and verify successful real responses.
- [ ] Inventory group types, identities, payloads, protected defaults, and pagination semantics before choosing a replacement route.
- [ ] Test complete IP-group CRUD/import and ACL references; retain compatibility declarations per supported controller version.

### L5 — Empty device inventory becomes Terraform null

- [x] Observe the real devices endpoint returning `[]` and successful client decoding to zero devices.
- [x] Locate the uninitialized nil slice in the data source and the failing existing acceptance assertion.
- [ ] Initialize empty collections consistently and test `length(data.omada_devices.example.devices) == 0` on a controller without devices.
- [ ] Audit other collection data sources for the same nil-to-null behavior; retain populated-list coverage.

### L6 — Open SSID PMF defaults disagree with the controller

- [x] After L1 unblocked discovery, reproduce an inconsistent apply: `security = 0` with the provider default `pmf_mode = 2` is read back as `0` by 6.1.0.19.
- [ ] Verify PMF/security combinations against controller UI and API, including secured SSIDs; do not infer enum meanings from mocks.
- [ ] Define compatible defaults and validation, preserving explicit user intent and documenting any migration.
- [ ] Add mock and live create/update/import/stable-plan gates for the supported combinations.

An explicit `pmf_mode = 0` create was rejected with API `-1001`. The final L1 lifecycle uses a secured SSID with mode 2; the open-network mismatch remains unresolved.

### L7 — Real SSID fields exceed the D1 writable contract

Implementation and validation: [L7 investigation](../investigations/ssid-controller-fields/README.md).

- [x] A secured SSID is created, but rename fails with “unsupported SSID field in SSID”; the fail-closed D1 check prevents the PATCH.
- [x] Capture default/custom VLAN response fixtures and compare the bundled controller UI serializers.
- [x] Exclude response summaries `vlanEnable`/top-level `vlanId` and create-only `deviceType` from PATCH; keep `vlanSetting` and existing unknown-field rejection.
- [x] Add default/custom real-response fixtures and pass secured rename, password rotation, VLAN assignment/clearing, import, replacement, and stable-plan checks on 6.1.0.19.
- [ ] Open/secured transitions remain gated by L6; physical-device validation remains separate.

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
| Certificate apply | Upload rejected before activation | Nonempty certificate/key IDs; verified activation and TLS presentation |
| Refresh after lost session | HTTP 302 diagnostic | One bounded, verified reauthentication attempt or an explicit unsupported result |
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
