package fwresource

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
)

// FlattenOptionalBool converts a decoded API JSON value into a Plugin Framework
// types.Bool, returning types.BoolNull() when absent or nil.
func FlattenOptionalBool(v interface{}, diags *diag.Diagnostics) types.Bool {
	if v == nil {
		return types.BoolNull()
	}
	b, ok := v.(bool)
	if !ok {
		diags.AddError("Unexpected API value type for optional bool", fmt.Sprintf("Expected bool, got %T (%v)", v, v))
		return types.BoolNull()
	}
	return types.BoolValue(b)
}

// FlattenOptionalString converts a decoded API JSON value into a Plugin Framework
// types.String, returning types.StringNull() when absent or nil.
func FlattenOptionalString(v interface{}, diags *diag.Diagnostics) types.String {
	if v == nil {
		return types.StringNull()
	}
	s, ok := v.(string)
	if !ok {
		diags.AddError("Unexpected API value type for optional string", fmt.Sprintf("Expected string, got %T (%v)", v, v))
		return types.StringNull()
	}
	return types.StringValue(s)
}

// FlattenOptionalInt64 converts a decoded API JSON value (including fixed64 strings,
// json.Number, or float64 numbers) into a Plugin Framework types.Int64, returning
// types.Int64Null() when absent or nil.
func FlattenOptionalInt64(v interface{}, diags *diag.Diagnostics) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	switch val := v.(type) {
	case int64:
		return types.Int64Value(val)
	case int:
		return types.Int64Value(int64(val))
	case float64:
		return types.Int64Value(int64(val))
	case json.Number:
		if i, err := val.Int64(); err == nil {
			return types.Int64Value(i)
		} else if f, ferr := val.Float64(); ferr == nil {
			return types.Int64Value(int64(f))
		}
	case string:
		if i, err := tpgresource.StringToFixed64(val); err == nil {
			return types.Int64Value(i)
		}
	}
	diags.AddError("Unexpected API value type for optional int64", fmt.Sprintf("Expected integer or numeric string, got %T (%v)", v, v))
	return types.Int64Null()
}

// FlattenOptionalFloat64 converts a decoded API JSON value into a Plugin Framework
// types.Float64, returning types.Float64Null() when absent or nil.
func FlattenOptionalFloat64(v interface{}, diags *diag.Diagnostics) types.Float64 {
	if v == nil {
		return types.Float64Null()
	}
	switch val := v.(type) {
	case float64:
		return types.Float64Value(val)
	case int64:
		return types.Float64Value(float64(val))
	case int:
		return types.Float64Value(float64(val))
	case json.Number:
		if f, err := val.Float64(); err == nil {
			return types.Float64Value(f)
		}
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return types.Float64Value(f)
		}
	}
	diags.AddError("Unexpected API value type for optional float64", fmt.Sprintf("Expected float64, got %T (%v)", v, v))
	return types.Float64Null()
}
