# G1 — Complete collection reads

Prepared for Will on 2026-10-19. Implemented on `fix/complete-pagination` in `/home/will/terraform-provider-tplink-omada-pagination`, **committed and pushed** as [9343503](https://github.com/wbniv/terraform-provider-tplink-omada/commit/9343503d13a5428e91d121e0104ce7c36cd93cd6), based on F `c210dec9a4bb10a15881f15a250b7c1843543d31` (includes A/E). Parent: [G1/G2 plan](../../plans/pagination-and-disappearance.md), [reliability tracker](../../plans/provider-reliability-prs.md#g--pagination-and-disappearance), and original F09 finding. F and its evidence were committed/pushed before G1: code `c210dec`, documentation `bb5f22c`; both fork refs were verified.

## Scope

- [x] Replace fixed page-1 reads in ten collection methods: sites, networks, WLAN groups, SSIDs, port profiles, ACLs, IP groups, mDNS, SAML IdPs, and SAML roles.
- [x] Use the same complete listing for raw SSID lookup; preserve returned attributes for update builders.
- [x] Preserve endpoint scope, ACL type filters, authentication, response order, and first-response direct arrays. The devices endpoint remains a direct-array read.
- [x] Validate supplied metadata, stable totals, complete row counts, and unique identities. If totalRows is absent, continue until an empty page; short pages alone cannot prove completion.
- [x] Return no partial collection on later-page errors, inconsistent responses, duplicate identities, malformed rows, cancellation, or excessive page/byte counts.
- [x] Bound a collection to 10,000 pages and 64 MiB of cumulative result JSON, retaining F's per-response limit.
- [x] Remove the obsolete single-page decoder and its two helper-only tests; replace them with public-method regression coverage.
- [x] Correct the existing client fixture helper to report its actual item count instead of always reporting one.
- [x] Verify later-page import, failed-refresh recovery, and complete data-source contents using Terraform.
- [x] Pass final combined compatibility and client race gates; retain logs and integration patches.

Seven maintained files change: two production Go files, two client test files, one provider acceptance test file, README, and a [collection-read guide](https://github.com/wbniv/terraform-provider-tplink-omada/blob/9343503d13a5428e91d121e0104ce7c36cd93cd6/docs/guides/collection-reads.md). No resource schema or state format changes. G2/F10 state removal remains a separate patch; this code continues reporting a missing-resource error after a successful lookup finds no match.

## Regression evidence

| Gate | Result | Evidence |
| --- | --- | --- |
| Same final client regressions on F baseline | 69 failures / 25 passes among 94 leaf cases | [Baseline](baseline-pagination.log) |
| G1 client regressions | All 94 pass | [Patched](patched-pagination.log) |
| Same final Terraform scenarios on F baseline | All three fail | [Baseline Terraform](baseline-acceptance.log) |
| G1 Terraform | All three scenarios / 11 steps pass | [Patched Terraform](patched-acceptance.log) |
| Formatting / vet / build / full unit suite | Pass; client 53.0%, resources 11.7%, provider 2.9% coverage | [Full gates](full-gates.log) |
| Final client race suite | Pass, including A/F/G1, 74.519 s | [Race log](race.log) |
| Combined A + D1–D3 + E + F + G1 | Full unit suite and all 44 Terraform scenarios / 216 steps pass; Terraform run 143.690 s; integration adjustments below are included | [Combined log](combined-compatibility.log) |

The 94 client cases comprise 60 collection cases (ten endpoints × six response modes), 27 invalid/inconsistent response cases, four typed/raw/site-resolution lookups, one cancellation case, and two finite-work limits. Cases cover 201 rows, controller page-size caps, omitted metadata, direct arrays, empty collections, later-page HTTP failures, null/invalid metadata and data, repeated identities/pages, changing totals and response shapes, raw attribute preservation, and secret-free errors.

The three Terraform scenarios are: import an object on page 3 and recover after a later-page failure (five steps); keep a page-1 object's managed identity while a later page fails and then recovers (five steps); verify a data source returns all 201 entries, including index 200 (one step). The old provider cannot import the late object, overlooks the injected error after page 1, and returns only 100 data-source entries. Test failures/cleanup concern synthetic in-memory objects only.

The final two new test files are byte-identical in baseline and patched copies. The baseline is an archive-equivalent copy of F before G1 production edits, with these regression files added. The existing incorrect fixture helper is corrected only in the patched tree.

## SSID integration findings and resolution

The combined build exposed two integration requirements, retained separately from G1's production branch:

1. D's raw-SSID decoder hunk overlaps G1. G1 supersedes it with complete pagination and direct-array support. The [client integration patch](ssid-client-integration.patch) retains D's other two client changes: nullable VLAN mode and authoritative update readback. All other D production files are applied unchanged.
2. Some existing D mocks return the same nonempty data wrapper indefinitely without totalRows; one emits null for an empty collection. The [fixture patch](ssid-pagination-fixtures.patch) adds accurate totals and an empty array in five D test files. It preserves the scenarios and assertions. The [initial combined failures](compatibility-initial.log) show why these adjustments are needed.

F's [six diagnostic assertion adjustments](../http-response-fix/ssid-diagnostic-assertions.patch) are also required. None of the original D worktrees is edited. Do not describe combined results as passing against completely unmodified D tests.

## Reproduction

Environment: Go `1.25.6 linux/amd64`, Terraform `1.15.8 linux_amd64`, image `golang:1.25.6@sha256:06d1251c59a75761ce4ebc8b299030576233d7437c886a68b43464bad62d4bb1`, disposable container `omada-pagination-fix`. The worktree is mounted read-only at `/src`; writable copies are `/tmp/baseline`, `/tmp/review`, and `/tmp/compat`. Terraform is copied from `/home/will/.local/bin/terraform` to `/usr/local/bin/terraform`. Go and Terraform run inside Docker. No real controller, cloud account, or physical device is used.

Run the same maintained regressions in baseline and patched copies:

```sh
go test ./internal/client -run TestPagination -count=1 -v -timeout=120s
TF_ACC=1 TF_ACC_TERRAFORM_PATH=/usr/local/bin/terraform go test ./internal/provider -run TestAccPagination -count=1 -v -timeout=180s
```

Patched gates:

```sh
test -z "$(gofmt -l .)"
go vet ./...
go build ./...
go test ./... -count=1 -cover
go test -race ./internal/client -count=1 -v -timeout=240s
```

To prepare a disposable combined checkout, copy G1 and remove its copied `.git` pointer. Generate D's non-client changes in a repository containing the commits with `git diff 6bc71b3 ec2d949 -- . ':(exclude)internal/client/client.go'`. Apply that patch, `ssid-client-integration.patch`, F's diagnostic assertion patch, and `ssid-pagination-fixtures.patch`. Run:

```sh
go test ./... -count=1
TF_ACC=1 TF_ACC_TERRAFORM_PATH=/usr/local/bin/terraform go test ./internal/provider -run 'TestAcc(Pagination|HTTPResponseDelete|SiteCredentials|WirelessNetwork)' -count=1 -v -timeout=360s
```

Do not copy an entire older client.go over G1 when integrating; that would discard pagination or response/authentication repairs.

## PR package and limits

Suggested title: `fix(client)!: read complete paginated collections`.

Suggested migration note: collection reads now fail on inconsistent metadata, missing/duplicate identities, malformed objects, and unfinished listings. Data-only wrappers must advance to an empty page. Maximum collection size is 10,000 pages / 64 MiB of result JSON. Correct the controller/proxy behavior and retry; no partial result is returned. Schemas and import IDs are unchanged.

- [x] Record dependency, baseline failures, passing targeted checks, behavioral policy, source scope, and integration adjustments.
- [x] Recheck contribution standards: conventional commits, fmt/vet/tests, documentation, and real-controller test declaration. No explicit AI attribution rule found; no session metadata guessed.
- [x] Retain final evidence and matching seven host/container hashes. Remove the disposable container and both temporary host patches after saving logs and integration patches. Requested worktrees and deliverables are preserved. Final link checks pass with the previously documented README-to-LICENSE baseline exception; all seven prior code worktrees remain clean.
- [x] Commit/push G1 as [9343503](https://github.com/wbniv/terraform-provider-tplink-omada/commit/9343503d13a5428e91d121e0104ce7c36cd93cd6); all seven committed file hashes match the tested sources and the fork ref is verified. The commit also updates generated README history.
- [x] Run the [software-controller lab](../software-controller-lab/README.md) on 6.1.0.19: 102-network listing succeeds.
- [ ] Correct the measured WLAN-group envelope regression under [L1](../../plans/controller-compatibility.md). The earlier mock evidence does not establish real compatibility for this endpoint.
- [ ] Implement G2 with typed confirmed absence, audited resource Read/Delete behavior, and Terraform refresh/recreation tests. HTTP 404 remains insufficient evidence of deletion.
- [ ] Recheck upstream/standards before publication; record PR, CI, reviews, and merge separately. No upstream PR or comment has been posted.

Complete pagination cannot guarantee a consistent snapshot while a remote collection changes. Stable totals/unique identities catch detectable inconsistencies; controller behavior and access scope still require real-controller verification. Device provisioning, radio and switch-port behavior require physical test devices separately.

## Tested file hashes

All seven host/worktree hashes match the final container `/tmp/review` files. SHA-256:

| File | SHA-256 |
| --- | --- |
| `README.md` | `077535c720a169f4ac99bf46b51231cdda63aa86d4eae47a404c776619fd7328` |
| `docs/guides/collection-reads.md` | `84ae16217a79dafe27785c467140dcc40a2715febbfbb72d41a3aa6bcece9f86` |
| `internal/client/client.go` | `06fba1414169d20eaf6d71fdb90d6bb7b4adf7a8b539ff5ab2dbe095d2e330a4` |
| `internal/client/client_test.go` | `f2dc258ad0484b2f5df921f8143ceab2a34a0a982d06bef7080269a0ea878bad` |
| `internal/client/pagination.go` | `7f5aeac1262c51870847dc2098057495acf1a827f021fe31362b0ca39e4f529c` |
| `internal/client/pagination_test.go` | `7de2c1de7e522f0a306f459ece177dfb1c47a6f0edab8e1f987681eef4f2c7a4` |
| `internal/provider/pagination_test.go` | `c0a4a325b34497ba21bd5b45d4e42a20d678b5dc10f8db6d1491c55857067b9d` |
