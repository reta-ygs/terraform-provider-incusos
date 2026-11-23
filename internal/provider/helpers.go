package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/reta-ygs/terraform-provider-incusos/internal/client"
)

func extractClient(providerData any, diags *diag.Diagnostics) *client.Client {
	if providerData == nil {
		return nil
	}

	c, ok := providerData.(*client.Client)
	if !ok {
		diags.AddError(
			"Unexpected Provider Data Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", providerData),
		)

		return nil
	}

	return c
}

func listToStringSlice(ctx context.Context, list types.List, diags *diag.Diagnostics) []string {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}

	var out []string
	diags.Append(list.ElementsAs(ctx, &out, false)...)

	return out
}

func stringSliceToList(ctx context.Context, in []string, preserveNull bool, diags *diag.Diagnostics) types.List {
	if len(in) == 0 && preserveNull {
		return types.ListNull(types.StringType)
	}

	if in == nil {
		in = []string{}
	}

	list, d := types.ListValueFrom(ctx, types.StringType, in)
	diags.Append(d...)

	return list
}

func listToIntSlice(ctx context.Context, list types.List, diags *diag.Diagnostics) []int {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}

	var raw []int64
	diags.Append(list.ElementsAs(ctx, &raw, false)...)

	out := make([]int, len(raw))
	for i, v := range raw {
		out[i] = int(v)
	}

	return out
}

func intSliceToList(ctx context.Context, in []int, preserveNull bool, diags *diag.Diagnostics) types.List {
	if len(in) == 0 && preserveNull {
		return types.ListNull(types.Int64Type)
	}

	raw := make([]int64, len(in))
	for i, v := range in {
		raw[i] = int64(v)
	}

	list, d := types.ListValueFrom(ctx, types.Int64Type, raw)
	diags.Append(d...)

	return list
}

func mapToStringMap(ctx context.Context, m types.Map, diags *diag.Diagnostics) map[string]string {
	if m.IsNull() || m.IsUnknown() {
		return nil
	}

	var out map[string]string
	diags.Append(m.ElementsAs(ctx, &out, false)...)

	return out
}

func stringMapToMap(ctx context.Context, in map[string]string, preserveNull bool, diags *diag.Diagnostics) types.Map {
	if len(in) == 0 && preserveNull {
		return types.MapNull(types.StringType)
	}

	if in == nil {
		in = map[string]string{}
	}

	m, d := types.MapValueFrom(ctx, types.StringType, in)
	diags.Append(d...)

	return m
}

func stringValueOrNull(s string, wasNull bool) types.String {
	if s == "" && wasNull {
		return types.StringNull()
	}

	return types.StringValue(s)
}

func intValueOrNull(v int, wasNull bool) types.Int64 {
	if v == 0 && wasNull {
		return types.Int64Null()
	}

	return types.Int64Value(int64(v))
}

func boolValueOrNull(v bool, wasNull bool) types.Bool {
	if !v && wasNull {
		return types.BoolNull()
	}

	return types.BoolValue(v)
}
