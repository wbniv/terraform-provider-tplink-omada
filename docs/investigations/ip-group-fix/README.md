# L4 — IP/Port group protocol

Work for Will on 2026-10-19. Branch: `fix/ip-group-protocol`, based on L3 (`58610fd`). Parent: [controller compatibility plan](../../plans/controller-compatibility.md).

## Controller findings

Software Controller 6.1.0.19 uses `/setting/profiles/groups`, not `/setting/firewall/ipGroups`. Type-specific list routes return complete `data` envelopes without pagination metadata, including when a page size of one is requested. Identities are `groupId`; creation returns the new identity as a JSON string; PATCH and DELETE return empty success envelopes. Mutation routes include both type and identity: `/setting/profiles/groups/1/<groupId>`.

Default inventory contains IPv4 IP-only (type 0), IPv6 IP-only (type 3), and domain (type 7) groups. Type 1 is IPv4 IP/Port. The provider's existing resource and data source specifically describe IP/Port groups, so their collection is now restricted to type 1. Other profile types are not reinterpreted or modified.

IP subnets use separate `ip` and `mask` fields. Ports are a single group-wide `portList`, with `portType=0` for number/range mode. The controller rejects an empty port list. Per-entry port lists in the Terraform schema must therefore be nonempty and identical. Differing lists are rejected instead of silently widening the match. Port-only groups with no IP entries and port-mask mode are explicitly unsupported rather than decoded as an empty port list.

## Changes and gates

- [x] Correct routes, identity decoding, subnet masks, and shared port encoding.
- [x] Read back controller state after creation and updates. Retain the known created identity in Terraform state when readback fails, so recovery can clean up the recorded group.
- [x] Preserve group and unchanged-subnet descriptions on update; exclude response metadata from writes.
- [x] Block mutations to groups marked built-in and reject unsupported group types or port-mask representations.
- [x] Keep strict pagination validation when metadata exists; use the explicit complete-envelope policy for this verified endpoint.
- [x] Run Go vet, full tests, client race detector, and builds.
- [x] Verify Terraform create, stable plan, rename/port update, stable plan, import, and destroy.
- [x] Verify the complete supported group population is listed and cleaned up, and page/size arguments do not truncate it.
- [x] Verify disabled ACL source/destination references survive group updates.
- [x] Verify accepted creation followed by readback failure retains identity and is recovered without an orphan.
- [x] Save sanitized evidence and clean up the disposable lab.

## Compatibility and migration

`port_list` is now required on each `ip_list` entry. Existing configurations that omit it or specify different lists must be corrected; the verified controller cannot implement those semantics. Use separate groups for different port sets. Bare IPv4 addresses mean /32; an explicitly configured /32 is preserved during normal refresh, while import uses the canonical bare-address representation. The import test ignores only that textual /32 difference; other fields remain verified.

Configuration is testable without adopted devices. Actual ACL enforcement remains a hardware gate. Other controller versions, IPv6 groups, MAC groups, domain groups, and port-mask mode are outside this correction's compatibility declaration.

## New finding: L8

A raw disabled gateway ACL referencing a synthetic IP/Port group was successfully created, read back, and deleted without adopted hardware. Its creation response is `{errorCode:0,msg:"Success."}` with no result. Existing `CreateACLRule` tries to decode this as an ACL object, so the Terraform ACL resource cannot complete that successful create. Track this separately as L8; raw ACL-reference verification does not claim the existing Terraform ACL resource works.

## Initial test round

Go vet, full tests, client race detection, builds, and the five-step real Terraform lifecycle passed. The attempted 101-group stress test hit controller error `-33725` (IP/Port-group capacity reached). The test cleaned up its fixtures. The final probe accepts only that documented quota rejection, verifies every successfully created group, and checks the complete response despite `currentPage=2&currentPageSize=1`. Synthetic pagination tests still cover 201 rows, capped pages, duplicate identities, malformed metadata, and late failures. The quota is a controller capability limit, not evidence of truncated provider reads.

## Live verification results

The lab's IP/Port-group capacity is **16**. All 16 synthetic groups were returned, including a request for page 2 with size 1; all were deleted after the test. A disabled gateway ACL retained its source and destination group IDs after rename and port changes, and was deleted before group cleanup. The five-step Terraform lifecycle, populated data source, and destroy verification pass.

The final combined lab invocation returned `acceptance_exit=1`, `client_exit=0`: the live Terraform test passed, but the added mock recovery test used an unsupported `insecure` provider argument and failed before apply. That test-only argument was removed. A focused container rerun verifies the corrected scenario; its result and exact source hashes are recorded separately. Do not read the historical combined status as a passing wrapper result.

## Completion and review notes

The focused failed-readback test passes (three Terraform steps). It confirms that a failed create with a returned ID remains recorded, the tainted group is deleted before replacement, and final destroy leaves no orphan. Final production source hashes match the prior passing Go/race/live run; the focused rerun changes only the mock provider configuration. `recovery-focused.log` records those hashes and exit success.

All required checks pass across the combined lab run and focused correction: vet, full Go tests, client race detector, builds, five live Terraform lifecycle steps, three mock Terraform recovery steps, full 16-group inventory, and ACL-reference persistence. The combined wrapper's earlier fixture failure remains explicitly recorded in `test-status.txt`; it has not been rewritten as success. `tested-followup.patch` includes all new source/test/probe files as well as tracked changes.

This fix changes the port-list schema from optional to required and requires identical lists across entries. Call this out as a breaking compatibility correction when preparing a PR. Code is committed as `a2b1baa` on `fix/ip-group-protocol`. No upstream PR has been published.

Next implementation: L8 ACL creation acknowledgment, then L5 empty collections and G2 confirmed-disappearance recovery. Natural session expiry, additional controller versions, and physical hardware/traffic validation remain separate gates.
