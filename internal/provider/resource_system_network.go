package provider

import (
	"context"
	"errors"
	"fmt"
	"time"

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

const (
	confirmInitialBackoff = 200 * time.Millisecond
	confirmMaxBackoff     = 2 * time.Second
	confirmAttemptTimeout = 5 * time.Second
)

var (
	_ resource.Resource                = (*systemNetworkResource)(nil)
	_ resource.ResourceWithImportState = (*systemNetworkResource)(nil)
)

type systemNetworkResource struct {
	client                     *client.Client
	networkConfirmationTimeout time.Duration
}

type systemNetworkModel struct {
	ID                         types.String                  `tfsdk:"id"`
	ConfirmationTimeout        types.String                  `tfsdk:"confirmation_timeout"`
	DefaultConfirmationTimeout types.String                  `tfsdk:"default_confirmation_timeout"`
	IgnoreServiceChecks        types.Bool                    `tfsdk:"ignore_service_checks"`
	AutoConfirm                types.Bool                    `tfsdk:"auto_confirm"`
	DNS                        *systemNetworkDNSModel        `tfsdk:"dns"`
	Time                       *systemNetworkTimeModel       `tfsdk:"time"`
	Proxy                      *systemNetworkProxyModel      `tfsdk:"proxy"`
	Interfaces                 []systemNetworkInterfaceModel `tfsdk:"interfaces"`
	Bonds                      []systemNetworkBondModel      `tfsdk:"bonds"`
	VLANs                      []systemNetworkVLANModel      `tfsdk:"vlans"`
	Wireguard                  []systemNetworkWireguardModel `tfsdk:"wireguard"`
}

type systemNetworkDNSModel struct {
	Domain        types.String `tfsdk:"domain"`
	Hostname      types.String `tfsdk:"hostname"`
	Nameservers   types.List   `tfsdk:"nameservers"`
	SearchDomains types.List   `tfsdk:"search_domains"`
	DNSOverTLS    types.Bool   `tfsdk:"dns_over_tls"`
}

type systemNetworkTimeModel struct {
	NTPServers types.List   `tfsdk:"ntp_servers"`
	Timezone   types.String `tfsdk:"timezone"`
}

type systemNetworkProxyModel struct {
	Rules   []systemNetworkProxyRuleModel            `tfsdk:"rules"`
	Servers map[string]systemNetworkProxyServerModel `tfsdk:"servers"`
}

type systemNetworkProxyRuleModel struct {
	Destination types.String `tfsdk:"destination"`
	Target      types.String `tfsdk:"target"`
}

type systemNetworkProxyServerModel struct {
	Auth     types.String `tfsdk:"auth"`
	Host     types.String `tfsdk:"host"`
	Password types.String `tfsdk:"password"`
	Realm    types.String `tfsdk:"realm"`
	Username types.String `tfsdk:"username"`
	UseTLS   types.Bool   `tfsdk:"use_tls"`
}

type systemNetworkRouteModel struct {
	To     types.String `tfsdk:"to"`
	Via    types.String `tfsdk:"via"`
	OnLink types.Bool   `tfsdk:"onlink"`
}

type systemNetworkFirewallRuleModel struct {
	Action   types.String `tfsdk:"action"`
	Source   types.String `tfsdk:"source"`
	Protocol types.String `tfsdk:"protocol"`
	Port     types.Int64  `tfsdk:"port"`
}

type systemNetworkBridgeModel struct {
	DisableMulticastSnooping types.Bool `tfsdk:"disable_multicast_snooping"`
	STP                      types.Bool `tfsdk:"stp"`
}

type systemNetworkInterfaceModel struct {
	Name              types.String                     `tfsdk:"name"`
	Hwaddr            types.String                     `tfsdk:"hwaddr"`
	Addresses         types.List                       `tfsdk:"addresses"`
	Roles             types.List                       `tfsdk:"roles"`
	MTU               types.Int64                      `tfsdk:"mtu"`
	LLDP              types.Bool                       `tfsdk:"lldp"`
	RequiredForOnline types.String                     `tfsdk:"required_for_online"`
	StrictHwaddr      types.Bool                       `tfsdk:"strict_hwaddr"`
	VLANTags          types.List                       `tfsdk:"vlan_tags"`
	Bridge            *systemNetworkBridgeModel        `tfsdk:"bridge"`
	Routes            []systemNetworkRouteModel        `tfsdk:"routes"`
	FirewallRules     []systemNetworkFirewallRuleModel `tfsdk:"firewall_rules"`
}

type systemNetworkBondOptionsModel struct {
	ARPInterval        types.Int64  `tfsdk:"arp_interval"`
	ARPIPTargets       types.List   `tfsdk:"arp_ip_targets"`
	DownDelay          types.Int64  `tfsdk:"down_delay"`
	LACPRate           types.String `tfsdk:"lacp_rate"`
	MIIMonitorInterval types.Int64  `tfsdk:"mii_monitor_interval"`
	TransmitHashPolicy types.String `tfsdk:"transmit_hash_policy"`
	UpDelay            types.Int64  `tfsdk:"up_delay"`
}

type systemNetworkBondModel struct {
	Name              types.String                     `tfsdk:"name"`
	Mode              types.String                     `tfsdk:"mode"`
	Members           types.List                       `tfsdk:"members"`
	Hwaddr            types.String                     `tfsdk:"hwaddr"`
	Addresses         types.List                       `tfsdk:"addresses"`
	Roles             types.List                       `tfsdk:"roles"`
	MTU               types.Int64                      `tfsdk:"mtu"`
	LLDP              types.Bool                       `tfsdk:"lldp"`
	RequiredForOnline types.String                     `tfsdk:"required_for_online"`
	VLANTags          types.List                       `tfsdk:"vlan_tags"`
	Options           *systemNetworkBondOptionsModel   `tfsdk:"options"`
	Bridge            *systemNetworkBridgeModel        `tfsdk:"bridge"`
	Routes            []systemNetworkRouteModel        `tfsdk:"routes"`
	FirewallRules     []systemNetworkFirewallRuleModel `tfsdk:"firewall_rules"`
}

type systemNetworkVLANModel struct {
	Name              types.String                     `tfsdk:"name"`
	ID                types.Int64                      `tfsdk:"id"`
	Parent            types.String                     `tfsdk:"parent"`
	Addresses         types.List                       `tfsdk:"addresses"`
	Roles             types.List                       `tfsdk:"roles"`
	MTU               types.Int64                      `tfsdk:"mtu"`
	RequiredForOnline types.String                     `tfsdk:"required_for_online"`
	Routes            []systemNetworkRouteModel        `tfsdk:"routes"`
	FirewallRules     []systemNetworkFirewallRuleModel `tfsdk:"firewall_rules"`
}

type systemNetworkWireguardPeerModel struct {
	AllowedIPs          types.List   `tfsdk:"allowed_ips"`
	Endpoint            types.String `tfsdk:"endpoint"`
	PersistentKeepalive types.Int64  `tfsdk:"persistent_keepalive"`
	PresharedKey        types.String `tfsdk:"preshared_key"`
	PublicKey           types.String `tfsdk:"public_key"`
}

type systemNetworkWireguardModel struct {
	Name              types.String                      `tfsdk:"name"`
	Addresses         types.List                        `tfsdk:"addresses"`
	Port              types.Int64                       `tfsdk:"port"`
	PrivateKey        types.String                      `tfsdk:"private_key"`
	MTU               types.Int64                       `tfsdk:"mtu"`
	Roles             types.List                        `tfsdk:"roles"`
	RequiredForOnline types.String                      `tfsdk:"required_for_online"`
	Peers             []systemNetworkWireguardPeerModel `tfsdk:"peers"`
	Routes            []systemNetworkRouteModel         `tfsdk:"routes"`
	FirewallRules     []systemNetworkFirewallRuleModel  `tfsdk:"firewall_rules"`
}

func NewSystemNetworkResource() resource.Resource {
	return &systemNetworkResource{}
}

func (r *systemNetworkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_network"
}

func networkRoutesAttr() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Optional: true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"to":     schema.StringAttribute{Required: true},
				"via":    schema.StringAttribute{Required: true},
				"onlink": schema.BoolAttribute{Optional: true},
			},
		},
	}
}

