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

// ==================== incusos_service_ceph ====================

var (
	_ resource.Resource                = (*serviceCephResource)(nil)
	_ resource.ResourceWithImportState = (*serviceCephResource)(nil)
)

type serviceCephResource struct{ client *client.Client }

type serviceCephModel struct {
	ID       types.String                      `tfsdk:"id"`
	Enabled  types.Bool                        `tfsdk:"enabled"`
	Clusters map[string]serviceCephClusterItem `tfsdk:"clusters"`
}

type serviceCephClusterItem struct {
	FSID         types.String `tfsdk:"fsid"`
	Monitors     types.List   `tfsdk:"monitors"`
	Keyrings     types.Map    `tfsdk:"keyrings"`
	ClientConfig types.Map    `tfsdk:"client_config"`
}

func NewServiceCephResource() resource.Resource { return &serviceCephResource{} }

func (r *serviceCephResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_ceph"
}

func (r *serviceCephResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the IncusOS Ceph client service (`/1.0/services/ceph`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"clusters": schema.MapNestedAttribute{
				MarkdownDescription: "Map of Ceph cluster name to cluster configuration.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"fsid":          schema.StringAttribute{Required: true},
						"monitors":      schema.ListAttribute{ElementType: types.StringType, Required: true},
						"keyrings":      schema.MapAttribute{ElementType: types.StringType, Optional: true, Sensitive: true},
						"client_config": schema.MapAttribute{ElementType: types.StringType, Optional: true},
					},
				},
			},
		},
	}
}

