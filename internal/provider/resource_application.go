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
	_ resource.Resource                = (*applicationResource)(nil)
	_ resource.ResourceWithImportState = (*applicationResource)(nil)
)

type applicationResource struct {
	client *client.Client
}

type applicationModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	PinCurrentVersions types.Bool   `tfsdk:"pin_current_versions"`
	IsPrimary          types.Bool   `tfsdk:"is_primary"`
	Initialized        types.Bool   `tfsdk:"initialized"`
	FriendlyVersion    types.String `tfsdk:"friendly_version"`
	Version            types.String `tfsdk:"version"`
	AvailableVersions  types.List   `tfsdk:"available_versions"`
}

func NewApplicationResource() resource.Resource {
	return &applicationResource{}
}

func (r *applicationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_application"
}

func (r *applicationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an IncusOS application (`/1.0/applications/{name}`), such as `gpu-support`, `debug`, `incus`, `migration-manager`, or `operations-center`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Application name (e.g., `gpu-support`, `debug`, `incus`).",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"pin_current_versions": schema.BoolAttribute{
				MarkdownDescription: "When true, prevent newer versions of this application from being downloaded.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"is_primary": schema.BoolAttribute{
				MarkdownDescription: "Whether this is the primary application on the IncusOS host.",
				Computed:            true,
			},
			"initialized": schema.BoolAttribute{
				MarkdownDescription: "Whether the application has been initialized.",
				Computed:            true,
			},
			"friendly_version": schema.StringAttribute{
				MarkdownDescription: "Human-readable application version string.",
				Computed:            true,
			},
			"version": schema.StringAttribute{
				MarkdownDescription: "Installed application version identifier.",
				Computed:            true,
			},
			"available_versions": schema.ListAttribute{
				MarkdownDescription: "List of locally available versions for this application.",
				ElementType:         types.StringType,
				Computed:            true,
			},
		},
	}
}

func (r *applicationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (r *applicationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan applicationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := plan.Name.ValueString()
	if err := r.client.InstallApplication(ctx, name); err != nil {
		resp.Diagnostics.AddError("Failed to install application", err.Error())

		return
	}

	if plan.PinCurrentVersions.ValueBool() {
		err := r.client.UpdateApplication(ctx, name, osapi.Application{
			Config: osapi.ApplicationConfig{
				PinCurrentVersions: true,
			},
		})
		if err != nil {
			resp.Diagnostics.AddError("Failed to configure application", err.Error())

			return
		}
	}

	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *applicationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state applicationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.Name.IsNull() || state.Name.ValueString() == "" {
		state.Name = state.ID
	}

	found := r.readIntoModel(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)

		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *applicationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan applicationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.UpdateApplication(ctx, plan.Name.ValueString(), osapi.Application{
		Config: osapi.ApplicationConfig{
			PinCurrentVersions: plan.PinCurrentVersions.ValueBool(),
		},
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to update application", err.Error())

		return
	}

	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *applicationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state applicationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.RemoveApplication(ctx, state.Name.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to remove application", err.Error())
	}
}

func (r *applicationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func (r *applicationResource) readIntoModel(ctx context.Context, model *applicationModel, diags *diag.Diagnostics) bool {
	res, err := r.client.GetApplication(ctx, model.Name.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			return false
		}

		diags.AddError("Failed to read application", err.Error())

		return false
	}

	model.ID = model.Name
	model.PinCurrentVersions = types.BoolValue(res.Config.PinCurrentVersions)
	model.IsPrimary = types.BoolValue(res.State.IsPrimary)
	model.Initialized = types.BoolValue(res.State.Initialized)
	model.FriendlyVersion = types.StringValue(res.State.FriendlyVersion)
	model.Version = types.StringValue(res.State.Version)
	model.AvailableVersions = stringSliceToList(ctx, res.State.AvailableVersions, false, diags)

	return true
}