func networkFirewallRulesAttr() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Optional: true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"action":   schema.StringAttribute{Required: true},
				"source":   schema.StringAttribute{Optional: true},
				"protocol": schema.StringAttribute{Optional: true},
				"port":     schema.Int64Attribute{Optional: true},
			},
		},
	}
}

func networkBridgeAttr() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional: true,
		Attributes: map[string]schema.Attribute{
			"disable_multicast_snooping": schema.BoolAttribute{Optional: true},
			"stp":                        schema.BoolAttribute{Optional: true},
		},
	}
}

func (r *systemNetworkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages IncusOS host network configuration (`/1.0/system/network`), including DNS, NTP/timezone, proxy, physical interfaces, bonds, VLANs, and WireGuard interfaces.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"confirmation_timeout": schema.StringAttribute{
				MarkdownDescription: "Optional per-resource rollback timeout duration (e.g., `60s`, `2m`) overriding the provider's `network_confirmation_timeout`. Set to `0` or `0s` to disable sending a confirmation timeout on apply.",
				Optional:            true,
			},
			"default_confirmation_timeout": schema.StringAttribute{
				MarkdownDescription: "Persistent OS-level default rollback timeout duration stored on IncusOS for network changes that do not specify `confirmation_timeout`.",
				Optional:            true,
			},
			"ignore_service_checks": schema.BoolAttribute{
				MarkdownDescription: "Skip waiting for DNS and NTP availability when bringing up the network.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"auto_confirm": schema.BoolAttribute{
				MarkdownDescription: "Automatically call `POST /1.0/system/network/:confirm` (with retry) after applying changes when a confirmation is pending. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"dns": schema.SingleNestedAttribute{
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"domain":         schema.StringAttribute{Optional: true},
					"hostname":       schema.StringAttribute{Optional: true},
					"nameservers":    schema.ListAttribute{ElementType: types.StringType, Optional: true},
					"search_domains": schema.ListAttribute{ElementType: types.StringType, Optional: true},
					"dns_over_tls":   schema.BoolAttribute{Optional: true},
				},
			},
			"time": schema.SingleNestedAttribute{
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"ntp_servers": schema.ListAttribute{ElementType: types.StringType, Optional: true},
					"timezone":    schema.StringAttribute{Optional: true},
				},
			},
			"proxy": schema.SingleNestedAttribute{
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"rules": schema.ListNestedAttribute{
						Optional: true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"destination": schema.StringAttribute{Required: true},
								"target":      schema.StringAttribute{Required: true},
							},
						},
					},
					"servers": schema.MapNestedAttribute{
						Optional: true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"auth":     schema.StringAttribute{Optional: true},
								"host":     schema.StringAttribute{Required: true},
								"password": schema.StringAttribute{Optional: true, Sensitive: true},
								"realm":    schema.StringAttribute{Optional: true},
								"username": schema.StringAttribute{Optional: true},
								"use_tls":  schema.BoolAttribute{Optional: true},
							},
						},
					},
				},
			},
			"interfaces": schema.ListNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":                schema.StringAttribute{Required: true},
						"hwaddr":              schema.StringAttribute{Required: true},
						"addresses":           schema.ListAttribute{ElementType: types.StringType, Optional: true},
						"roles":               schema.ListAttribute{ElementType: types.StringType, Optional: true},
						"mtu":                 schema.Int64Attribute{Optional: true},
						"lldp":                schema.BoolAttribute{Optional: true},
						"required_for_online": schema.StringAttribute{Optional: true},
						"strict_hwaddr":       schema.BoolAttribute{Optional: true},
						"vlan_tags":           schema.ListAttribute{ElementType: types.Int64Type, Optional: true},
						"bridge":              networkBridgeAttr(),
						"routes":              networkRoutesAttr(),
						"firewall_rules":      networkFirewallRulesAttr(),
					},
				},
			},
			"bonds": schema.ListNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":                schema.StringAttribute{Required: true},
						"mode":                schema.StringAttribute{Required: true},
						"members":             schema.ListAttribute{ElementType: types.StringType, Optional: true},
						"hwaddr":              schema.StringAttribute{Optional: true},
						"addresses":           schema.ListAttribute{ElementType: types.StringType, Optional: true},
						"roles":               schema.ListAttribute{ElementType: types.StringType, Optional: true},
						"mtu":                 schema.Int64Attribute{Optional: true},
						"lldp":                schema.BoolAttribute{Optional: true},
						"required_for_online": schema.StringAttribute{Optional: true},
						"vlan_tags":           schema.ListAttribute{ElementType: types.Int64Type, Optional: true},
						"options": schema.SingleNestedAttribute{
							Optional: true,
							Attributes: map[string]schema.Attribute{
								"arp_interval":         schema.Int64Attribute{Optional: true},
								"arp_ip_targets":       schema.ListAttribute{ElementType: types.StringType, Optional: true},
								"down_delay":           schema.Int64Attribute{Optional: true},
								"lacp_rate":            schema.StringAttribute{Optional: true},
								"mii_monitor_interval": schema.Int64Attribute{Optional: true},
								"transmit_hash_policy": schema.StringAttribute{Optional: true},
								"up_delay":             schema.Int64Attribute{Optional: true},
							},
						},
						"bridge":         networkBridgeAttr(),
						"routes":         networkRoutesAttr(),
						"firewall_rules": networkFirewallRulesAttr(),
					},
				},
			},
			"vlans": schema.ListNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":                schema.StringAttribute{Required: true},
						"id":                  schema.Int64Attribute{Required: true},
						"parent":              schema.StringAttribute{Required: true},
						"addresses":           schema.ListAttribute{ElementType: types.StringType, Optional: true},
						"roles":               schema.ListAttribute{ElementType: types.StringType, Optional: true},
						"mtu":                 schema.Int64Attribute{Optional: true},
						"required_for_online": schema.StringAttribute{Optional: true},
						"routes":              networkRoutesAttr(),
						"firewall_rules":      networkFirewallRulesAttr(),
					},
				},
			},
			"wireguard": schema.ListNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":                schema.StringAttribute{Required: true},
						"addresses":           schema.ListAttribute{ElementType: types.StringType, Optional: true},
						"port":                schema.Int64Attribute{Optional: true},
						"private_key":         schema.StringAttribute{Optional: true, Sensitive: true},
						"mtu":                 schema.Int64Attribute{Optional: true},
						"roles":               schema.ListAttribute{ElementType: types.StringType, Optional: true},
						"required_for_online": schema.StringAttribute{Optional: true},
						"peers": schema.ListNestedAttribute{
							Optional: true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"allowed_ips":          schema.ListAttribute{ElementType: types.StringType, Required: true},
									"endpoint":             schema.StringAttribute{Optional: true},
									"persistent_keepalive": schema.Int64Attribute{Optional: true},
									"preshared_key":        schema.StringAttribute{Optional: true, Sensitive: true},
									"public_key":           schema.StringAttribute{Required: true},
								},
							},
						},
						"routes":         networkRoutesAttr(),
						"firewall_rules": networkFirewallRulesAttr(),
					},
				},
			},
		},
	}
}

