package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	osapi "github.com/lxc/incus-os/incus-osd/api"

	"github.com/reta-ygs/terraform-provider-incusos/internal/client"
)

// --- incusos_system_storage (host-level scrub/trim schedules) ---

var (
	_ resource.Resource                = (*systemStorageResource)(nil)
	_ resource.ResourceWithImportState = (*systemStorageResource)(nil)
)

type systemStorageResource struct {
	client *client.Client
}

type systemStorageModel struct {
	ID            types.String `tfsdk:"id"`
	ScrubSchedule types.String `tfsdk:"scrub_schedule"`
	TrimSchedule  types.String `tfsdk:"trim_schedule"`
}

func NewSystemStorageResource() resource.Resource {
	return &systemStorageResource{}
}

func (r *systemStorageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_storage"
}

func (r *systemStorageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages IncusOS host storage maintenance schedules (`scrub_schedule`, `trim_schedule`) at `/1.0/system/storage`. To manage individual ZFS pools or volumes, use `incusos_system_storage_pool` and `incusos_system_storage_volume`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"scrub_schedule": schema.StringAttribute{
				MarkdownDescription: "Cron expression for scheduled ZFS pool scrubs.",
				Optional:            true,
			},
			"trim_schedule": schema.StringAttribute{
				MarkdownDescription: "Cron expression for scheduled ZFS pool trims.",
				Optional:            true,
			},
		},
	}
}

