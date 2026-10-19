# Plans for the three open pull requests

Reviewed 2026-10-18 against `main` at `6bc71b3`. Metadata refreshed 2026-10-19: `main` remains `6bc71b3`; #23 remains `2ec46a4` with no posted checks; #20 at `e54b759` has successful Lint/Build/Unit Tests; #18 at `272ef99` reports failed Lint/Build and skipped Unit Tests. These are the reported check results, not newly executed CI runs. Recheck again before submission or merge.

Subsequent reliability work is tracked in the [PR tracker](provider-reliability-prs.md): A (authentication), B (switch-port preservation), C (AP radio preservation), D1–D3 (SSID preservation/identity/drift), and E (credentials/settings preservation) are implemented, locally verified, committed, and pushed to Will's fork as of 2026-10-19. F/F12 (HTTP status/envelope validation) is now locally verified and pushed as [c210dec](https://github.com/wbniv/terraform-provider-tplink-omada/commit/c210dec9a4bb10a15881f15a250b7c1843543d31); G/F09 pagination is next. No upstream PR or review comment has been published by this work. B has also been tested with #23's unchanged head; see its [compatibility evidence](../investigations/switch-port-fix/README.md).

Local verification used the `golang:1.25.8` Docker image. `main`, PR #18, and PR #23 each passed `go vet ./...`, `go build ./...`, and `go test ./...`. PR #18 and #23 also had no `gofmt -l .` output. The PR #23 mock-controller acceptance tests passed with Terraform 1.15.8. These local results do not replace GitHub CI or a release workflow run.

## PR #23 — switch port operational speed is read-only

Source: https://github.com/emanuelbesliu/terraform-provider-tplink-omada/pull/23

The controller reports `ports[].speed` as a changing link observation. The reviewed baseline permits users to configure it, so a link renegotiation can create a false Terraform diff. The proposed change makes the attribute computed-only. At the recorded review, the PR was open with no review or posted CI checks. It changes the configuration contract for users who set `speed` explicitly.

- [ ] Review the schema and state mapping together: keep `speed`'s name and type, confirm it is never sent as a port setting, and confirm link status changes only update state.
- [x] Run the PR's Go tests, vet, and build in Docker; `gofmt -l .` was clean.
- [x] Run the mock-controller acceptance tests with `TF_ACC=1` and Terraform 1.15.8. Both `TestAccDeviceSwitchOperationalSpeed` and `TestAccDeviceSwitchRejectConfiguredSpeed` passed, covering the 10 Mbps → 1 Gbps → disconnected sequence, empty plans, and read-only validation.
- [x] Verify compatibility with local reliability patch B using stored #23 head `2ec46a4ba19cdbda4a2ee6c1546a40bb5324b594`: the patches combine cleanly, and port preservation plus both speed acceptance tests pass in Go 1.25.6 with Terraform 1.15.8. B's scope remains separate from #23's schema change.
- [ ] Review the exact read-only error text and imported-switch refresh behavior against a real controller if available; the local mock tests passed.
- [ ] Review the generated resource documentation and release notes for the breaking configuration change: remove explicit speed assignments before upgrading; existing state can refresh without re-import. Confirm whether the project's release process requires a major version or another migration notice.
- [ ] Request maintainer review and ensure CI runs on the PR head. Merge only after the migration wording and tests pass.

**Done when:** observed speed changes do not trigger updates, configured speed is rejected, import and refresh retain the value, and the user-facing migration is explicit.

## PR #20 — upgrade `actions/setup-go` from v6 to v7

Source: https://github.com/emanuelbesliu/terraform-provider-tplink-omada/pull/20

The PR changes `actions/setup-go` in all three CI jobs and the release workflow. Its Lint, Build, and Unit Tests checks passed at the recorded review. The release workflow is not exercised by those checks. Dependabot also reported that configured `ci` and `dependencies` labels do not exist.

- [ ] Review the v7 runtime and runner requirements against GitHub-hosted `ubuntu-latest` and the project's release workflow, including the Go version read from `.go-version`.
- [ ] Confirm CI checks still pass after rebasing or refreshing the PR against current `main`; inspect the exact workflow diff to ensure all intended setup-go uses are upgraded.
- [ ] Validate the release workflow separately with a safe workflow check or a nonpublishing test of its setup/build steps before a tag release; preserve the GPG and GoReleaser configuration.
- [ ] Fix Dependabot's label configuration by creating the intended labels or removing invalid label names from `.github/dependabot.yml`.
- [ ] Merge after CI and release-path review, then monitor the next release run.

**Done when:** CI stays green on the current base, the release path is checked, and Dependabot no longer reports missing labels.

## PR #18 — upgrade `terraform-plugin-testing` from 1.15.0 to 1.16.0

Source: https://github.com/emanuelbesliu/terraform-provider-tplink-omada/pull/18

The PR changes `go.mod` and `go.sum`, including several indirect dependencies. At the recorded review, its old CI run had failed Lint and Build checks and skipped Unit Tests. GitHub returned HTTP 410 for that run's logs, so the precise failure is unconfirmed. A concrete version mismatch needs investigation: the reviewed PR changes the `go` directive to `1.25.8`, while the reviewed `main` pins `1.25.6` in `.go-version`.

- [ ] Rebase or refresh the PR against current `main`, then inspect fresh CI logs before attributing the historical failures to any single cause.
- [ ] Align the Go toolchain pin with the module minimum: either update `.go-version` and CI consistently to a supported Go version, or determine whether the dependency can be upgraded without raising the module directive. Do not edit only one pin.
- [ ] Run `go mod tidy` with the selected toolchain, inspect the direct and indirect dependency changes, and ensure `go.mod` and `go.sum` are reproducible.
- [x] Run `go vet ./...`, `go build ./...`, `go test ./...`, and `gofmt -l .` on the PR head in the Go 1.25.8 container; all passed.
- [ ] Rerun relevant provider acceptance tests if the test framework behavior changes; check import and plan assertions in particular.
- [ ] Fix Dependabot's missing `go` and `dependencies` labels, or remove those invalid entries from its configuration.
- [ ] Merge only after fresh Lint, Build, and Unit Tests checks pass and the Go pin decision is documented.

**Done when:** the module and CI use a compatible Go version, dependency files are clean, and all fresh checks pass.

## Suggested order for the three upstream PRs

This order concerns the upstream dependency/workflow/speed PRs. For the local reliability fixes, the next implementation is G/F09 complete pagination, followed by F10 confirmed disappearance; A–F await their remaining hardware validation and submission gates.

1. Triage and refresh #18 first because its checks fail and its Go version change may affect CI and the other PRs.
2. Review #23's behavior and migration independently; it fixes user-visible Terraform drift.
3. Finish #20 after the CI baseline is healthy, with separate attention to the release workflow.
