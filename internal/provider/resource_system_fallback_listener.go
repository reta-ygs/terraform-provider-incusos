package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	osapi "github.com/lxc/incus-os/incus-osd/api"

	"github.com/reta-ygs/terraform-provider-incusos/internal/client"
)

var (
	_ resource.Resource                = (*systemFallbackListenerResource)(nil)
	_ resource.ResourceWithImportState = (*systemFallbackListenerResource)(nil)
)

type systemFallbackListenerResource struct {
	client *client.Client
}

type systemFallbackListenerModel struct {
	ID                        types.String `tfsdk:"id"`
	ListenAddress             types.String `tfsdk:"listen_address"`
	Persistent                types.Bool   `tfsdk:"persistent"`
	TrustedClientCertificates types.List   `tfsdk:"trusted_client_certificates"`
	Active                    types.Bool   `tfsdk:"active"`
}

func NewSystemFallbackListenerResource() resource.Resource {
	return &systemFallbackListenerResource{}
}

func (r *systemFallbackListenerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_fallback_listener"
}

func (r *systemFallbackListenerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the IncusOS fallback HTTPS listener (`/1.0/system/fallback-listener`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"listen_address": schema.StringAttribute{
				MarkdownDescription: "IP:port or :port address to listen on. If omitted, listens on all interfaces on a random port.",
				Optional:            true,
			},
			"persistent": schema.BoolAttribute{
				MarkdownDescription: "Always start the fallback listener at boot rather than only when the primary application is unavailable.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"trusted_client_certificates": schema.ListAttribute{
				MarkdownDescription: "List of PEM-encoded trusted client TLS certificates allowed to connect to the fallback listener.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"active": schema.BoolAttribute{
				MarkdownDescription: "Whether the fallback listener is currently active.",
				Computed:            true,
			},
		},
	}
}

func (r *systemFallbackListenerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (r *systemFallbackListenerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan systemFallbackListenerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := osapi.SystemFallbackListener{
		Config: osapi.SystemFallbackListenerConfig{
			ListenAddress:             plan.ListenAddress.ValueString(),
			Persistent:                plan.Persistent.ValueBool(),
			TrustedClientCertificates: listToStringSlice(ctx, plan.TrustedClientCertificates, &resp.Diagnostics),
		},
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateSystemFallbackListener(ctx, payload); err != nil {
		resp.Diagnostics.AddError("Failed to configure fallback listener", err.Error())

		return
	}

	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *systemFallbackListenerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state systemFallbackListenerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.readIntoModel(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *systemFallbackListenerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan systemFallbackListenerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := osapi.SystemFallbackListener{
		Config: osapi.SystemFallbackListenerConfig{
			ListenAddress:             plan.ListenAddress.ValueString(),
			Persistent:                plan.Persistent.ValueBool(),
			TrustedClientCertificates: listToStringSlice(ctx, plan.TrustedClientCertificates, &resp.Diagnostics),
		},
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateSystemFallbackListener(ctx, payload); err != nil {
		resp.Diagnostics.AddError("Failed to update fallback listener", err.Error())

		return
	}

	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *systemFallbackListenerResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.UpdateSystemFallbackListener(ctx, osapi.SystemFallbackListener{}); err != nil {
		resp.Diagnostics.AddError("Failed to reset fallback listener", err.Error())
	}
}

func (r *systemFallbackListenerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *systemFallbackListenerResource) readIntoModel(ctx context.Context, model *systemFallbackListenerModel, diags *diag.Diagnostics) {
	res, err := r.client.GetSystemFallbackListener(ctx)
	if err != nil {
		diags.AddError("Failed to read fallback listener", err.Error())

		return
	}

	model.ID = types.StringValue("system-fallback-listener")
	model.ListenAddress = stringValueOrNull(res.Config.ListenAddress, model.ListenAddress.IsNull())
	model.Persistent = types.BoolValue(res.Config.Persistent)
	model.TrustedClientCertificates = stringSliceToList(ctx, res.Config.TrustedClientCertificates, model.TrustedClientCertificates.IsNull(), diags)
	model.Active = types.BoolValue(res.State.Active)
}