func (r *systemStorageResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (r *systemStorageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan systemStorageModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.applySchedules(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Failed to configure system storage schedules", err.Error())

		return
	}

	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *systemStorageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state systemStorageModel
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

func (r *systemStorageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan systemStorageModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.applySchedules(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Failed to update system storage schedules", err.Error())

		return
	}

	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *systemStorageResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Removing from state leaves existing host pools and schedules untouched.
}

func (r *systemStorageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *systemStorageResource) applySchedules(ctx context.Context, plan *systemStorageModel) error {
	// Serialize read-modify-write so concurrent pool updates do not clobber schedules.
	return r.client.MutateSystemStorage(ctx, func(current *osapi.SystemStorage) error {
		current.Config.ScrubSchedule = plan.ScrubSchedule.ValueString()
		current.Config.TrimSchedule = plan.TrimSchedule.ValueString()

		return nil
	})
}

func (r *systemStorageResource) readIntoModel(ctx context.Context, model *systemStorageModel, diags *diag.Diagnostics) {
	res, err := r.client.GetSystemStorage(ctx)
	if err != nil {
		diags.AddError("Failed to read system storage", err.Error())

		return
	}

	model.ID = types.StringValue("system-storage")
	model.ScrubSchedule = stringValueOrNull(res.Config.ScrubSchedule, model.ScrubSchedule.IsNull())
	model.TrimSchedule = stringValueOrNull(res.Config.TrimSchedule, model.TrimSchedule.IsNull())
}

// --- incusos_system_storage_pool ---

var (
	_ resource.Resource                = (*systemStoragePoolResource)(nil)
	_ resource.ResourceWithImportState = (*systemStoragePoolResource)(nil)
)

type systemStoragePoolResource struct {
	client *client.Client
}

type systemStoragePoolModel struct {
	ID                        types.String                   `tfsdk:"id"`
	Name                      types.String                   `tfsdk:"name"`
	Type                      types.String                   `tfsdk:"type"`
	AllowMixedDevSizes        types.Bool                     `tfsdk:"allow_mixed_dev_sizes"`
	Alignment                 types.Int64                    `tfsdk:"alignment"`
	Devices                   types.List                     `tfsdk:"devices"`
	Cache                     types.List                     `tfsdk:"cache"`
	Log                       types.List                     `tfsdk:"log"`
	Special                   *systemStoragePoolSpecialModel `tfsdk:"special"`
	Managed                   types.Bool                     `tfsdk:"managed"`
	State                     types.String                   `tfsdk:"state"`
	EncryptionKeyStatus       types.String                   `tfsdk:"encryption_key_status"`
	RawPoolSizeInBytes        types.Int64                    `tfsdk:"raw_pool_size_in_bytes"`
	UsablePoolSizeInBytes     types.Int64                    `tfsdk:"usable_pool_size_in_bytes"`
	PoolAllocatedSpaceInBytes types.Int64                    `tfsdk:"pool_allocated_space_in_bytes"`
}

type systemStoragePoolSpecialModel struct {
	Type                       types.String `tfsdk:"type"`
	SpecialSmallBlocksSizeInKB types.Int64  `tfsdk:"special_small_blocks_size_in_kb"`
	Devices                    types.List   `tfsdk:"devices"`
}

func NewSystemStoragePoolResource() resource.Resource {
	return &systemStoragePoolResource{}
}

func (r *systemStoragePoolResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_storage_pool"
}

func (r *systemStoragePoolResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an IncusOS local ZFS storage pool (`/1.0/system/storage` and `:delete-pool`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Storage pool name. Cannot be changed after creation.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "ZFS pool topology (`zfs-raid0`, `zfs-raid1`, `zfs-raid10`, `zfs-raidz1`, `zfs-raidz2`, `zfs-raidz3`). Cannot be changed after creation.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"allow_mixed_dev_sizes": schema.BoolAttribute{
				MarkdownDescription: "Allow creating a pool with devices of different sizes.",
				Optional:            true,
			},
			"alignment": schema.Int64Attribute{
				MarkdownDescription: "Sector alignment size in bytes (power of two, defaults to 4096). Can only be set on creation.",
				Optional:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"devices": schema.ListAttribute{
				MarkdownDescription: "List of device IDs (e.g., `/dev/disk/by-id/...`) forming the pool.",
				ElementType:         types.StringType,
				Required:            true,
			},
			"cache": schema.ListAttribute{
				MarkdownDescription: "Optional list of cache (L2ARC) device IDs.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"log": schema.ListAttribute{
				MarkdownDescription: "Optional list of log (SLOG) device IDs.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"special": schema.SingleNestedAttribute{
				MarkdownDescription: "Optional ZFS special vdev configuration.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						MarkdownDescription: "Special vdev topology (`zfs-raid0`, `zfs-raid1`, etc.).",
						Required:            true,
					},
					"special_small_blocks_size_in_kb": schema.Int64Attribute{
						MarkdownDescription: "Threshold in KB for storing small blocks on the special vdev.",
						Optional:            true,
					},
					"devices": schema.ListAttribute{
						MarkdownDescription: "Physical devices for the special vdev.",
						ElementType:         types.StringType,
						Required:            true,
					},
				},
			},
			"managed":                       schema.BoolAttribute{Computed: true},
			"state":                         schema.StringAttribute{Computed: true},
			"encryption_key_status":         schema.StringAttribute{Computed: true},
			"raw_pool_size_in_bytes":        schema.Int64Attribute{Computed: true},
			"usable_pool_size_in_bytes":     schema.Int64Attribute{Computed: true},
			"pool_allocated_space_in_bytes": schema.Int64Attribute{Computed: true},
		},
	}
}