func (r *systemNetworkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	cfg := extractProviderConfig(req.ProviderData, &resp.Diagnostics)
	if cfg == nil {
		return
	}

	r.client = cfg.client
	r.networkConfirmationTimeout = cfg.networkConfirmationTimeout
}

func (r *systemNetworkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan systemNetworkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.applyAndRead(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *systemNetworkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state systemNetworkModel
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

func (r *systemNetworkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan systemNetworkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.applyAndRead(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *systemNetworkResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Removing from Terraform state does not wipe host networking to avoid cutting off API connectivity.
}

func (r *systemNetworkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *systemNetworkResource) resolveConfirmationTimeout(plan *systemNetworkModel, diags *diag.Diagnostics) (time.Duration, string, bool) {
	if !plan.ConfirmationTimeout.IsNull() && !plan.ConfirmationTimeout.IsUnknown() && plan.ConfirmationTimeout.ValueString() != "" {
		raw := plan.ConfirmationTimeout.ValueString()
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			diags.AddError("Invalid confirmation_timeout", fmt.Sprintf("Failed to parse confirmation_timeout duration %q: %s", raw, err))

			return 0, "", false
		}

		if parsed < 0 {
			diags.AddError("Invalid confirmation_timeout", fmt.Sprintf("confirmation_timeout must be zero or positive, got %q", raw))

			return 0, "", false
		}

		if parsed == 0 {
			return 0, "", true
		}

		return parsed, raw, true
	}

	if r.networkConfirmationTimeout > 0 {
		return r.networkConfirmationTimeout, r.networkConfirmationTimeout.String(), true
	}

	return 0, "", true
}

func (r *systemNetworkResource) confirmWithRetry(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	confirmCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	backoff := confirmInitialBackoff
	var lastErr error

	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			if lastErr != nil {
				return lastErr
			}

			return confirmCtx.Err()
		}

		attemptTimeout := confirmAttemptTimeout
		if remaining < attemptTimeout {
			attemptTimeout = remaining
		}

		attemptCtx, attemptCancel := context.WithTimeout(confirmCtx, attemptTimeout)
		err := r.client.ConfirmSystemNetwork(attemptCtx)
		attemptCancel()
		if err == nil {
			return nil
		}

		var apiErr *client.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			return err
		}

		lastErr = err

		timer := time.NewTimer(backoff)
		select {
		case <-confirmCtx.Done():
			timer.Stop()

			return lastErr
		case <-timer.C:
		}

		backoff *= 2
		if backoff > confirmMaxBackoff {
			backoff = confirmMaxBackoff
		}
	}
}

