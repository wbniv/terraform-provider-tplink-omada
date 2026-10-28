# Combined G2/L4 live acceptance

**Date:** 2026-10-28  
**Branch:** `fix/confirmed-disappearance-l4`  
**Commit:** `0cf3c8a1768bbf290cf6bdb2cffaed6b94ccd223`  
**Controller:** Omada Software Controller 6.1.0.19, physical-test instance  
**Result:** PASS

## Scope and result

A focused Terraform acceptance scenario exercised the reconciled G2/L4 implementation using one synthetic IP/Port group in the `ES216G Acceptance` controller site:

1. Terraform created the group and recorded its controller ID.
2. The test deleted it out of band. `GetIPGroup` returned the typed `client.ErrNotFound`, and a second delete succeeded as already absent.
3. Terraform refresh removed the disappeared object from state and planned its recreation.
4. Apply created a replacement with a different ID; controller readback matched the configured name, and a follow-up plan was empty.
5. Terraform cleanup removed the replacement. The test's final controller inventory check confirmed no group with the synthetic test name remained.

The run did not issue device commands or change switch settings. It used the combined production source at commit `0cf3c8a`; the temporary live acceptance test was added only to a disposable worktree and was not committed. Its SHA-256 was `0bf0dc6a76bf0a90cb2d9908dc8d2662c6fb1507defe84a6716fe10159279957`.

The test passed in a disposable Go 1.25.8 container with Terraform 1.15.8. The focused command was:

```sh
go test -mod=vendor ./internal/provider \
  -run '^TestG2L4LiveIPGroupDisappearance$' -count=1 -v -timeout=5m
```

The controller target was supplied through the private `.es216g-lab/provider.env`; no credentials are included here. The combined G2/L4 branch's existing Go suite, vet, and provider build results remain recorded in the [branch reconciliation notes](README.md).

This is one live IP-group disappearance case on Controller 6.1.0.19. It does not establish compatibility with other controller versions or replace the separate G2 network scenario and upstream submission/dependency review.
