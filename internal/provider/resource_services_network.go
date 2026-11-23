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

// ==================== incusos_service_ovn ====================

var (
	_ resource.Resource                = (*serviceOVNResource)(nil)
	_ resource.ResourceWithImportState = (*serviceOVNResource)(nil)
)

type serviceOVNResource struct{ client *client.Client }

type serviceOVNModel struct {
	ID                   types.String `tfsdk:"id"`
	Enabled              types.Bool   `tfsdk:"enabled"`
	ICChassis            types.Bool   `tfsdk:"ic_chassis"`
	Database             types.String `tfsdk:"database"`
	TLSClientCertificate types.String `tfsdk:"tls_client_certificate"`
	TLSClientKey         types.String `tfsdk:"tls_client_key"`
	TLSCACertificate     types.String `tfsdk:"tls_ca_certificate"`
	TunnelAddress        types.String `tfsdk:"tunnel_address"`
	TunnelProtocol       types.String `tfsdk:"tunnel_protocol"`
}

func NewServiceOVNResource() resource.Resource { return &serviceOVNResource{} }

func (r *serviceOVNResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_ovn"
}

func (r *serviceOVNResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the IncusOS OVN service (`/1.0/services/ovn`).",
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
			"ic_chassis":             schema.BoolAttribute{Optional: true},
			"database":               schema.StringAttribute{Required: true},
			"tls_client_certificate": schema.StringAttribute{Optional: true, Sensitive: true},
			"tls_client_key":         schema.StringAttribute{Optional: true, Sensitive: true},
			"tls_ca_certificate":     schema.StringAttribute{Optional: true},
			"tunnel_address":         schema.StringAttribute{Optional: true},
			"tunnel_protocol":        schema.StringAttribute{Optional: true},
		},
	}
}

