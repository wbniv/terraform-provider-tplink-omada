# ES216G Terraform acceptance plan

**Goal:** verify that this exact ES216G can be adopted by Omada Software Controller and managed through this Terraform provider. The planned live acceptance is complete. The controller time zone and switch management mode/inform target were changed as part of setup; port 8's temporary test name was restored. A pre-adoption port-configuration snapshot is unavailable, so adoption side effects cannot be compared. No configuration backup was made, per Will's instruction.

## Execution status — 2026-10-28

- The dedicated Omada Software Controller 6.1.0.19 container is running at `192.168.4.24:8043`; cloud binding is disabled.
- The switch's standalone **System → Controller Settings** page was used to set **Inform URL/IP Address** to `192.168.4.24`; **Cloud-Based Controller Management** remained disabled and **Remote Adoption** enabled.
- Adoption first failed because the controller-generated site device-account credentials in `.es216g-lab/credentials.json` were not the switch's current standalone login. Retrying with the credentials used to sign in to `https://192.168.4.22` succeeded.
- The controller UI reports the ES216G as **CONNECTED**. The provider lab's device-list query reports MAC `D4-D6-DF-1C-3B-58`, model ES216G, firmware `1.0.1 Build 20260329 Rel.14489`.
- The success screen is preserved in the [ES216G operator instructions](../instructions/es216g-lab.md#successful-adoption).
- The provider's F15 worktree (`fix/switch-omitted-ports-plan`, commit `286a9a2`) imported the switch successfully using `<site ID>/<MAC>`. Two consecutive refresh/plans with `ports` omitted reported **No changes**. Terraform state and the temporary fixture are under `/tmp/es216g-terraform-acceptance`, outside the repository.
- Will confirmed the current physical map after moving cables: ports 8–15 are physically empty; port 16 is the uplink; all seven Lenovo cables are connected, but only one Lenovo was powered on during testing. The most recent controller read showed link up on ports 2, 4, and 16; link state alone does not identify every cabled host.
- Port 8's display name was changed to `Terraform acceptance test`, read back, and restored to `Port8`. Comparing the controller's writable port configuration before and after found no other port changes. Loopback and STP readback remained `true` and `0`. Plans after both the change and restore reported **No changes**.
- Terraform destroy removed the temporary state entry only; the ES216G remained listed in the controller. The controller stays running at `192.168.4.24:8043` for Terraform use. The imported Chrome credential CSV was securely deleted.
- The live import and reversible port acceptance passed. A pre-adoption port-configuration snapshot was not captured; no configuration backup was made, per Will's instruction.

## Decision

Use a dedicated Docker Compose project for the physical-device run. Reuse the already verified, digest-pinned Omada Controller 6.1.0.19 community image from the controller lab. Docker here is rootless, so its `host` network is not the workstation's LAN network namespace and cannot receive the switch's discovery broadcast. Use bridge networking with controller ports published only on the workstation's current LAN address; set the switch's standalone Controller Inform URL to that address before adoption. Keep the existing isolated controller lab unchanged: it uses an internal-only bridge and loopback console, so it cannot discover or adopt physical devices.

The image is community packaging of TP-Link's controller, not an official TP-Link container. The controller JARs in the pinned image were compared with TP-Link's official 6.1.0.19 archive in the existing lab evidence. A supported Ubuntu 24.04 VM is the fallback if the container's LAN discovery or host-network behavior fails; the host itself runs Ubuntu 26.04, which TP-Link does not list for the 6.1.0.19 Linux package.

## Device and controller facts

- Switch: TP-Link Omada ES216G, hardware revision 1.20, firmware 1.0.1 Build 20260329 Rel.14489, management IP `192.168.4.22`.
- Current mode: adopted by the physical-test Omada Controller 6.1.0.19 and reported **CONNECTED**. Before this run it was managed through its standalone web UI.
- TP-Link documents both standalone and controller modes for the ES216G. Its firmware notes recommend Omada Controller 6.1.0 and say adoption retains some standalone settings, such as the management IP and management VLAN. The installation guide still says to reconfigure the switch when changing modes; do not assume all standalone port settings will carry across.
- Provider target: Omada Software Controller 6.1.0.19, matching the controller version used by the verified provider lab and the switch firmware recommendation.
- Terraform import ID: `<site ID>/<switch MAC>`.
- The switch is carrying the wired lab and its uplink. Identify the uplink and choose an unused, non-uplink test port before any write test.

## Lab automation

The separate `dev/es216g-lab/` harness is in place; the existing `dev/controller-lab/` isolated acceptance harness remains untouched.

- `compose.yaml`: the existing controller image pinned by the recorded digest; bridge networking with explicit LAN-IP-only port mappings; separate labeled persistent data/log volumes; bounded CPU/RAM; no cloud binding.
- `run.sh`: `up`, `status`, `devices`, `environment`, `credentials-csv`, `shred-credentials-csv`, and `down` commands. `down` stops this controller only and preserves its data volume. Automated `preflight` and Terraform `test` commands are not implemented yet.
- Private runtime directory: ignored by Git and mode `0700`; generated controller credentials stored in a mode `0600` env file, never in command arguments or logs.
- Network preflight: confirm the host has a `192.168.4.x` address, can reach `192.168.4.22`, and has no conflicting controller ports. Limit controller UI and device ports to that LAN address. Do not add WAN port forwarding.
- Required device traffic: UDP 29810 for discovery and TCP 29811–29817 for controller management/adoption, as required by the controller version/device firmware. TCP 8043 is the local HTTPS console and provider API. Restrict these to the home LAN; do not broadly expose them.
- Initialization: one local controller account and one test site were created; cloud binding is disabled. The private runtime file contains controller credentials and the controller's site device-account credentials. Those generated site credentials are not the existing standalone login that the switch required during adoption.
- Terraform fixture: use a temporary directory and environment variables for controller URL/account. Never write credentials into the repository or Terraform files.

The existing verified controller image reference is `mbentley/omada-controller:6.1.0.19@sha256:94571a6544a5db9419681d3cecdaa7dccf9042e586cae9a1c34c3f22994e6b38`. Its source and verification are recorded in [the controller lab plan](software-controller-lab.md) and [lab evidence](../investigations/software-controller-lab/README.md).

## Test topology

```mermaid
flowchart LR
    TF[Terraform provider test] -->|HTTPS API 8043| OC[Omada Controller 6.1.0.19 container]
    PC[Browser on workstation] -->|HTTPS console 8043| OC
    OC <-->|Discovery UDP 29810<br/>Management TCP 29811–29817| SW[ES216G 1.20<br/>192.168.4.22]
    SW ---|LAN switching| HOSTS[Wired lab hosts]
    SW ---|Uplink| GW[Gateway]
    OC ---|Published TCP/UDP ports on 192.168.4.x| LAN[(Home LAN)]
    TF --- LAN
    PC --- LAN
```

```mermaid
flowchart TD
    A[Start dedicated physical-test controller] --> B[Initialize one local site]
    B --> C[Adopt only the ES216G]
    C --> D[Terraform import]
    D --> E{Stable plan with `ports` omitted?}
    E -->|No| X[Record failure; stop]
    E -->|Yes| F[Change one unused non-uplink port name]
    F --> G[Controller readback and stable plan]
    G --> H[Restore original port name]
    H --> I[Keep controller running for Terraform use]
```

## Execution sequence

### 1. Build the isolated physical-test controller

- [x] Add the dedicated Compose project and runner described above.
- [ ] Validate that `up`, `status`, and `down` touch only resources carrying the new project's ownership label.
- [x] Start the controller without cloud binding and confirm its console at `https://192.168.4.24:8043`.
- [x] Bind published controller ports to the workstation's LAN address. A separate port-reachability/security scan remains unverified.
- [x] Confirm the host can ping the switch and that the switch and controller are on the same subnet.
- [x] In the switch's standalone Controller Settings, set its Inform URL/IP Address to `192.168.4.24`; this replaces broadcast discovery that rootless Docker cannot receive.

### 2. Adopt the ES216G

- [x] Capture read-only device identity from the controller: MAC, model/revision, and firmware. The controller reports 16 configured ports named `Port1` through `Port16`, all enabled with profile `All`.
- [x] Capture current controller link state after Will moved cables: ports 2, 4, and 16 report link up. Earlier observations are historical and were superseded by the cable move.
- [x] Record Will's physical map: ports 8–15 are empty and port 16 is the uplink; the seven Lenovo cables remain connected, with one Lenovo powered on during this test.
- [x] Confirm port 8 is unused and non-uplink from the physical map before writing.
- [ ] Compare all controller settings with a pre-adoption standalone snapshot. None was captured before adoption; do not infer that adoption preserved every standalone setting.
- [x] Create the test site and adopt only the discovered ES216G. The first attempt with controller-generated site device-account credentials failed; retrying with the switch's current standalone UI credentials succeeded.
- [x] Confirm the controller reports the ES216G as connected. Record the MAC, model, and firmware above.
- [x] Reconcile the current controller port view against Will's physical cable map; identify uplink 16 and empty test port 8. A pre-adoption standalone configuration snapshot was not captured.

### 3. Verify Terraform read/import behavior

- [x] Configure the F15 worktree provider against the controller's LAN address and create a temporary resource block for the ES216G.
- [x] Import using `<site ID>/<MAC>`.
- [x] Omit the whole `ports` attribute and require a stable, empty second plan. Two consecutive plans were empty on the real controller/device path, with no port configuration changes.
- [x] Refresh preserved the controller-reported device identity and configured port state.

### 4. Verify a reversible port update

- [x] Configure all 16 ports from the controller snapshot, preserving every writable field; explicitly preserve loopback detection and STP service settings.
- [x] Change only the display name on verified-empty port 8 from `Port8` to `Terraform acceptance test`. Controller readback showed the requested value, no other writable port setting changed, and the next plan was empty.
- [x] Restore the original name `Port8`; verify readback, all port settings against the pre-test snapshot, and an empty next plan.
- [x] Display-name writes succeeded on this controller/device combination; no VLAN/PVID substitute was needed.
- [x] Verify `terraform destroy` removes only the temporary Terraform state entry; the switch remains adopted in the controller.

### 5. Record and stop

- [x] Record controller version, switch model/revision/firmware, import and plan results, request outcome, and selected test port 8. No credentials are included.
- [x] Keep the physical controller profile separate from existing mock/controller acceptance results.
- [x] Leave the controller running with its data volume retained so the switch remains available for Terraform use. `dev/es216g-lab/run.sh down` can stop it without deleting controller data.
- [x] Update the reliability tracker with the passing live ES216G results; retain the tested model, revision, firmware, and controller-version limits.

## Safety and limits

The test changed only port 8's display name and restored it. The port was physically empty; port 16 is the uplink. Avoid the uplink and ports in use by the seven wired lab hosts. Do not change VLAN membership, PVID, port profile, or link state in this acceptance run.

This plan intentionally omits a configuration-backup step at Will's request. If adoption appears to have changed port configuration, record the observed state and stop before further changes until its effect is understood.

Passing this sequence establishes compatibility only for ES216G hardware revision 1.20, firmware 1.0.1 Build 20260329, and Omada Controller 6.1.0.19. It does not establish support for other hardware revisions, firmware versions, or controller releases.

## References

- [TP-Link ES216G V1.20 firmware 1.0.1 release notes](https://static.tp-link.com/upload/firmware/2026/202604/20260428/ES216G%28UN%29_V1.20_1.0.1_Release%20Note.pdf)
- [TP-Link ES216G installation guide](https://static.tp-link.com/upload/manual/2024/202410/20241022/7106511600_ES220GMP%2CES228GMP%2CES216G%2CES224G%28UN%29_IG.pdf)
- [Omada Software Controller downloads](https://support.omadanetworks.com/en/download/software/omada-controller/)
- [Omada Software Controller Linux installation guide](https://support.omadanetworks.com/en/document/13088/)
- [Omada device discovery and adoption ports](https://support.omadanetworks.com/cac/document/13223/)
