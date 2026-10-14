package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"terraform-provider-tplink-router/internal/provider"
)

var version = "0.1.0-dev"

func main() {
	debug := flag.Bool("debug", false, "Enable Terraform debugger support")
	flag.Parse()
	if err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/local/tplinkrouter", Debug: *debug,
	}); err != nil {
		log.Fatal(err)
	}
}
