# Switch-port patch evidence

Date: 2026-10-18. Tracker: [work B](../../plans/provider-reliability-prs.md#b--switch-ports).

Scope: F02, preserving configured switch-port VLAN selections and honoring writable fields. Worktree: `/home/will/terraform-provider-tplink-omada-switch-ports`. Branch: `fix/switch-port-updates`, based directly on `6bc71b310bc81a90f3ad32194ba8c17947bf79d6`. Committed and pushed to Will's fork on 2026-10-19 as [1c7f535](https://github.com/wbniv/terraform-provider-tplink-omada/commit/1c7f535e86a4db9de53645c2c844d49429ee5694). No upstream PR has been opened. Authentication work A remains separate on `fix/client-authentication`; the original checkout now hosts the documentation branch.

**Status: implemented and locally verified.** Real-controller compatibility is still untested. The separate omitted-whole-list planning defect, F15, remains open and is documented below.

## Implementation and review scope

- Build a port payload from the current controller snapshot, restricted to the ten writable fields represented by the schema, then apply known planned values.
- Preserve omitted/unknown values, including VLAN lists. Honor explicit false, zero, empty strings, and empty lists.
- Send a port PATCH only when that writable payload differs from the current snapshot. Never include operational speed or arbitrary fields from GET responses.
- Validate the full configured list before any switch writes: port numbers must be known, positive, representable, unique, and present on the controller; include every port when configuring the list. Unresolved list elements also fail before writes.
- Remove implicit false defaults from four port booleans so omitted settings on imported devices do not silently become changes.
- Preserve configured/prior list order by port number during readback, allowing reordering without unnecessary port writes or inconsistent state.
- Update resource/schema descriptions and migration notes, and correct the import example to include the site ID.

Production scope is [device_switch.go](https://github.com/wbniv/terraform-provider-tplink-omada/blob/1c7f535e86a4db9de53645c2c844d49429ee5694/internal/resources/device_switch.go) and the new [device_switch_ports.go](https://github.com/wbniv/terraform-provider-tplink-omada/blob/1c7f535e86a4db9de53645c2c844d49429ee5694/internal/resources/device_switch_ports.go). Maintained regression tests are in [the resource package](https://github.com/wbniv/terraform-provider-tplink-omada/blob/1c7f535e86a4db9de53645c2c844d49429ee5694/internal/resources/device_switch_ports_test.go) and [the provider package](https://github.com/wbniv/terraform-provider-tplink-omada/blob/1c7f535e86a4db9de53645c2c844d49429ee5694/internal/provider/device_switch_ports_test.go). User-facing changes are in [the switch resource documentation](https://github.com/wbniv/terraform-provider-tplink-omada/blob/1c7f535e86a4db9de53645c2c844d49429ee5694/docs/resources/device_switch.md).

No authentication changes or PR #23 speed-schema changes are included in B. General switch/service update behavior retains its existing implementation; the tracker separately notes the unknown-loopback-setting concern. This patch's preservation guarantee concerns port configuration.

## Verification

All tests used Docker with Go `1.25.6 linux/amd64`, matching `.go-version`. Image: `golang:1.25.6@sha256:06d1251c59a75761ce4ebc8b299030576233d7437c886a68b43464bad62d4bb1`. Terraform CLI: `1.15.8 linux_amd64`. Mock servers use synthetic identifiers and credentials; no live switch or controller was accessed.

| Gate | Result | Evidence |
| --- | --- | --- |
| Same resource lifecycle probes on baseline | 11 of 13 cases fail; whole-list-null and whole-list-unknown skip cases pass | [Baseline lifecycle log](baseline-lifecycle.log) |
| Patched resource lifecycle probes | All 13 cases pass | [Patched lifecycle log](patched-lifecycle.log) |
| Same Terraform acceptance sequence on baseline | Both standard and ES cases fail during import/apply: false defaults conflict with existing true settings; unchanged ports receive writes | [Baseline acceptance log](baseline-acceptance.log) |
| Patched Terraform acceptance sequence | Eight steps pass for each of standard and ES routes | [Patched acceptance log](patched-acceptance.log) |
| `gofmt -l .` | No output | Checked in container |
| `go vet ./...` | Exit 0, no output | Checked in container |
| `go build ./...` | Exit 0, no output | Checked in container |
| `go test ./... -count=1 -cover` | Pass; resource statement coverage 9.4% in this run | [Full unit-suite log](patched-unit-suite.log) |
| B combined with stored PR #23 | Patch applies cleanly; port-preservation and both speed acceptance tests pass | [Compatibility log](pr23-compatibility.log) |

The lifecycle tests cover preserved nonempty tagged/untagged VLANs, formerly ignored fields, explicit clearing/false/zero/empty strings, unknown and null values, unchanged-port skipping, invalid later ports before any write, duplicate/unknown/missing ports, unknown list elements, failure stopping subsequent port writes, and port ordering.

The Terraform sequence covers import with all ports listed and individual settings omitted, switch rename, a stable empty plan, single-port rename, explicit writable-setting changes, explicit VLAN clearing and false/zero changes, port reordering, and another switch rename. It asserts request counts and resulting state. Both API route families are simulated; this does not prove firmware-specific field acceptance or inheritance behavior.

The full unit run leaves live acceptance tests gated off. Provider acceptance tests were run separately and are not currently included in the repository's ordinary unit-test CI job. A mock acceptance CI gate remains tracked work.

## Reproduction commands

Inside a disposable checkout using the pinned container:

- Lifecycle regression: `go test ./internal/resources -run TestSwitchPortUpdates -count=1 -v`.
- Local Terraform acceptance: `TF_ACC=1 TF_ACC_TERRAFORM_PATH=/usr/local/bin/terraform go test ./internal/provider -run TestAccDeviceSwitchPortPreservation -count=1 -v -timeout=180s`.
- Full gates: `gofmt -l .`, `go vet ./...`, `go build ./...`, and `go test ./... -count=1 -cover`.

For the baseline comparison, check out `6bc71b3` and copy only the two new test files from B. Both targeted commands fail on the baseline. For the patch comparison, also copy B's two production files and its resource documentation. No host Go installation is required.

For compatibility, apply the diff from `6bc71b3` to the stored PR #23 head `2ec46a4ba19cdbda4a2ee6c1546a40bb5324b594` on top of B in a disposable checkout. Run the provider acceptance command above with `-run TestAccDeviceSwitch` to include both PR #23 tests. GitHub metadata rechecked on 2026-10-19 confirms that #23 still has this head and no posted checks. Recheck again before upstream submission.

The five working files match these hashes. The final full gates and combined acceptance run use this implementation. The standalone lifecycle/acceptance logs precede the added integer-representability guard; normal port-number paths are covered again in the final full suite and combined acceptance run.

| File in B | SHA-256 |
| --- | --- |
| `internal/resources/device_switch.go` | `fbf1efb8a1c864e125591a83e3127e776eb0454ac253bc4efab9471866650f9c` |
| `internal/resources/device_switch_ports.go` | `d4dc541101e7d1988948e1aef6d1115dd9a85af753f051fe409e6fc88a08f248` |
| `internal/resources/device_switch_ports_test.go` | `5c9145a51854fab58cd09a7cf223c854dbfb87fc63661a5ca98e32bfdc05ebbf` |
| `internal/provider/device_switch_ports_test.go` | `daf235e5f90874c26f7ac8d641142735ab1e268f91eab6a8d5b8ba072e48dc6c` |
| `docs/resources/device_switch.md` | `6564efb5d2a9a1e79f19acd31854bfdd02ec401f64f268f84ce5c2774d01647d` |

## Compatibility and submission notes

Suggested conventional title: `fix!: preserve imported switch port settings during updates`.

The behavior change should be explicit in the PR template/release notes: omitted `disable`, `profile_override_enable`, `voice_network_enable`, and `voice_dscp_enable` now preserve current values instead of defaulting to false. Users who require false should configure it explicitly. Attribute names/types and the stored state shape are unchanged, so no re-import is required. Include the default change in release classification even though it repairs unsafe behavior.

- [x] Keep a separate branch and a five-file upstream patch scope.
- [x] Demonstrate failing baseline and passing patched regressions.
- [x] Run unit, formatting, vet, build, and mock Terraform acceptance gates.
- [x] Check compatibility with the stored #23 patch and both of its acceptance tests.
- [x] Document default changes, list semantics, validation, ordering, and the existing F15 limitation.
- [x] Verify the final working-file hashes, confirm A's source files are unchanged, check documentation links and whitespace, and remove the disposable test container. Preserve both worktrees and the recorded evidence.
- [ ] Test the emitted payloads against a controlled switch/controller, recording model and controller/firmware versions. Verify native/tagged/untagged VLAN changes, disable/voice fields, profile changes/inheritance, and unsupported-field handling on each relevant series.
- [ ] Keep the upstream template's real-controller test declaration unchecked until performed.
- [x] Recheck upstream heads/CI and contribution requirements on 2026-10-19; commit the five selected files as `1c7f535` and push to `fork/fix/switch-port-updates`. Tested hashes match the committed files.
- [ ] Prepare the upstream PR for authorized publication; verify any required attribution using actual metadata and recheck heads/CI then.
- [ ] Record PR URL, CI runs, reviews, and merge revision in the tracker.

## New finding F15: omitted whole ports list does not settle after import

With only the switch name configured, import completes but the following plan proposes another update. The framework trace reports a change at `ports` before marking computed attributes unknown. This reproduces on original main and B. The exact planning cause is unconfirmed; it must not be presented as a regression introduced by B or as fixed by B.

Evidence: [original probe](omitted-ports-probe.go.txt), [baseline failure](omitted-ports-baseline.log), and [B failure](omitted-ports-patch.log). In a disposable checkout, replace `internal/provider/device_switch_ports_test.go` with the probe and run `TestAccDeviceSwitchPortPreservation`; the first import step reproduces the issue. The regular maintained acceptance test instead configures the complete port list, matching the previously documented usage contract.

The resource exposes `ports` as Optional+Computed, so omitted-list planning needs a deliberate implementation. Terraform's planning contract allows provider-chosen computed values for null configuration and describes prior-state fallback for optional/computed attributes. That contract is the reference for the follow-up investigation, not proof of a particular root cause. [Terraform resource lifecycle specification](https://github.com/hashicorp/terraform/blob/main/docs/resource-instance-change-lifecycle.md#planresourcechange).

F15 is P2 and remains open as work I in the tracker. The resource documentation states the current complete-list workaround. Individual omitted port attributes are covered by B's passing preservation tests.
