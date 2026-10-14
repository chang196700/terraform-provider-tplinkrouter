---
page_title: "TP-LINK Router Provider"
description: |-
  Manage a TP-LINK TL-ER6220T router through its local SLP API.
---

# TP-LINK Router Provider

The provider manages a TP-LINK TL-ER6220T through its local SLP API. The
current supported model is hardware version 1.0. Every provider configuration
must set `model = "tl-er6220t"`; the provider verifies the connected device
before it performs an operation.

## Example Usage

```terraform
terraform {
  required_providers {
    tplinkrouter = {
      source  = "chang196700/tplinkrouter"
      version = "~> 0.1"
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
```

## Provider configuration

| Attribute | Type | Description |
|---|---|---|
| `model` | String, required | Must be `tl-er6220t`; selects TP-LINK TL-ER6220T v1.0. |
| `endpoint` | String | Router base URL. Defaults to `https://192.168.1.1`. |
| `username` | String | Administrator username. Overrides environment credentials. |
| `password` | String, sensitive | Administrator password. Overrides environment credentials. |
| `env_file` | String | Dotenv file containing `USERNAME` and `PASSWORD`. |
| `timeout` | String | Positive Go duration for each request. Defaults to `30s`. |
| `insecure_skip_verify` | Boolean | Disable HTTPS certificate verification explicitly. |
| `allow_legacy_tls` | Boolean | Enable the router's legacy RSA AES-GCM TLS 1.2 suites. |
| `auto_save` | Boolean | Save pending router changes after confirmed mutations. |
| `read_only` | Boolean | Reject all configuration mutations and saves. |

Credentials can also be supplied through `TPLINK_ROUTER_USERNAME` and
`TPLINK_ROUTER_PASSWORD`. Keep `.env` out of version control.

Generated resource and data source references are listed in the `docs/`
directory. The provider never falls back from an HTTPS endpoint to HTTP.

