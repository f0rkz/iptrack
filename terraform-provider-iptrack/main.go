package main

import (
	"context"
	"log"

	"github.com/f0rkz/terraform-provider-iptrack/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

var version = "dev"

func main() {
	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/f0rkz/iptrack",
	})
	if err != nil {
		log.Fatal(err)
	}
}
