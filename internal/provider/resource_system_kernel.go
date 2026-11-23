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
	_ resource.Resource                = (*systemKernelResource)(nil)
	_ resource.ResourceWithImportState = (*systemKernelResource)(nil)
)

type systemKernelResource struct {
	client *client.Client
}

type systemKernelModel struct {
	ID               types.String          `tfsdk:"id"`
	BlacklistModules types.List            `tfsdk:"blacklist_modules"`
	Console          []systemKernelConsole `tfsdk:"console"`
	CPU              *systemKernelCPU      `tfsdk:"cpu"`
	Memory           *systemKernelMemory   `tfsdk:"memory"`
	Network          *systemKernelNetwork  `tfsdk:"network"`
	PCI              *systemKernelPCI      `tfsdk:"pci"`
	ZFS              *systemKernelZFS      `tfsdk:"zfs"`
	ClockSource      types.String          `tfsdk:"clock_source"`
}

type systemKernelConsole struct {
	Device   types.String `tfsdk:"device"`
	BaudRate types.Int64  `tfsdk:"baud_rate"`
}

type systemKernelCPU struct {
	DisableThermalProtection types.Bool `tfsdk:"disable_thermal_protection"`
}

type systemKernelMemory struct {
	PersistentHugepages types.Int64  `tfsdk:"persistent_hugepages"`
	ZramSwapSize        types.String `tfsdk:"zram_swap_size"`
}

type systemKernelNetwork struct {
	BufferSize             types.Int64  `tfsdk:"buffer_size"`
	NetdevMaxBacklog       types.Int64  `tfsdk:"netdev_max_backlog"`
	QueuingDiscipline      types.String `tfsdk:"queuing_discipline"`
	TCPCongestionAlgorithm types.String `tfsdk:"tcp_congestion_algorithm"`
	TCPMTUProbing          types.Bool   `tfsdk:"tcp_mtu_probing"`
}

type systemKernelPCI struct {
	Passthrough []systemKernelPCIPassthrough `tfsdk:"passthrough"`
	SRIOV       []systemKernelPCISRIOV       `tfsdk:"sriov"`
}

type systemKernelPCIPassthrough struct {
	VendorID   types.String `tfsdk:"vendor_id"`
	ProductID  types.String `tfsdk:"product_id"`
	PCIAddress types.String `tfsdk:"pci_address"`
}

type systemKernelPCISRIOV struct {
	PCIAddress types.String `tfsdk:"pci_address"`
	Switchdev  types.Bool   `tfsdk:"switchdev"`
	VFCount    types.Int64  `tfsdk:"vf_count"`
}

type systemKernelZFS struct {
	ARCMaxSize types.String `tfsdk:"arc_max_size"`
}

func NewSystemKernelResource() resource.Resource {
	return &systemKernelResource{}
}

func (r *systemKernelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_kernel"
}

func (r *systemKernelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages IncusOS kernel-level configuration (`/1.0/system/kernel`), including sysctls, PCI passthrough/SR-IOV, ZFS ARC, hugepages, and console settings.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"blacklist_modules": schema.ListAttribute{
				MarkdownDescription: "Kernel modules to blacklist.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"console": schema.ListNestedAttribute{
				MarkdownDescription: "Serial/console devices configured for kernel output.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"device":    schema.StringAttribute{Required: true},
						"baud_rate": schema.Int64Attribute{Optional: true},
					},
				},
			},
			"cpu": schema.SingleNestedAttribute{
				MarkdownDescription: "CPU-specific kernel configuration.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"disable_thermal_protection": schema.BoolAttribute{Optional: true},
				},
			},
			"memory": schema.SingleNestedAttribute{
				MarkdownDescription: "Memory-specific kernel configuration.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"persistent_hugepages": schema.Int64Attribute{Optional: true},
					"zram_swap_size":       schema.StringAttribute{Optional: true},
				},
			},
			"network": schema.SingleNestedAttribute{
				MarkdownDescription: "Network-specific kernel tuning parameters.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"buffer_size":              schema.Int64Attribute{Optional: true},
					"netdev_max_backlog":       schema.Int64Attribute{Optional: true},
					"queuing_discipline":       schema.StringAttribute{Optional: true},
					"tcp_congestion_algorithm": schema.StringAttribute{Optional: true},
					"tcp_mtu_probing":          schema.BoolAttribute{Optional: true},
				},
			},
			"pci": schema.SingleNestedAttribute{
				MarkdownDescription: "PCI passthrough and SR-IOV configuration.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"passthrough": schema.ListNestedAttribute{
						Optional: true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"vendor_id":   schema.StringAttribute{Required: true},
								"product_id":  schema.StringAttribute{Required: true},
								"pci_address": schema.StringAttribute{Optional: true},
							},
						},
					},
					"sriov": schema.ListNestedAttribute{
						Optional: true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"pci_address": schema.StringAttribute{Required: true},
								"switchdev":   schema.BoolAttribute{Optional: true},
								"vf_count":    schema.Int64Attribute{Required: true},
							},
						},
					},
				},
			},
			"zfs": schema.SingleNestedAttribute{
				MarkdownDescription: "ZFS kernel module configuration.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"arc_max_size": schema.StringAttribute{Optional: true},
				},
			},
			"clock_source": schema.StringAttribute{
				MarkdownDescription: "Current kernel timekeeping clock source.",
				Computed:            true,
			},
		},
	}
}

