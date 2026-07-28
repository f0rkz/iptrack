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

var _ resource.ResourceWithImportState = &addressResource{}

type addressResource struct{ client *client.Client }
type addressModel struct {
	ID        types.String `tfsdk:"id"`
	NetworkID types.String `tfsdk:"network_id"`
	IP        types.String `tfsdk:"ip"`
	Hostname  types.String `tfsdk:"hostname"`
	Status    types.String `tfsdk:"status"`
	MAC       types.String `tfsdk:"mac"`
	Vendor    types.String `tfsdk:"vendor"`
	Metadata  types.Map    `tfsdk:"metadata"`
}

func NewAddressResource() resource.Resource { return &addressResource{} }
func (r *addressResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_address"
}
func (r *addressResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "An address tracked within an iptrack network. When ip is omitted, the next free address is allocated.", Attributes: map[string]schema.Attribute{
		"id":         schema.StringAttribute{Computed: true},
		"network_id": schema.StringAttribute{Required: true},
		"ip":         schema.StringAttribute{Optional: true, Computed: true},
		"hostname":   schema.StringAttribute{Optional: true, Computed: true},
		"status":     schema.StringAttribute{Optional: true, Computed: true, Description: "assigned, reserved, or discovered"},
		"mac":        schema.StringAttribute{Optional: true, Computed: true},
		"vendor":     schema.StringAttribute{Optional: true, Computed: true},
		"metadata":   schema.MapAttribute{Optional: true, Computed: true, ElementType: types.StringType},
	}}
}
func (r *addressResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	r.client, ok = req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", "Unable to configure iptrack address resource.")
	}
}
func (r *addressResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan addressModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := addressFromModel(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	var out client.Address
	var err error
	if plan.IP.IsNull() || plan.IP.IsUnknown() {
		out, err = r.client.AllocateAddress(ctx, in)
	} else {
		out, err = r.client.CreateAddress(ctx, in)
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to create address", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, addressToModel(ctx, out, &resp.Diagnostics))...)
}
func (r *addressResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state addressModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.Address(ctx, state.ID.ValueString())
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read address", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, addressToModel(ctx, out, &resp.Diagnostics))...)
}
func (r *addressResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan addressModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := addressFromModel(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.UpdateAddress(ctx, plan.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update address", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, addressToModel(ctx, out, &resp.Diagnostics))...)
}
func (r *addressResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state addressModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteAddress(ctx, state.ID.ValueString())
	var apiErr *client.APIError
	if err != nil && !(errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound) {
		resp.Diagnostics.AddError("Unable to delete address", err.Error())
	}
}
func (r *addressResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func addressFromModel(ctx context.Context, m addressModel, d *diag.Diagnostics) client.Address {
	metadata := map[string]string{}
	if !m.Metadata.IsNull() && !m.Metadata.IsUnknown() {
		d.Append(m.Metadata.ElementsAs(ctx, &metadata, false)...)
	}
	return client.Address{ID: m.ID.ValueString(), NetworkID: m.NetworkID.ValueString(), IP: m.IP.ValueString(), Hostname: m.Hostname.ValueString(), Status: m.Status.ValueString(), MAC: m.MAC.ValueString(), Vendor: m.Vendor.ValueString(), Metadata: metadata}
}
func addressToModel(ctx context.Context, a client.Address, d *diag.Diagnostics) addressModel {
	metadata, diags := types.MapValueFrom(ctx, types.StringType, a.Metadata)
	d.Append(diags...)
	return addressModel{ID: types.StringValue(a.ID), NetworkID: types.StringValue(a.NetworkID), IP: types.StringValue(a.IP), Hostname: types.StringValue(a.Hostname), Status: types.StringValue(a.Status), MAC: types.StringValue(a.MAC), Vendor: types.StringValue(a.Vendor), Metadata: metadata}
}
