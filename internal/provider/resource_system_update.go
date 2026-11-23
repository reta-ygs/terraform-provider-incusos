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
	_ resource.Resource                = (*systemUpdateResource)(nil)
	_ resource.ResourceWithImportState = (*systemUpdateResource)(nil)
)

type systemUpdateResource struct {
	client *client.Client
}

type systemUpdateModel struct {
	ID                 types.String                      `tfsdk:"id"`
	AutoReboot         types.Bool                        `tfsdk:"auto_reboot"`
	Channel            types.String                      `tfsdk:"channel"`
	CheckFrequency     types.String                      `tfsdk:"check_frequency"`
	MaintenanceWindows []systemUpdateMaintenanceWinModel `tfsdk:"maintenance_windows"`
	Status             types.String                      `tfsdk:"status"`
	NeedsReboot        types.Bool                        `tfsdk:"needs_reboot"`
}

type systemUpdateMaintenanceWinModel struct {
	StartDayOfWeek types.String `tfsdk:"start_day_of_week"`
	StartHour      types.Int64  `tfsdk:"start_hour"`
	StartMinute    types.Int64  `tfsdk:"start_minute"`
	EndDayOfWeek   types.String `tfsdk:"end_day_of_week"`
	EndHour        types.Int64  `tfsdk:"end_hour"`
	EndMinute      types.Int64  `tfsdk:"end_minute"`
}

func NewSystemUpdateResource() resource.Resource {
	return &systemUpdateResource{}
}

func (r *systemUpdateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_update"
}

func (r *systemUpdateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages IncusOS system update policy (`/1.0/system/update`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"auto_reboot": schema.BoolAttribute{
				MarkdownDescription: "Automatically reboot when an OS update requires a reboot.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"channel": schema.StringAttribute{
				MarkdownDescription: "Update channel (e.g., `stable`, `testing`).",
				Required:            true,
			},
			"check_frequency": schema.StringAttribute{
				MarkdownDescription: "Update check frequency as a Go duration (e.g., `6h`) or `never`.",
				Required:            true,
			},
			"maintenance_windows": schema.ListNestedAttribute{
				MarkdownDescription: "Optional maintenance windows during which updates may be checked and applied.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"start_day_of_week": schema.StringAttribute{
							MarkdownDescription: "Start day of week (`Sunday`..`Saturday`). Must be set together with `end_day_of_week`.",
							Optional:            true,
						},
						"start_hour": schema.Int64Attribute{
							MarkdownDescription: "Start hour (0-23).",
							Required:            true,
						},
						"start_minute": schema.Int64Attribute{
							MarkdownDescription: "Start minute (0-59).",
							Required:            true,
						},
						"end_day_of_week": schema.StringAttribute{
							MarkdownDescription: "End day of week (`Sunday`..`Saturday`). Must be set together with `start_day_of_week`.",
							Optional:            true,
						},
						"end_hour": schema.Int64Attribute{
							MarkdownDescription: "End hour (0-23).",
							Required:            true,
						},
						"end_minute": schema.Int64Attribute{
							MarkdownDescription: "End minute (0-59).",
							Required:            true,
						},
					},
				},
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Current update daemon status.",
				Computed:            true,
			},
			"needs_reboot": schema.BoolAttribute{
				MarkdownDescription: "Whether the host requires a reboot to apply a pending OS update.",
				Computed:            true,
			},
		},
	}
}

func (r *systemUpdateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (r *systemUpdateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan systemUpdateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateSystemUpdate(ctx, plan.toAPI()); err != nil {
		resp.Diagnostics.AddError("Failed to configure system update policy", err.Error())

		return
	}

	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *systemUpdateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state systemUpdateModel
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

func (r *systemUpdateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan systemUpdateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateSystemUpdate(ctx, plan.toAPI()); err != nil {
		resp.Diagnostics.AddError("Failed to update system update policy", err.Error())

		return
	}

	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *systemUpdateResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Removing from Terraform state does not alter the host's update policy.
}

func (r *systemUpdateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *systemUpdateModel) toAPI() osapi.SystemUpdate {
	mws := make([]osapi.SystemUpdateMaintenanceWindow, 0, len(m.MaintenanceWindows))
	for _, w := range m.MaintenanceWindows {
		mws = append(mws, osapi.SystemUpdateMaintenanceWindow{
			StartDayOfWeek: osapi.Weekday(w.StartDayOfWeek.ValueString()),
			StartHour:      int(w.StartHour.ValueInt64()),
			StartMinute:    int(w.StartMinute.ValueInt64()),
			EndDayOfWeek:   osapi.Weekday(w.EndDayOfWeek.ValueString()),
			EndHour:        int(w.EndHour.ValueInt64()),
			EndMinute:      int(w.EndMinute.ValueInt64()),
		})
	}

	return osapi.SystemUpdate{
		Config: osapi.SystemUpdateConfig{
			AutoReboot:         m.AutoReboot.ValueBool(),
			Channel:            m.Channel.ValueString(),
			CheckFrequency:     m.CheckFrequency.ValueString(),
			MaintenanceWindows: mws,
		},
	}
}

func (r *systemUpdateResource) readIntoModel(ctx context.Context, model *systemUpdateModel, diags *diag.Diagnostics) {
	res, err := r.client.GetSystemUpdate(ctx)
	if err != nil {
		diags.AddError("Failed to read system update policy", err.Error())

		return
	}

	model.ID = types.StringValue("system-update")
	model.AutoReboot = types.BoolValue(res.Config.AutoReboot)
	model.Channel = types.StringValue(res.Config.Channel)
	model.CheckFrequency = types.StringValue(res.Config.CheckFrequency)
	model.Status = types.StringValue(res.State.Status)
	model.NeedsReboot = types.BoolValue(res.State.NeedsReboot)

	if len(res.Config.MaintenanceWindows) == 0 {
		if model.MaintenanceWindows != nil {
			model.MaintenanceWindows = []systemUpdateMaintenanceWinModel{}
		}
	} else {
		mws := make([]systemUpdateMaintenanceWinModel, 0, len(res.Config.MaintenanceWindows))
		for i, w := range res.Config.MaintenanceWindows {
			wasStartDayNull := i >= len(model.MaintenanceWindows) || model.MaintenanceWindows[i].StartDayOfWeek.IsNull()
			wasEndDayNull := i >= len(model.MaintenanceWindows) || model.MaintenanceWindows[i].EndDayOfWeek.IsNull()
			mws = append(mws, systemUpdateMaintenanceWinModel{
				StartDayOfWeek: stringValueOrNull(string(w.StartDayOfWeek), wasStartDayNull),
				StartHour:      types.Int64Value(int64(w.StartHour)),
				StartMinute:    types.Int64Value(int64(w.StartMinute)),
				EndDayOfWeek:   stringValueOrNull(string(w.EndDayOfWeek), wasEndDayNull),
				EndHour:        types.Int64Value(int64(w.EndHour)),
				EndMinute:      types.Int64Value(int64(w.EndMinute)),
			})
		}

		model.MaintenanceWindows = mws
	}
}
