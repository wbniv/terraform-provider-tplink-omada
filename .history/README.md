| Date | Change |
|------|--------|
| [2026-10-19](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/commit/0730101) | fix(client): handle unpaginated WLAN group envelopes |
| [2026-10-19](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/commit/0d26e72) | test: add vendored Omada controller lab |
| [2026-10-19](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/commit/9343503) | fix(client)!: read complete paginated collections |
| [2026-10-19](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/commit/c210dec) | fix(client)!: validate HTTP responses and protect error diagnostics |
| [2026-04-09](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/commit/2f3f5c7) | feat: add omada_controller_certificate resource for TLS certificate management |
| [2026-03-30](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/commit/ace2169) | feat: add CI workflow, release-please, dependabot, issue/PR templates, Makefile, and README |

<!--history-meta v1
0730101	author	Will Norris
0730101	added	1
0730101	deleted	1
0730101	files	1
0d26e72	author	Will Norris
0d26e72	added	20
0d26e72	deleted	2
0d26e72	files	1
9343503	author	Will Norris
9343503	added	4
9343503	deleted	0
9343503	files	1
9343503	body	Read all pages for ten collections and raw SSID lookup. Preserve filters and authentication, validate pagination and unique identities, and return no partial results. Add bounded reads, client regressions, Terraform import/refresh recovery coverage, and collection documentation.\n\nBREAKING CHANGE: Collection reads reject inconsistent metadata, missing or duplicate identities, malformed objects, and unfinished listings. Data-only page wrappers must advance to an empty page. Reads are bounded to 10000 pages and 64 MiB of result JSON. Schemas and import IDs are unchanged.
c210dec	author	Will Norris
c210dec	added	4
c210dec	deleted	0
c210dec	files	1
c210dec	body	Validate status, strict JSON envelopes, response sizes, and redirects across discovery, login, JSON requests, and multipart uploads. Preserve typed causes and bounded authentication replay. Add client and Terraform failure-recovery regressions and an API response migration guide.\n\nBREAKING CHANGE: API redirects and nonstandard or empty response envelopes now fail; configure the final controller URL. Diagnostics omit raw controller messages and response bodies; use numeric status and API codes. HTTP 401/403 expiry responses do not trigger automatic replay.
2f3f5c7	author	emanuelbesliu
2f3f5c7	added	2
2f3f5c7	deleted	0
2f3f5c7	files	1
2f3f5c7	body	- Add client methods for certificate upload, key upload, and activation\n- Implement omada_controller_certificate resource with Create, Read, Update, Delete\n- Delete is a no-op to prevent accidental certificate removal\n- Add multipart form-data support for certificate/key file uploads\n- Generate tfplugindocs documentation\n- Add example usage with file-based and acme_certificate sources\n- Update README with new resource
ace2169	author	emanuelbesliu
ace2169	added	158
ace2169	deleted	0
ace2169	files	1
ace2169	body	- CI workflow: build, vet, test on push/PR to main\n- release-please: automated versioning via conventional commits\n- dependabot: weekly updates for Go modules and GitHub Actions\n- Issue templates: bug report and feature request (YAML forms)\n- PR template: checklist for contributors\n- Makefile: build, test, fmt, fmtcheck, vet, dev, clean targets\n- README: provider overview, usage, development guide
-->