func (r *serviceOVNResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (r *serviceOVNResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serviceOVNModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateService(ctx, "ovn", plan.toAPI()); err != nil {
		resp.Diagnostics.AddError("Failed to configure OVN service", err.Error())
		return
	}
	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceOVNResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serviceOVNModel
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

func (r *serviceOVNResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serviceOVNModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateService(ctx, "ovn", plan.toAPI()); err != nil {
		resp.Diagnostics.AddError("Failed to update OVN service", err.Error())
		return
	}
	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceOVNResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.UpdateService(ctx, "ovn", osapi.ServiceOVN{}); err != nil {
		resp.Diagnostics.AddError("Failed to disable OVN service", err.Error())
	}
}

func (r *serviceOVNResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *serviceOVNModel) toAPI() osapi.ServiceOVN {
	return osapi.ServiceOVN{
		Config: osapi.ServiceOVNConfig{
			Enabled:              m.Enabled.ValueBool(),
			ICChassis:            m.ICChassis.ValueBool(),
			Database:             m.Database.ValueString(),
			TLSClientCertificate: m.TLSClientCertificate.ValueString(),
			TLSClientKey:         m.TLSClientKey.ValueString(),
			TLSCACertificate:     m.TLSCACertificate.ValueString(),
			TunnelAddress:        m.TunnelAddress.ValueString(),
			TunnelProtocol:       m.TunnelProtocol.ValueString(),
		},
	}
}

func (r *serviceOVNResource) readIntoModel(ctx context.Context, model *serviceOVNModel, diags *diag.Diagnostics) {
	var res osapi.ServiceOVN
	if err := r.client.GetService(ctx, "ovn", &res); err != nil {
		diags.AddError("Failed to read OVN service", err.Error())
		return
	}
	model.ID = types.StringValue("service-ovn")
	model.Enabled = types.BoolValue(res.Config.Enabled)
	model.ICChassis = boolValueOrNull(res.Config.ICChassis, model.ICChassis.IsNull())
	model.Database = types.StringValue(res.Config.Database)
	model.TLSClientCertificate = stringValueOrNull(res.Config.TLSClientCertificate, model.TLSClientCertificate.IsNull())
	model.TLSClientKey = stringValueOrNull(res.Config.TLSClientKey, model.TLSClientKey.IsNull())
	model.TLSCACertificate = stringValueOrNull(res.Config.TLSCACertificate, model.TLSCACertificate.IsNull())
	model.TunnelAddress = stringValueOrNull(res.Config.TunnelAddress, model.TunnelAddress.IsNull())
	model.TunnelProtocol = stringValueOrNull(res.Config.TunnelProtocol, model.TunnelProtocol.IsNull())
}

// ==================== incusos_service_tailscale ====================

var (
	_ resource.Resource                = (*serviceTailscaleResource)(nil)
	_ resource.ResourceWithImportState = (*serviceTailscaleResource)(nil)
)

type serviceTailscaleResource struct{ client *client.Client }

type serviceTailscaleModel struct {
	ID                     types.String `tfsdk:"id"`
	Enabled                types.Bool   `tfsdk:"enabled"`
	LoginServer            types.String `tfsdk:"login_server"`
	AuthKey                types.String `tfsdk:"auth_key"`
	Port                   types.Int64  `tfsdk:"port"`
	AcceptRoutes           types.Bool   `tfsdk:"accept_routes"`
	AcceptDNS              types.Bool   `tfsdk:"accept_dns"`
	AdvertisedRoutes       types.List   `tfsdk:"advertised_routes"`
	AdvertiseExitNode      types.Bool   `tfsdk:"advertise_exit_node"`
	ExitNode               types.String `tfsdk:"exit_node"`
	ExitNodeAllowLanAccess types.Bool   `tfsdk:"exit_node_allow_lan_access"`
	ServeEnabled           types.Bool   `tfsdk:"serve_enabled"`
	ServePort              types.Int64  `tfsdk:"serve_port"`
	ServeService           types.String `tfsdk:"serve_service"`
	BackendState           types.String `tfsdk:"backend_state"`
	TailnetName            types.String `tfsdk:"tailnet_name"`
	Version                types.String `tfsdk:"version"`
}

func NewServiceTailscaleResource() resource.Resource { return &serviceTailscaleResource{} }

func (r *serviceTailscaleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_tailscale"
}

func (r *serviceTailscaleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the IncusOS Tailscale service (`/1.0/services/tailscale`).",
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
			"login_server":               schema.StringAttribute{Optional: true},
			"auth_key":                   schema.StringAttribute{Optional: true, Sensitive: true},
			"port":                       schema.Int64Attribute{Optional: true},
			"accept_routes":              schema.BoolAttribute{Optional: true},
			"accept_dns":                 schema.BoolAttribute{Optional: true},
			"advertised_routes":          schema.ListAttribute{ElementType: types.StringType, Optional: true},
			"advertise_exit_node":        schema.BoolAttribute{Optional: true},
			"exit_node":                  schema.StringAttribute{Optional: true},
			"exit_node_allow_lan_access": schema.BoolAttribute{Optional: true},
			"serve_enabled":              schema.BoolAttribute{Optional: true},
			"serve_port":                 schema.Int64Attribute{Optional: true},
			"serve_service":              schema.StringAttribute{Optional: true},
			"backend_state":              schema.StringAttribute{Computed: true},
			"tailnet_name":               schema.StringAttribute{Computed: true},
			"version":                    schema.StringAttribute{Computed: true},
		},
	}
}