func (r *systemKernelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = extractClient(req.ProviderData, &resp.Diagnostics)
}

func (r *systemKernelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan systemKernelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := plan.toAPI(ctx, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateSystemKernel(ctx, payload); err != nil {
		resp.Diagnostics.AddError("Failed to configure system kernel", err.Error())

		return
	}

	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *systemKernelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state systemKernelModel
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

func (r *systemKernelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan systemKernelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := plan.toAPI(ctx, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateSystemKernel(ctx, payload); err != nil {
		resp.Diagnostics.AddError("Failed to update system kernel", err.Error())

		return
	}

	r.readIntoModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *systemKernelResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.UpdateSystemKernel(ctx, osapi.SystemKernel{}); err != nil {
		resp.Diagnostics.AddError("Failed to reset system kernel", err.Error())
	}
}

func (r *systemKernelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *systemKernelModel) toAPI(ctx context.Context, diags *diag.Diagnostics) osapi.SystemKernel {
	cfg := osapi.SystemKernelConfig{
		BlacklistModules: listToStringSlice(ctx, m.BlacklistModules, diags),
	}

	for _, c := range m.Console {
		cfg.Console = append(cfg.Console, osapi.SystemKernelConfigConsole{
			Device:   c.Device.ValueString(),
			BaudRate: int(c.BaudRate.ValueInt64()),
		})
	}

	if m.CPU != nil {
		cfg.CPU = &osapi.SystemKernelConfigCPU{
			DisableThermalProtection: m.CPU.DisableThermalProtection.ValueBool(),
		}
	}

	if m.Memory != nil {
		cfg.Memory = &osapi.SystemKernelConfigMemory{
			PersistentHugepages: int(m.Memory.PersistentHugepages.ValueInt64()),
			ZramSwapSize:        m.Memory.ZramSwapSize.ValueString(),
		}
	}

	if m.Network != nil {
		cfg.Network = &osapi.SystemKernelConfigNetwork{
			BufferSize:             int(m.Network.BufferSize.ValueInt64()),
			NetdevMaxBacklog:       int(m.Network.NetdevMaxBacklog.ValueInt64()),
			QueuingDiscipline:      m.Network.QueuingDiscipline.ValueString(),
			TCPCongestionAlgorithm: m.Network.TCPCongestionAlgorithm.ValueString(),
			TCPMTUProbing:          m.Network.TCPMTUProbing.ValueBool(),
		}
	}

	if m.PCI != nil {
		pci := &osapi.SystemKernelConfigPCI{}
		for _, p := range m.PCI.Passthrough {
			pci.Passthrough = append(pci.Passthrough, osapi.SystemKernelConfigPCIPassthrough{
				VendorID:   p.VendorID.ValueString(),
				ProductID:  p.ProductID.ValueString(),
				PCIAddress: p.PCIAddress.ValueString(),
			})
		}

		for _, s := range m.PCI.SRIOV {
			pci.SRIOV = append(pci.SRIOV, osapi.SystemKernelConfigPCISRIOV{
				PCIAddress: s.PCIAddress.ValueString(),
				Switchdev:  s.Switchdev.ValueBool(),
				VFCount:    int(s.VFCount.ValueInt64()),
			})
		}

		cfg.PCI = pci
	}

	if m.ZFS != nil {
		cfg.ZFS = &osapi.SystemKernelConfigZFS{
			ARCMaxSize: m.ZFS.ARCMaxSize.ValueString(),
		}
	}

	return osapi.SystemKernel{Config: cfg}
}

func (r *systemKernelResource) readIntoModel(ctx context.Context, model *systemKernelModel, diags *diag.Diagnostics) {
	res, err := r.client.GetSystemKernel(ctx)
	if err != nil {
		diags.AddError("Failed to read system kernel", err.Error())

		return
	}

	model.ID = types.StringValue("system-kernel")
	model.BlacklistModules = stringSliceToList(ctx, res.Config.BlacklistModules, model.BlacklistModules.IsNull(), diags)

	if len(res.Config.Console) == 0 {
		if model.Console != nil {
			model.Console = []systemKernelConsole{}
		}
	} else {
		consoles := make([]systemKernelConsole, 0, len(res.Config.Console))
		for i, c := range res.Config.Console {
			wasBaudNull := i >= len(model.Console) || model.Console[i].BaudRate.IsNull()
			consoles = append(consoles, systemKernelConsole{
				Device:   types.StringValue(c.Device),
				BaudRate: intValueOrNull(c.BaudRate, wasBaudNull),
			})
		}

		model.Console = consoles
	}

	if res.Config.CPU != nil {
		wasNull := model.CPU == nil || model.CPU.DisableThermalProtection.IsNull()
		model.CPU = &systemKernelCPU{
			DisableThermalProtection: boolValueOrNull(res.Config.CPU.DisableThermalProtection, wasNull),
		}
	} else {
		model.CPU = nil
	}

	if res.Config.Memory != nil {
		wasHPNull := model.Memory == nil || model.Memory.PersistentHugepages.IsNull()
		wasZramNull := model.Memory == nil || model.Memory.ZramSwapSize.IsNull()
		model.Memory = &systemKernelMemory{
			PersistentHugepages: intValueOrNull(res.Config.Memory.PersistentHugepages, wasHPNull),
			ZramSwapSize:        stringValueOrNull(res.Config.Memory.ZramSwapSize, wasZramNull),
		}
	} else {
		model.Memory = nil
	}

	if res.Config.Network != nil {
		prev := model.Network
		model.Network = &systemKernelNetwork{
			BufferSize:             intValueOrNull(res.Config.Network.BufferSize, prev == nil || prev.BufferSize.IsNull()),
			NetdevMaxBacklog:       intValueOrNull(res.Config.Network.NetdevMaxBacklog, prev == nil || prev.NetdevMaxBacklog.IsNull()),
			QueuingDiscipline:      stringValueOrNull(res.Config.Network.QueuingDiscipline, prev == nil || prev.QueuingDiscipline.IsNull()),
			TCPCongestionAlgorithm: stringValueOrNull(res.Config.Network.TCPCongestionAlgorithm, prev == nil || prev.TCPCongestionAlgorithm.IsNull()),
			TCPMTUProbing:          boolValueOrNull(res.Config.Network.TCPMTUProbing, prev == nil || prev.TCPMTUProbing.IsNull()),
		}
	} else {
		model.Network = nil
	}

	if res.Config.PCI != nil {
		prev := model.PCI
		pci := &systemKernelPCI{}
		if len(res.Config.PCI.Passthrough) > 0 {
			for i, p := range res.Config.PCI.Passthrough {
				wasAddrNull := prev == nil || i >= len(prev.Passthrough) || prev.Passthrough[i].PCIAddress.IsNull()
				pci.Passthrough = append(pci.Passthrough, systemKernelPCIPassthrough{
					VendorID:   types.StringValue(p.VendorID),
					ProductID:  types.StringValue(p.ProductID),
					PCIAddress: stringValueOrNull(p.PCIAddress, wasAddrNull),
				})
			}
		}

		if len(res.Config.PCI.SRIOV) > 0 {
			for i, s := range res.Config.PCI.SRIOV {
				wasSwNull := prev == nil || i >= len(prev.SRIOV) || prev.SRIOV[i].Switchdev.IsNull()
				pci.SRIOV = append(pci.SRIOV, systemKernelPCISRIOV{
					PCIAddress: types.StringValue(s.PCIAddress),
					Switchdev:  boolValueOrNull(s.Switchdev, wasSwNull),
					VFCount:    types.Int64Value(int64(s.VFCount)),
				})
			}
		}

		model.PCI = pci
	} else {
		model.PCI = nil
	}

	if res.Config.ZFS != nil {
		wasArcNull := model.ZFS == nil || model.ZFS.ARCMaxSize.IsNull()
		model.ZFS = &systemKernelZFS{
			ARCMaxSize: stringValueOrNull(res.Config.ZFS.ARCMaxSize, wasArcNull),
		}
	} else {
		model.ZFS = nil
	}

	if res.State.Clock != nil {
		model.ClockSource = types.StringValue(res.State.Clock.Source)
	} else {
		model.ClockSource = types.StringValue("")
	}
}
