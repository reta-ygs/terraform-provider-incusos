package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/reta-ygs/terraform-provider-incusos/internal/client"
)

// ==================== data.incusos_server ====================

var _ datasource.DataSource = (*serverDataSource)(nil)

type serverDataSource struct{ client *client.Client }

type serverDataSourceModel struct {
	ID                 types.String `tfsdk:"id"`
	Hostname           types.String `tfsdk:"hostname"`
	MachineID          types.String `tfsdk:"machine_id"`
	OSName             types.String `tfsdk:"os_name"`
	OSVersion          types.String `tfsdk:"os_version"`
	OSVersionAlternate types.String `tfsdk:"os_version_alternate"`
	OSVersionNext      types.String `tfsdk:"os_version_next"`
	SystemIsReady      types.Bool   `tfsdk:"system_is_ready"`
	Uptime             types.Int64  `tfsdk:"uptime"`
}

func NewServerDataSource() datasource.DataSource { return &serverDataSource{} }

func (d *serverDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server"
}

func (d *serverDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads basic IncusOS server environment information from `GET /1.0`.",
		Attributes: map[string]schema.Attribute{
			"id":                   schema.StringAttribute{Computed: true},
			"hostname":             schema.StringAttribute{Computed: true},
			"machine_id":           schema.StringAttribute{Computed: true},
			"os_name":              schema.StringAttribute{Computed: true},
			"os_version":           schema.StringAttribute{Computed: true},
			"os_version_alternate": schema.StringAttribute{Computed: true},
			"os_version_next":      schema.StringAttribute{Computed: true},
			"system_is_ready":      schema.BoolAttribute{Computed: true},
			"uptime":               schema.Int64Attribute{Computed: true},
		},
	}
}