func (r *systemNetworkResource) applyAndRead(ctx context.Context, plan *systemNetworkModel, diags *diag.Diagnostics) {
	effectiveTimeout, effectiveTimeoutStr, ok := r.resolveConfirmationTimeout(plan, diags)
	if !ok {
		return
	}

	payload := plan.toAPI(ctx, diags)
	if diags.HasError() {
		return
	}

	payload.Config.ConfirmationTimeout = effectiveTimeoutStr

	// IncusOS triggers a pending confirmation when either the PUT carries a positive
	// confirmation_timeout OR the prior running config had default_confirmation_timeout set.
	pendingTimeout := effectiveTimeout
	if pendingTimeout == 0 && plan.AutoConfirm.ValueBool() {
		if existing, err := r.client.GetSystemNetwork(ctx); err == nil && existing.Config != nil && existing.Config.DefaultConfirmationTimeout != "" {
			if parsed, parseErr := time.ParseDuration(existing.Config.DefaultConfirmationTimeout); parseErr == nil && parsed > 0 {
				pendingTimeout = parsed
			}
		}
	}

	if err := r.client.UpdateSystemNetwork(ctx, payload); err != nil {
		diags.AddError("Failed to update system network", err.Error())

		return
	}

	if plan.AutoConfirm.ValueBool() && pendingTimeout > 0 {
		if err := r.confirmWithRetry(ctx, pendingTimeout); err != nil {
			diags.AddError(
				"Failed to confirm system network changes",
				fmt.Sprintf("%s (IncusOS will automatically roll back unconfirmed network changes after %s)", err.Error(), pendingTimeout),
			)

			return
		}
	}

	r.readIntoModel(ctx, plan, diags)
}

func routesToAPI(in []systemNetworkRouteModel) []osapi.SystemNetworkRoute {
	if len(in) == 0 {
		return nil
	}

	out := make([]osapi.SystemNetworkRoute, 0, len(in))
	for _, r := range in {
		out = append(out, osapi.SystemNetworkRoute{
			To:     r.To.ValueString(),
			Via:    r.Via.ValueString(),
			OnLink: r.OnLink.ValueBool(),
		})
	}

	return out
}

func routesFromAPI(in []osapi.SystemNetworkRoute, prev []systemNetworkRouteModel) []systemNetworkRouteModel {
	if len(in) == 0 {
		if prev != nil {
			return []systemNetworkRouteModel{}
		}

		return nil
	}

	out := make([]systemNetworkRouteModel, 0, len(in))
	for i, r := range in {
		wasOnLinkNull := i >= len(prev) || prev[i].OnLink.IsNull()
		out = append(out, systemNetworkRouteModel{
			To:     types.StringValue(r.To),
			Via:    types.StringValue(r.Via),
			OnLink: boolValueOrNull(r.OnLink, wasOnLinkNull),
		})
	}

	return out
}

func fwRulesToAPI(in []systemNetworkFirewallRuleModel) []osapi.SystemNetworkFirewallRule {
	if len(in) == 0 {
		return nil
	}

	out := make([]osapi.SystemNetworkFirewallRule, 0, len(in))
	for _, r := range in {
		out = append(out, osapi.SystemNetworkFirewallRule{
			Action:   r.Action.ValueString(),
			Source:   r.Source.ValueString(),
			Protocol: r.Protocol.ValueString(),
			Port:     int(r.Port.ValueInt64()),
		})
	}

	return out
}

