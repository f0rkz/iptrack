package provider

import (
	"context"

	"github.com/f0rkz/terraform-provider-iptrack/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &networkDataSource{}

type networkDataSource struct{ client *client.Client }

func NewNetworkDataSource() datasource.DataSource { return &networkDataSource{} }
func (d *networkDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network"
}
func (d *networkDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Look up an iptrack network by ID.", Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{Required: true}, "name": schema.StringAttribute{Computed: true}, "cidr": schema.StringAttribute{Computed: true}, "description": schema.StringAttribute{Computed: true}, "tags": schema.MapAttribute{Computed: true, ElementType: types.StringType},
	}}
}
func (d *networkDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	d.client, ok = req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", "Unable to configure iptrack network data source.")
	}
}
func (d *networkDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config networkModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := d.client.Network(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read network", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, networkToModel(ctx, out, &resp.Diagnostics))...)
}
