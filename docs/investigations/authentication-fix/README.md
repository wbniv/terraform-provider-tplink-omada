# Authentication patch evidence

Date: 2026-10-18. Local PR tracker: [work A](../../plans/provider-reliability-prs.md#a--authentication-and-certificates).

Scope: F01 (certificate deadlock), F07 (expired query token on retry), and F08 (concurrent token access). The local patch is on `fix/client-authentication`, based on `6bc71b310bc81a90f3ad32194ba8c17947bf79d6`. Committed and pushed to Will's fork on 2026-10-19 as [52ed681](https://github.com/wbniv/terraform-provider-tplink-omada/commit/52ed681674e5a0058ae7807fcd08399c21fa021a). No upstream PR has been opened. F02 and F03 are now implemented and locally verified in separate [switch-port patch B](../switch-port-fix/README.md) and [AP radio patch C](../ap-radio-fix/README.md); the remaining findings are open in the tracker. A's code and test hashes remain unchanged. C's evidence includes a passing combined A + C full unit suite and AP acceptance run on 2026-10-19.

## Implementation

- Session state is protected by one mutex, which is released before network I/O.
- An in-progress authentication attempt is shared. A waiting caller can cancel without canceling the initiating caller's login.
- A session generation prevents a delayed expired-session response from causing another login after a successful refresh.
- Each request takes one token snapshot for its header and existing token query parameter. Query parameters are parsed and encoded rather than replaced as raw strings.
- Certificate/key uploads use the same bounded authentication retry as JSON requests, replaying the encoded multipart body.
- Certificate activation/read no longer acquire an outer authentication lock.
- Login requests honor their context and reject an empty token.

The public resource schema, state format, certificate endpoint paths, multipart field names, and certificate read-only checks are preserved. No live controller was accessed. HTTP status/envelope validation remains separate work F12, and controller discovery still has its existing startup-context limitations.

## Regressions

The same maintained `internal/client/auth_test.go` was installed in a clean checkout of the baseline and in the patched checkout. Tests use local HTTP servers and synthetic tokens/PEM contents. The certificate tests check all four operations with a valid, absent, and expired token; they assert results, endpoint/method, multipart filename/bytes/metadata, activation payload, and login/request counts.

| Test group | Baseline | Patch |
| --- | --- | --- |
| Certificate operations, 12 cases | All time out | Pass |
| Concurrent expiry, including one delayed old response | Fails; races and repeated refreshes | Pass; one refresh |
| Concurrent requests and forced refresh | Fails; token races/mismatches | Pass under race detector |
| Login/waiter cancellation and recovery | Fails | Pass |
| Login denial, empty token, repeated expiry | Empty-token case fails | All pass; retry bounded |
| Read-only certificate writes | Pass | Pass |

Evidence: [baseline regressions](baseline-regressions.log), [patched regressions](patched-regressions.log), [patched full unit suite](patched-unit-suite.log). Baseline exit status was 1 as expected; patched commands exited 0. Baseline failures are deliberate evidence of defect detection, not a failed patch validation.

## Environment and commands

Go: `go1.25.6 linux/amd64`, matching `.go-version`. Docker image: `golang:1.25.6@sha256:06d1251c59a75761ce4ebc8b299030576233d7437c886a68b43464bad62d4bb1`.

| Command, run inside the disposable checkout | Result |
| --- | --- |
| `gofmt -l .` | No output |
| `go vet ./...` | Exit 0, no output |
| `go build ./...` | Exit 0, no output |
| `go test ./... -count=1 -cover` | Pass; client statement coverage 40.1% |
| Targeted race command below | All six groups pass; no detected races |

Targeted race command: `go test -race ./internal/client -run 'Test(Certificate|Concurrent|Authentication|Session)' -count=1 -v -timeout=60s`.

The full unit run leaves live-controller acceptance tests gated off; it is not an end-to-end controller compatibility run. Resource coverage in this A-only run was 0.4%. B's separate evidence records its additional resource and Terraform acceptance coverage.

Reproduction: use the pinned image above, clone the local repository into a disposable container, check out the baseline commit, and restore only `internal/client/auth_test.go` from commit `52ed681674e5a0058ae7807fcd08399c21fa021a` to run the failing regressions. In a second checkout, use that full fix commit and run the commands above. The original checkout now hosts the documentation branch, so its production files are the baseline; use the recorded fix commit for patched code. This mirrors the actual comparison and needs no host Go installation.

The exact tested working files have these SHA-256 hashes:

| File | SHA-256 |
| --- | --- |
| `internal/client/client.go` | `f78f338e0e2298ed3a5c66403c3967f32e55eff451bf93c2fd90f754b398fa57` |
| `internal/client/auth_test.go` | `b30adaa98d5a330e5521f183666af80b1442643056930fe5052c16c1a61d537d` |

## Before submission

- [x] Limit production changes to the client authentication/certificate boundary.
- [x] Demonstrate regressions on the baseline and passing behavior on the patch.
- [x] Run formatting, vet, build, full unit tests, and targeted race tests in Docker.
- [x] Confirm the working files match the tested files by SHA-256.
- [x] Commit the two selected code/test files with the conventional title and push `52ed681` to `fork/fix/client-authentication`; tested hashes match the committed files.
- [x] Check upstream and CI on 2026-10-19: main remains `6bc71b3`; feature-branch pushes do not trigger the existing CI workflow. Recheck before PR submission.
- [ ] Review controller compatibility for certificate upload/activation; record the actual controller version and result. Keep the upstream template's real-controller checkbox unchecked until then.
- [ ] Verify any required attribution using actual session metadata.
- [ ] Prepare/publish the upstream PR when authorized, then record its URL, CI, reviews, and merge status in the tracker.

Local plans, historical investigations, and captured logs are review support. Select their inclusion in an upstream PR deliberately; the behavior patch itself consists of the two Go files above.
