package provider

import (
	"context"
	"fmt"

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
	_ resource.Resource                = (*systemLoggingResource)(nil)
	_ resource.ResourceWithImportState = (*systemLoggingResource)(nil)
)

type systemLoggingResource struct {
	client *client.Client
}

type systemLoggingModel struct {
	ID            types.String                `tfsdk:"id"`
	Syslog        *systemLoggingSyslogModel   `tfsdk:"syslog"`
	JournalUpload *systemLoggingJournalUpload `tfsdk:"journal_upload"`
}

type systemLoggingSyslogModel struct {
	Address   types.String `tfsdk:"address"`
	Protocol  types.String `tfsdk:"protocol"`
	LogFormat types.String `tfsdk:"log_format"`
}

type systemLoggingJournalUpload struct {
	URL                  types.String `tfsdk:"url"`
	TLSClientCertificate types.String `tfsdk:"tls_client_certificate"`
	TLSClientKey         types.String `tfsdk:"tls_client_key"`
	TLSCACertificate     types.String `tfsdk:"tls_ca_certificate"`
}

func NewSystemLoggingResource() resource.Resource {
	return &systemLoggingResource{}
}

func (r *systemLoggingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_logging"
}

func (r *systemLoggingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages IncusOS system logging configuration (`/1.0/system/logging`), including remote syslog and systemd-journal-upload.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"syslog": schema.SingleNestedAttribute{
				MarkdownDescription: "Remote syslog server configuration.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"address": schema.StringAttribute{
						MarkdownDescription: "Remote syslog server address (host:port).",
						Required:            true,
					},
					"protocol": schema.StringAttribute{
						MarkdownDescription: "Transport protocol (e.g., `tcp`, `udp`).",
						Optional:            true,
					},
					"log_format": schema.StringAttribute{
						MarkdownDescription: "Syslog message format.",
						Optional:            true,
					},
				},
			},
			"journal_upload": schema.SingleNestedAttribute{
				MarkdownDescription: "systemd-journal-upload remote server configuration.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"url": schema.StringAttribute{
						MarkdownDescription: "Remote journal upload URL.",
						Required:            true,
					},
					"tls_client_certificate": schema.StringAttribute{
						MarkdownDescription: "PEM-encoded TLS client certificate.",
						Optional:            true,
						Sensitive:           true,
					},
					"tls_client_key": schema.StringAttribute{
						MarkdownDescription: "PEM-encoded TLS client private key.",
						Optional:            true,
						Sensitive:           true,
					},
					"tls_ca_certificate": schema.StringAttribute{
						MarkdownDescription: "PEM-encoded TLS CA certificate.",
						Optional:            true,
					},
				},
			},
		},
	}
}

func (r *systemLoggingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (r *systemLoggingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan systemLoggingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateSystemLogging(ctx, plan.toAPI()); err != nil {
		resp.Diagnostics.AddError("Failed to configure system logging", err.Error())

		return
	}

	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *systemLoggingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state systemLoggingModel
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

func (r *systemLoggingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan systemLoggingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateSystemLogging(ctx, plan.toAPI()); err != nil {
		resp.Diagnostics.AddError("Failed to update system logging", err.Error())

		return
	}

	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *systemLoggingResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.UpdateSystemLogging(ctx, osapi.SystemLogging{}); err != nil {
		resp.Diagnostics.AddError("Failed to reset system logging", fmt.Sprintf("Error resetting system logging: %s", err))
	}
}

func (r *systemLoggingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *systemLoggingModel) toAPI() osapi.SystemLogging {
	out := osapi.SystemLogging{}
	if m.Syslog != nil {
		out.Config.Syslog = osapi.SystemLoggingSyslog{
			Address:   m.Syslog.Address.ValueString(),
			Protocol:  m.Syslog.Protocol.ValueString(),
			LogFormat: m.Syslog.LogFormat.ValueString(),
		}
	}

	if m.JournalUpload != nil {
		out.Config.JournalUpload = osapi.SystemLoggingJournalUpload{
			URL:                  m.JournalUpload.URL.ValueString(),
			TLSClientCertificate: m.JournalUpload.TLSClientCertificate.ValueString(),
			TLSClientKey:         m.JournalUpload.TLSClientKey.ValueString(),
			TLSCACertificate:     m.JournalUpload.TLSCACertificate.ValueString(),
		}
	}

	return out
}

func (r *systemLoggingResource) readIntoModel(ctx context.Context, model *systemLoggingModel, diags interface{ AddError(string, string) }) {
	res, err := r.client.GetSystemLogging(ctx)
	if err != nil {
		diags.AddError("Failed to read system logging", err.Error())

		return
	}

	model.ID = types.StringValue("system-logging")

	s := res.Config.Syslog
	if s.Address == "" && s.Protocol == "" && s.LogFormat == "" {
		model.Syslog = nil
	} else {
		wasProtoNull := model.Syslog == nil || model.Syslog.Protocol.IsNull()
		wasFmtNull := model.Syslog == nil || model.Syslog.LogFormat.IsNull()
		model.Syslog = &systemLoggingSyslogModel{
			Address:   types.StringValue(s.Address),
			Protocol:  stringValueOrNull(s.Protocol, wasProtoNull),
			LogFormat: stringValueOrNull(s.LogFormat, wasFmtNull),
		}
	}

	j := res.Config.JournalUpload
	if j.URL == "" && j.TLSClientCertificate == "" && j.TLSClientKey == "" && j.TLSCACertificate == "" {
		model.JournalUpload = nil
	} else {
		wasCertNull := model.JournalUpload == nil || model.JournalUpload.TLSClientCertificate.IsNull()
		wasKeyNull := model.JournalUpload == nil || model.JournalUpload.TLSClientKey.IsNull()
		wasCANull := model.JournalUpload == nil || model.JournalUpload.TLSCACertificate.IsNull()
		model.JournalUpload = &systemLoggingJournalUpload{
			URL:                  types.StringValue(j.URL),
			TLSClientCertificate: stringValueOrNull(j.TLSClientCertificate, wasCertNull),
			TLSClientKey:         stringValueOrNull(j.TLSClientKey, wasKeyNull),
			TLSCACertificate:     stringValueOrNull(j.TLSCACertificate, wasCANull),
		}
	}
}