func (r *serviceTailscaleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (r *serviceTailscaleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serviceTailscaleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateService(ctx, "tailscale", plan.toAPI(ctx, &resp.Diagnostics)); err != nil {
		resp.Diagnostics.AddError("Failed to configure Tailscale service", err.Error())
		return
	}
	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceTailscaleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serviceTailscaleModel
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

func (r *serviceTailscaleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serviceTailscaleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateService(ctx, "tailscale", plan.toAPI(ctx, &resp.Diagnostics)); err != nil {
		resp.Diagnostics.AddError("Failed to update Tailscale service", err.Error())
		return
	}
	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceTailscaleResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.UpdateService(ctx, "tailscale", osapi.ServiceTailscale{}); err != nil {
		resp.Diagnostics.AddError("Failed to disable Tailscale service", err.Error())
	}
}

func (r *serviceTailscaleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *serviceTailscaleModel) toAPI(ctx context.Context, diags *diag.Diagnostics) osapi.ServiceTailscale {
	return osapi.ServiceTailscale{
		Config: osapi.ServiceTailscaleConfig{
			Enabled:                m.Enabled.ValueBool(),
			LoginServer:            m.LoginServer.ValueString(),
			AuthKey:                m.AuthKey.ValueString(),
			Port:                   int(m.Port.ValueInt64()),
			AcceptRoutes:           m.AcceptRoutes.ValueBool(),
			AcceptDNS:              m.AcceptDNS.ValueBool(),
			AdvertisedRoutes:       listToStringSlice(ctx, m.AdvertisedRoutes, diags),
			AdvertiseExitNode:      m.AdvertiseExitNode.ValueBool(),
			ExitNode:               m.ExitNode.ValueString(),
			ExitNodeAllowLanAccess: m.ExitNodeAllowLanAccess.ValueBool(),
			ServeEnabled:           m.ServeEnabled.ValueBool(),
			ServePort:              int(m.ServePort.ValueInt64()),
			ServeService:           m.ServeService.ValueString(),
		},
	}
}

func (r *serviceTailscaleResource) readIntoModel(ctx context.Context, model *serviceTailscaleModel, diags *diag.Diagnostics) {
	var res osapi.ServiceTailscale
	if err := r.client.GetService(ctx, "tailscale", &res); err != nil {
		diags.AddError("Failed to read Tailscale service", err.Error())
		return
	}
	model.ID = types.StringValue("service-tailscale")
	model.Enabled = types.BoolValue(res.Config.Enabled)
	model.LoginServer = stringValueOrNull(res.Config.LoginServer, model.LoginServer.IsNull())
	model.AuthKey = stringValueOrNull(res.Config.AuthKey, model.AuthKey.IsNull())
	model.Port = intValueOrNull(res.Config.Port, model.Port.IsNull())
	model.AcceptRoutes = boolValueOrNull(res.Config.AcceptRoutes, model.AcceptRoutes.IsNull())
	model.AcceptDNS = boolValueOrNull(res.Config.AcceptDNS, model.AcceptDNS.IsNull())
	model.AdvertisedRoutes = stringSliceToList(ctx, res.Config.AdvertisedRoutes, model.AdvertisedRoutes.IsNull(), diags)
	model.AdvertiseExitNode = boolValueOrNull(res.Config.AdvertiseExitNode, model.AdvertiseExitNode.IsNull())
	model.ExitNode = stringValueOrNull(res.Config.ExitNode, model.ExitNode.IsNull())
	model.ExitNodeAllowLanAccess = boolValueOrNull(res.Config.ExitNodeAllowLanAccess, model.ExitNodeAllowLanAccess.IsNull())
	model.ServeEnabled = boolValueOrNull(res.Config.ServeEnabled, model.ServeEnabled.IsNull())
	model.ServePort = intValueOrNull(res.Config.ServePort, model.ServePort.IsNull())
	model.ServeService = stringValueOrNull(res.Config.ServeService, model.ServeService.IsNull())
	model.BackendState = types.StringValue(string(res.State.BackendState))
	model.TailnetName = types.StringValue(res.State.TailnetName)
	model.Version = types.StringValue(res.State.Version)
}

// ==================== incusos_service_netbird ====================

var (
	_ resource.Resource                = (*serviceNetbirdResource)(nil)
	_ resource.ResourceWithImportState = (*serviceNetbirdResource)(nil)
)

type serviceNetbirdResource struct{ client *client.Client }

type serviceNetbirdModel struct {
	ID                  types.String `tfsdk:"id"`
	Enabled             types.Bool   `tfsdk:"enabled"`
	SetupKey            types.String `tfsdk:"setup_key"`
	ManagementURL       types.String `tfsdk:"management_url"`
	AdminURL            types.String `tfsdk:"admin_url"`
	Anonymize           types.Bool   `tfsdk:"anonymize"`
	BlockInbound        types.Bool   `tfsdk:"block_inbound"`
	BlockLanAccess      types.Bool   `tfsdk:"block_lan_access"`
	DisableClientRoutes types.Bool   `tfsdk:"disable_client_routes"`
	DisableServerRoutes types.Bool   `tfsdk:"disable_server_routes"`
	DisableDNS          types.Bool   `tfsdk:"disable_dns"`
	DisableFirewall     types.Bool   `tfsdk:"disable_firewall"`
	DNSResolverAddress  types.String `tfsdk:"dns_resolver_address"`
	ExternalIPMap       types.List   `tfsdk:"external_ip_map"`
	ExtraDNSLabels      types.List   `tfsdk:"extra_dns_labels"`
}

func NewServiceNetbirdResource() resource.Resource { return &serviceNetbirdResource{} }

func (r *serviceNetbirdResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_netbird"
}

func (r *serviceNetbirdResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the IncusOS NetBird service (`/1.0/services/netbird`).",
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
			"setup_key":             schema.StringAttribute{Optional: true, Sensitive: true},
			"management_url":        schema.StringAttribute{Optional: true},
			"admin_url":             schema.StringAttribute{Optional: true},
			"anonymize":             schema.BoolAttribute{Optional: true},
			"block_inbound":         schema.BoolAttribute{Optional: true},
			"block_lan_access":      schema.BoolAttribute{Optional: true},
			"disable_client_routes": schema.BoolAttribute{Optional: true},
			"disable_server_routes": schema.BoolAttribute{Optional: true},
			"disable_dns":           schema.BoolAttribute{Optional: true},
			"disable_firewall":      schema.BoolAttribute{Optional: true},
			"dns_resolver_address":  schema.StringAttribute{Optional: true},
			"external_ip_map":       schema.ListAttribute{ElementType: types.StringType, Optional: true},
			"extra_dns_labels":      schema.ListAttribute{ElementType: types.StringType, Optional: true},
		},
	}
}

