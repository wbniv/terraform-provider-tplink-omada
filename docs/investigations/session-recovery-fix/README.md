# L3 — Lost-session recovery

Work for Will on 2026-10-19. Branch: `fix/controller-session-recovery`, based on L2 (`4c21666`). Parent: [controller compatibility plan](../../plans/controller-compatibility.md).

## Verified response shapes

The disposable Software Controller 6.1.0.19 produces:

| Case | Response | Policy |
| --- | --- | --- |
| Valid cookie/token | HTTP 200, JSON errorCode 0 | Normal success |
| Missing cookie, valid or invalid token | HTTP 302, single same-origin Location `/<controller-id>/login`, no query or fragment | Allow one reauthentication, then replay the original request |
| Invalid token with cookie | Same HTTP 302 login redirect | Same bounded recovery |
| Logged out with retained local cookie/token | Same HTTP 302 login redirect | Same bounded recovery |

No natural session timeout has been measured. Logout is not treated as evidence of natural expiry. The raw probe is opt-in and bound to the compose-owned lab. It records shapes and normalized paths, never cookies, tokens, HTML bodies, or credentials.

## Implementation and gates

- [x] Retain the controller version from discovery; enable the new redirect signal only for verified 6.1.0.19.
- [x] Require a protected API request on the configured origin and one exact login destination; reject query/fragment/userinfo, other paths, other statuses, duplicate Location headers, and unknown versions.
- [x] Keep redirect following disabled. Login uses the configured controller URL, never the response Location.
- [x] Use the existing bounded replay and shared authentication generation logic for JSON and multipart requests.
- [x] Run unit, vet, build, and extended authentication/HTTP gates.
- [x] Run real missing-cookie, invalid-token, logout, concurrent, and multipart probes.
- [x] Rerun the Terraform certificate lifecycle as a compatibility gate.
- [x] Preserve evidence and clean up the lab.

## Remaining limits

Generic HTML is not proof that replaying a mutation is safe and remains rejected. Missing cookies, invalid tokens, and logout all use the verified redirect. Natural expiry and other controller versions remain unverified. Keep these limitations visible when preparing upstream PRs.

## Probe correction

The first Python probe incorrectly installed its no-redirect handler alongside the default handler for one opener. It therefore followed invalid-token/logout redirects and reported the login page as HTTP 200 HTML. The first live Go test run recovered those cases successfully, disproving the probe's expectations. The raw probe now constructs every opener with redirect following disabled from the outset. Its final five-case log confirms HTTP 302 for all four rejected-session variants. The initial Go logs are retained: only the incorrect invalid-token/logout expectations failed; Go gates, Terraform compatibility, missing-cookie, multipart, and concurrent recovery passed.

## Final results

- [x] Go 1.25.6: vet, full tests, client race detector, provider build, and both test binary builds pass.
- [x] Six live cases pass: missing cookie; missing cookie plus invalid token; invalid token; logout; multipart upload; eight concurrent reads. Each case advances the authentication generation exactly once.
- [x] Terraform 1.15.8: certificate create, stable plan, rotation, stable plan, and import pass (five steps).
- [x] Final harness, acceptance, and client exit statuses are all zero.

All Go/Terraform execution used containers, vendored dependencies, and the isolated controller network. The exact tracked patch, untracked redirect test source, raw probe, compiled source hashes, and controller provenance are retained alongside this report. No private cookies, credentials, keys, or Terraform state are included.

L3 is locally verified and committed as `58610fd` on `fix/controller-session-recovery`. L4, L5, and G2 are now implemented and have separate evidence. Upstream publication remains pending; natural session expiry and applicable hardware validation remain explicit gates.
