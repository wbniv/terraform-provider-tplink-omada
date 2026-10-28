# G2 — Confirmed resource disappearance

Prepared 2026-10-27 for the `fix/confirmed-disappearance` branch, head `3a327c926157b9aace3178b09c4e4564fba5b5e7`. The branch is pushed to Will's fork. Parent: [G1 pagination evidence](../pagination-fix/README.md) and the [G1/G2 plan](../../plans/pagination-and-disappearance.md).

## Submission review

- Upstream `main` was fetched and remains `6bc71b310bc81a90f3ad32194ba8c17947bf79d6`.
- At review time, upstream had three open PRs: #23 for switch operational speed and Dependabot PRs #20 and #18. None overlaps G2. No G1/G2 PR is open.
- The repository PR template asks for conventional commits, formatting/vet/tests, documentation where applicable, and an explicit real-controller test declaration.
- G2 is stacked on G1 `9343503`, which itself follows F and earlier A–E commits (`52ed681` → `8f24c37` → `c210dec` → `9343503` → G2). Opening the current head against upstream `main` would include that entire series. Keep this draft unsubmitted until the dependency chain has an agreed upstream base or is otherwise integrated.
- The current G2 branch contains conventional commit subjects and its tested changes. The available record confirms `gofmt` and `go test ./...`; the PR template's exact `make fmt`, `make vet`, and `make test` commands have not been run as a set.

## L4 branch reconciliation

The G2 change was also integrated on top of the L4/L8 compatibility branch as `fix/confirmed-disappearance-l4`, head `0cf3c8a1768bbf290cf6bdb2cffaed6b94ccd223`, based on `5b1b14f` (`fix/acl-create-readback`). It is pushed to Will's fork. L4 had moved IP-group client methods from `client.go` to `internal/client/ip_groups.go`; the G2 patch was reconciled by preserving the new typed `notFound` result in that replacement `GetIPGroup` method. IP-group resource Read and Delete therefore recognize confirmed absence on the integrated branch. The original G1-based G2 branch remains unchanged.

The integrated branch has now passed a focused live IP-group disappearance acceptance check on Omada Software Controller 6.1.0.19. The [combined L4 live record](combined-l4-live-acceptance.md) covers typed confirmed absence, Terraform state removal/recreation, idempotent delete, and cleanup on `fix/confirmed-disappearance-l4` (`0cf3c8a`). The original G2 live network scenario and the repository PR gates remain separately recorded; this focused check does not replace the upstream base/dependency review.

## Draft PR description

**Title:** `fix(resources): remove confirmed absent resources from state`

### Problem

After a resource is deleted outside Terraform, a complete successful collection lookup can show that it no longer exists. The provider currently reports this as a read error, leaving Terraform unable to plan its recreation normally. Failed or incomplete discovery must not be mistaken for absence.

### Changes

- Return a typed not-found result only when a complete successful collection excludes the requested identity.
- Remove Terraform state for confirmed-absent networks, WLAN groups, SSIDs, port profiles, ACLs, IP groups, mDNS rules, SAML identity providers, and SAML roles.
- Before Delete, use the same complete collection evidence to treat an already absent supported object as deleted.
- Preserve state on transport, authorization, protocol, incomplete-list, and direct-endpoint failures. In particular, HTTP 404 alone does not prove resource absence.
- Leave direct/singleton endpoint resources outside this inference. No schema, import ID, or state format changes.

### Testing

- Go 1.25.8: `gofmt` and `go test ./...` passed.
- Client regressions cover complete-list absence and incomplete/error responses.
- All five `TestAccPagination` mock Terraform acceptance scenarios passed, including refresh failure recovery, recreation planning, and already-absent Delete.
- `TestControllerLabNetworkDisappearance` passed against isolated Omada Software Controller 6.1.0.19: out-of-band deletion, recreation plan/apply, controller readback, and stable follow-up plan.
- The broader controller selector also exercises unrelated baseline cases and is not claimed as passing.

### Compatibility

There are no schema or import changes. A confirmed-absent managed object is removed from state during refresh, allowing Terraform to propose creating it again from configuration. Direct endpoint errors remain diagnostics and retain state.

### Repository checklist

- [x] Conventional commit titles.
- [x] Formatting and full Go unit suite recorded above.
- [ ] Run the repository's exact `make fmt`, `make vet`, and `make test` commands before submission, or update this declaration with equivalent verified project gates.
- [x] Real-controller test declaration: focused G2 scenario passed on Software Controller 6.1.0.19 in an isolated lab; no AP or switch hardware validation is in scope.
- [x] Updated maintained plan/tracker documentation.
