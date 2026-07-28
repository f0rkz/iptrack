package provider

import (
	"context"

	"github.com/f0rkz/terraform-provider-iptrack/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &addressDataSource{}

type addressDataSource struct{ client *client.Client }

func NewAddressDataSource() datasource.DataSource { return &addressDataSource{} }
func (d *addressDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_address"
}
func (d *addressDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Look up an iptrack address by ID.", Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{Required: true}, "network_id": schema.StringAttribute{Computed: true}, "ip": schema.StringAttribute{Computed: true}, "hostname": schema.StringAttribute{Computed: true}, "status": schema.StringAttribute{Computed: true}, "mac": schema.StringAttribute{Computed: true}, "vendor": schema.StringAttribute{Computed: true}, "metadata": schema.MapAttribute{Computed: true, ElementType: types.StringType},
	}}
}
func (d *addressDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	d.client, ok = req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", "Unable to configure iptrack address data source.")
	}
}
func (d *addressDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config addressModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := d.client.Address(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read address", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, addressToModel(ctx, out, &resp.Diagnostics))...)
}
