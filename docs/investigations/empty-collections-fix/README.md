# L5 — Empty collection state

Work for Will on 2026-10-19. Branch: `fix/empty-data-source-collections`, based on L4 (`a2b1baa`), in `/home/will/terraform-provider-tplink-omada-empty-collections`. L8 was developed independently and is committed as `5b1b14f` in its separate worktree. Parent: [controller compatibility plan](../../plans/controller-compatibility.md).

## Audit and changes

The controller correctly returns an empty device array when no devices are adopted. Five data sources initialized their output slices only while appending entries, leaving nil slices that Terraform encoded as null: networks, wireless networks, port profiles, sites, and devices. Their output slices now start empty. ACL, IP/Port-group, and mDNS top-level slices were already initialized.

The nested-list audit also found nil-to-null conversion in port-profile tagged networks, ACL protocols/source/destination IDs, and mDNS profile/service/client lists when an AP or OSG settings object exists but omits those lists. These now return empty lists. IP/Port-group mapping uses the same list conversion; L4 still rejects unsupported or empty port groups in the client before state conversion. Nullable scalar fields are unchanged.

The devices decoder previously accepted a JSON null as a nil slice. It now requires the documented array response so a malformed top-level response cannot become an empty inventory. Existing collection pagination and error checks remain active.

## Verification plan

- [x] Audit all eight collection data sources and nested lists.
- [x] Add seven mock Terraform steps across all eight sources: empty, populated with omitted nested lists, populated nested lists, refresh back to empty, HTTP failure, malformed result, recovery.
- [x] Add direct device-decoder rejection cases for null, object, envelope, and string responses.
- [x] Add live Terraform checks for empty devices/SSIDs/ACLs/IP groups/mDNS and populated sites/networks/port profiles, with a stable plan.
- [x] Pass Go vet, full tests, race detector, and builds in the pinned container.
- [x] Pass all Terraform checks; retain the existing live IP/Port-group lifecycle as populated-list coverage.
- [x] Save source hashes, logs, and controller provenance; clean up the disposable lab.

Terraform outputs evaluate both `length()` and list iteration so a null value cannot satisfy the empty-list checks. Nested-list outputs evaluate each nested list's length. Populated fixtures exercise existing row mapping; the live device inventory remains empty because hardware is not adopted.

## Compatibility and status

Successful empty reads now produce `[]`. Configurations that used `== null` to detect an empty collection should use `length(collection) == 0`. HTTP and protocol failures remain errors. G2 resource-disappearance recovery is complete, including the combined L4 live result linked below. Physical-device behavior and other controller versions remain separate compatibility gates.

Verification passed with wrapper exit 0, `acceptance_exit=0`, and `client_exit=0`. Committed as `6d5a721`; no upstream PR has been published.

## Results

- [x] Go vet, full Go tests, client race detector, provider build, and test-binary builds pass in Go 1.25.6 containers.
- [x] Terraform 1.15.8 passes all seven mock steps across eight collections, including nested empty/populated values and failure recovery.
- [x] The controller with no adopted devices passes all eight live collection checks and the stable-plan step. Devices, SSIDs, ACLs, IP/Port groups, and mDNS are empty; sites, networks, and port profiles are populated.
- [x] The five-step live IP/Port-group lifecycle still passes, including populated data-source checks and verified destruction.
- [x] Four malformed device response shapes are rejected by direct client tests.

`build.log` records exact compiled source hashes and tool versions. `provenance.json` verifies all 533 controller application JARs against the pinned official 6.1.0.19 archive. `tested-followup.patch` includes the new Terraform regression test. No dependency versions or vendor contents changed.

G2 confirmed-disappearance recovery is complete, including the combined L4 live check recorded [here](../confirmed-disappearance-fix/combined-l4-live-acceptance.md). Additional controller versions and populated variants of some collections remain verification gates.

Cleanup completed with exit 0. The runner, controller, relay, lab networks/volumes, credentials, and disposable runtime files were removed. Source worktrees, evidence, the vendored archive, and reusable container images were preserved.
