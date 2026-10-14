package main

import (
	"context"
	"flag"
	"log"

	"github.com/chang196700/terraform-provider-tplinkrouter/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

var version = "0.1.0-dev"

func main() {
	debug := flag.Bool("debug", false, "Enable Terraform debugger support")
	flag.Parse()
	if err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/chang196700/tplinkrouter", Debug: *debug,
	}); err != nil {
		log.Fatal(err)
	}
}

