# L1: WLAN group collection policy

Work for Will, 2026-10-19. Code committed as `0730101` on `fix/wlan-group-collection`. Parent: [controller compatibility plan](../../plans/controller-compatibility.md).

## Change

- [x] Give WLAN-group discovery an explicit endpoint policy: a `data` envelope without pagination metadata is complete on the first response.
- [x] Preserve strict paginated behavior when any pagination metadata is present, including on WLAN groups. All other endpoints keep their existing policy.
- [x] Keep row validation, duplicate-identity rejection, byte limits, authentication, and error handling shared.
- [x] Add regression coverage for a populated WLAN envelope, default/custom lookup, empty lists, malformed data, and invalid identities.
- [x] Add a live Terraform SSID lifecycle scenario covering default-group create/import, replacement into a custom group, a stable plan, and a final rename attempt.
- [x] Complete container regression and real-controller probes; remaining SSID update failure recorded below.

## Reproduction environment

The `fix/wlan-group-collection` branch is based on `c953da6`. Tests integrate the D1–D3 SSID stack using the existing reviewed patches. Go 1.25.6 and Terraform 1.15.8 run inside the isolated Docker lab against Omada 6.1.0.19, with vendored modules and no external network route.

A fresh initialization rejected the previous generated device password with API `-30105`. A shorter password with fixed character classes was accepted; the harness now generates that format. This does not establish the full controller password policy.

## Additional findings

- [x] With discovery unblocked, the default PMF value (`2`) for an open SSID (`security = 0`) causes an inconsistent apply: the controller returns `0`. Retained in `acceptance-initial.log`; tracked as L6. An explicit mode 0 retry was rejected at create with API `-1001` (`acceptance.log`). The final scenario uses a secured SSID.
- [x] Wireless data-source discovery now reaches its empty inventory assertion, which fails. Include wireless collections in the existing L5 empty-list audit.
- [x] Secured SSID creation succeeds, but rename fails before PATCH because the controller returns a field outside D1's writable contract. Tracked as L7; see `ssid-secured-initial.log`.

## Validation

- [x] Vendored Go vet, unit tests (including malformed-page and pagination bounds), build, and both test binaries pass.
- [x] All 24 D1–D3 mock Terraform scenarios pass (3 creation, 13 identity, 5 preservation, 3 VLAN).
- [x] Real WLAN discovery passes; the real 102-network test passes and removes all temporary networks.
- [x] Final secured-SSID run passes steps 1–4: create, composite-ID import, custom-group replacement, and stable plan.
- [ ] Step 5 rename remains blocked by L7. Open-network default behavior remains blocked by L6.

The full suite intentionally exits nonzero for outstanding controller failures. `acceptance.log` records the explicit mode 0 attempt plus D regressions; `acceptance-initial.log` records the default PMF mismatch. `client-probes.log` retains the expected L2/L3/L4 failures. These are not all-green results.

`ssid-lifecycle.log` records the final step-5 failure; the preceding four steps passed, and cleanup reported no failure. `wlan-discovery.log` passes. `focused-status.txt` honestly records acceptance exit 1 and client exit 0. `focused-build.log` hashes the compiled final test source; `source.sha256` identifies the final checkout source. The synthetic passphrase in the test fixture is not a real credential.

An intermediate import attempt incorrectly used the bare SSID ID. The final test uses the required `siteID/wlanGroupID/ssidID` composite; no production import defect was inferred from that fixture mistake.

## Scope and limits

The production correction is limited to WLAN-group discovery. L2–L7 and G2 remain open. No physical AP or switch behavior is claimed. Existing metadata-free paged collection tests for other endpoints remain unchanged.

- [x] Removed disposable containers, owned networks/volumes, and private runtime files after preserving evidence.

## L7 follow-up

The [SSID field correction](../ssid-controller-fields/README.md) now passes the previously blocked secured rename, as well as password rotation, custom/default VLAN updates, and stable plans. L6 open-network PMF behavior remains unresolved; the failures above are the historical L1 results.
