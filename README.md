# Terraform Provider for TP-Link Omada

A Terraform provider for managing [TP-Link Omada Software Controller](https://www.tp-link.com/us/omada-sdn/) 6.x resources as infrastructure-as-code.

This review branch is maintained in [Will's standalone repository](https://github.com/wbniv/terraform-provider-tplink-omada), with the original project history preserved. The [upstream project](https://github.com/emanuelbesliu/terraform-provider-tplink-omada) and [preserved upstream fork](https://github.com/wbniv/terraform-provider-tplink-omada-upstream-fork) remain available for contributions. See the [current investigation and follow-ups](https://github.com/wbniv/terraform-provider-tplink-omada/blob/docs/provider-reliability-review/docs/investigations/software-controller-lab/README.md) for exact test results.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.25.0 (to build the provider; lab pinned to 1.25.6)
- TP-Link Omada Software Controller 6.x


## Resources

| Resource | Description |
|---|---|
| `omada_controller_certificate` | Manages the TLS certificate for the Omada SDN Controller |
| `omada_site` | Manages sites on the controller |
| `omada_network` | Manages LAN networks (VLANs) |
| `omada_wireless_network` | Manages wireless SSIDs |
| `omada_wlan_group` | Manages WLAN groups |
| `omada_port_profile` | Manages switch port profiles |
| `omada_site_settings` | Manages site-level settings |
| `omada_device_ap` | Manages access point device configurations |
| `omada_device_switch` | Manages managed switch device configurations |

## Data Sources

| Data Source | Description |
|---|---|
| `omada_sites` | Lists all sites |
| `omada_networks` | Lists networks for a site |
| `omada_wireless_networks` | Lists wireless SSIDs for a site |
| `omada_port_profiles` | Lists port profiles for a site |
| `omada_site_settings` | Reads site settings |
| `omada_devices` | Lists devices for a site |

## Installation

### From the Terraform Registry

```hcl
terraform {
  required_providers {
    omada = {
      source = "emanuelbesliu/tplink-omada"
    }
  }
}
```

### Building from Source

```sh
git clone --branch test/software-controller-lab https://github.com/wbniv/terraform-provider-tplink-omada.git
cd terraform-provider-tplink-omada
make build
```

Go dependencies are checked into `vendor/` at the versions in `go.mod` and `go.sum`. Ordinary builds use them automatically; `go build -mod=vendor` makes that choice explicit. Regenerate with Go 1.25.6 using `go mod vendor` after an intentional dependency update.

### Real-controller development lab

The [isolated lab](dev/controller-lab/README.md) runs the real Omada Software Controller **6.1.0.19** and Terraform **1.15.8** in Docker. It needs no adopted hardware for controller API tests. AP radio behavior, switch ports, provisioning, and traffic remain hardware checks.

The [official controller archive](third_party/omada/README.md) is vendored through **Git LFS**, with its SHA-256 and source recorded. Run `git lfs pull` after cloning. The lab verifies the archive and compares all 533 controller JARs with its pinned community runtime image before testing. Go dependencies use ordinary Git; the Omada distribution uses LFS.

```sh
dev/controller-lab/run.sh up
dev/controller-lab/run.sh test
dev/controller-lab/run.sh down
```

The first real-controller run passed network/profile CRUD, site credential rotation, and discovery of **102 networks**. It also exposed open compatibility failures: G1 treats the unpaginated WLAN-group response as paginated; certificate uploads use incorrect metadata/identity fields; a lost session can return HTTP 302; the IP-group route is rejected; and an empty device inventory becomes Terraform `null`. This branch now corrects WLAN-group discovery with an endpoint-specific collection policy; live WLAN discovery and the 102-network regression pass. Subsequent SSID probes exposed an open-network PMF default/readback mismatch and a fail-closed update rejection for controller fields absent from the writable contract. The remaining failures are retained as probes. This branch is a development/review branch, and the real-controller suite is not yet all green.

## Provider Configuration

```hcl
provider "omada" {
  url             = "https://192.168.1.1:8043"
  username        = "admin"
  password        = var.omada_password
  site            = "Default"
  skip_tls_verify = true
}
```

All attributes can also be set via environment variables:

| Attribute | Environment Variable |
|---|---|
| `url` | `OMADA_URL` |
| `username` | `OMADA_USERNAME` |
| `password` | `OMADA_PASSWORD` |
| `site` | `OMADA_SITE` |
| `skip_tls_verify` | `OMADA_SKIP_TLS_VERIFY` |

## Usage Example

```hcl
resource "omada_site" "home" {
  name = "Home"
}

resource "omada_network" "iot" {
  name        = "IoT"
  purpose     = "VLAN"
  vlan_id     = 100
  igmp_snoop_enable = false
}

resource "omada_wireless_network" "iot_wifi" {
  name           = "IoT-WiFi"
  wlan_group_id  = omada_wlan_group.default.id
  band           = "2g5g"
  security_mode  = "wpa2"
  psk_passphrase = var.iot_passphrase
  vlan_enable    = true
  vlan_id        = 100
}
```

See the [`docs/`](docs/) directory and the [Terraform Registry documentation](https://registry.terraform.io/providers/emanuelbesliu/tplink-omada/latest/docs) for full resource and data source schemas.

## Complete collection reads

Collection reads follow all pages and fail if an incomplete or inconsistent result cannot be trusted. Successful collection data-source reads return `[]` for no matches, including empty device inventories; `length()` and iteration are safe. See [Complete collection reads](docs/guides/collection-reads.md) for supported response shapes, limits, and refresh recovery.

## API response handling

The provider validates HTTP status and explicit Omada success envelopes, rejects redirects, and omits raw response bodies and URLs from errors. See [API response and error handling](docs/guides/api-responses.md) for URL configuration, response limits, diagnostics, and failure recovery.

## Development

### Prerequisites

- Go >= 1.23
- A running Omada Software Controller 6.x for testing

### Build and Test

```sh
make build      # Build the provider binary
make test       # Run unit tests
make fmt        # Format Go source files
make fmtcheck   # Check formatting without modifying files
make vet        # Run go vet
make dev        # Build and print dev_overrides path
make clean      # Remove built binary
```

### Local Development with dev_overrides

Add the following to `~/.terraformrc` to use a locally built binary:

```hcl
provider_installation {
  dev_overrides {
    "emanuelbesliu/tplink-omada" = "/path/to/terraform-provider-tplink-omada"
  }
  direct {}
}
```

Then build with `make dev` and run Terraform as usual. Note that `terraform init` will fail with dev_overrides enabled -- this is expected. Run `terraform plan` and `terraform apply` directly.

### Commit Conventions

This project uses [Conventional Commits](https://www.conventionalcommits.org/) for automated releases via [release-please](https://github.com/googleapis/release-please):

- `fix:` -- patch release (bug fix)
- `feat:` -- minor release (new feature)
- `feat!:` or `BREAKING CHANGE:` -- major release

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feat/my-feature`)
3. Commit using conventional commits (`git commit -m 'feat: add my feature'`)
4. Push to your branch (`git push origin feat/my-feature`)
5. Open a Pull Request

## License

See [LICENSE](LICENSE) for details.
