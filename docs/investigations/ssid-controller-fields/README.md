# L7 — SSID update response fields

Prepared for Will, 2026-10-19. Plan: [controller compatibility](../../plans/controller-compatibility.md). Production commit: `ce80af6` on `fix/ssid-controller-fields`, based on D3 `ec2d949`. Lab harness commit: `1c4da91` on `fix/wlan-group-collection`. Lab integration uses L1 `0730101`, the established D integration patches, and the L7 diff.

## Controller evidence and policy

The real secured-SSID response contains three fields absent from D1's writable contract:

| Field | Controller UI evidence | PATCH policy |
| --- | --- | --- |
| `vlanEnable`, top-level `vlanId` | Advanced form serializes `vlanSetting`; default response has `vlanEnable: false` and mode 0; a custom response adds `vlanId` alongside authoritative nested VLAN data. | Exclude the summary field; retain authoritative `vlanSetting`. Verify custom/default readback in the live variant test. |
| `deviceType` | Basic form reads its target bitmask, disables target editing, and serializes it only for add mode. | Exclude from PATCH; verify existing target survives rename. |

UI source was extracted from the official pinned archive's `omada-web-6.1.0.19-local.jar`. `ui-files.json` records exact asset hashes. No remote documentation or inferred enum names are needed for this correction. The final production diff continues rejecting unrecognized fields.

The same advanced form omits `pmfMode` for open security. This supports further L6 investigation; L7 does not change PMF defaults.

## Checks

- [x] Capture a sanitized real response fixture and add regression assertions for response/create-only exclusions and unchanged writable settings.
- [x] Keep existing unknown root/nested field rejection tests.
- [x] Go vet/unit/build pass; all 24 D mock Terraform scenarios pass with the integrated stack.
- [x] Real secured SSID create/import/group replacement/rename/stable plan pass, with unrelated-settings readback.
- [x] Password rotation and VLAN assignment/clearing pass, with stable plans, unchanged SSID identity, exact synthetic-password readback, and VLAN summary checks.
- [x] Save reviewed evidence and remove the disposable containers, owned networks/volumes, private runtime files, and temporary UI/probe files. Cleanup exits zero.

## Harness observations

The previous device password generator was again rejected with API `-30105`. A 15-character value with punctuation was accepted. The revised generator guarantees upper/lowercase, digits, punctuation and a short length; this is a lab reliability adjustment, not a complete statement of controller password rules.

The first integrated run passed the secured rename lifecycle and all 24 D mock scenarios. The added variant test passed password rotation and VLAN assignment/stable plan, then failed clearing VLAN because custom responses include top-level `vlanId`. That summary field is now also excluded, and the final suite passes.

A direct real-controller probe created a nondefault device target (bitmask 1), omitted `deviceType`/`vlanEnable` on rename, and read back target 1 unchanged. `device-target.log` records this check. This establishes preservation beyond the default target value.

Exploratory raw requests to the default network and a network create missing required booleans were rejected; these were probe setup errors, not provider defects. The complete payload with a disposable VLAN network produced the sanitized custom-VLAN fixture and was cleaned up. No physical device or traffic behavior is claimed.

## Reproduce

Export the production worktree diff as a patch, then run from the lab worktree:

```sh
LAB_SSID_PATCH=/path/to/ssid-controller-fields.patch \
LAB_SSID_INTEGRATION=1 \
LAB_EVIDENCE_DIR=/home/will/terraform-provider-tplink-omada/docs/investigations \
LAB_ACCEPTANCE_RUN='^TestControllerLab(WirelessLifecycle|SSIDUpdateVariants)$|^TestAccWirelessNetwork' \
LAB_CLIENT_RUN='^TestControllerLabCollections/wlan_groups$' \
dev/controller-lab/run.sh test
```

This runs Go vet/unit/build and the focused Terraform/client selection. It does not claim that the full controller suite passes: L2–L6 and G2 remain open. Compilation and execution use the isolated internal Docker network and vendored Go dependencies.

The new custom-VLAN fixture initially compared response-only nested `currentVlanId`/`currentVlanIds` against the writable copy. That assertion was corrected to require their exclusion while comparing the writable VLAN fields. No production policy was broadened to satisfy the mistaken assertion.

## Final result and limits

`acceptance.log` contains 24 passing mock scenarios and two passing live scenarios (12 live Terraform steps). `client-probes.log` confirms WLAN discovery. `build.log` records Go checks and compiled source hashes; `tested-followup.patch` is the exact L7 production/test overlay used. `source.sha256` records final worktree sources, including the later harness log-cleanup adjustment.

Both test binaries returned zero. The outer shell returned 2 after the tests because its file was edited while running; this is recorded in `wrapper-status.txt`, not hidden as an all-zero harness run. The final script passes `bash -n`; its cleanup command is verified separately. No test binary or production source was changed during the final test run.

L7 is verified for the recorded secured-SSID contract on 6.1.0.19. L6 open-network PMF handling, L2–L5, G2, physical-device validation and upstream publication remain separate work. No new schema or state migration is required by these three exclusions.
