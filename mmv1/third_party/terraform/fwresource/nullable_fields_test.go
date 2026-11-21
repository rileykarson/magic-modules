package fwresource

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

func TestFlattenOptionalPrimitives(t *testing.T) {
	t.Parallel()

	var diags diag.Diagnostics
	if got := FlattenOptionalBool(nil, &diags); !got.IsNull() {
		t.Errorf("FlattenOptionalBool(nil) = %v, want null", got)
	}
	if got := FlattenOptionalBool(false, &diags); got.IsNull() || got.ValueBool() != false {
		t.Errorf("FlattenOptionalBool(false) = %v, want false", got)
	}
	if got := FlattenOptionalString(nil, &diags); !got.IsNull() {
		t.Errorf("FlattenOptionalString(nil) = %v, want null", got)
	}
	if got := FlattenOptionalString("", &diags); got.IsNull() || got.ValueString() != "" {
		t.Errorf("FlattenOptionalString(\"\") = %v, want \"\"", got)
	}
	if got := FlattenOptionalInt64(nil, &diags); !got.IsNull() {
		t.Errorf("FlattenOptionalInt64(nil) = %v, want null", got)
	}
	if got := FlattenOptionalInt64(float64(0), &diags); got.IsNull() || got.ValueInt64() != 0 {
		t.Errorf("FlattenOptionalInt64(0) = %v, want 0", got)
	}
	if got := FlattenOptionalInt64("9007199254740993", &diags); got.IsNull() || got.ValueInt64() != 9007199254740993 {
		t.Errorf("FlattenOptionalInt64(fixed64 string) = %v, want 9007199254740993", got)
	}
	if got := FlattenOptionalFloat64(json.Number("0.0"), &diags); got.IsNull() || got.ValueFloat64() != 0.0 {
		t.Errorf("FlattenOptionalFloat64(0.0) = %v, want 0.0", got)
	}
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}
