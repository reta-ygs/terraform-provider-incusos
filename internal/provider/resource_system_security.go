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

var (
	_ resource.Resource                = (*systemSecurityResource)(nil)
	_ resource.ResourceWithImportState = (*systemSecurityResource)(nil)
)

type systemSecurityResource struct {
	client *client.Client
}

type systemSecurityModel struct {
	ID                     types.String `tfsdk:"id"`
	CustomCACerts          types.List   `tfsdk:"custom_ca_certs"`
	EncryptionRecoveryKeys types.List   `tfsdk:"encryption_recovery_keys"`
	SecureBootEnabled      types.Bool   `tfsdk:"secure_boot_enabled"`
	SystemStateIsTrusted   types.Bool   `tfsdk:"system_state_is_trusted"`
	SystemStateStatus      types.String `tfsdk:"system_state_status"`
	TPMStatus              types.String `tfsdk:"tpm_status"`
}

func NewSystemSecurityResource() resource.Resource {
	return &systemSecurityResource{}
}

func (r *systemSecurityResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_security"
}

func (r *systemSecurityResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages IncusOS system security configuration (`/1.0/system/security`), including custom CA certificates and disk encryption recovery keys.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"custom_ca_certs": schema.ListAttribute{
				MarkdownDescription: "List of PEM-encoded custom CA certificates trusted by the OS.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"encryption_recovery_keys": schema.ListAttribute{
				MarkdownDescription: "List of disk encryption recovery passphrases.",
				ElementType:         types.StringType,
				Optional:            true,
				Sensitive:           true,
			},
			"secure_boot_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether Secure Boot is enabled.",
				Computed:            true,
			},
			"system_state_is_trusted": schema.BoolAttribute{
				MarkdownDescription: "Whether the system state is currently trusted.",
				Computed:            true,
			},
			"system_state_status": schema.StringAttribute{
				MarkdownDescription: "Current system security state status.",
				Computed:            true,
			},
			"tpm_status": schema.StringAttribute{
				MarkdownDescription: "Current TPM status (`ok`, `pcr mismatch`, `swtpm`).",
				Computed:            true,
			},
		},
	}
}

func (r *systemSecurityResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (r *systemSecurityResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan systemSecurityModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := osapi.SystemSecurity{
		Config: osapi.SystemSecurityConfig{
			CustomCACerts:          listToStringSlice(ctx, plan.CustomCACerts, &resp.Diagnostics),
			EncryptionRecoveryKeys: listToStringSlice(ctx, plan.EncryptionRecoveryKeys, &resp.Diagnostics),
		},
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateSystemSecurity(ctx, payload); err != nil {
		resp.Diagnostics.AddError("Failed to configure system security", err.Error())

		return
	}

	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *systemSecurityResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state systemSecurityModel
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

func (r *systemSecurityResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan systemSecurityModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := osapi.SystemSecurity{
		Config: osapi.SystemSecurityConfig{
			CustomCACerts:          listToStringSlice(ctx, plan.CustomCACerts, &resp.Diagnostics),
			EncryptionRecoveryKeys: listToStringSlice(ctx, plan.EncryptionRecoveryKeys, &resp.Diagnostics),
		},
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateSystemSecurity(ctx, payload); err != nil {
		resp.Diagnostics.AddError("Failed to update system security", err.Error())

		return
	}

	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *systemSecurityResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Removing from Terraform state does not wipe encryption recovery keys or CA certs on the host.
}

func (r *systemSecurityResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *systemSecurityResource) readIntoModel(ctx context.Context, model *systemSecurityModel, diags *diag.Diagnostics) {
	res, err := r.client.GetSystemSecurity(ctx)
	if err != nil {
		diags.AddError("Failed to read system security", err.Error())

		return
	}

	model.ID = types.StringValue("system-security")
	model.CustomCACerts = stringSliceToList(ctx, res.Config.CustomCACerts, model.CustomCACerts.IsNull(), diags)
	model.EncryptionRecoveryKeys = stringSliceToList(ctx, res.Config.EncryptionRecoveryKeys, model.EncryptionRecoveryKeys.IsNull(), diags)
	model.SecureBootEnabled = types.BoolValue(res.State.SecureBootEnabled)
	model.SystemStateIsTrusted = types.BoolValue(res.State.SystemStateIsTrusted)
	model.SystemStateStatus = types.StringValue(res.State.SystemStateStatus)
	model.TPMStatus = types.StringValue(string(res.State.TPMStatus))
}
