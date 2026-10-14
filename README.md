# Terraform Provider for TP-LINK Router

This provider manages the local SLP API of a TP-LINK TL-ER6220T. It was developed against hardware `TL-ER6220T v1.0`, firmware `1.2.2 Build 240829 Rel.70225n`. The catalog exposes typed Terraform resources and read-only data sources for the router's network, security, wireless, VPN, AP, system, and advanced configuration surfaces.

The provider reads credentials from explicit Terraform arguments first, then `TPLINK_ROUTER_USERNAME` / `TPLINK_ROUTER_PASSWORD`, and finally the file named by `env_file` (`USERNAME` / `PASSWORD`). A safe starting configuration is:

```hcl
terraform {
  required_providers {
    tplinkrouter = {
      source = "chang196700/tplinkrouter"
    }
  }
}

provider "tplinkrouter" {
  model                = "tl-er6220t"
  endpoint             = "https://192.168.1.1"
  env_file             = "${path.root}/.env"
  insecure_skip_verify = true
  allow_legacy_tls     = true
  read_only            = true
}

data "tplinkrouter_device" "this" {}
data "tplinkrouter_dhcp_server" "lan" {
  interface = "lan"
}
```

`model` is required. The current release supports `tl-er6220t`, which selects and verifies TP-LINK TL-ER6220T hardware version 1.0 before any operation.

Copy `.env.example` to `.env` and keep that file out of version control. The current firmware uses a self-signed certificate and a static RSA TLS 1.2 AES-GCM suite; `insecure_skip_verify` and `allow_legacy_tls` are separate explicit switches. HTTP is also supported by changing `endpoint` to `http://192.168.1.1`.

`read_only = true` rejects every mutation and save operation. Keep it enabled for discovery, import, and plan validation. Resources default to `auto_save = false`; setting `auto_save = true` persists all pending router changes after a confirmed mutation, including changes made outside Terraform. Resource destroy removes only the managed object where the firmware exposes a reversible CRUD operation. Settings and existing hardware objects release Terraform management without resetting or deleting the device.

The administrator resource supports a write-only password rotation with `password_wo` and `password_wo_version`. The provider never writes `.env`; update the credential source before the next Terraform command after a successful rotation.

## Build and local development

```text
go build -o terraform-provider-tplinkrouter.exe .
go vet ./...
go test ./...
go run ./cmd/catalog generate
```

Use a Terraform CLI development override to load the local binary. Generated reference documentation is under [`docs/`](docs/), including the capability matrix and one page per resource/data source. `go run ./cmd/catalog audit ...` performs a forced read-only audit and writes only sanitized metadata (row counts, field names, errors, and firmware version) to the requested report path.

The catalog deliberately excludes one-shot or cloud-only actions such as firmware upgrade, reboot, restore, AP adoption, cloud template creation, and TP-LINK DDNS domain registration. Local portal pages that already exist on the router are represented as existing read-only identities with typed JSON configuration editing; multi-stage cloud/template/image workflows remain outside Terraform CRUD.

