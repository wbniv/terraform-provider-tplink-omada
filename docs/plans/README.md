# Plans

| Plan | Purpose | Status |
| --- | --- | --- |
| [Provider reliability PR tracker](provider-reliability-prs.md) | Track bug-fix scope, acceptance criteria, commits, evidence, hardware validation, and upstream submission. | A–E pushed; F locally verified and pushed as [c210dec](https://github.com/wbniv/terraform-provider-tplink-omada/commit/c210dec9a4bb10a15881f15a250b7c1843543d31); G pagination/disappearance is next. |
| [SSID preservation, identity, and VLAN state](ssid-reliability.md) | Plan D1–D3 fixes, ownership/API decisions, Terraform review flows, regressions, and submission gates. | D1–D3 locally verified and pushed; hardware/submission gates remain open. |
| [Site device credentials](site-credentials.md) | Plan E credential ownership, rotation/state, unrelated-settings preservation, migration, and tests. | Committed/pushed as `8f24c37`; hardware/submission gates remain open. |
| [HTTP response validation](http-responses.md) | Plan F12 status/envelope checks, error types, redaction, redirects, and Terraform failure recovery. | Locally verified on `fix/http-responses`, based on E; pushed as [c210dec](https://github.com/wbniv/terraform-provider-tplink-omada/commit/c210dec9a4bb10a15881f15a250b7c1843543d31). |
| [Software-controller validation lab](software-controller-lab.md) | Validate real controller APIs in an isolated software environment and track physical-device checks separately. | Official software availability verified; lab planned, not started. |
| [Existing upstream PRs](open-pull-requests.md) | Review dependency/workflow upgrades and the switch operational-speed change. | Open follow-ups; metadata refreshed 2026-10-19. |

The [baseline investigation](../investigations/2026-10-18-code-review.md) links each finding to its reproductions and current repair status. The documentation is maintained on `docs/provider-reliability-review`; code fixes have separate branches listed in the tracker.

Each of the three original upstream PRs has a separate plan section in `open-pull-requests.md`. All repair work A–I has scope, dependencies, and acceptance criteria in the reliability tracker. D, E, and F also have standalone detailed plans; G–I currently have tracker checklists and need detailed design before implementation.
