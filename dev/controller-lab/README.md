# Real Omada controller lab

This runs TP-Link's real Software Controller **6.1.0.19** with zero adopted devices. It is an opt-in compatibility test, with temporary sites/networks and synthetic credentials. It can replace the lab's HTTPS certificate. Run it on a development machine with Docker Engine/Compose, Python 3.11+, Git LFS, and a Linux amd64 Terraform **1.15.8** binary. Run `git lfs pull` to obtain the vendored controller archive. Go and Terraform execute in Docker.

## Isolation and versions

- Controller: community `mbentley/omada-controller` packaging, pinned in `compose.yaml`. This is not an official TP-Link image. The first investigation verified all **533 bundled application JARs** against TP-Link's official 6.1.0.19 Linux archive.
- Controller limits: 2 GiB RAM, two CPUs, JVM 128–768 MiB, MongoDB cache 256 MiB. The test runner has a separate 2 GiB/two-CPU limit. Allow additional disk space for images and Go compilation.
- Only `https://127.0.0.1:18043` is published. The fixed-destination Python relay passes TLS to `controller:8043`; it does not decrypt or log application traffic. No device-discovery/adoption ports are published. The controller has only an internal Docker network.
- Fresh named volumes belong to Compose project `omada-provider-lab`. Cloud binding, notifications, firmware checks, and telemetry are disabled during initialization.
- The Go 1.25.6 runner uses the checked-in `vendor/` modules with `GOPROXY=off`, `GOSUMDB=off`, and `GOTOOLCHAIN=local`. Compilation and tests run on the internal network only. The source checkout is mounted read-only and copied into the disposable container.
- The loopback console uses a self-signed certificate. TLS verification is disabled only for this disposable target. No production URL/account is accepted by the harness.

The pinned controller image currently includes OpenJDK 17.0.20.1 and MongoDB 8.0.32. Package and image hashes belong in the accompanying investigation, not an assertion that the community image itself is an official distribution.

## Run

From the checkout:

```sh
dev/controller-lab/run.sh up
dev/controller-lab/run.sh probe
dev/controller-lab/run.sh test
dev/controller-lab/run.sh down
```

`up` verifies the vendored Omada archive, waits up to five minutes, creates a synthetic local administrator and one site, and is safe to repeat for the same initialized lab. It compares every controller JAR with the vendored package and saves `provenance.json`. Credentials are generated into ignored `.controller-lab/credentials.json`; the directory is private and credential files have mode 0600. An existing configured controller without matching local credentials is rejected.

`test` defaults to `$HOME/.local/bin/terraform`. Set `TERRAFORM_BINARY` to another Linux amd64 binary if needed; the runner verifies version 1.15.8 before execution. It builds the **current working tree**, not just its committed HEAD. Logs and test exit statuses go into `.controller-lab/`. Preserve the source hashes from `build.log` with each result.

For the review stack used in the first investigation:

```sh
LAB_SSID_INTEGRATION=1 \
LAB_EVIDENCE_DIR=/home/will/terraform-provider-tplink-omada/docs/investigations \
dev/controller-lab/run.sh test
```

This adds the exact D1–D3 stack and the three recorded integration patches to the disposable build copy. The checkout needs those Git commits; `LAB_EVIDENCE_DIR` needs the pagination/HTTP evidence patches. Without this option, the runner tests its current checkout alone. The acceptance selection includes D1–D3 mock SSID scenarios when integrated, and a real secured-SSID lifecycle. The L6 overlay adds a nine-step PMF scenario covering open creation, rename, open/secured transitions, and secured modes 1–3. The L7 overlay excludes verified response/create-only fields; its tests cover secured rename, password rotation, and custom/default VLAN transitions with controller readback. Set `LAB_ACCEPTANCE_RUN` and `LAB_CLIENT_RUN` to Go test regular expressions for focused reruns; defaults run the full lab selection.

**Known failures are reported as failures.** The initial G1-based run discovered WLAN pagination, certificate upload, session recovery, and IP-group compatibility issues. Do not interpret a completed test command as a passing compatibility declaration.

## Test scope

- Existing Terraform acceptance tests: eight data sources, network create/update/import/destroy, and profile create/update/destroy.
- Additional Terraform scenario: site bootstrap, settings credential ownership/rotation, preservation of selected unrelated sections, and a stable final plan.
- Client probes: ten collections; rejected local-session recovery; creation/listing/lookup/cleanup of 101 temporary networks; site create/read/update/delete and the response to a second delete; certificate upload/activation/readback/TLS presentation.
- Diagnostic certificate comparison: multipart metadata and response identity names taken from the bundled UI. This uses a separate raw request and **does not establish that the provider's upload methods work**.
- API probe: sanitized page-one/page-two metadata, duplicate identity counts, and empty SSID behavior. It logs no response values or credential bodies.