func (d *serverDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (d *serverDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	info, err := d.client.GetServerInfo(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read IncusOS server info", err.Error())
		return
	}
	env := info.Environment
	state := serverDataSourceModel{
		ID:                 types.StringValue(env.MachineID),
		Hostname:           types.StringValue(env.Hostname),
		MachineID:          types.StringValue(env.MachineID),
		OSName:             types.StringValue(env.OSName),
		OSVersion:          types.StringValue(env.OSVersion),
		OSVersionAlternate: types.StringValue(env.OSVersionAlternate),
		OSVersionNext:      types.StringValue(env.OSVersionNext),
		SystemIsReady:      types.BoolValue(env.SystemIsReady),
		Uptime:             types.Int64Value(env.Uptime),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ==================== data.incusos_system_storage ====================

var _ datasource.DataSource = (*systemStorageDataSource)(nil)

type systemStorageDataSource struct{ client *client.Client }

type systemStorageDataSourceModel struct {
	ID                      types.String                  `tfsdk:"id"`
	ScrubSchedule           types.String                  `tfsdk:"scrub_schedule"`
	TrimSchedule            types.String                  `tfsdk:"trim_schedule"`
	RootPartitionSizeBytes  types.Int64                   `tfsdk:"root_partition_size_in_bytes"`
	RootPartitionAvailBytes types.Int64                   `tfsdk:"root_partition_available_in_bytes"`
	Drives                  []systemStorageDriveDataModel `tfsdk:"drives"`
	PoolNames               types.List                    `tfsdk:"pool_names"`
}

type systemStorageDriveDataModel struct {
	ID              types.String `tfsdk:"id"`
	ModelFamily     types.String `tfsdk:"model_family"`
	ModelName       types.String `tfsdk:"model_name"`
	SerialNumber    types.String `tfsdk:"serial_number"`
	Bus             types.String `tfsdk:"bus"`
	CapacityInBytes types.Int64  `tfsdk:"capacity_in_bytes"`
	Boot            types.Bool   `tfsdk:"boot"`
	Multipath       types.Bool   `tfsdk:"multipath"`
	Removable       types.Bool   `tfsdk:"removable"`
	Remote          types.Bool   `tfsdk:"remote"`
	WWN             types.String `tfsdk:"wwn"`
	MemberPool      types.String `tfsdk:"member_pool"`
	Encrypted       types.Bool   `tfsdk:"encrypted"`
}

func NewSystemStorageDataSource() datasource.DataSource { return &systemStorageDataSource{} }

func (d *systemStorageDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_storage"
}

func (d *systemStorageDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads IncusOS physical drives, root partition usage, and ZFS storage pools from `GET /1.0/system/storage`.",
		Attributes: map[string]schema.Attribute{
			"id":                                schema.StringAttribute{Computed: true},
			"scrub_schedule":                    schema.StringAttribute{Computed: true},
			"trim_schedule":                     schema.StringAttribute{Computed: true},
			"root_partition_size_in_bytes":      schema.Int64Attribute{Computed: true},
			"root_partition_available_in_bytes": schema.Int64Attribute{Computed: true},
			"pool_names":                        schema.ListAttribute{ElementType: types.StringType, Computed: true},
			"drives": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                schema.StringAttribute{Computed: true},
						"model_family":      schema.StringAttribute{Computed: true},
						"model_name":        schema.StringAttribute{Computed: true},
						"serial_number":     schema.StringAttribute{Computed: true},
						"bus":               schema.StringAttribute{Computed: true},
						"capacity_in_bytes": schema.Int64Attribute{Computed: true},
						"boot":              schema.BoolAttribute{Computed: true},
						"multipath":         schema.BoolAttribute{Computed: true},
						"removable":         schema.BoolAttribute{Computed: true},
						"remote":            schema.BoolAttribute{Computed: true},
						"wwn":               schema.StringAttribute{Computed: true},
						"member_pool":       schema.StringAttribute{Computed: true},
						"encrypted":         schema.BoolAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *systemStorageDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (d *systemStorageDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	res, err := d.client.GetSystemStorage(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read system storage", err.Error())
		return
	}

	drives := make([]systemStorageDriveDataModel, 0, len(res.State.Drives))
	for _, dr := range res.State.Drives {
		drives = append(drives, systemStorageDriveDataModel{
			ID:              types.StringValue(dr.ID),
			ModelFamily:     types.StringValue(dr.ModelFamily),
			ModelName:       types.StringValue(dr.ModelName),
			SerialNumber:    types.StringValue(dr.SerialNumber),
			Bus:             types.StringValue(dr.Bus),
			CapacityInBytes: types.Int64Value(int64(dr.CapacityInBytes)),
			Boot:            types.BoolValue(dr.Boot),
			Multipath:       types.BoolValue(dr.Multipath),
			Removable:       types.BoolValue(dr.Removable),
			Remote:          types.BoolValue(dr.Remote),
			WWN:             types.StringValue(dr.WWN),
			MemberPool:      types.StringValue(dr.MemberPool),
			Encrypted:       types.BoolValue(dr.Encrypted),
		})
	}

	poolNames := make([]string, 0, len(res.Config.Pools))
	for _, p := range res.Config.Pools {
		poolNames = append(poolNames, p.Name)
	}

	state := systemStorageDataSourceModel{
		ID:                      types.StringValue("system-storage"),
		ScrubSchedule:           types.StringValue(res.Config.ScrubSchedule),
		TrimSchedule:            types.StringValue(res.Config.TrimSchedule),
		RootPartitionSizeBytes:  types.Int64Value(int64(res.State.RootPartition.SizeInBytes)),
		RootPartitionAvailBytes: types.Int64Value(int64(res.State.RootPartition.AvailableInBytes)),
		Drives:                  drives,
		PoolNames:               stringSliceToList(ctx, poolNames, false, &resp.Diagnostics),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ==================== data.incusos_system_security ====================

var _ datasource.DataSource = (*systemSecurityDataSource)(nil)

type systemSecurityDataSource struct{ client *client.Client }

type systemSecurityDataSourceModel struct {
	ID                              types.String                        `tfsdk:"id"`
	SecureBootEnabled               types.Bool                          `tfsdk:"secure_boot_enabled"`
	SystemStateIsTrusted            types.Bool                          `tfsdk:"system_state_is_trusted"`
	SystemStateStatus               types.String                        `tfsdk:"system_state_status"`
	TPMStatus                       types.String                        `tfsdk:"tpm_status"`
	TPMPublicKey                    types.String                        `tfsdk:"tpm_public_key"`
	EncryptionRecoveryKeysRetrieved types.Bool                          `tfsdk:"encryption_recovery_keys_retrieved"`
	EncryptedVolumes                []systemSecurityEncryptedVolModel   `tfsdk:"encrypted_volumes"`
	SecureBootCertificates          []systemSecuritySecureBootCertModel `tfsdk:"secure_boot_certificates"`
}

type systemSecurityEncryptedVolModel struct {
	Volume types.String `tfsdk:"volume"`
	State  types.String `tfsdk:"state"`
}

type systemSecuritySecureBootCertModel struct {
	Type        types.String `tfsdk:"type"`
	Fingerprint types.String `tfsdk:"fingerprint"`
	Subject     types.String `tfsdk:"subject"`
	Issuer      types.String `tfsdk:"issuer"`
}

func NewSystemSecurityDataSource() datasource.DataSource { return &systemSecurityDataSource{} }

func (d *systemSecurityDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_security"
}

func (d *systemSecurityDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads IncusOS system security state (Secure Boot, TPM, encrypted volumes) from `GET /1.0/system/security`.",
		Attributes: map[string]schema.Attribute{
			"id":                                 schema.StringAttribute{Computed: true},
			"secure_boot_enabled":                schema.BoolAttribute{Computed: true},
			"system_state_is_trusted":            schema.BoolAttribute{Computed: true},
			"system_state_status":                schema.StringAttribute{Computed: true},
			"tpm_status":                         schema.StringAttribute{Computed: true},
			"tpm_public_key":                     schema.StringAttribute{Computed: true},
			"encryption_recovery_keys_retrieved": schema.BoolAttribute{Computed: true},
			"encrypted_volumes": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"volume": schema.StringAttribute{Computed: true},
						"state":  schema.StringAttribute{Computed: true},
					},
				},
			},
			"secure_boot_certificates": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type":        schema.StringAttribute{Computed: true},
						"fingerprint": schema.StringAttribute{Computed: true},
						"subject":     schema.StringAttribute{Computed: true},
						"issuer":      schema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *systemSecurityDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (d *systemSecurityDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	res, err := d.client.GetSystemSecurity(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read system security", err.Error())
		return
	}

	vols := make([]systemSecurityEncryptedVolModel, 0, len(res.State.EncryptedVolumes))
	for _, v := range res.State.EncryptedVolumes {
		vols = append(vols, systemSecurityEncryptedVolModel{
			Volume: types.StringValue(v.Volume),
			State:  types.StringValue(v.State),
		})
	}

	certs := make([]systemSecuritySecureBootCertModel, 0, len(res.State.SecureBootCertificates))
	for _, c := range res.State.SecureBootCertificates {
		certs = append(certs, systemSecuritySecureBootCertModel{
			Type:        types.StringValue(c.Type),
			Fingerprint: types.StringValue(c.Fingerprint),
			Subject:     types.StringValue(c.Subject),
			Issuer:      types.StringValue(c.Issuer),
		})
	}

	state := systemSecurityDataSourceModel{
		ID:                              types.StringValue("system-security"),
		SecureBootEnabled:               types.BoolValue(res.State.SecureBootEnabled),
		SystemStateIsTrusted:            types.BoolValue(res.State.SystemStateIsTrusted),
		SystemStateStatus:               types.StringValue(res.State.SystemStateStatus),
		TPMStatus:                       types.StringValue(string(res.State.TPMStatus)),
		TPMPublicKey:                    types.StringValue(res.State.TPMPublicKey),
		EncryptionRecoveryKeysRetrieved: types.BoolValue(res.State.EncryptionRecoveryKeysRetrieved),
		EncryptedVolumes:                vols,
		SecureBootCertificates:          certs,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ==================== data.incusos_system_network ====================

var _ datasource.DataSource = (*systemNetworkDataSource)(nil)

type systemNetworkDataSource struct{ client *client.Client }

type systemNetworkDataSourceModel struct {
	ID                     types.String                                  `tfsdk:"id"`
	ConfigurationInProcess types.Bool                                    `tfsdk:"configuration_in_process"`
	Interfaces             map[string]systemNetworkInterfaceStateDataObj `tfsdk:"interfaces"`
}

type systemNetworkInterfaceStateDataObj struct {
	Addresses types.List   `tfsdk:"addresses"`
	Hwaddr    types.String `tfsdk:"hwaddr"`
	MTU       types.Int64  `tfsdk:"mtu"`
	Roles     types.List   `tfsdk:"roles"`
	Speed     types.String `tfsdk:"speed"`
	State     types.String `tfsdk:"state"`
	Type      types.String `tfsdk:"type"`
}

func NewSystemNetworkDataSource() datasource.DataSource { return &systemNetworkDataSource{} }

func (d *systemNetworkDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_network"
}

func (d *systemNetworkDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads runtime host network interface state from `GET /1.0/system/network`.",
		Attributes: map[string]schema.Attribute{
			"id":                       schema.StringAttribute{Computed: true},
			"configuration_in_process": schema.BoolAttribute{Computed: true},
			"interfaces": schema.MapNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"addresses": schema.ListAttribute{ElementType: types.StringType, Computed: true},
						"hwaddr":    schema.StringAttribute{Computed: true},
						"mtu":       schema.Int64Attribute{Computed: true},
						"roles":     schema.ListAttribute{ElementType: types.StringType, Computed: true},
						"speed":     schema.StringAttribute{Computed: true},
						"state":     schema.StringAttribute{Computed: true},
						"type":      schema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *systemNetworkDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (d *systemNetworkDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	res, err := d.client.GetSystemNetwork(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read system network", err.Error())
		return
	}

	ifaces := make(map[string]systemNetworkInterfaceStateDataObj, len(res.State.Interfaces))
	for name, st := range res.State.Interfaces {
		ifaces[name] = systemNetworkInterfaceStateDataObj{
			Addresses: stringSliceToList(ctx, st.Addresses, false, &resp.Diagnostics),
			Hwaddr:    types.StringValue(st.Hwaddr),
			MTU:       types.Int64Value(int64(st.MTU)),
			Roles:     stringSliceToList(ctx, st.Roles, false, &resp.Diagnostics),
			Speed:     types.StringValue(st.Speed),
			State:     types.StringValue(st.State),
			Type:      types.StringValue(st.Type),
		}
	}

	state := systemNetworkDataSourceModel{
		ID:                     types.StringValue("system-network"),
		ConfigurationInProcess: types.BoolValue(res.State.ConfigurationInProcess),
		Interfaces:             ifaces,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ==================== data.incusos_system_resources ====================

var _ datasource.DataSource = (*systemResourcesDataSource)(nil)

type systemResourcesDataSource struct{ client *client.Client }

type systemResourcesDataSourceModel struct {
	ID   types.String `tfsdk:"id"`
	JSON types.String `tfsdk:"json"`
}

func NewSystemResourcesDataSource() datasource.DataSource { return &systemResourcesDataSource{} }

func (d *systemResourcesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_resources"
}

func (d *systemResourcesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads low-level host hardware resources from `GET /1.0/system/resources` as JSON (can be parsed with Terraform's `jsondecode()`).",
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{Computed: true},
			"json": schema.StringAttribute{Computed: true},
		},
	}
}

func (d *systemResourcesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (d *systemResourcesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	raw, err := d.client.GetSystemResources(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read system resources", err.Error())
		return
	}

	state := systemResourcesDataSourceModel{
		ID:   types.StringValue("system-resources"),
		JSON: types.StringValue(string(raw)),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ==================== data.incusos_applications ====================

var _ datasource.DataSource = (*applicationsDataSource)(nil)

type applicationsDataSource struct{ client *client.Client }

type applicationsDataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	Applications types.List   `tfsdk:"applications"`
}

func NewApplicationsDataSource() datasource.DataSource { return &applicationsDataSource{} }

func (d *applicationsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_applications"
}

func (d *applicationsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists installed IncusOS applications from `GET /1.0/applications`.",
		Attributes: map[string]schema.Attribute{
			"id":           schema.StringAttribute{Computed: true},
			"applications": schema.ListAttribute{ElementType: types.StringType, Computed: true},
		},
	}
}

func (d *applicationsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (d *applicationsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	apps, err := d.client.ListApplications(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list applications", err.Error())
		return
	}

	state := applicationsDataSourceModel{
		ID:           types.StringValue("applications"),
		Applications: stringSliceToList(ctx, apps, false, &resp.Diagnostics),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ==================== data.incusos_services ====================

var _ datasource.DataSource = (*servicesDataSource)(nil)

type servicesDataSource struct{ client *client.Client }

type servicesDataSourceModel struct {
	ID       types.String `tfsdk:"id"`
	Services types.List   `tfsdk:"services"`
}

func NewServicesDataSource() datasource.DataSource { return &servicesDataSource{} }

func (d *servicesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_services"
}

func (d *servicesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists available IncusOS services from `GET /1.0/services`.",
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Computed: true},
			"services": schema.ListAttribute{ElementType: types.StringType, Computed: true},
		},
	}
}

func (d *servicesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (d *servicesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	svcs, err := d.client.ListServices(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list services", err.Error())
		return
	}

	state := servicesDataSourceModel{
		ID:       types.StringValue("services"),
		Services: stringSliceToList(ctx, svcs, false, &resp.Diagnostics),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
