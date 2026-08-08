// Terraform provider for Paddle Billing (products, prices, discounts). Not
// affiliated with or endorsed by Paddle — calls Paddle's public REST API
// directly, no third-party service in the request path.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/vivantel/terraform-provider-paddle/internal/provider"
)

// version is overridden at build time via -ldflags "-X main.version=..."
// (see .goreleaser.yml). "dev" identifies a local/unreleased build.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with support for debuggers")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/vivantel/paddle",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err.Error())
	}
}
