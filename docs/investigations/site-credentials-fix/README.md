# E — Site device credentials and settings preservation

Real-controller follow-up (2026-10-19): the [6.1.0.19 lab](../software-controller-lab/README.md) passes a four-step Terraform bootstrap/settings-ownership/rotation/stable-plan scenario and preserves the checked unrelated settings sections. Delivery of credentials to physical devices remains unverified.

Prepared for Will on 2026-10-19. Implemented, locally verified, committed, and pushed as [8f24c37](https://github.com/wbniv/terraform-provider-tplink-omada/commit/8f24c37c643a968e51cc77aeb63d3ffe2f0a96e5) on `fix/site-credentials` in `/home/will/terraform-provider-tplink-omada-credentials`. Base: authentication A `52ed681674e5a0058ae7807fcd08399c21fa021a`; upstream `main` remains `6bc71b310bc81a90f3ad32194ba8c17947bf79d6`. Parent: [credential repair plan](../../plans/site-credentials.md) and [PR tracker](../../plans/provider-reliability-prs.md#e--credentials).

## Scope and decisions

- [x] Resolve F05 by making site credentials creation-only. Reject changed credentials on an existing site with instructions to use `omada_site_settings`; do not replace a site for rotation.
- [x] Resolve F06 by retaining the newly acknowledged password in settings state, including when subsequent readback fails.
- [x] Require both explicit settings credential inputs. Never reuse API-returned passwords, masks, or the old state secret in a credential request.
- [x] Validate site bootstrap and settings pairs before writes, with unknown references deferred to apply.
- [x] Resolve F16, found during E: preserve unknown unrelated settings instead of serializing them as zero values.
- [x] Verify direct lifecycle cases, actual Terraform planning/apply against mock controllers, full Go gates, and combined A + D1–D3 + E behavior.
- [x] Update both resource docs, generation templates, Terraform examples, and import examples.
- [x] Commit/push E as [8f24c37](https://github.com/wbniv/terraform-provider-tplink-omada/commit/8f24c37c643a968e51cc77aeb63d3ffe2f0a96e5), with its breaking-change note; the remote branch matches and all 15 committed hashes match the tested sources.
- [ ] Validate on controlled hardware and prepare upstream submission when authorized.

E deliberately depends on A so its new settings reader/writer use the repaired authentication helper. It changes four production files, two test files, one synthetic fixture, and eight documentation/template/example files. It does not add dependencies or change CI.

## Behavior and migration

| Operation | Verified behavior |
| --- | --- |
| Create a site | Require a known, nonempty username/password pair before POST; creation failures before POST leave null state. |
| Change credentials on an existing site | Planning error with settings migration instructions; no site replacement or credential write. |
| Remove both site credential inputs | Remove them from state without an API mutation when metadata is unchanged. A partial pair is rejected. |
| Import site settings | Read the actual username; leave password null regardless of plaintext, mask, or omitted API password. Import ID is the site ID. |
| Rotate settings username, password, or both | Send the full explicitly configured pair; retain the accepted configured password. The unchanged counterpart must remain configured. |
| Remove both settings credential inputs | Relinquish ownership without changing remote credentials; remove optional password from state after apply. |
| Refresh settings | Retain the existing managed password; never replace it from an API response. Remote password drift cannot be detected this way. |
| Discovery/PATCH rejected | Preserve prior state; report an error. Failed settings adoption before acknowledgment leaves null state. |
| PATCH acknowledged, readback fails | Save accepted values and the new password with an error. A retry/refresh can settle the plan without losing the accepted secret. |
| Readback username/settings disagree | Report an error and save the observed nonsecret values with the accepted secret. |
| Credential-only update | Send the `deviceAccount` section and leave unrelated sections untouched when their planned values match the snapshot. |

Migration for existing sites:

1. Remove both credential inputs from `omada_site`; retain its other configuration.
2. Manage one `omada_site_settings` resource for that site. Import it using the site ID, or adopt it through its existing create behavior.
3. Configure both desired credential inputs on settings. Review the full plan before applying.
4. Apply the in-place settings update and verify the next plan is empty.

There is no cross-resource uniqueness enforcement: configurations must still use one settings owner per site. Site bootstrap fields may remain unchanged until ownership is transferred, but changing them now errors instead of falsely reporting success. No schema type/version change or re-import is required. The settings password changes from Optional+Computed to Optional+Sensitive, so removing its configuration now removes the secret from state after apply. State snapshots/backups may still contain earlier secrets.

Existing noncredential schema defaults remain in effect and can propose changes during import/adoption. Configure intended values and review the entire plan; this patch does not redefine default ownership. API-returned or previously retained passwords are never reused in PATCH requests, even when they look like plaintext.

The [HashiCorp Update contract](https://developer.hashicorp.com/terraform/plugin/framework/resources/update) supports preserving state that reflects known applied changes after an error. Its [planning guidance](https://developer.hashicorp.com/terraform/plugin/framework/resources/plan-modification) explains why omitted computed settings can become unknown. Those contracts do not prove Omada firmware compatibility.

## New finding F16: unknown site settings become destructive zero values

**Priority: P1.** The old `planToSiteSettings` serializer builds every top-level section and calls `ValueBool`/`ValueInt64` on unknown values. A credential update can therefore send `meshEnable=false`, `autoFailoverEnable=false`, and `fastRoamingEnable=false` for unrelated settings that are enabled remotely.

`TestSiteSettingsCredentials/unknown_unrelated_settings` keeps the configured password unchanged to isolate this from F06, makes the three unrelated plan values unknown, and captures the outgoing payload. On A it reports “unknown settings were zeroed”; see [baseline lifecycle evidence](baseline-lifecycle.log). A's site/settings resource sources are identical to upstream `6bc71b3`. The patched test passes and sends no unrelated sections. Terraform scenarios additionally assert preservation and reject unrelated section writes.

The fix resolves unconfigured unknown values from a fresh snapshot, compares 49 mapped writable attributes, and emits only changed sections. Known writable siblings in a changed section are preserved; an unrecognized sibling stops the update before mutation. Read-only `site.key` is excluded. Unknown fields in an unchanged section are untouched because that section is omitted. The emitted destructive request is reproduced; connectivity consequences depend on controller acceptance and were not tested on hardware.

## Environment and verification

Go `1.25.6` and Terraform `1.15.8` (`linux_amd64`) ran in Docker container `omada-site-credentials-fix`, image `golang:1.25.6@sha256:06d1251c59a75761ce4ebc8b299030576233d7437c886a68b43464bad62d4bb1`. Terraform was copied from `/home/will/.local/bin/terraform` to `/usr/local/bin/terraform` inside Docker. The source worktree was mounted read-only at `/src`; baseline, patched, and compatibility copies were writable under container `/tmp`. No host Go installation was used.

Local `httptest` servers simulate Omada. All credentials, IDs, and responses are synthetic; no real controller or cloud account was used. The same final two test files and fixture were verified by SHA-256 on baseline and patch.

| Gate | Result | Evidence |
| --- | --- | --- |
| A baseline: direct lifecycle | 44 fail, four pass among 48 cases | [Baseline lifecycle](baseline-lifecycle.log) |
| A baseline: Terraform | All 12 scenarios fail | [Baseline acceptance](baseline-acceptance.log) |
| E: direct lifecycle | All 48 pass: 32 settings, eight existing-site ownership, eight bootstrap validation cases | [Patched lifecycle](patched-lifecycle.log) |
| E: Terraform | All 12 scenarios, 65 steps, pass | [Patched acceptance](patched-acceptance.log) |
| Full unit suite | All packages pass; client coverage 39.4%, resources 11.7%, provider 2.9% on A + E | [Unit suite](full-unit.log) |
| Formatting / static / build | Empty `gofmt -l .`, `go vet ./...`, `go build ./...`, and both modified Terraform examples' formatting checks pass | Container gates, exit 0 |
| A + D1 + D2 + D3 + E | Full unit suite and all 36 Terraform scenarios, 185 steps, pass | [Combined compatibility](ssid-compatibility.log) |

The 12 Terraform scenarios comprise four nine-step response variants (plaintext, absent, `***`, and `********`), adoption (three steps), unknown configured credentials (two), and six four-step sequences: rejected PATCH/retry, accepted PATCH/failed readback, site-to-settings ownership migration, site bootstrap, unknown site bootstrap, and partial credentials rejection. They inspect actual payloads, remote credentials, state secrets, site identity, absence of site replacement, preserved mesh/roaming settings, and stable subsequent plans.

Baseline failures can stop a scenario at its first assertion, often unrelated-settings preservation. They are not evidence that all later steps executed on the baseline. The direct cases independently isolate password state, preservation, rejection, redaction, and recovery. All 65 steps pass on E.

### Commands and source preparation

Baseline `/tmp/baseline` is A with only the final two E test files and `internal/testdata/site_settings.json` overlaid. Patched `/tmp/review` is the final E worktree copy. Reproduce the recorded lifecycle and acceptance checks in each checkout with these commands; run formatting/static/build/full-suite/example gates on E:

```sh
go test ./internal/resources -run 'Test(SiteSettingsCredentials|SiteCredentialOwnership|SiteCredentialBootstrapValidation)' -count=1 -v -timeout=120s
TF_ACC=1 TF_ACC_TERRAFORM_PATH=/usr/local/bin/terraform go test ./internal/provider -run TestAccSiteCredentials -count=1 -v -timeout=180s
test -z "$(gofmt -l .)"
go vet ./...
go build ./...
go test ./... -count=1 -cover
terraform fmt -check examples/resources/omada_site/resource.tf examples/resources/omada_site_settings/resource.tf
```

For compatibility, archive D3 `ec2d949699274c3a6dea6cf3d272f22bde13d12e` into a disposable directory, apply A's two-file diff (`internal/client/client.go` and `internal/client/auth_test.go`) from `6bc71b3` to `52ed681` with `git apply --check` followed by `git apply`, then overlay E's four production files, two test files, and fixture. Their hashes match the tested E copy. Run:

```sh
go test ./... -count=1
TF_ACC=1 TF_ACC_TERRAFORM_PATH=/usr/local/bin/terraform go test ./internal/provider -run 'TestAcc(SiteCredentials|WirelessNetwork)' -count=1 -v -timeout=300s
```

The ordinary unit run leaves live acceptance disabled. Mock acceptance is a separate recorded gate; existing CI does not run it. No race run was added because E changes no production concurrency behavior. A's existing race evidence is retained separately.

## Tested file hashes

SHA-256 of all 15 E files; host, patched container, and committed copies match. The baseline test/fixture copies and seven-file compatibility overlay also match.

| File | SHA-256 |
| --- | --- |
| `internal/client/site_credentials.go` | `7c5412371fdbba08ab1606676b4d9f6db714348627172eaebbc1ae8c55b4f670` |
| `internal/resources/site.go` | `94bb8835adba6a41533aad405a4c2154e7ee5ad26d0ff199c16e6347344c43e0` |
| `internal/resources/site_settings.go` | `2c30176a8f309e5cb0b4e7cba246f214936679e38df0cb61f5076d5a267eec9e` |
| `internal/resources/site_credentials.go` | `322f03e22f58e32543e0687d4a05e79b3310f9938079b42202420ba4861bccce` |
| `internal/resources/site_credentials_test.go` | `51ba7a6776d486e2742db55bfbd86fd6a931d7215c1fedf9684c47e8a2acfd7f` |
| `internal/provider/site_credentials_test.go` | `de127a34685e70d233a12c33121c0e0d81faae28ac69b8f8d945bcf2022c4a49` |
| `internal/testdata/site_settings.json` | `6529a356e00c8ce05dff8780bb88acc1820b9f8c754bbf026edf2e087084fb4b` |
| `docs/resources/site.md` | `4d167f3bec373825b2601d92c4c2483e468e7dff3ec535cc6af590df2c1df73a` |
| `docs/resources/site_settings.md` | `f55a0bb89376a5ac7689ae171b747cc3ad12be7177b5a7fedbb3c2347698730d` |
| `templates/resources/site.md.tmpl` | `895571c62bba945d50eeee087ccd80b8c67247fbb0570baa7c74a416fe3956fc` |
| `templates/resources/site_settings.md.tmpl` | `ba77bd1fb06501555d4ad7538354851cd57c5de43f9e9ca2e0d460f2d450d97c` |
| `examples/resources/omada_site/resource.tf` | `a7e198166f5fcb9e3aa674616074661a666b606d31237714b44521bac5f74243` |
| `examples/resources/omada_site/import.sh` | `22e17f90b226a3098cb8e07eb605f6b89c63989122b075393b4ee0db2ebccfa4` |
| `examples/resources/omada_site_settings/resource.tf` | `8531a6540da3fcbd7c1555ab41981f5b80128794d8601e9d04d05d38d3cb5398` |
| `examples/resources/omada_site_settings/import.sh` | `5a0fa11471940911d054de77d4d625a3a5bc7e90d22464244d2f701150a18f0b` |

## PR package and remaining gates

Suggested title: `fix(resources)!: make site credential rotation explicit and preserve settings`.

Suggested breaking-change note: site credential fields are creation-only; migrate rotations to one `omada_site_settings` resource with both credentials configured. Removing both settings credential inputs relinquishes ownership and removes the optional password from state. A partial pair now errors before writes. No site replacement or state migration is required.

- [x] Record A dependency, baseline/patch evidence, exact versions, commands, source hashes, and migration steps.
- [x] Recheck upstream main and contribution files at E start and before committing. Conventional commits, formatting/vet/tests/docs, and a real-controller declaration are expected. No explicit AI-attribution policy was found; no tool/model/reasoning metadata was guessed.
- [x] Finish documentation link/hash/status checks and remove the container and temporary compatibility archive/patch after saving evidence. Dedicated worktrees and saved logs are retained.
- [x] Commit/push E as [8f24c37](https://github.com/wbniv/terraform-provider-tplink-omada/commit/8f24c37c643a968e51cc77aeb63d3ffe2f0a96e5); verify the code remote ref. Plans and evidence accompany it on `docs/provider-reliability-review`.
- [ ] Validate partial PATCH and nested-section semantics, account pair/password-policy rules, readback/failure responses, and device credential propagation on a controlled controller. Keep the upstream hardware checkbox unchecked.
- [ ] Recheck current upstream and attribution requirements before publication; record PR URL, CI, reviews, and merge separately.

General HTTP/envelope classification and bounded diagnostics remain F12/work F. E redacts known old/new configured passwords from settings write errors, but does not claim to fix every client diagnostic path. Site POST-success/readback-failure identity recovery remains a separate concern. The settings raw reader checks for an object result and changed-section presence after a write; it does not establish a complete cross-version schema for every controller response. No upstream PR/comment has been posted.
