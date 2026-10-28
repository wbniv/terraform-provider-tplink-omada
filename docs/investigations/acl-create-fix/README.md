# L8 — ACL creation acknowledgment and recovery

Work for Will on 2026-10-19. Branch: `fix/acl-create-readback`, based on L4 (`a2b1baa`). Parent: [controller compatibility plan](../../plans/controller-compatibility.md).

## Finding and correction

Software Controller 6.1.0.19 accepts creation of a disabled gateway ACL and returns an empty success acknowledgment. Decoding that response as an ACL object failed after the rule had already been created. The response contract and raw reproduction are recorded in [L4 evidence](../ip-group-fix/README.md).

Creation now obtains a complete inventory before the write, refuses an existing same-name rule, sends one create request, then obtains another complete inventory. It requires exactly one new same-name identity with the requested configuration. Existing identities, ambiguous matches, configuration mismatches, missing matches, and incomplete inventory reads cannot establish ownership. The existing authenticated request layer still supports its bounded session-recovery replay; L8 adds no mutation retry.

If the accepted write cannot be identified, the provider reports that outcome and directs the operator to inspect/import the existing rule. It does not invent an ID or automatically delete a candidate. A later create refuses an existing same-name rule before writing. Concurrent external creation/rename of indistinguishable rules cannot be attributed reliably with this API; avoid concurrent edits during creation.

Updates read the current raw rule, overlay only Terraform-owned fields, retain its ordering index and unmodeled settings, and read it back. The direction overlay preserves additional selectors. Identity and site response fields are excluded from PUT. ACL type changes require replacement.

## Verification

- [x] Add client cases for empty/object acknowledgments, existing names, failed preflight/readback, missing/ambiguous/mismatched matches, and late-page failure.
- [x] Add update coverage for ordering, syslog, state mode, custom ports, unknown fields, and additional direction selectors.
- [x] Pass Go vet, full tests, client race detector, and builds in the pinned container.
- [x] Pass five live Terraform steps: create, stable plan, rename/policy update, stable plan, import, plus verified destroy.
- [x] Pass four mock Terraform steps: accepted create/readback failure, duplicate-prevention retry, explicit import, and stable plan, plus verified destroy without another POST.
- [x] Verify live second-rule ordering, syslog preservation, and unchanged neighboring ACL.
- [x] Save evidence and clean up the disposable lab.

The initial build stopped before tests because the new `int64planmodifier` import was absent from the vendor tree. `go mod vendor` using Go 1.25.6 regenerated the existing pinned dependencies and added that framework package; `go.mod` and `go.sum` did not change. The initial failure remains in `initial-build.log`.

## Review and limits

The live scope is disabled gateway ACL configuration referencing IPv4 IP/Port groups on 6.1.0.19, without adopted hardware. Traffic enforcement, switch/EAP variants, and other controller versions remain open. G2 disappearance recovery has since been validated on the combined G2/L4 branch. Committed as `5b1b14f`; no upstream PR has been published.

## Initial live round

Go vet, full tests, client race detector, and builds passed. The four-step mock Terraform recovery scenario passed, including a duplicate-free retry and explicit import. Live creation passed, but both live update checks failed with controller error `-1600`. The bundled 6.1.0.19 ACL UI specifies **PUT**, while the provider had used PATCH. The update method is corrected to PUT and the regression test now requires it. `ui-files.json` records the inspected asset hashes; no proprietary UI source is copied into this report. The failed round remains in `initial-test-*` logs.

## Final verification

The final wrapper exits **0** with `acceptance_exit=0` and `client_exit=0`. Go vet, all Go tests, the client race detector, provider/test builds, all five live Terraform steps, and all four mock recovery steps pass. The client suite passes all nine creation cases, unmanaged-field preservation, and the live two-rule scenario. The latter verifies the second rule retains its ID, order, and externally configured syslog setting while the neighboring ACL remains unchanged.

`build.log` records Go 1.25.6, Terraform 1.15.8, and compiled source hashes. `provenance.json` verifies all 533 controller JARs against the pinned official archive. `tested-followup.patch` includes the new source/test files and regenerated vendor package. Earlier failures are retained separately rather than overwritten as successful runs.

L5 empty collections is verified separately as `6d5a721`; G2 confirmed resource-disappearance recovery is also complete, including combined G2/L4 live acceptance. Hardware traffic enforcement remains a separate gate.

Cleanup completed with exit 0: the runner, controller, relay, lab networks/volumes, runtime credentials, and disposable logs were removed after saving evidence. Reusable images, the vendored archive, source worktrees, and report artifacts were preserved.
