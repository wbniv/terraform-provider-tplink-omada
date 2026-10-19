| Date | Change |
|------|--------|
| [2026-04-09](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/commit/2f3f5c7) | feat: add omada_controller_certificate resource for TLS certificate management |
| [2026-03-30](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/commit/ace2169) | feat: add CI workflow, release-please, dependabot, issue/PR templates, Makefile, and README |

<!--history-meta v1
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
