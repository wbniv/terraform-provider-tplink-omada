# Plans

| Plan | Purpose | Status |
| --- | --- | --- |
| [Provider reliability PR tracker](provider-reliability-prs.md) | Track bug-fix scope, acceptance criteria, commits, evidence, hardware validation, and upstream submission. | A–E locally verified and pushed to the fork; F/F12 response validation is next. |
| [SSID preservation, identity, and VLAN state](ssid-reliability.md) | Plan D1–D3 fixes, ownership/API decisions, Terraform review flows, regressions, and submission gates. | D1–D3 locally verified and pushed; hardware/submission gates remain open. |
| [Site device credentials](site-credentials.md) | Plan E credential ownership, rotation/state, unrelated-settings preservation, migration, and tests. | Committed/pushed as `8f24c37`; hardware/submission gates remain open. |
| [Existing upstream PRs](open-pull-requests.md) | Review dependency/workflow upgrades and the switch operational-speed change. | Open follow-ups; metadata refreshed 2026-10-19. |

The [baseline investigation](../investigations/2026-10-18-code-review.md) links each finding to its reproductions and current repair status. The documentation is maintained on `docs/provider-reliability-review`; code fixes have separate branches listed in the tracker.

Each of the three original upstream PRs has a separate plan section in `open-pull-requests.md`. All repair work A–I has scope, dependencies, and acceptance criteria in the reliability tracker. D and E also have standalone detailed plans; F–I currently have tracker checklists and need detailed design before implementation.