func fwRulesFromAPI(in []osapi.SystemNetworkFirewallRule, prev []systemNetworkFirewallRuleModel) []systemNetworkFirewallRuleModel {
	if len(in) == 0 {
		if prev != nil {
			return []systemNetworkFirewallRuleModel{}
		}

		return nil
	}

	out := make([]systemNetworkFirewallRuleModel, 0, len(in))
	for i, r := range in {
		wasSrcNull := i >= len(prev) || prev[i].Source.IsNull()
		wasProtoNull := i >= len(prev) || prev[i].Protocol.IsNull()
		wasPortNull := i >= len(prev) || prev[i].Port.IsNull()
		out = append(out, systemNetworkFirewallRuleModel{
			Action:   types.StringValue(r.Action),
			Source:   stringValueOrNull(r.Source, wasSrcNull),
			Protocol: stringValueOrNull(r.Protocol, wasProtoNull),
			Port:     intValueOrNull(r.Port, wasPortNull),
		})
	}

	return out
}

func bridgeToAPI(b *systemNetworkBridgeModel) *osapi.SystemNetworkBridge {
	if b == nil {
		return nil
	}

	return &osapi.SystemNetworkBridge{
		DisableMulticastSnooping: b.DisableMulticastSnooping.ValueBool(),
		STP:                      b.STP.ValueBool(),
	}
}

func bridgeFromAPI(b *osapi.SystemNetworkBridge, prev *systemNetworkBridgeModel) *systemNetworkBridgeModel {
	if b == nil {
		return nil
	}

	return &systemNetworkBridgeModel{
		DisableMulticastSnooping: boolValueOrNull(b.DisableMulticastSnooping, prev == nil || prev.DisableMulticastSnooping.IsNull()),
		STP:                      boolValueOrNull(b.STP, prev == nil || prev.STP.IsNull()),
	}
}

func (m *systemNetworkModel) toAPI(ctx context.Context, diags *diag.Diagnostics) osapi.SystemNetwork {
	cfg := &osapi.SystemNetworkConfig{
		ConfirmationTimeout:        m.ConfirmationTimeout.ValueString(),
		DefaultConfirmationTimeout: m.DefaultConfirmationTimeout.ValueString(),
		IgnoreServiceChecks:        m.IgnoreServiceChecks.ValueBool(),
	}

	if m.DNS != nil {
		cfg.DNS = &osapi.SystemNetworkDNS{
			Domain:        m.DNS.Domain.ValueString(),
			Hostname:      m.DNS.Hostname.ValueString(),
			Nameservers:   listToStringSlice(ctx, m.DNS.Nameservers, diags),
			SearchDomains: listToStringSlice(ctx, m.DNS.SearchDomains, diags),
			DNSOverTLS:    m.DNS.DNSOverTLS.ValueBool(),
		}
	}

	if m.Time != nil {
		cfg.Time = &osapi.SystemNetworkTime{
			NTPServers: listToStringSlice(ctx, m.Time.NTPServers, diags),
			Timezone:   m.Time.Timezone.ValueString(),
		}
	}

	if m.Proxy != nil {
		p := &osapi.SystemNetworkProxy{}
		for _, r := range m.Proxy.Rules {
			p.Rules = append(p.Rules, osapi.SystemNetworkProxyRule{
				Destination: r.Destination.ValueString(),
				Target:      r.Target.ValueString(),
			})
		}

		if len(m.Proxy.Servers) > 0 {
			p.Servers = make(map[string]osapi.SystemNetworkProxyServer, len(m.Proxy.Servers))
			for k, s := range m.Proxy.Servers {
				p.Servers[k] = osapi.SystemNetworkProxyServer{
					Auth:     s.Auth.ValueString(),
					Host:     s.Host.ValueString(),
					Password: s.Password.ValueString(),
					Realm:    s.Realm.ValueString(),
					Username: s.Username.ValueString(),
					UseTLS:   s.UseTLS.ValueBool(),
				}
			}
		}

		cfg.Proxy = p
	}

	for _, iface := range m.Interfaces {
		cfg.Interfaces = append(cfg.Interfaces, osapi.SystemNetworkInterface{
			Name:              iface.Name.ValueString(),
			Hwaddr:            iface.Hwaddr.ValueString(),
			Addresses:         listToStringSlice(ctx, iface.Addresses, diags),
			Roles:             listToStringSlice(ctx, iface.Roles, diags),
			MTU:               int(iface.MTU.ValueInt64()),
			LLDP:              iface.LLDP.ValueBool(),
			RequiredForOnline: iface.RequiredForOnline.ValueString(),
			StrictHwaddr:      iface.StrictHwaddr.ValueBool(),
			VLANTags:          listToIntSlice(ctx, iface.VLANTags, diags),
			Bridge:            bridgeToAPI(iface.Bridge),
			Routes:            routesToAPI(iface.Routes),
			FirewallRules:     fwRulesToAPI(iface.FirewallRules),
		})
	}

	for _, bond := range m.Bonds {
		b := osapi.SystemNetworkBond{
			Name:              bond.Name.ValueString(),
			Mode:              bond.Mode.ValueString(),
			Members:           listToStringSlice(ctx, bond.Members, diags),
			Hwaddr:            bond.Hwaddr.ValueString(),
			Addresses:         listToStringSlice(ctx, bond.Addresses, diags),
			Roles:             listToStringSlice(ctx, bond.Roles, diags),
			MTU:               int(bond.MTU.ValueInt64()),
			LLDP:              bond.LLDP.ValueBool(),
			RequiredForOnline: bond.RequiredForOnline.ValueString(),
			VLANTags:          listToIntSlice(ctx, bond.VLANTags, diags),
			Bridge:            bridgeToAPI(bond.Bridge),
			Routes:            routesToAPI(bond.Routes),
			FirewallRules:     fwRulesToAPI(bond.FirewallRules),
		}

		if bond.Options != nil {
			b.Options = &osapi.SystemNetworkBondOptions{
				ARPInterval:        int(bond.Options.ARPInterval.ValueInt64()),
				ARPIPTargets:       listToStringSlice(ctx, bond.Options.ARPIPTargets, diags),
				DownDelay:          int(bond.Options.DownDelay.ValueInt64()),
				LACPRate:           bond.Options.LACPRate.ValueString(),
				MIIMonitorInterval: int(bond.Options.MIIMonitorInterval.ValueInt64()),
				TransmitHashPolicy: bond.Options.TransmitHashPolicy.ValueString(),
				UpDelay:            int(bond.Options.UpDelay.ValueInt64()),
			}
		}

		cfg.Bonds = append(cfg.Bonds, b)
	}

	for _, vlan := range m.VLANs {
		cfg.VLANs = append(cfg.VLANs, osapi.SystemNetworkVLAN{
			Name:              vlan.Name.ValueString(),
			ID:                int(vlan.ID.ValueInt64()),
			Parent:            vlan.Parent.ValueString(),
			Addresses:         listToStringSlice(ctx, vlan.Addresses, diags),
			Roles:             listToStringSlice(ctx, vlan.Roles, diags),
			MTU:               int(vlan.MTU.ValueInt64()),
			RequiredForOnline: vlan.RequiredForOnline.ValueString(),
			Routes:            routesToAPI(vlan.Routes),
			FirewallRules:     fwRulesToAPI(vlan.FirewallRules),
		})
	}

	for _, wg := range m.Wireguard {
		w := osapi.SystemNetworkWireguard{
			Name:              wg.Name.ValueString(),
			Addresses:         listToStringSlice(ctx, wg.Addresses, diags),
			Port:              int(wg.Port.ValueInt64()),
			PrivateKey:        wg.PrivateKey.ValueString(),
			MTU:               int(wg.MTU.ValueInt64()),
			Roles:             listToStringSlice(ctx, wg.Roles, diags),
			RequiredForOnline: wg.RequiredForOnline.ValueString(),
			Routes:            routesToAPI(wg.Routes),
			FirewallRules:     fwRulesToAPI(wg.FirewallRules),
		}

		for _, peer := range wg.Peers {
			w.Peers = append(w.Peers, osapi.SystemNetworkWireguardPeer{
				AllowedIPs:          listToStringSlice(ctx, peer.AllowedIPs, diags),
				Endpoint:            peer.Endpoint.ValueString(),
				PersistentKeepalive: int(peer.PersistentKeepalive.ValueInt64()),
				PresharedKey:        peer.PresharedKey.ValueString(),
				PublicKey:           peer.PublicKey.ValueString(),
			})
		}

		cfg.Wireguard = append(cfg.Wireguard, w)
	}

	return osapi.SystemNetwork{Config: cfg}
}