func (r *systemStoragePoolResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (r *systemStoragePoolResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan systemStoragePoolModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.upsertPool(ctx, &plan, &resp.Diagnostics); err != nil {
		resp.Diagnostics.AddError("Failed to create storage pool", err.Error())

		return
	}
	if resp.Diagnostics.HasError() {
		return
	}

	found := r.readPool(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		resp.Diagnostics.AddError("Storage pool not found after creation", fmt.Sprintf("Pool %q was not returned by /1.0/system/storage", plan.Name.ValueString()))

		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *systemStoragePoolResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state systemStoragePoolModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.Name.IsNull() || state.Name.ValueString() == "" {
		state.Name = state.ID
	}

	found := r.readPool(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)

		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *systemStoragePoolResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan systemStoragePoolModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.upsertPool(ctx, &plan, &resp.Diagnostics); err != nil {
		resp.Diagnostics.AddError("Failed to update storage pool", err.Error())

		return
	}
	if resp.Diagnostics.HasError() {
		return
	}

	r.readPool(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *systemStoragePoolResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state systemStoragePoolModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteStoragePool(ctx, state.Name.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete storage pool", err.Error())
	}
}

func (r *systemStoragePoolResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func (r *systemStoragePoolResource) upsertPool(ctx context.Context, plan *systemStoragePoolModel, diags *diag.Diagnostics) error {
	pool := osapi.SystemStoragePool{
		Name:               plan.Name.ValueString(),
		Type:               plan.Type.ValueString(),
		AllowMixedDevSizes: plan.AllowMixedDevSizes.ValueBool(),
		Alignment:          int(plan.Alignment.ValueInt64()),
		Devices:            listToStringSlice(ctx, plan.Devices, diags),
		Cache:              listToStringSlice(ctx, plan.Cache, diags),
		Log:                listToStringSlice(ctx, plan.Log, diags),
	}

	if plan.Special != nil {
		pool.Special = &osapi.SystemStoragePoolSpecial{
			Type:                       plan.Special.Type.ValueString(),
			SpecialSmallBlocksSizeInKB: int(plan.Special.SpecialSmallBlocksSizeInKB.ValueInt64()),
			Devices:                    listToStringSlice(ctx, plan.Special.Devices, diags),
		}
	}

	if diags.HasError() {
		return nil
	}

	return r.client.MutateSystemStorage(ctx, func(current *osapi.SystemStorage) error {
		replaced := false
		for i, existing := range current.Config.Pools {
			if existing.Name == pool.Name {
				current.Config.Pools[i] = pool
				replaced = true

				break
			}
		}

		if !replaced {
			current.Config.Pools = append(current.Config.Pools, pool)
		}

		return nil
	})
}

func (r *systemStoragePoolResource) readPool(ctx context.Context, model *systemStoragePoolModel, diags *diag.Diagnostics) bool {
	res, err := r.client.GetSystemStorage(ctx)
	if err != nil {
		diags.AddError("Failed to read system storage", err.Error())

		return false
	}

	targetName := model.Name.ValueString()
	for _, p := range res.Config.Pools {
		if p.Name != targetName {
			continue
		}

		model.ID = types.StringValue(p.Name)
		model.Name = types.StringValue(p.Name)
		model.Type = types.StringValue(p.Type)
		model.AllowMixedDevSizes = boolValueOrNull(p.AllowMixedDevSizes, model.AllowMixedDevSizes.IsNull())
		model.Alignment = intValueOrNull(p.Alignment, model.Alignment.IsNull())
		model.Devices = stringSliceToList(ctx, p.Devices, false, diags)
		model.Cache = stringSliceToList(ctx, p.Cache, model.Cache.IsNull(), diags)
		model.Log = stringSliceToList(ctx, p.Log, model.Log.IsNull(), diags)

		if p.Special != nil {
			wasKBNull := model.Special == nil || model.Special.SpecialSmallBlocksSizeInKB.IsNull()
			model.Special = &systemStoragePoolSpecialModel{
				Type:                       types.StringValue(p.Special.Type),
				SpecialSmallBlocksSizeInKB: intValueOrNull(p.Special.SpecialSmallBlocksSizeInKB, wasKBNull),
				Devices:                    stringSliceToList(ctx, p.Special.Devices, false, diags),
			}
		} else {
			model.Special = nil
		}

		model.Managed = types.BoolValue(p.Managed)
		model.State = types.StringValue(p.State)
		model.EncryptionKeyStatus = types.StringValue(p.EncryptionKeyStatus)
		model.RawPoolSizeInBytes = types.Int64Value(int64(p.RawPoolSizeInBytes))
		model.UsablePoolSizeInBytes = types.Int64Value(int64(p.UsablePoolSizeInBytes))
		model.PoolAllocatedSpaceInBytes = types.Int64Value(int64(p.PoolAllocatedSpaceInBytes))

		return true
	}

	return false
}

// --- incusos_system_storage_volume ---

var (
	_ resource.Resource                = (*systemStorageVolumeResource)(nil)
	_ resource.ResourceWithImportState = (*systemStorageVolumeResource)(nil)
)

type systemStorageVolumeResource struct {
	client *client.Client
}

type systemStorageVolumeModel struct {
	ID           types.String `tfsdk:"id"`
	Pool         types.String `tfsdk:"pool"`
	Name         types.String `tfsdk:"name"`
	QuotaInBytes types.Int64  `tfsdk:"quota_in_bytes"`
	Use          types.String `tfsdk:"use"`
	ForceDelete  types.Bool   `tfsdk:"force_delete"`
	UsageInBytes types.Int64  `tfsdk:"usage_in_bytes"`
}

func NewSystemStorageVolumeResource() resource.Resource {
	return &systemStorageVolumeResource{}
}

func (r *systemStorageVolumeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_storage_volume"
}

func (r *systemStorageVolumeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an IncusOS-managed volume on a local ZFS storage pool (`:create-volume` / `:delete-volume`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"pool": schema.StringAttribute{
				MarkdownDescription: "Name of the storage pool.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Volume name.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"quota_in_bytes": schema.Int64Attribute{
				MarkdownDescription: "Optional volume quota in bytes (0 for unlimited).",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"use": schema.StringAttribute{
				MarkdownDescription: "Intended use of the volume (e.g., `incus`, `linstor`).",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"force_delete": schema.BoolAttribute{
				MarkdownDescription: "Force deletion of the volume on destroy.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"usage_in_bytes": schema.Int64Attribute{
				MarkdownDescription: "Current space usage in bytes.",
				Computed:            true,
			},
		},
	}
}

func (r *systemStorageVolumeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (r *systemStorageVolumeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan systemStorageVolumeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.CreateStorageVolume(ctx, client.CreateStorageVolumeRequest{
		Pool:  plan.Pool.ValueString(),
		Name:  plan.Name.ValueString(),
		Quota: int(plan.QuotaInBytes.ValueInt64()),
		Use:   plan.Use.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create storage volume", err.Error())

		return
	}

	found := r.readVolume(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		resp.Diagnostics.AddError("Storage volume not found after creation", fmt.Sprintf("Volume %s/%s not found", plan.Pool.ValueString(), plan.Name.ValueString()))

		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *systemStorageVolumeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state systemStorageVolumeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	found := r.readVolume(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)

		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *systemStorageVolumeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan systemStorageVolumeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Only force_delete can be updated in place.
	r.readVolume(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *systemStorageVolumeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state systemStorageVolumeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteStorageVolume(ctx, client.DeleteStorageVolumeRequest{
		Pool:  state.Pool.ValueString(),
		Name:  state.Name.ValueString(),
		Force: state.ForceDelete.ValueBool(),
	})
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete storage volume", err.Error())
	}
}

func (r *systemStorageVolumeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	pool, name, ok := strings.Cut(req.ID, "/")
	if !ok || pool == "" || name == "" {
		resp.Diagnostics.AddError("Invalid Import ID", "Expected import ID in the format `<pool>/<volume_name>`.")

		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pool"), pool)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("force_delete"), false)...)
}

func (r *systemStorageVolumeResource) readVolume(ctx context.Context, model *systemStorageVolumeModel, diags *diag.Diagnostics) bool {
	res, err := r.client.GetSystemStorage(ctx)
	if err != nil {
		diags.AddError("Failed to read system storage", err.Error())

		return false
	}

	for _, p := range res.Config.Pools {
		if p.Name != model.Pool.ValueString() {
			continue
		}

		for _, v := range p.Volumes {
			if v.Name != model.Name.ValueString() {
				continue
			}

			model.ID = types.StringValue(p.Name + "/" + v.Name)
			model.QuotaInBytes = types.Int64Value(int64(v.QuotaInBytes))
			model.Use = types.StringValue(v.Use)
			model.UsageInBytes = types.Int64Value(int64(v.UsageInBytes))

			return true
		}
	}

	return false
}
