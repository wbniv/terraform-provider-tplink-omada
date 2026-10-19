# L2 — Controller certificate protocol

Work for Will on 2026-10-19. Branch: `fix/controller-certificate-protocol`, based on lab commit `c026124` (includes authentication, HTTP-response, pagination, and WLAN corrections). Parent: [controller compatibility plan](../../plans/controller-compatibility.md).

## Changes

- [x] Send filename metadata as `cerName` for certificates and `keyName` for keys using JSON encoding.
- [x] Decode `cerId` and `keyId` according to the upload endpoint. Reject absent, blank, wrong-type, generic-only, and opposite-endpoint identities.
- [x] Send activation under `certificate`, with `enable: true` and `cerType: "PEM"`; decode the nested section on readback. Reject empty activation identities and absent certificate sections.
- [x] Preserve authenticated multipart replay and existing read-only guards.
- [x] Verify Go gates and authentication/HTTP regression coverage in containers.
- [x] Verify real upload, activation, and settings readback on 6.1.0.19.
- [x] Verify Terraform create, stable plan, rotation, stable plan, and import (five steps).
- [x] Verify fresh TLS presentation after restart: exact SHA-256 fingerprint matches the uploaded certificate; stored IDs persist.
- [x] Record activation/restart behavior and any remaining limitations.

No private keys, credentials, cookies, Terraform state, or complete controller settings should be published with this evidence.

## Findings during verification

The upload-only correction passed all Go tests after updating two HTTP success fixtures, but failed the real activation/readback/TLS probe. The controller accepted the old flat `{cerId, keyId}` PATCH without applying it. The bundled UI serializes a nested `certificate` object with PEM type and enable flag; its settings read path also uses the nested object. Exact UI asset hashes are in `ui-files.json`.

Historical logs preserve the initial fixture failure and the upload-only live failure. Neither is a passing lifecycle result.

Activation also requires `cerName` and `keyName`: omitting them produces `-30027`. A separate OpenSSL probe isolated this requirement. On success, 6.1.0.19 converts the PEM certificate to JKS storage, so returned settings identities need not equal upload identities. The resource now reads settings after create/update to populate computed IDs. The bundled UI explicitly instructs operators to restart the controller for the certificate to take effect; the provider emits a warning and does not restart it.

## Verification scope

Go 1.25.6 and Terraform 1.15.8 ran in Docker against the pinned real Software Controller 6.1.0.19, with vendored dependencies and zero adopted devices. `go vet`, `go test ./...`, provider build, and both test binary builds passed. The focused final Terraform and client tests both returned zero. Other controller versions and hardware controllers are not validated by this run. No live SSID overlay was needed for the certificate tests.

The prior L6 work was committed and pushed before L2: production `a046a36`, lab `c026124`, documentation `3a9b68d`. L2 is committed as `4c21666` on `fix/controller-certificate-protocol`; upstream publication remains pending.

Final wrapper exit: 0. Restart verifier exit: 0. See `test-status.txt`, `acceptance.log`, `client-probes.log`, and `certificate-restart.log`. `tested-followup.patch` records tracked changes; `verify-certificate.py` is saved separately because it was untracked when the patch was captured. No upstream PR has been published.

Remaining prehardware work: L3 session recovery, L4 IP groups, L5 empty collections, and G2 confirmed-disappearance recovery. Hardware provisioning/radio/switch/traffic checks remain separate gates.