func (r *serviceCephResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (r *serviceCephResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serviceCephModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateService(ctx, "ceph", plan.toAPI(ctx, &resp.Diagnostics)); err != nil {
		resp.Diagnostics.AddError("Failed to configure Ceph service", err.Error())
		return
	}
	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceCephResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serviceCephModel
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

func (r *serviceCephResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serviceCephModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateService(ctx, "ceph", plan.toAPI(ctx, &resp.Diagnostics)); err != nil {
		resp.Diagnostics.AddError("Failed to update Ceph service", err.Error())
		return
	}
	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceCephResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.UpdateService(ctx, "ceph", osapi.ServiceCeph{}); err != nil {
		resp.Diagnostics.AddError("Failed to disable Ceph service", err.Error())
	}
}

func (r *serviceCephResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *serviceCephModel) toAPI(ctx context.Context, diags *diag.Diagnostics) osapi.ServiceCeph {
	clusters := make(map[string]osapi.ServiceCephCluster, len(m.Clusters))
	for name, c := range m.Clusters {
		krMap := mapToStringMap(ctx, c.Keyrings, diags)
		keyrings := make(map[string]osapi.ServiceCephKeyring, len(krMap))
		for k, v := range krMap {
			keyrings[k] = osapi.ServiceCephKeyring{Key: v}
		}
		clusters[name] = osapi.ServiceCephCluster{
			FSID:         c.FSID.ValueString(),
			Monitors:     listToStringSlice(ctx, c.Monitors, diags),
			Keyrings:     keyrings,
			ClientConfig: mapToStringMap(ctx, c.ClientConfig, diags),
		}
	}
	return osapi.ServiceCeph{
		Config: osapi.ServiceCephConfig{
			Enabled:  m.Enabled.ValueBool(),
			Clusters: clusters,
		},
	}
}

func (r *serviceCephResource) readIntoModel(ctx context.Context, model *serviceCephModel, diags *diag.Diagnostics) {
	var res osapi.ServiceCeph
	if err := r.client.GetService(ctx, "ceph", &res); err != nil {
		diags.AddError("Failed to read Ceph service", err.Error())
		return
	}
	model.ID = types.StringValue("service-ceph")
	model.Enabled = types.BoolValue(res.Config.Enabled)
	if len(res.Config.Clusters) == 0 {
		if model.Clusters != nil {
			model.Clusters = map[string]serviceCephClusterItem{}
		}
	} else {
		out := make(map[string]serviceCephClusterItem, len(res.Config.Clusters))
		for name, c := range res.Config.Clusters {
			var prev *serviceCephClusterItem
			if existing, ok := model.Clusters[name]; ok {
				prev = &existing
			}
			krMap := make(map[string]string, len(c.Keyrings))
			for k, v := range c.Keyrings {
				krMap[k] = v.Key
			}
			out[name] = serviceCephClusterItem{
				FSID:         types.StringValue(c.FSID),
				Monitors:     stringSliceToList(ctx, c.Monitors, false, diags),
				Keyrings:     stringMapToMap(ctx, krMap, prev == nil || prev.Keyrings.IsNull(), diags),
				ClientConfig: stringMapToMap(ctx, c.ClientConfig, prev == nil || prev.ClientConfig.IsNull(), diags),
			}
		}
		model.Clusters = out
	}
}

// ==================== incusos_service_iscsi ====================

var (
	_ resource.Resource                = (*serviceISCSIResource)(nil)
	_ resource.ResourceWithImportState = (*serviceISCSIResource)(nil)
)

type serviceISCSIResource struct{ client *client.Client }

type serviceISCSIModel struct {
	ID            types.String              `tfsdk:"id"`
	Enabled       types.Bool                `tfsdk:"enabled"`
	Targets       []serviceISCSITargetModel `tfsdk:"targets"`
	InitiatorName types.String              `tfsdk:"initiator_name"`
}

type serviceISCSITargetModel struct {
	Target  types.String `tfsdk:"target"`
	Address types.String `tfsdk:"address"`
	Port    types.Int64  `tfsdk:"port"`
}

func NewServiceISCSIResource() resource.Resource { return &serviceISCSIResource{} }

func (r *serviceISCSIResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_iscsi"
}

func (r *serviceISCSIResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the IncusOS iSCSI initiator service (`/1.0/services/iscsi`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"targets": schema.ListNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"target":  schema.StringAttribute{Required: true},
						"address": schema.StringAttribute{Required: true},
						"port":    schema.Int64Attribute{Optional: true},
					},
				},
			},
			"initiator_name": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *serviceISCSIResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (r *serviceISCSIResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serviceISCSIModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateService(ctx, "iscsi", plan.toAPI()); err != nil {
		resp.Diagnostics.AddError("Failed to configure iSCSI service", err.Error())
		return
	}
	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceISCSIResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serviceISCSIModel
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

func (r *serviceISCSIResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serviceISCSIModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateService(ctx, "iscsi", plan.toAPI()); err != nil {
		resp.Diagnostics.AddError("Failed to update iSCSI service", err.Error())
		return
	}
	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceISCSIResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.UpdateService(ctx, "iscsi", osapi.ServiceISCSI{}); err != nil {
		resp.Diagnostics.AddError("Failed to disable iSCSI service", err.Error())
	}
}

func (r *serviceISCSIResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *serviceISCSIModel) toAPI() osapi.ServiceISCSI {
	targets := make([]osapi.ServiceISCSITarget, 0, len(m.Targets))
	for _, t := range m.Targets {
		targets = append(targets, osapi.ServiceISCSITarget{
			Target:  t.Target.ValueString(),
			Address: t.Address.ValueString(),
			Port:    int(t.Port.ValueInt64()),
		})
	}
	return osapi.ServiceISCSI{
		Config: osapi.ServiceISCSIConfig{
			Enabled: m.Enabled.ValueBool(),
			Targets: targets,
		},
	}
}

func (r *serviceISCSIResource) readIntoModel(ctx context.Context, model *serviceISCSIModel, diags *diag.Diagnostics) {
	var res osapi.ServiceISCSI
	if err := r.client.GetService(ctx, "iscsi", &res); err != nil {
		diags.AddError("Failed to read iSCSI service", err.Error())
		return
	}
	model.ID = types.StringValue("service-iscsi")
	model.Enabled = types.BoolValue(res.Config.Enabled)
	model.InitiatorName = types.StringValue(res.State.InitiatorName)
	if len(res.Config.Targets) == 0 {
		if model.Targets != nil {
			model.Targets = []serviceISCSITargetModel{}
		}
	} else {
		targets := make([]serviceISCSITargetModel, 0, len(res.Config.Targets))
		for i, t := range res.Config.Targets {
			wasPortNull := i >= len(model.Targets) || model.Targets[i].Port.IsNull()
			targets = append(targets, serviceISCSITargetModel{
				Target:  types.StringValue(t.Target),
				Address: types.StringValue(t.Address),
				Port:    intValueOrNull(t.Port, wasPortNull),
			})
		}
		model.Targets = targets
	}
}

// ==================== incusos_service_linstor ====================

var (
	_ resource.Resource                = (*serviceLinstorResource)(nil)
	_ resource.ResourceWithImportState = (*serviceLinstorResource)(nil)
)

type serviceLinstorResource struct{ client *client.Client }

type serviceLinstorModel struct {
	ID                     types.String `tfsdk:"id"`
	Enabled                types.Bool   `tfsdk:"enabled"`
	ListenAddress          types.String `tfsdk:"listen_address"`
	TLSServerCertificate   types.String `tfsdk:"tls_server_certificate"`
	TLSServerKey           types.String `tfsdk:"tls_server_key"`
	TLSTrustedCertificates types.List   `tfsdk:"tls_trusted_certificates"`
}

func NewServiceLinstorResource() resource.Resource { return &serviceLinstorResource{} }

func (r *serviceLinstorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_linstor"
}

func (r *serviceLinstorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the IncusOS LINSTOR satellite service (`/1.0/services/linstor`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"listen_address":           schema.StringAttribute{Optional: true},
			"tls_server_certificate":   schema.StringAttribute{Optional: true, Sensitive: true},
			"tls_server_key":           schema.StringAttribute{Optional: true, Sensitive: true},
			"tls_trusted_certificates": schema.ListAttribute{ElementType: types.StringType, Optional: true},
		},
	}
}

func (r *serviceLinstorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (r *serviceLinstorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serviceLinstorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	payload := osapi.ServiceLinstor{
		Config: osapi.ServiceLinstorConfig{
			Enabled:                plan.Enabled.ValueBool(),
			ListenAddress:          plan.ListenAddress.ValueString(),
			TLSServerCertificate:   plan.TLSServerCertificate.ValueString(),
			TLSServerKey:           plan.TLSServerKey.ValueString(),
			TLSTrustedCertificates: listToStringSlice(ctx, plan.TLSTrustedCertificates, &resp.Diagnostics),
		},
	}
	if err := r.client.UpdateService(ctx, "linstor", payload); err != nil {
		resp.Diagnostics.AddError("Failed to configure LINSTOR service", err.Error())
		return
	}
	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceLinstorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serviceLinstorModel
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

func (r *serviceLinstorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serviceLinstorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	payload := osapi.ServiceLinstor{
		Config: osapi.ServiceLinstorConfig{
			Enabled:                plan.Enabled.ValueBool(),
			ListenAddress:          plan.ListenAddress.ValueString(),
			TLSServerCertificate:   plan.TLSServerCertificate.ValueString(),
			TLSServerKey:           plan.TLSServerKey.ValueString(),
			TLSTrustedCertificates: listToStringSlice(ctx, plan.TLSTrustedCertificates, &resp.Diagnostics),
		},
	}
	if err := r.client.UpdateService(ctx, "linstor", payload); err != nil {
		resp.Diagnostics.AddError("Failed to update LINSTOR service", err.Error())
		return
	}
	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceLinstorResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.UpdateService(ctx, "linstor", osapi.ServiceLinstor{}); err != nil {
		resp.Diagnostics.AddError("Failed to disable LINSTOR service", err.Error())
	}
}

func (r *serviceLinstorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *serviceLinstorResource) readIntoModel(ctx context.Context, model *serviceLinstorModel, diags *diag.Diagnostics) {
	var res osapi.ServiceLinstor
	if err := r.client.GetService(ctx, "linstor", &res); err != nil {
		diags.AddError("Failed to read LINSTOR service", err.Error())
		return
	}
	model.ID = types.StringValue("service-linstor")
	model.Enabled = types.BoolValue(res.Config.Enabled)
	model.ListenAddress = stringValueOrNull(res.Config.ListenAddress, model.ListenAddress.IsNull())
	model.TLSServerCertificate = stringValueOrNull(res.Config.TLSServerCertificate, model.TLSServerCertificate.IsNull())
	model.TLSServerKey = stringValueOrNull(res.Config.TLSServerKey, model.TLSServerKey.IsNull())
	model.TLSTrustedCertificates = stringSliceToList(ctx, res.Config.TLSTrustedCertificates, model.TLSTrustedCertificates.IsNull(), diags)
}

// ==================== incusos_service_lvm ====================

var (
	_ resource.Resource                = (*serviceLVMResource)(nil)
	_ resource.ResourceWithImportState = (*serviceLVMResource)(nil)
)

type serviceLVMResource struct{ client *client.Client }

type serviceLVMModel struct {
	ID              types.String `tfsdk:"id"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	SystemID        types.Int64  `tfsdk:"system_id"`
	IOTimeout       types.Int64  `tfsdk:"io_timeout"`
	WatchdogTimeout types.Int64  `tfsdk:"watchdog_timeout"`
}

func NewServiceLVMResource() resource.Resource { return &serviceLVMResource{} }

func (r *serviceLVMResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_lvm"
}

func (r *serviceLVMResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the IncusOS shared LVM (lvmlockd/sanlock) service (`/1.0/services/lvm`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"system_id":        schema.Int64Attribute{Required: true},
			"io_timeout":       schema.Int64Attribute{Optional: true},
			"watchdog_timeout": schema.Int64Attribute{Optional: true},
		},
	}
}

func (r *serviceLVMResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (r *serviceLVMResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serviceLVMModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	payload := osapi.ServiceLVM{
		Config: osapi.ServiceLVMConfig{
			Enabled:         plan.Enabled.ValueBool(),
			SystemID:        int(plan.SystemID.ValueInt64()),
			IOTimeout:       int(plan.IOTimeout.ValueInt64()),
			WatchdogTimeout: int(plan.WatchdogTimeout.ValueInt64()),
		},
	}
	if err := r.client.UpdateService(ctx, "lvm", payload); err != nil {
		resp.Diagnostics.AddError("Failed to configure LVM service", err.Error())
		return
	}
	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceLVMResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serviceLVMModel
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

func (r *serviceLVMResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serviceLVMModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	payload := osapi.ServiceLVM{
		Config: osapi.ServiceLVMConfig{
			Enabled:         plan.Enabled.ValueBool(),
			SystemID:        int(plan.SystemID.ValueInt64()),
			IOTimeout:       int(plan.IOTimeout.ValueInt64()),
			WatchdogTimeout: int(plan.WatchdogTimeout.ValueInt64()),
		},
	}
	if err := r.client.UpdateService(ctx, "lvm", payload); err != nil {
		resp.Diagnostics.AddError("Failed to update LVM service", err.Error())
		return
	}
	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceLVMResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.UpdateService(ctx, "lvm", osapi.ServiceLVM{}); err != nil {
		resp.Diagnostics.AddError("Failed to disable LVM service", err.Error())
	}
}

func (r *serviceLVMResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *serviceLVMResource) readIntoModel(ctx context.Context, model *serviceLVMModel, diags *diag.Diagnostics) {
	var res osapi.ServiceLVM
	if err := r.client.GetService(ctx, "lvm", &res); err != nil {
		diags.AddError("Failed to read LVM service", err.Error())
		return
	}
	model.ID = types.StringValue("service-lvm")
	model.Enabled = types.BoolValue(res.Config.Enabled)
	model.SystemID = types.Int64Value(int64(res.Config.SystemID))
	model.IOTimeout = intValueOrNull(res.Config.IOTimeout, model.IOTimeout.IsNull())
	model.WatchdogTimeout = intValueOrNull(res.Config.WatchdogTimeout, model.WatchdogTimeout.IsNull())
}

// ==================== incusos_service_multipath ====================

var (
	_ resource.Resource                = (*serviceMultipathResource)(nil)
	_ resource.ResourceWithImportState = (*serviceMultipathResource)(nil)
)

type serviceMultipathResource struct{ client *client.Client }

type serviceMultipathModel struct {
	ID                   types.String `tfsdk:"id"`
	Enabled              types.Bool   `tfsdk:"enabled"`
	WWNs                 types.List   `tfsdk:"wwns"`
	AutomaticTransports  types.List   `tfsdk:"automatic_transports"`
	AutomaticWWNPrefixes types.List   `tfsdk:"automatic_wwn_prefixes"`
}

func NewServiceMultipathResource() resource.Resource { return &serviceMultipathResource{} }

func (r *serviceMultipathResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_multipath"
}

func (r *serviceMultipathResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the IncusOS Multipath service (`/1.0/services/multipath`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"wwns":                   schema.ListAttribute{ElementType: types.StringType, Optional: true},
			"automatic_transports":   schema.ListAttribute{ElementType: types.StringType, Optional: true},
			"automatic_wwn_prefixes": schema.ListAttribute{ElementType: types.StringType, Optional: true},
		},
	}
}

func (r *serviceMultipathResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (r *serviceMultipathResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serviceMultipathModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	payload := osapi.ServiceMultipath{
		Config: osapi.ServiceMultipathConfig{
			Enabled:              plan.Enabled.ValueBool(),
			WWNs:                 listToStringSlice(ctx, plan.WWNs, &resp.Diagnostics),
			AutomaticTransports:  listToStringSlice(ctx, plan.AutomaticTransports, &resp.Diagnostics),
			AutomaticWWNPrefixes: listToStringSlice(ctx, plan.AutomaticWWNPrefixes, &resp.Diagnostics),
		},
	}
	if err := r.client.UpdateService(ctx, "multipath", payload); err != nil {
		resp.Diagnostics.AddError("Failed to configure Multipath service", err.Error())
		return
	}
	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceMultipathResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serviceMultipathModel
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

func (r *serviceMultipathResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serviceMultipathModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	payload := osapi.ServiceMultipath{
		Config: osapi.ServiceMultipathConfig{
			Enabled:              plan.Enabled.ValueBool(),
			WWNs:                 listToStringSlice(ctx, plan.WWNs, &resp.Diagnostics),
			AutomaticTransports:  listToStringSlice(ctx, plan.AutomaticTransports, &resp.Diagnostics),
			AutomaticWWNPrefixes: listToStringSlice(ctx, plan.AutomaticWWNPrefixes, &resp.Diagnostics),
		},
	}
	if err := r.client.UpdateService(ctx, "multipath", payload); err != nil {
		resp.Diagnostics.AddError("Failed to update Multipath service", err.Error())
		return
	}
	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceMultipathResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.UpdateService(ctx, "multipath", osapi.ServiceMultipath{}); err != nil {
		resp.Diagnostics.AddError("Failed to disable Multipath service", err.Error())
	}
}

func (r *serviceMultipathResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *serviceMultipathResource) readIntoModel(ctx context.Context, model *serviceMultipathModel, diags *diag.Diagnostics) {
	var res osapi.ServiceMultipath
	if err := r.client.GetService(ctx, "multipath", &res); err != nil {
		diags.AddError("Failed to read Multipath service", err.Error())
		return
	}
	model.ID = types.StringValue("service-multipath")
	model.Enabled = types.BoolValue(res.Config.Enabled)
	model.WWNs = stringSliceToList(ctx, res.Config.WWNs, model.WWNs.IsNull(), diags)
	model.AutomaticTransports = stringSliceToList(ctx, res.Config.AutomaticTransports, model.AutomaticTransports.IsNull(), diags)
	model.AutomaticWWNPrefixes = stringSliceToList(ctx, res.Config.AutomaticWWNPrefixes, model.AutomaticWWNPrefixes.IsNull(), diags)
}

// ==================== incusos_service_nvme ====================

var (
	_ resource.Resource                = (*serviceNVMEResource)(nil)
	_ resource.ResourceWithImportState = (*serviceNVMEResource)(nil)
)

type serviceNVMEResource struct{ client *client.Client }

type serviceNVMEModel struct {
	ID      types.String             `tfsdk:"id"`
	Enabled types.Bool               `tfsdk:"enabled"`
	Targets []serviceNVMETargetModel `tfsdk:"targets"`
	HostID  types.String             `tfsdk:"host_id"`
	HostNQN types.String             `tfsdk:"host_nqn"`
}

type serviceNVMETargetModel struct {
	Transport   types.String `tfsdk:"transport"`
	Address     types.String `tfsdk:"address"`
	HostAddress types.String `tfsdk:"host_address"`
	Port        types.Int64  `tfsdk:"port"`
	NQN         types.String `tfsdk:"nqn"`
}

func NewServiceNVMEResource() resource.Resource { return &serviceNVMEResource{} }

func (r *serviceNVMEResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_nvme"
}

func (r *serviceNVMEResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the IncusOS NVMe-oF service (`/1.0/services/nvme`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"targets": schema.ListNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"transport":    schema.StringAttribute{Required: true},
						"address":      schema.StringAttribute{Required: true},
						"host_address": schema.StringAttribute{Optional: true},
						"port":         schema.Int64Attribute{Optional: true},
						"nqn":          schema.StringAttribute{Optional: true},
					},
				},
			},
			"host_id":  schema.StringAttribute{Computed: true},
			"host_nqn": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *serviceNVMEResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (r *serviceNVMEResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serviceNVMEModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateService(ctx, "nvme", plan.toAPI()); err != nil {
		resp.Diagnostics.AddError("Failed to configure NVMe service", err.Error())
		return
	}
	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceNVMEResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serviceNVMEModel
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

func (r *serviceNVMEResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serviceNVMEModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateService(ctx, "nvme", plan.toAPI()); err != nil {
		resp.Diagnostics.AddError("Failed to update NVMe service", err.Error())
		return
	}
	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceNVMEResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.UpdateService(ctx, "nvme", osapi.ServiceNVME{}); err != nil {
		resp.Diagnostics.AddError("Failed to disable NVMe service", err.Error())
	}
}

func (r *serviceNVMEResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *serviceNVMEModel) toAPI() osapi.ServiceNVME {
	targets := make([]osapi.ServiceNVMETarget, 0, len(m.Targets))
	for _, t := range m.Targets {
		targets = append(targets, osapi.ServiceNVMETarget{
			Transport:   t.Transport.ValueString(),
			Address:     t.Address.ValueString(),
			HostAddress: t.HostAddress.ValueString(),
			Port:        int(t.Port.ValueInt64()),
			NQN:         t.NQN.ValueString(),
		})
	}
	return osapi.ServiceNVME{
		Config: osapi.ServiceNVMEConfig{
			Enabled: m.Enabled.ValueBool(),
			Targets: targets,
		},
	}
}

func (r *serviceNVMEResource) readIntoModel(ctx context.Context, model *serviceNVMEModel, diags *diag.Diagnostics) {
	var res osapi.ServiceNVME
	if err := r.client.GetService(ctx, "nvme", &res); err != nil {
		diags.AddError("Failed to read NVMe service", err.Error())
		return
	}
	model.ID = types.StringValue("service-nvme")
	model.Enabled = types.BoolValue(res.Config.Enabled)
	model.HostID = types.StringValue(res.State.HostID)
	model.HostNQN = types.StringValue(res.State.HostNQN)
	if len(res.Config.Targets) == 0 {
		if model.Targets != nil {
			model.Targets = []serviceNVMETargetModel{}
		}
	} else {
		targets := make([]serviceNVMETargetModel, 0, len(res.Config.Targets))
		for i, t := range res.Config.Targets {
			var prev *serviceNVMETargetModel
			if i < len(model.Targets) {
				prev = &model.Targets[i]
			}
			targets = append(targets, serviceNVMETargetModel{
				Transport:   types.StringValue(t.Transport),
				Address:     types.StringValue(t.Address),
				HostAddress: stringValueOrNull(t.HostAddress, prev == nil || prev.HostAddress.IsNull()),
				Port:        intValueOrNull(t.Port, prev == nil || prev.Port.IsNull()),
				NQN:         stringValueOrNull(t.NQN, prev == nil || prev.NQN.IsNull()),
			})
		}
		model.Targets = targets
	}
}
