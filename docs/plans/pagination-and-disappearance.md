# G — Complete pagination and confirmed disappearance

Prepared for Will on 2026-10-19. Parent: [reliability tracker](provider-reliability-prs.md). Findings: F09 and F10 in the [baseline investigation](../investigations/2026-10-18-code-review.md). F is committed/pushed as `c210dec`; G1 starts on that commit in `/home/will/terraform-provider-tplink-omada-pagination`, branch `fix/complete-pagination`.

## Review boundaries

- [x] **G1 / F09: complete collection reads.** Iterate pages in every affected list method and raw SSID lookup. Verify the entire response before returning anything to callers. Keep the public resource schemas and existing absence behavior unchanged.
- [x] **G2 / F10: confirmed disappearance.** Add a semantic absence type for successful complete lookups, audit supported Read/Delete paths, and remove state only with positive evidence of absence. Direct endpoint HTTP 404, permission errors, incomplete lists, and decoding failures retain state. See the G2 completion record below.

Splitting G avoids coupling pagination changes to automatic Terraform state removal. G1 is implemented and locally verified, pushed as [9343503](https://github.com/wbniv/terraform-provider-tplink-omada/commit/9343503d13a5428e91d121e0104ce7c36cd93cd6). G2 is implemented in a separate patch. The [software-controller lab](software-controller-lab.md) supplies real API evidence, including 102 networks; the unpaginated WLAN-group compatibility follow-up is tracked under [L1](controller-compatibility.md). Mocks remain necessary for injected failures.

## G1 collection inventory and policy

| Collection | Scope / API route | Additional requirement |
| --- | --- | --- |
| Sites | Global `/sites` | Site name/ID resolution must reach later pages |
| Networks | Site `/setting/lan/networks` | Resource lookup and network data source |
| WLAN groups | Site `/setting/wlans` | Group lookup and default selection keep existing semantics |
| SSIDs | Site/group `/setting/wlans/{id}/ssids` | Typed list and raw PATCH lookup both complete; preserve unknown JSON fields |
| Port profiles | Site `/setting/lan/profiles` | Lookup and data source |
| ACL rules | Site `/setting/firewall/acls` | Preserve `type` filter on every request |
| IP groups | Site `/setting/firewall/ipGroups` | Lookup and data source |
| mDNS rules | Site `/setting/service/mdns` | Accept pagination metadata alongside capability fields |
| SAML IdPs | Global `/idps` | Identity is `idpId` in the wire response; name/create readback inherits complete listing |
| SAML roles | Global `/extendUserGroups` | Lookup and create readback inherit complete listing |
| Devices | Site `/devices` | Existing direct-array endpoint; verify unchanged behavior, do not invent paging support |

- [x] Request pages beginning at 1 with size 100, preserving route/filter parameters and refreshed authentication tokens.
- [x] Accept a direct array as the complete unpaginated response on the first request. Reject a change from paginated objects to direct arrays midway.
- [x] For object responses, require an array `data`. Validate present metadata; distinguish absent fields from zero/null. Use stable `totalRows` when supplied; otherwise request until an empty page. Never infer completion merely from a short page because the server may cap page size.
- [x] Fail with no partial results for later-page errors, early empty pages before a stated total, changed totals, mismatched page numbers, duplicate identities, malformed rows/metadata, and excessive page count or cumulative response size (10,000 pages / 64 MiB of result JSON). Preserve cancellation and F's safe diagnostics.
- [x] Keep result order and raw SSID attributes. A list with concurrent mutations can fail and be retried; no snapshot guarantee is possible without controller support.
- [x] Correct existing synthetic fixture counts if strict validation exposes inconsistent totals; document these fixture changes explicitly.

## Concrete Terraform review screens

| Screen / action | Expected visible result | Subsequent action |
| --- | --- | --- |
| `terraform plan` importing network 101 | Existing network recognized; import plan uses its actual configuration | Apply import, then verify an empty plan |
| Network data source with 201 entries | All 201 entries, including the last page | Verify ordered IDs and count |
| Refresh: page 2 returns HTTP 503 | Error with HTTP status; managed network identity remains in state | Restore controller and retry refresh |
| Refresh: pagination repeats an identity | Fixed invalid-response diagnostic; no partial success | Retry after controller consistency is restored |
| G2 refresh: complete inventory excludes managed ID | Refresh records disappearance; normal plan proposes creation | Review and apply recreation, then verify stable state |
| G2 refresh: authorization fails or route returns 404 | Diagnostic; no automatic state removal | Correct authorization/route and retry |

```mermaid
flowchart TD
    A[Request page 1] --> B{Valid response?}
    B -->|No| X[Error; return no partial collection]
    B -->|Direct array| C[Validate identities; return collection]
    B -->|Page object| D[Validate metadata, rows and identity uniqueness]
    D -->|Invalid or changed| X
    D --> E{Stated total reached or no total and empty page?}
    E -->|No| F[Request next page with same scope and filters]
    F --> B
    E -->|Yes| G[Return complete collection]
    G --> H[G2: find managed identity]
    H -->|Present| I[Refresh current values]
    H -->|Absent| J[Typed confirmed absence; resource Read removes state]
```

## Agenda and gates

### 2026-10-19 — G1 implementation

- [x] Create isolated worktree based on F; inventory all fixed-page calls and direct-array devices.
- [x] Recheck contribution requirements: conventional commits, format/vet/tests, documentation, real-controller declaration. No explicit AI attribution rule found.
- [x] Write maintained regressions and run the same behavioral tests against F baseline and G1.
- [x] Cover all ten collections plus raw SSID lookup, direct arrays, more than 100 rows, empty collections, capped pages, omitted metadata, changed totals, invalid rows, repeated pages, cancellation, and errors after a partial read.
- [x] Add Terraform import/data-source/failed-refresh recovery evidence with the managed object beyond page 1.
- [x] Run Go 1.25.6 fmt/vet/build/full unit/race and Terraform 1.15.8 tests in Docker. Save exact commands, logs, counts, and hashes under `docs/investigations/pagination-fix/`.
- [x] Check D1–D3 integration, including the existing F diagnostic assertion patch; preserve other worktrees.
- [x] Update this plan, tracker, and investigation; remove disposable test services and files.

### G2 scope and completion

- [x] Audit resource Read/Delete behavior and apply confirmed-absence handling to the nine collection-backed resource types with complete discovery.
- [x] Keep direct/singleton endpoint resources outside automatic state removal where endpoint failures do not independently prove absence; direct HTTP 404 and other errors retain state.
- [x] Cover confirmed-absence state removal, failure retention, idempotent already-absent Delete, and out-of-band deletion/recreation in client and Terraform tests.
- [x] Validate recreation and stable follow-up planning against the isolated controller lab.

### 2026-10-27 — G2 implementation started

Branch `fix/confirmed-disappearance` is based on verified G1 `9343503`; implementation commits are `20dfdd1` and `48e2aad`, with mock regressions in `3d8bb18`. The patch adds typed `client.ErrNotFound` only to successful complete-collection lookups, removes resource state on that typed result for networks, WLAN groups, SSIDs, port profiles, ACLs, IP groups, mDNS rules, SAML IdPs, and SAML roles, and preflights those same resources before Delete so confirmed-absent objects are already deleted. Direct endpoint errors (including HTTP 404), transport/authentication failures, and incomplete collection errors remain diagnostics and retain state. Site, site settings, AP/switch configuration, and controller certificates use direct or singleton endpoints and do not infer absence from endpoint failure. Go 1.25.8 formatting, the full `go test ./...` suite, focused client absence/error tests, and all five `TestAccPagination` mock Terraform acceptance scenarios pass. They cover later-page failure retaining state, out-of-band deletion followed by a recreation plan, and already-absent Delete preflight with no redundant DELETE request. The opt-in live test also passes against isolated Omada Software Controller 6.1.0.19: out-of-band network deletion, recreation plan/apply, controller readback, and a stable follow-up plan.

- [x] Add the implementation on an isolated branch based on G1.
- [x] Run `gofmt` and the full Go unit suite in the Go 1.25.8 container.
- [x] Add client and Terraform regressions for confirmed absence, incomplete/error responses, state retention, and refresh/recreation after out-of-band deletion.
- [x] Add direct regression coverage for already-absent Delete preflight behavior.
- [x] Run all `TestAccPagination` mock Terraform acceptance scenarios, including later-page failure recovery and out-of-band deletion/recreation.
- [x] Verify deletion/recreation against an isolated real controller using the focused `TestControllerLabNetworkDisappearance` scenario. The broader lab selector also runs unrelated baseline cases and is not claimed as passing.
- [x] Update the tracker with Go, mock acceptance, Delete-preflight, and focused real-controller results.

### Conditional — Controller validation and submission

- [x] Validate initial page metadata and collection behavior on Software Controller 6.1.0.19, including 102 real networks; record zero-device limitations.
- [ ] Repair the unpaginated WLAN-group envelope regression before claiming compatibility. Populated variants of the other empty collections remain unverified.
- [x] Commit/push the G2 patch and tracker update; verify remote refs.
- [ ] Recheck upstream/standards, prepare separate PR descriptions, and publish only when authorized. Record CI/review/merge separately.

## Progress

- [x] G1 is implemented and locally verified, pushed as [9343503](https://github.com/wbniv/terraform-provider-tplink-omada/commit/9343503d13a5428e91d121e0104ce7c36cd93cd6). All 94 client cases, full Go/race gates, three new Terraform scenarios/11 steps, and combined 44 scenarios/216 steps pass. [Evidence and integration notes](../investigations/pagination-fix/README.md).
- [x] G2 implementation and focused validation are complete for the nine collection-backed resource types. Direct/singleton endpoint resources remain outside automatic state removal because endpoint failures do not prove absence.

Next: review and submit the G2 change; retain the focused controller test as the live evidence. Broader controller-suite findings outside G2 remain tracked under [L1](controller-compatibility.md) and related follow-ups.