func (r *systemNetworkResource) readIntoModel(ctx context.Context, model *systemNetworkModel, diags *diag.Diagnostics) {
	res, err := r.client.GetSystemNetwork(ctx)
	if err != nil {
		diags.AddError("Failed to read system network", err.Error())

		return
	}

	model.ID = types.StringValue("system-network")
	if model.AutoConfirm.IsNull() || model.AutoConfirm.IsUnknown() {
		model.AutoConfirm = types.BoolValue(true)
	}

	if res.Config == nil {
		return
	}

	cfg := res.Config
	// ConfirmationTimeout is a one-shot apply parameter cleared by IncusOS after PUT
	// and never returned by GET; preserve the configured value in state.
	model.DefaultConfirmationTimeout = stringValueOrNull(cfg.DefaultConfirmationTimeout, model.DefaultConfirmationTimeout.IsNull())
	model.IgnoreServiceChecks = types.BoolValue(cfg.IgnoreServiceChecks)

	if cfg.DNS != nil {
		prev := model.DNS
		model.DNS = &systemNetworkDNSModel{
			Domain:        stringValueOrNull(cfg.DNS.Domain, prev == nil || prev.Domain.IsNull()),
			Hostname:      stringValueOrNull(cfg.DNS.Hostname, prev == nil || prev.Hostname.IsNull()),
			Nameservers:   stringSliceToList(ctx, cfg.DNS.Nameservers, prev == nil || prev.Nameservers.IsNull(), diags),
			SearchDomains: stringSliceToList(ctx, cfg.DNS.SearchDomains, prev == nil || prev.SearchDomains.IsNull(), diags),
			DNSOverTLS:    boolValueOrNull(cfg.DNS.DNSOverTLS, prev == nil || prev.DNSOverTLS.IsNull()),
		}
	} else {
		model.DNS = nil
	}

	if cfg.Time != nil {
		prev := model.Time
		model.Time = &systemNetworkTimeModel{
			NTPServers: stringSliceToList(ctx, cfg.Time.NTPServers, prev == nil || prev.NTPServers.IsNull(), diags),
			Timezone:   stringValueOrNull(cfg.Time.Timezone, prev == nil || prev.Timezone.IsNull()),
		}
	} else {
		model.Time = nil
	}

	if cfg.Proxy != nil {
		prev := model.Proxy
		p := &systemNetworkProxyModel{}
		for _, rule := range cfg.Proxy.Rules {
			p.Rules = append(p.Rules, systemNetworkProxyRuleModel{
				Destination: types.StringValue(rule.Destination),
				Target:      types.StringValue(rule.Target),
			})
		}

		if len(cfg.Proxy.Servers) > 0 {
			p.Servers = make(map[string]systemNetworkProxyServerModel, len(cfg.Proxy.Servers))
			for k, s := range cfg.Proxy.Servers {
				var ps *systemNetworkProxyServerModel
				if prev != nil {
					if existing, ok := prev.Servers[k]; ok {
						ps = &existing
					}
				}

				p.Servers[k] = systemNetworkProxyServerModel{
					Auth:     stringValueOrNull(s.Auth, ps == nil || ps.Auth.IsNull()),
					Host:     types.StringValue(s.Host),
					Password: stringValueOrNull(s.Password, ps == nil || ps.Password.IsNull()),
					Realm:    stringValueOrNull(s.Realm, ps == nil || ps.Realm.IsNull()),
					Username: stringValueOrNull(s.Username, ps == nil || ps.Username.IsNull()),
					UseTLS:   boolValueOrNull(s.UseTLS, ps == nil || ps.UseTLS.IsNull()),
				}
			}
		}

		model.Proxy = p
	} else {
		model.Proxy = nil
	}

	if len(cfg.Interfaces) == 0 {
		if model.Interfaces != nil {
			model.Interfaces = []systemNetworkInterfaceModel{}
		}
	} else {
		ifaces := make([]systemNetworkInterfaceModel, 0, len(cfg.Interfaces))
		for i, iface := range cfg.Interfaces {
			var prev *systemNetworkInterfaceModel
			if i < len(model.Interfaces) {
				prev = &model.Interfaces[i]
			}

			var prevBridge *systemNetworkBridgeModel
			var prevRoutes []systemNetworkRouteModel
			var prevFW []systemNetworkFirewallRuleModel
			if prev != nil {
				prevBridge = prev.Bridge
				prevRoutes = prev.Routes
				prevFW = prev.FirewallRules
			}

			ifaces = append(ifaces, systemNetworkInterfaceModel{
				Name:              types.StringValue(iface.Name),
				Hwaddr:            types.StringValue(iface.Hwaddr),
				Addresses:         stringSliceToList(ctx, iface.Addresses, prev == nil || prev.Addresses.IsNull(), diags),
				Roles:             stringSliceToList(ctx, iface.Roles, prev == nil || prev.Roles.IsNull(), diags),
				MTU:               intValueOrNull(iface.MTU, prev == nil || prev.MTU.IsNull()),
				LLDP:              boolValueOrNull(iface.LLDP, prev == nil || prev.LLDP.IsNull()),
				RequiredForOnline: stringValueOrNull(iface.RequiredForOnline, prev == nil || prev.RequiredForOnline.IsNull()),
				StrictHwaddr:      boolValueOrNull(iface.StrictHwaddr, prev == nil || prev.StrictHwaddr.IsNull()),
				VLANTags:          intSliceToList(ctx, iface.VLANTags, prev == nil || prev.VLANTags.IsNull(), diags),
				Bridge:            bridgeFromAPI(iface.Bridge, prevBridge),
				Routes:            routesFromAPI(iface.Routes, prevRoutes),
				FirewallRules:     fwRulesFromAPI(iface.FirewallRules, prevFW),
			})
		}

		model.Interfaces = ifaces
	}

	if len(cfg.Bonds) == 0 {
		if model.Bonds != nil {
			model.Bonds = []systemNetworkBondModel{}
		}
	} else {
		bonds := make([]systemNetworkBondModel, 0, len(cfg.Bonds))
		for i, bond := range cfg.Bonds {
			var prev *systemNetworkBondModel
			if i < len(model.Bonds) {
				prev = &model.Bonds[i]
			}

			var prevBridge *systemNetworkBridgeModel
			var prevRoutes []systemNetworkRouteModel
			var prevFW []systemNetworkFirewallRuleModel
			var prevOpts *systemNetworkBondOptionsModel
			if prev != nil {
				prevBridge = prev.Bridge
				prevRoutes = prev.Routes
				prevFW = prev.FirewallRules
				prevOpts = prev.Options
			}

			var opts *systemNetworkBondOptionsModel
			if bond.Options != nil {
				opts = &systemNetworkBondOptionsModel{
					ARPInterval:        intValueOrNull(bond.Options.ARPInterval, prevOpts == nil || prevOpts.ARPInterval.IsNull()),
					ARPIPTargets:       stringSliceToList(ctx, bond.Options.ARPIPTargets, prevOpts == nil || prevOpts.ARPIPTargets.IsNull(), diags),
					DownDelay:          intValueOrNull(bond.Options.DownDelay, prevOpts == nil || prevOpts.DownDelay.IsNull()),
					LACPRate:           stringValueOrNull(bond.Options.LACPRate, prevOpts == nil || prevOpts.LACPRate.IsNull()),
					MIIMonitorInterval: intValueOrNull(bond.Options.MIIMonitorInterval, prevOpts == nil || prevOpts.MIIMonitorInterval.IsNull()),
					TransmitHashPolicy: stringValueOrNull(bond.Options.TransmitHashPolicy, prevOpts == nil || prevOpts.TransmitHashPolicy.IsNull()),
					UpDelay:            intValueOrNull(bond.Options.UpDelay, prevOpts == nil || prevOpts.UpDelay.IsNull()),
				}
			}

			bonds = append(bonds, systemNetworkBondModel{
				Name:              types.StringValue(bond.Name),
				Mode:              types.StringValue(bond.Mode),
				Members:           stringSliceToList(ctx, bond.Members, prev == nil || prev.Members.IsNull(), diags),
				Hwaddr:            stringValueOrNull(bond.Hwaddr, prev == nil || prev.Hwaddr.IsNull()),
				Addresses:         stringSliceToList(ctx, bond.Addresses, prev == nil || prev.Addresses.IsNull(), diags),
				Roles:             stringSliceToList(ctx, bond.Roles, prev == nil || prev.Roles.IsNull(), diags),
				MTU:               intValueOrNull(bond.MTU, prev == nil || prev.MTU.IsNull()),
				LLDP:              boolValueOrNull(bond.LLDP, prev == nil || prev.LLDP.IsNull()),
				RequiredForOnline: stringValueOrNull(bond.RequiredForOnline, prev == nil || prev.RequiredForOnline.IsNull()),
				VLANTags:          intSliceToList(ctx, bond.VLANTags, prev == nil || prev.VLANTags.IsNull(), diags),
				Options:           opts,
				Bridge:            bridgeFromAPI(bond.Bridge, prevBridge),
				Routes:            routesFromAPI(bond.Routes, prevRoutes),
				FirewallRules:     fwRulesFromAPI(bond.FirewallRules, prevFW),
			})
		}

		model.Bonds = bonds
	}

	if len(cfg.VLANs) == 0 {
		if model.VLANs != nil {
			model.VLANs = []systemNetworkVLANModel{}
		}
	} else {
		vlans := make([]systemNetworkVLANModel, 0, len(cfg.VLANs))
		for i, vlan := range cfg.VLANs {
			var prev *systemNetworkVLANModel
			if i < len(model.VLANs) {
				prev = &model.VLANs[i]
			}

			var prevRoutes []systemNetworkRouteModel
			var prevFW []systemNetworkFirewallRuleModel
			if prev != nil {
				prevRoutes = prev.Routes
				prevFW = prev.FirewallRules
			}

			vlans = append(vlans, systemNetworkVLANModel{
				Name:              types.StringValue(vlan.Name),
				ID:                types.Int64Value(int64(vlan.ID)),
				Parent:            types.StringValue(vlan.Parent),
				Addresses:         stringSliceToList(ctx, vlan.Addresses, prev == nil || prev.Addresses.IsNull(), diags),
				Roles:             stringSliceToList(ctx, vlan.Roles, prev == nil || prev.Roles.IsNull(), diags),
				MTU:               intValueOrNull(vlan.MTU, prev == nil || prev.MTU.IsNull()),
				RequiredForOnline: stringValueOrNull(vlan.RequiredForOnline, prev == nil || prev.RequiredForOnline.IsNull()),
				Routes:            routesFromAPI(vlan.Routes, prevRoutes),
				FirewallRules:     fwRulesFromAPI(vlan.FirewallRules, prevFW),
			})
		}

		model.VLANs = vlans
	}

	if len(cfg.Wireguard) == 0 {
		if model.Wireguard != nil {
			model.Wireguard = []systemNetworkWireguardModel{}
		}
	} else {
		wgs := make([]systemNetworkWireguardModel, 0, len(cfg.Wireguard))
		for i, wg := range cfg.Wireguard {
			var prev *systemNetworkWireguardModel
			if i < len(model.Wireguard) {
				prev = &model.Wireguard[i]
			}

			var prevRoutes []systemNetworkRouteModel
			var prevFW []systemNetworkFirewallRuleModel
			if prev != nil {
				prevRoutes = prev.Routes
				prevFW = prev.FirewallRules
			}

			var peers []systemNetworkWireguardPeerModel
			for j, peer := range wg.Peers {
				var prevPeer *systemNetworkWireguardPeerModel
				if prev != nil && j < len(prev.Peers) {
					prevPeer = &prev.Peers[j]
				}

				peers = append(peers, systemNetworkWireguardPeerModel{
					AllowedIPs:          stringSliceToList(ctx, peer.AllowedIPs, false, diags),
					Endpoint:            stringValueOrNull(peer.Endpoint, prevPeer == nil || prevPeer.Endpoint.IsNull()),
					PersistentKeepalive: intValueOrNull(peer.PersistentKeepalive, prevPeer == nil || prevPeer.PersistentKeepalive.IsNull()),
					PresharedKey:        stringValueOrNull(peer.PresharedKey, prevPeer == nil || prevPeer.PresharedKey.IsNull()),
					PublicKey:           types.StringValue(peer.PublicKey),
				})
			}

			wgs = append(wgs, systemNetworkWireguardModel{
				Name:              types.StringValue(wg.Name),
				Addresses:         stringSliceToList(ctx, wg.Addresses, prev == nil || prev.Addresses.IsNull(), diags),
				Port:              intValueOrNull(wg.Port, prev == nil || prev.Port.IsNull()),
				PrivateKey:        stringValueOrNull(wg.PrivateKey, prev == nil || prev.PrivateKey.IsNull()),
				MTU:               intValueOrNull(wg.MTU, prev == nil || prev.MTU.IsNull()),
				Roles:             stringSliceToList(ctx, wg.Roles, prev == nil || prev.Roles.IsNull(), diags),
				RequiredForOnline: stringValueOrNull(wg.RequiredForOnline, prev == nil || prev.RequiredForOnline.IsNull()),
				Peers:             peers,
				Routes:            routesFromAPI(wg.Routes, prevRoutes),
				FirewallRules:     fwRulesFromAPI(wg.FirewallRules, prevFW),
			})
		}

		model.Wireguard = wgs
	}
}
