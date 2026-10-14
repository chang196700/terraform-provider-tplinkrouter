terraform {
  required_providers {
    tplinkrouter = {
      source = "chang196700/tplinkrouter"
    }
  }
}

provider "tplinkrouter" {
  endpoint             = "https://192.168.1.1"
  env_file             = "${path.root}/.env"
  insecure_skip_verify = true
  allow_legacy_tls     = true
  read_only            = true
}

data "tplinkrouter_device" "this" {}

data "tplinkrouter_interface" "all" {}

output "router_model" {
  value = data.tplinkrouter_device.this.model
}

output "interface_count" {
  value = data.tplinkrouter_interface.all.row_count
}

