package provider

import (
	"context"
	"errors"
	"net/http"

	"github.com/f0rkz/terraform-provider-iptrack/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.ResourceWithImportState = &networkResource{}

type networkResource struct{ client *client.Client }
type networkModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	CIDR        types.String `tfsdk:"cidr"`
	Description types.String `tfsdk:"description"`
	Tags        types.Map    `tfsdk:"tags"`
}

func NewNetworkResource() resource.Resource { return &networkResource{} }
func (r *networkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network"
}
func (r *networkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "A managed IPv4 or IPv6 network.", Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true}, "name": schema.StringAttribute{Required: true}, "cidr": schema.StringAttribute{Required: true}, "description": schema.StringAttribute{Optional: true, Computed: true}, "tags": schema.MapAttribute{Optional: true, Computed: true, ElementType: types.StringType},
	}}
}
func (r *networkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	r.client, ok = req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", "Unable to configure iptrack network resource.")
	}
}
func (r *networkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan networkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := networkFromModel(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.CreateNetwork(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create network", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, networkToModel(ctx, out, &resp.Diagnostics))...)
}
func (r *networkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state networkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.Network(ctx, state.ID.ValueString())
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read network", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, networkToModel(ctx, out, &resp.Diagnostics))...)
}
func (r *networkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan networkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := networkFromModel(ctx, plan, &resp.Diagnostics)
	out, err := r.client.UpdateNetwork(ctx, plan.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update network", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, networkToModel(ctx, out, &resp.Diagnostics))...)
}
func (r *networkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state networkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteNetwork(ctx, state.ID.ValueString())
	var apiErr *client.APIError
	if err != nil && !(errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound) {
		resp.Diagnostics.AddError("Unable to delete network", err.Error())
	}
}
func (r *networkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
func networkFromModel(ctx context.Context, m networkModel, d *diag.Diagnostics) client.Network {
	tags := map[string]string{}
	if !m.Tags.IsNull() && !m.Tags.IsUnknown() {
		d.Append(m.Tags.ElementsAs(ctx, &tags, false)...)
	}
	return client.Network{ID: m.ID.ValueString(), Name: m.Name.ValueString(), CIDR: m.CIDR.ValueString(), Description: m.Description.ValueString(), Tags: tags}
}
func networkToModel(ctx context.Context, n client.Network, d *diag.Diagnostics) networkModel {
	tags, diags := types.MapValueFrom(ctx, types.StringType, n.Tags)
	d.Append(diags...)
	return networkModel{ID: types.StringValue(n.ID), Name: types.StringValue(n.Name), CIDR: types.StringValue(n.CIDR), Description: types.StringValue(n.Description), Tags: tags}
}