func (r *serviceNetbirdResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (r *serviceNetbirdResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serviceNetbirdModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateService(ctx, "netbird", plan.toAPI(ctx, &resp.Diagnostics)); err != nil {
		resp.Diagnostics.AddError("Failed to configure NetBird service", err.Error())
		return
	}
	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceNetbirdResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serviceNetbirdModel
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

func (r *serviceNetbirdResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serviceNetbirdModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateService(ctx, "netbird", plan.toAPI(ctx, &resp.Diagnostics)); err != nil {
		resp.Diagnostics.AddError("Failed to update NetBird service", err.Error())
		return
	}
	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceNetbirdResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.UpdateService(ctx, "netbird", osapi.ServiceNetbird{}); err != nil {
		resp.Diagnostics.AddError("Failed to disable NetBird service", err.Error())
	}
}

func (r *serviceNetbirdResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *serviceNetbirdModel) toAPI(ctx context.Context, diags *diag.Diagnostics) osapi.ServiceNetbird {
	return osapi.ServiceNetbird{
		Config: osapi.ServiceNetbirdConfig{
			Enabled:             m.Enabled.ValueBool(),
			SetupKey:            m.SetupKey.ValueString(),
			ManagementURL:       m.ManagementURL.ValueString(),
			AdminURL:            m.AdminURL.ValueString(),
			Anonymize:           m.Anonymize.ValueBool(),
			BlockInbound:        m.BlockInbound.ValueBool(),
			BlockLanAccess:      m.BlockLanAccess.ValueBool(),
			DisableClientRoutes: m.DisableClientRoutes.ValueBool(),
			DisableServerRoutes: m.DisableServerRoutes.ValueBool(),
			DisableDNS:          m.DisableDNS.ValueBool(),
			DisableFirewall:     m.DisableFirewall.ValueBool(),
			DNSResolverAddress:  m.DNSResolverAddress.ValueString(),
			ExternalIPMap:       listToStringSlice(ctx, m.ExternalIPMap, diags),
			ExtraDNSLabels:      listToStringSlice(ctx, m.ExtraDNSLabels, diags),
		},
	}
}

func (r *serviceNetbirdResource) readIntoModel(ctx context.Context, model *serviceNetbirdModel, diags *diag.Diagnostics) {
	var res osapi.ServiceNetbird
	if err := r.client.GetService(ctx, "netbird", &res); err != nil {
		diags.AddError("Failed to read NetBird service", err.Error())
		return
	}
	model.ID = types.StringValue("service-netbird")
	model.Enabled = types.BoolValue(res.Config.Enabled)
	model.SetupKey = stringValueOrNull(res.Config.SetupKey, model.SetupKey.IsNull())
	model.ManagementURL = stringValueOrNull(res.Config.ManagementURL, model.ManagementURL.IsNull())
	model.AdminURL = stringValueOrNull(res.Config.AdminURL, model.AdminURL.IsNull())
	model.Anonymize = boolValueOrNull(res.Config.Anonymize, model.Anonymize.IsNull())
	model.BlockInbound = boolValueOrNull(res.Config.BlockInbound, model.BlockInbound.IsNull())
	model.BlockLanAccess = boolValueOrNull(res.Config.BlockLanAccess, model.BlockLanAccess.IsNull())
	model.DisableClientRoutes = boolValueOrNull(res.Config.DisableClientRoutes, model.DisableClientRoutes.IsNull())
	model.DisableServerRoutes = boolValueOrNull(res.Config.DisableServerRoutes, model.DisableServerRoutes.IsNull())
	model.DisableDNS = boolValueOrNull(res.Config.DisableDNS, model.DisableDNS.IsNull())
	model.DisableFirewall = boolValueOrNull(res.Config.DisableFirewall, model.DisableFirewall.IsNull())
	model.DNSResolverAddress = stringValueOrNull(res.Config.DNSResolverAddress, model.DNSResolverAddress.IsNull())
	model.ExternalIPMap = stringSliceToList(ctx, res.Config.ExternalIPMap, model.ExternalIPMap.IsNull(), diags)
	model.ExtraDNSLabels = stringSliceToList(ctx, res.Config.ExtraDNSLabels, model.ExtraDNSLabels.IsNull(), diags)
}

// ==================== incusos_service_usbip ====================

var (
	_ resource.Resource                = (*serviceUSBIPResource)(nil)
	_ resource.ResourceWithImportState = (*serviceUSBIPResource)(nil)
)

type serviceUSBIPResource struct{ client *client.Client }

type serviceUSBIPModel struct {
	ID      types.String              `tfsdk:"id"`
	Enabled types.Bool                `tfsdk:"enabled"`
	Targets []serviceUSBIPTargetModel `tfsdk:"targets"`
}

type serviceUSBIPTargetModel struct {
	Address types.String `tfsdk:"address"`
	BusID   types.String `tfsdk:"bus_id"`
}

func NewServiceUSBIPResource() resource.Resource { return &serviceUSBIPResource{} }

func (r *serviceUSBIPResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_usbip"
}

func (r *serviceUSBIPResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the IncusOS USB/IP service (`/1.0/services/usbip`).",
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
						"address": schema.StringAttribute{Required: true},
						"bus_id":  schema.StringAttribute{Required: true},
					},
				},
			},
		},
	}
}

