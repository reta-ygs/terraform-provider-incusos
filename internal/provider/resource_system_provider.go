package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	osapi "github.com/lxc/incus-os/incus-osd/api"

	"github.com/reta-ygs/terraform-provider-incusos/internal/client"
)

type diagDiagnosticsAlias = diag.Diagnostics

var (
	_ resource.Resource                = (*systemProviderResource)(nil)
	_ resource.ResourceWithImportState = (*systemProviderResource)(nil)
)

type systemProviderResource struct {
	client *client.Client
}

type systemProviderModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Config     types.Map    `tfsdk:"config"`
	Registered types.Bool   `tfsdk:"registered"`
}

func NewSystemProviderResource() resource.Resource {
	return &systemProviderResource{}
}

func (r *systemProviderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_provider"
}

func (r *systemProviderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the IncusOS update and configuration provider (`/1.0/system/provider`), such as `images` or `operations-center`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Provider name (e.g., `images`, `operations-center`).",
				Required:            true,
			},
			"config": schema.MapAttribute{
				MarkdownDescription: "Provider-specific key-value configuration.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"registered": schema.BoolAttribute{
				MarkdownDescription: "Whether the host is currently registered with the provider.",
				Computed:            true,
			},
		},
	}
}

func (r *systemProviderResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (r *systemProviderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan systemProviderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := osapi.SystemProvider{
		Config: osapi.SystemProviderConfig{
			Name:   plan.Name.ValueString(),
			Config: mapToStringMap(ctx, plan.Config, &resp.Diagnostics),
		},
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateSystemProvider(ctx, payload); err != nil {
		resp.Diagnostics.AddError("Failed to configure system provider", err.Error())

		return
	}

	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *systemProviderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state systemProviderModel
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

func (r *systemProviderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan systemProviderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := osapi.SystemProvider{
		Config: osapi.SystemProviderConfig{
			Name:   plan.Name.ValueString(),
			Config: mapToStringMap(ctx, plan.Config, &resp.Diagnostics),
		},
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateSystemProvider(ctx, payload); err != nil {
		resp.Diagnostics.AddError("Failed to update system provider", err.Error())

		return
	}

	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *systemProviderResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Removing from Terraform state does not unregister the host's update provider.
}

func (r *systemProviderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *systemProviderResource) readIntoModel(ctx context.Context, model *systemProviderModel, diags *diag.Diagnostics) {
	res, err := r.client.GetSystemProvider(ctx)
	if err != nil {
		diags.AddError("Failed to read system provider", err.Error())

		return
	}

	model.ID = types.StringValue("system-provider")
	model.Name = types.StringValue(res.Config.Name)
	model.Config = stringMapToMap(ctx, res.Config.Config, model.Config.IsNull(), diags)
	model.Registered = types.BoolValue(res.State.Registered)
}
