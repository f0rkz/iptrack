package provider

import (
	"context"
	"os"

	"github.com/f0rkz/terraform-provider-iptrack/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = &iptrackProvider{}

type iptrackProvider struct{ version string }
type configModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	Token    types.String `tfsdk:"token"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider { return &iptrackProvider{version: version} }
}
func (p *iptrackProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "iptrack"
	resp.Version = p.version
}
func (p *iptrackProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{Attributes: map[string]schema.Attribute{
		"endpoint": schema.StringAttribute{Optional: true, Description: "Base URL of the iptrack server. May also be set with IPTRACK_ENDPOINT."},
		"token":    schema.StringAttribute{Optional: true, Sensitive: true, Description: "Bearer token for an authenticating reverse proxy. May also be set with IPTRACK_TOKEN."},
	}}
}
func (p *iptrackProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg configModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	endpoint := cfg.Endpoint.ValueString()
	if endpoint == "" {
		endpoint = os.Getenv("IPTRACK_ENDPOINT")
	}
	token := cfg.Token.ValueString()
	if token == "" {
		token = os.Getenv("IPTRACK_TOKEN")
	}
	if endpoint == "" {
		resp.Diagnostics.AddError("Missing iptrack endpoint", "Set provider endpoint or IPTRACK_ENDPOINT.")
		return
	}
	c, err := client.New(endpoint, token)
	if err != nil {
		resp.Diagnostics.AddError("Invalid iptrack configuration", err.Error())
		return
	}
	resp.ResourceData = c
	resp.DataSourceData = c
}
func (p *iptrackProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{NewNetworkResource, NewAddressResource}
}
func (p *iptrackProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{NewNetworkDataSource, NewAddressDataSource}
}