func (r *serviceUSBIPResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (r *serviceUSBIPResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serviceUSBIPModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateService(ctx, "usbip", plan.toAPI()); err != nil {
		resp.Diagnostics.AddError("Failed to configure USB/IP service", err.Error())
		return
	}
	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceUSBIPResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serviceUSBIPModel
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

func (r *serviceUSBIPResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serviceUSBIPModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateService(ctx, "usbip", plan.toAPI()); err != nil {
		resp.Diagnostics.AddError("Failed to update USB/IP service", err.Error())
		return
	}
	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceUSBIPResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.UpdateService(ctx, "usbip", osapi.ServiceUSBIP{}); err != nil {
		resp.Diagnostics.AddError("Failed to disable USB/IP service", err.Error())
	}
}

func (r *serviceUSBIPResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *serviceUSBIPModel) toAPI() osapi.ServiceUSBIP {
	targets := make([]osapi.ServiceUSBIPTarget, 0, len(m.Targets))
	for _, t := range m.Targets {
		targets = append(targets, osapi.ServiceUSBIPTarget{
			Address: t.Address.ValueString(),
			BusID:   t.BusID.ValueString(),
		})
	}
	return osapi.ServiceUSBIP{
		Config: osapi.ServiceUSBIPConfig{
			Enabled: m.Enabled.ValueBool(),
			Targets: targets,
		},
	}
}

func (r *serviceUSBIPResource) readIntoModel(ctx context.Context, model *serviceUSBIPModel, diags *diag.Diagnostics) {
	var res osapi.ServiceUSBIP
	if err := r.client.GetService(ctx, "usbip", &res); err != nil {
		diags.AddError("Failed to read USB/IP service", err.Error())
		return
	}
	model.ID = types.StringValue("service-usbip")
	model.Enabled = types.BoolValue(res.Config.Enabled)
	if len(res.Config.Targets) == 0 {
		if model.Targets != nil {
			model.Targets = []serviceUSBIPTargetModel{}
		}
	} else {
		targets := make([]serviceUSBIPTargetModel, 0, len(res.Config.Targets))
		for _, t := range res.Config.Targets {
			targets = append(targets, serviceUSBIPTargetModel{
				Address: types.StringValue(t.Address),
				BusID:   types.StringValue(t.BusID),
			})
		}
		model.Targets = targets
	}
}
