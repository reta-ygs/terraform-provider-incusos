package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	osapi "github.com/lxc/incus-os/incus-osd/api"

	"github.com/reta-ygs/terraform-provider-incusos/internal/client"
)

var (
	_ resource.Resource                = (*systemPowerResource)(nil)
	_ resource.ResourceWithImportState = (*systemPowerResource)(nil)
)

type systemPowerResource struct {
	client *client.Client
}

type systemPowerModel struct {
	ID                             types.String `tfsdk:"id"`
	CPUScalingGovernor             types.String `tfsdk:"cpu_scaling_governor"`
	CPUEnergyPerformancePreference types.String `tfsdk:"cpu_energy_performance_preference"`
	CPUScalingDriver               types.String `tfsdk:"cpu_scaling_driver"`
	CPUAvailableGovernors          types.List   `tfsdk:"cpu_available_governors"`
	CPUAvailablePreferences        types.List   `tfsdk:"cpu_available_preferences"`
	ScreenBlankTimeout             types.Int64  `tfsdk:"screen_blank_timeout"`
}

func NewSystemPowerResource() resource.Resource {
	return &systemPowerResource{}
}

func (r *systemPowerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_power"
}

func (r *systemPowerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages IncusOS system power management configuration (`/1.0/system/power`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cpu_scaling_governor": schema.StringAttribute{
				MarkdownDescription: "CPU frequency scaling governor (e.g., `performance`, `powersave`).",
				Optional:            true,
			},
			"cpu_energy_performance_preference": schema.StringAttribute{
				MarkdownDescription: "CPU energy performance preference (e.g., `balance_performance`, `power`).",
				Optional:            true,
			},
			"cpu_scaling_driver": schema.StringAttribute{
				MarkdownDescription: "Active CPU scaling driver reported by the host.",
				Computed:            true,
			},
			"cpu_available_governors": schema.ListAttribute{
				MarkdownDescription: "Available CPU scaling governors reported by the host.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"cpu_available_preferences": schema.ListAttribute{
				MarkdownDescription: "Available CPU energy performance preferences reported by the host.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"screen_blank_timeout": schema.Int64Attribute{
				MarkdownDescription: "Console screen blanking timeout in minutes without keyboard input (0 to 60; 0 disables blanking).",
				Optional:            true,
			},
		},
	}
}

func (r *systemPowerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (r *systemPowerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan systemPowerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateSystemPower(ctx, plan.toAPI()); err != nil {
		resp.Diagnostics.AddError("Failed to configure system power", err.Error())

		return
	}

	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *systemPowerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state systemPowerModel
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

func (r *systemPowerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan systemPowerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateSystemPower(ctx, plan.toAPI()); err != nil {
		resp.Diagnostics.AddError("Failed to update system power", err.Error())

		return
	}

	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *systemPowerResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.UpdateSystemPower(ctx, osapi.SystemPower{}); err != nil {
		resp.Diagnostics.AddError("Failed to reset system power", err.Error())
	}
}

func (r *systemPowerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *systemPowerModel) toAPI() osapi.SystemPower {
	out := osapi.SystemPower{}
	if !m.CPUScalingGovernor.IsNull() || !m.CPUEnergyPerformancePreference.IsNull() {
		out.Config.CPU = &osapi.SystemPowerConfigCPU{
			ScalingGovernor:             m.CPUScalingGovernor.ValueString(),
			EnergyPerformancePreference: m.CPUEnergyPerformancePreference.ValueString(),
		}
	}

	if !m.ScreenBlankTimeout.IsNull() {
		out.Config.Screen = &osapi.SystemPowerConfigScreen{
			BlankTimeout: int(m.ScreenBlankTimeout.ValueInt64()),
		}
	}

	return out
}

func (r *systemPowerResource) readIntoModel(ctx context.Context, model *systemPowerModel, diags *diagDiagnosticsAlias) {
	res, err := r.client.GetSystemPower(ctx)
	if err != nil {
		diags.AddError("Failed to read system power", err.Error())

		return
	}

	model.ID = types.StringValue("system-power")
	if res.Config.CPU != nil {
		model.CPUScalingGovernor = stringValueOrNull(res.Config.CPU.ScalingGovernor, model.CPUScalingGovernor.IsNull())
		model.CPUEnergyPerformancePreference = stringValueOrNull(res.Config.CPU.EnergyPerformancePreference, model.CPUEnergyPerformancePreference.IsNull())
	} else {
		model.CPUScalingGovernor = types.StringNull()
		model.CPUEnergyPerformancePreference = types.StringNull()
	}

	if res.Config.Screen != nil {
		model.ScreenBlankTimeout = types.Int64Value(int64(res.Config.Screen.BlankTimeout))
	} else {
		model.ScreenBlankTimeout = types.Int64Null()
	}

	model.CPUScalingDriver = types.StringValue(res.State.CPU.ScalingDriver)
	model.CPUAvailableGovernors = stringSliceToList(ctx, res.State.CPU.ScalingAvailableGovernors, false, diags)
	model.CPUAvailablePreferences = stringSliceToList(ctx, res.State.CPU.EnergyPerformanceAvailablePreferences, false, diags)
}
