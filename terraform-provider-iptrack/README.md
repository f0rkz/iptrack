# iptrack Terraform provider

This provider manages `iptrack_network` and `iptrack_address` resources and provides matching ID-based data sources against the iptrack HTTP API. See the repository root README for configuration and examples.

Build from this directory with:

```sh
go build
```

For local Terraform development, add a `dev_overrides` entry for `registry.terraform.io/f0rkz/iptrack` to your Terraform CLI configuration.
