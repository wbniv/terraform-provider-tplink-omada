# F — HTTP response validation

Real-controller follow-up (2026-10-19): the [6.1.0.19 lab](../software-controller-lab/README.md) accepts normal discovery/login/network/profile/site/settings responses, but lost local session state produces HTTP 302 and no recovery. L3 must specify a narrow reauthentication signal while retaining arbitrary-redirect rejection.

Prepared for Will on 2026-10-19. Implemented and locally verified on `fix/http-responses` in `/home/will/terraform-provider-tplink-omada-http-responses`, **committed and pushed** as [c210dec](https://github.com/wbniv/terraform-provider-tplink-omada/commit/c210dec9a4bb10a15881f15a250b7c1843543d31). Base: E `8f24c37c643a968e51cc77aeb63d3ffe2f0a96e5`, including A. Upstream main was rechecked at `6bc71b310bc81a90f3ad32194ba8c17947bf79d6`. Parent: [response repair plan](../../plans/http-responses.md) and [reliability tracker](../../plans/provider-reliability-prs.md#f--response-handling).

## Scope and decisions

- [x] Fix F12 across discovery, login, JSON requests, and multipart uploads using one response validator.
- [x] Require HTTP 2xx and a single explicit success envelope; reject noninteger/missing/null codes, duplicate envelope fields, invalid/trailing JSON, and oversized bodies.
- [x] Reject redirects, avoiding method changes and forwarding API requests to another URL.
- [x] Return typed HTTP/API/protocol/transport errors; keep cancellation/transport causes available for programmatic inspection without printing raw URLs or transport messages.
- [x] Keep one expiry refresh/replay only for a valid HTTP 2xx code -1. Preserve matching header/query tokens, multipart replay, and A's concurrency/cancellation behavior.
- [x] Use a typed API code for Agile Series fallback; untrusted response text and HTTP errors cannot select that route.
- [x] Verify Terraform rejected-delete state retention and successful retry, and retain the complete E credential regression.
- [x] Pass formatting, vet, build, full unit, and full client race tests in Docker.
- [x] Pass combined A + E + F + D1–D3 full unit and 41 Terraform scenarios/205 steps with the six-assertion SSID diagnostic adjustment.
- [x] Finish final evidence checks and cleanup; retain all requested worktrees, logs, and the compatibility assertion patch.
- [x] Commit/push F as [c210dec](https://github.com/wbniv/terraform-provider-tplink-omada/commit/c210dec9a4bb10a15881f15a250b7c1843543d31); all nine tested file hashes match the commit. The pre-commit hook added `.history/README.md`. Controller validation and upstream publication remain separate gates.

The patch changes two production files, five test files, and two documentation files. It is stacked on E because the diagnostic contract deliberately changes E's maintained acceptance assertions. E production code is unchanged. D1–D3 remain independent branches; the retained compatibility adjustment changes test expectations only.

## Response contract and migration

| Response / operation | Verified policy |
| --- | --- |
| HTTP 401/403/404/429/5xx, including an apparent success envelope | Typed HTTP error; no API acknowledgment and no automatic authentication replay |
| HTTP redirect | Reject without following; configure the final controller URL directly |
| HTTP 2xx with integer code 0 | Accept the envelope; individual operations still validate their required result |
| Absent/null/empty `result` inside explicit success | Valid for writes that require no result; discovery/login still require usable identity/token |
| Empty HTTP body, including 204 | Reject; no endpoint-specific exception is established by current repository evidence |
| HTTP 2xx with nonzero API code | Typed API error; arbitrary server message excluded |
| Valid HTTP 2xx code -1 on authenticated request | One refresh and one replay; a repeated expiry fails |
| Response larger than 16 MiB | Reject after reading at most limit + 1 bytes; close body |
| Failed network DELETE | Error retains Terraform identity and remote object; retry with explicit success removes it |

Existing schema/state/import formats are unchanged. Configure final URLs instead of relying on redirects. Error text now provides numeric HTTP/Omada codes or fixed protocol/transport categories, excluding arbitrary bodies, headers, URLs, and server messages. Automation must stop depending on raw controller messages. The user-facing [API guide](https://github.com/wbniv/terraform-provider-tplink-omada/blob/c210dec9a4bb10a15881f15a250b7c1843543d31/docs/guides/api-responses.md) documents these changes.

Transport causes remain wrapped for `errors.Is`/`errors.As`; those underlying causes can contain URLs and should not themselves be printed as diagnostics. Error string handling at the shared request boundary is covered here; this does not claim to sanitize every resource-specific decoding or logging path.

A 404 response is not converted to semantic resource absence. A proxy/unsupported route can also return 404. API not-found mapping, pagination, and state removal on confirmed absence remain G/F09/F10. A failed or malformed response can follow a real remote mutation; F rejects an unreliable acknowledgment but cannot infer or roll back that remote outcome. E retains its accepted-write/readback-failure password recovery.

Go's [HTTP client contract](https://pkg.go.dev/net/http#Client.Do) leaves status inspection to callers and defines redirects; [typed error inspection](https://pkg.go.dev/errors) permits classification without parsing message text. HashiCorp's [Delete contract](https://developer.hashicorp.com/terraform/plugin/framework/resources/delete) retains resources when deletion returns an error. The tests verify this provider's integration.

## Environment and verification

Go `1.25.6 linux/amd64` and Terraform `1.15.8 linux_amd64` run inside Docker container `omada-http-responses-fix`, using `golang:1.25.6@sha256:06d1251c59a75761ce4ebc8b299030576233d7437c886a68b43464bad62d4bb1`. Terraform was copied from `/home/will/.local/bin/terraform` into `/usr/local/bin/terraform` in the container. The F worktree is mounted read-only at `/src`; writable baseline/review/compatibility copies are under container `/tmp`. No host Go installation, real controller, or cloud account was used. All credentials and responses are synthetic.

| Gate | Result | Evidence |
| --- | --- | --- |
| E baseline behavior | 149 failures, 65 passes among 214 cases | [Baseline response log](baseline-response.log) |
| F behavior | All 214 pass; existing client tests also pass | [Patched client log](patched-client.log) |
| F typed errors/read bounds | All nine pass | [Typed checks](typed-errors.log) |
| E baseline Terraform | All five new scenarios fail | [Baseline acceptance](baseline-acceptance.log) |
| F + E + D1–D3 (includes A) | Full unit suite and all 41 Terraform scenarios/205 steps pass, including five F scenarios/20 steps and 12 E scenarios/65 steps | [Combined compatibility](combined-compatibility.log) |
| Full unit suite | All packages pass; client coverage 44.4%, resources 11.7%, provider 2.9% on A + E + F | [Full gates](full-gates.log) |
| Formatting / static / build | Empty `gofmt -l .`, `go vet ./...`, `go build ./...`; exit 0 | Same container gate |
| Race | Full client suite passes, including A's authentication/concurrency/cancellation and F response cases | [Race log](race.log) |

The 214 behavior cases comprise 150 status/envelope cases across DELETE, PATCH, discovery, login, and multipart; three empty-result cases; 25 redirects; 25 transport/read/cancellation/URL cases; six expiry/retry cases; two size-boundary cases; and three missing-controller-ID cases. Nine additional cases inspect typed/wrapped errors and exact read bounds/body closure. The two behavioral regression files are byte-identical between the baseline and final F sources; type-specific tests use F's new types and run only on F.

Five new Terraform scenarios cover 403 JSON, HTTP 500 with a success-looking envelope, HTTP 200 missing code, HTTP 200 null envelope, and HTTP 502 HTML. Each has four steps: import, rejected destroy, an unchanged plan with identity/remote-state checks, and acknowledged destroy. Four baseline cases fail to return the required error; the 502 case already errors but reports the old raw-body diagnostic. A failed baseline can terminate before later steps. Baseline cleanup warnings concern in-memory fixtures only; no live object is left behind.

## SSID compatibility adjustment

The first combined unit run found three D1 assertions requiring the literal server message `fixture failure`; [initial compatibility log](compatibility-initial.log). D3 also has three acceptance assertions requiring raw messages for failed create/delete and duplicate names. The recorded [six-assertion patch](ssid-diagnostic-assertions.patch) changes only these expected substrings to `API error -1001`, retaining all payload, state, ordering, and retry assertions. F's two E acceptance expressions similarly use the numeric code; these E test changes are included in the F code worktree.

- [ ] Include the retained SSID test adjustment when combining D1–D3 with F for upstream submission. The original D worktrees are unchanged; combined results include this explicit adjustment and must not be represented as completely unmodified D tests.

## Reproduction commands

Baseline `/tmp/baseline` is an archive of E `8f24c37`, with only `internal/client/response_behavior_test.go` and `internal/provider/http_response_test.go` overlaid from F. Patched `/tmp/review` is F's final worktree copy. Run the shared behavior and Terraform regressions in both; run the remaining gates on F:

```sh
go test ./internal/client -run TestResponse -count=1 -v -timeout=120s
TF_ACC=1 TF_ACC_TERRAFORM_PATH=/usr/local/bin/terraform go test ./internal/provider -run TestAccHTTPResponseDelete -count=1 -v -timeout=120s
test -z "$(gofmt -l .)"
go vet ./...
go build ./...
go test ./... -count=1 -cover
go test -race ./internal/client -count=1 -v -timeout=180s
```

For the compatibility checkout, copy F, apply the D1–D3 diff `git diff 6bc71b3 ec2d949` after `git apply --check`, then apply `ssid-diagnostic-assertions.patch`. No D production code is edited during combination. Run:

```sh
go test ./... -count=1
TF_ACC=1 TF_ACC_TERRAFORM_PATH=/usr/local/bin/terraform go test ./internal/provider -run 'TestAcc(HTTPResponseDelete|SiteCredentials|WirelessNetwork)' -count=1 -v -timeout=300s
```

Live acceptance remains gated off in ordinary unit tests. Mock provider acceptance is an explicit separate run; current CI does not run these scenarios. Formatting/vet/build emitted no errors; their successful exit status is recorded here alongside unit logs.

## Tested file hashes

All nine final host/worktree hashes match `/tmp/review`. SHA-256:

| File | SHA-256 |
| --- | --- |
| `README.md` | `5a3d5db42aaded211f69db6c3cca0078a9272e10e4bc7356fe17b7adced42de5` |
| `docs/guides/api-responses.md` | `e54ca11b6c8e06221495da92d3d40f2ccbe39fa30c3c4902e580d411b449435f` |
| `internal/client/client.go` | `443d70e982a3b588ed85eac4db00e931f85b5cae42ffcd41cb1b7689f1e342b8` |
| `internal/client/client_test.go` | `8c591af5a6cd168349c0336f87af840614f26e9fcac1aebcb911c11683d510a4` |
| `internal/client/response.go` | `712489a7c4cd8259e5b4eb1fb9afa0d83625b4f75d17b71055cf9d1efa614b58` |
| `internal/client/response_behavior_test.go` | `5e90dd3e937db8121ce44c9fbae96bed71257eb5b381a278b92e28ff95a4f285` |
| `internal/client/response_errors_test.go` | `f82caec2ccc14522a0dbc622f884373f33fae1c7e5bafffea9b71a0c813b806f` |
| `internal/provider/http_response_test.go` | `f7b78c53da041cf3c8db23bd4de5a94208b59fa0ebb05bf8869c5993225cdc2b` |
| `internal/provider/site_credentials_test.go` | `a78ec86d7afa61fb7e504f0979bc4024cffc1badea81a1c1c5cb56bcddd918d6` |

## PR package and remaining gates

Suggested title: `fix(client)!: validate HTTP responses and protect error diagnostics`.

Suggested breaking-change note: API redirects and nonstandard/empty response envelopes now fail; use the final controller URL. Diagnostics omit raw controller messages and response bodies; use numeric status/error codes. HTTP 401/403 expiry responses do not trigger automatic replay. Schema/import formats remain unchanged.

- [x] Record E dependency, exact versions, baseline failures, passing local checks, source hashes, policy decisions, and migration notes.
- [x] Recheck contribution and AI-attribution requirements: conventional commits, fmt/vet/test/docs, and hardware declaration are expected. No explicit AI-attribution requirement was found; no session metadata was guessed.
- [x] Validate final documentation links and all nine source hashes; remove the temporary container and host archive/patch after saving evidence. All six prior code worktrees remain clean. Combined compatibility is complete. Link checking found one existing broken `README.md` → `LICENSE` reference on the baseline; it is recorded separately in the tracker and unchanged by F.
- [x] Commit/push F as [c210dec](https://github.com/wbniv/terraform-provider-tplink-omada/commit/c210dec9a4bb10a15881f15a250b7c1843543d31); all nine tested hashes match the committed files and the fork ref is verified. Evidence is maintained on `docs/provider-reliability-review`.
- [ ] Validate actual HTTP/envelope/expiry/empty-body behavior, redirects/proxies, response sizes, and failed-write recovery on controlled hardware.
- [ ] Apply the recorded SSID test assertion patch when integrating the stacks; preserve its separate scope.
- [ ] Recheck upstream/attribution requirements before publication; record PR, CI, reviews, and merge separately. No upstream PR or comment has been posted.