The `.go.txt` files are copied into the corresponding internal packages only inside the test container. Ordinary `go test ./...` does not start this lab. The lab tests also require `OMADA_CONTROLLER_LAB=1` and the exact internal URL.

## Evidence and cleanup

Inspect logs before copying them into `docs/investigations/`. Never publish `credentials.json`, `runner.env`, `last-response.json`, Terraform state, cookies, private keys, or full settings responses. Initial passwords are synthetic but still private runtime material.

`test` removes its runner on exit, including failures. `down` removes the controller, relay, owned networks/volumes, and private credential/response files; it preserves logs for review. After saving sanitized evidence, remove the remaining `.controller-lab/` directory. Docker images remain reusable cache. Do not run a broad Docker prune.

Passing controller tests does not validate physical adoption, AP radios, switch-port behavior, traffic forwarding, or delivery of credentials to hardware. Record those as separate open gates.

For an uncommitted SSID follow-up, set `LAB_SSID_PATCH` to a Git patch in addition to `LAB_SSID_INTEGRATION=1`. The harness copies it into the private runtime directory and applies it after D1–D3 and the client integration patches. Save the exact patch and compiled source hashes with the test evidence. Each test invocation clears the previous build/acceptance/client logs and status file so an interrupted build cannot retain stale successful results.

Device passwords are generated with the wizard's verified constraints: strong character classes, length 10–64, and no adjacent identical characters. The earlier shorter hexadecimal candidates could violate the repeated-character rule.

## Certificate protocol check (L2)

Run the focused lifecycle, then explicitly restart the disposable controller and verify its TLS certificate:

```sh
LAB_ACCEPTANCE_RUN='^TestControllerLabCertificateResource$' \
LAB_CLIENT_RUN='^TestControllerLabCertificate$' dev/controller-lab/run.sh test
python3 dev/controller-lab/verify-certificate.py
```

The Terraform scenario covers create, a stable plan, rotation, another stable plan, and import. The client probe records only a public certificate fingerprint. The restart verifier compares that fingerprint with a fresh TLS handshake and checks stored identity persistence. Software Controller 6.1.0.19 converts PEM uploads to JKS identities and requires a restart to serve the certificate; the production provider reports this requirement and leaves restart scheduling to the operator. Run this sequence before `down`.

## Session recovery check (L3)

```sh
python3 dev/controller-lab/session-probe.py
LAB_ACCEPTANCE_RUN='^TestControllerLabCertificateResource$' \
LAB_CLIENT_RUN='^TestControllerLabReauthentication$' dev/controller-lab/run.sh test
```

The raw probe disables redirect following for every opener and records only sanitized response shapes. Live cases cover missing cookies, invalid tokens, logout, multipart replay, and eight concurrent reads. The client now also runs under Go's race detector during the normal lab build gates. The new HTTP 302 recovery rule is limited to controller 6.1.0.19 and the exact same-origin login destination, with one shared reauthentication attempt and at most one replay per request. No redirected API request is followed. Natural timeout and other controller versions need separate verification.

## IP/Port group check (L4)

```sh
python3 dev/controller-lab/ip-group-probe.py
LAB_ACCEPTANCE_RUN='^TestControllerLabIPGroup$|^TestAccIPGroupCreateReadbackFailure$' \
LAB_CLIENT_RUN='^TestControllerLabIPGroups$' dev/controller-lab/run.sh test
```

The raw probe inventories group types and response formats, then creates and removes synthetic groups and a disabled ACL. The live tests verify the Terraform lifecycle and data source, listing the complete supported group population (up to the controller quota or 101 fixtures), and retained ACL references after a group update. A mock Terraform scenario checks accepted creation followed by readback failure and recovery. All temporary groups and ACLs are cleaned up. These tests do not verify traffic enforcement.

ACL creation's empty acknowledgment is a separate L8 finding: the raw client proves references can be stored, but the existing Terraform ACL create decoder still needs correction.

## Empty collection check (L5)

```sh
LAB_ACCEPTANCE_RUN='^TestAccCollectionEmptyState$|^TestControllerLabEmptyCollections$|^TestControllerLabIPGroup$' \
LAB_CLIENT_RUN='^TestListDevices' dev/controller-lab/run.sh test
```

The seven-step mock Terraform scenario covers all eight collection data sources, empty and populated states, nested lists, populated-to-empty refresh, HTTP failure, malformed response rejection, and recovery. Outputs exercise `length()` and iteration. The live scenario checks empty device, SSID, ACL, IP/Port-group, and mDNS inventories alongside populated sites, networks, and port profiles, followed by a stable plan. The existing IP/Port-group lifecycle retains populated-list coverage. No physical device behavior is asserted.
