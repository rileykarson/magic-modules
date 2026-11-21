package tpgresource

import (
	"context"
	"reflect"
	"testing"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func testNullableSpecs() []NullableFieldSpec {
	return []NullableFieldSpec{
		{
			Segments: []NullablePathSegment{
				{TFName: "opt_bool", APIName: "optBool", Kind: NullableSegmentAttr},
			},
		},
		{
			Segments: []NullablePathSegment{
				{TFName: "imm_int", APIName: "immInt", Kind: NullableSegmentAttr},
			},
			ForceNew: true,
		},
		{
			Segments: []NullablePathSegment{
				{APIName: "wrapper", Kind: NullableSegmentFlattenObject},
				{TFName: "flat_str", APIName: "flatStr", Kind: NullableSegmentAttr},
			},
		},
		{
			Segments: []NullablePathSegment{
				{TFName: "config", APIName: "config", Kind: NullableSegmentNestedObject},
				{TFName: "opt_double", APIName: "optDouble", Kind: NullableSegmentAttr},
			},
		},
		{
			Segments: []NullablePathSegment{
				{TFName: "rules", APIName: "rules", Kind: NullableSegmentArray},
				{TFName: "opt_int", APIName: "optInt", Kind: NullableSegmentAttr},
			},
		},
		{
			Segments: []NullablePathSegment{
				{TFName: "envs", APIName: "envs", Kind: NullableSegmentMap, KeyName: "env_id"},
				{TFName: "enabled", APIName: "enabled", Kind: NullableSegmentAttr},
			},
		},
	}
}

func TestEvaluateAndExtractNullableFieldPresence(t *testing.T) {
	t.Parallel()
	specs := testNullableSpecs()

	cfgObj := cty.ObjectVal(map[string]cty.Value{
		"opt_bool": cty.False,
		"imm_int":  cty.NullVal(cty.Number),
		"flat_str": cty.StringVal(""),
		"config": cty.ListVal([]cty.Value{
			cty.ObjectVal(map[string]cty.Value{
				"opt_double": cty.NumberFloatVal(0.0),
			}),
		}),
		"rules": cty.ListVal([]cty.Value{
			cty.ObjectVal(map[string]cty.Value{
				"opt_int": cty.NullVal(cty.Number),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"opt_int": cty.Zero,
			}),
		}),
		"envs": cty.SetVal([]cty.Value{
			cty.ObjectVal(map[string]cty.Value{
				"env_id":  cty.StringVal("prod.us"),
				"enabled": cty.False,
			}),
			cty.ObjectVal(map[string]cty.Value{
				"env_id":  cty.StringVal("dev"),
				"enabled": cty.NullVal(cty.Bool),
			}),
		}),
	})

	gotCfg, hasUnknown, hasUnknownForceNew := EvaluateNullableConfigPresence(cfgObj, specs)
	if hasUnknown || hasUnknownForceNew {
		t.Fatalf("expected known config presence, got hasUnknown=%v hasUnknownForceNew=%v", hasUnknown, hasUnknownForceNew)
	}
	want := map[string]interface{}{
		"opt_bool":                true,
		"flat_str":                true,
		"config.0.opt_double":     true,
		"rules.1.opt_int":         true,
		`envs["prod.us"].enabled`: true,
	}
	if !reflect.DeepEqual(gotCfg, want) {
		t.Errorf("EvaluateNullableConfigPresence mismatch:\ngot:  %#v\nwant: %#v", gotCfg, want)
	}

	apiRes := map[string]interface{}{
		"optBool": false,
		"immInt":  nil,
		"wrapper": map[string]interface{}{
			"flatStr": "",
		},
		"config": map[string]interface{}{
			"optDouble": float64(0),
		},
		"rules": []interface{}{
			map[string]interface{}{}, // empty object skipped by flattenPropertyMethod
			map[string]interface{}{"other": "x"},
			map[string]interface{}{"optInt": float64(0)},
		},
		"envs": map[string]interface{}{
			"prod.us": map[string]interface{}{"enabled": false},
			"dev":     map[string]interface{}{},
		},
	}
	gotAPI := ExtractNullableFieldPresenceFromAPIResponse(apiRes, specs)
	if !reflect.DeepEqual(gotAPI, want) {
		t.Errorf("ExtractNullableFieldPresenceFromAPIResponse mismatch:\ngot:  %#v\nwant: %#v", gotAPI, want)
	}
}

func TestMatchNullableFieldKey(t *testing.T) {
	t.Parallel()
	specs := testNullableSpecs()

	if !MatchNullableFieldKey("imm_int", specs[1]) {
		t.Errorf("expected imm_int to match specs[1]")
	}
	if MatchNullableFieldKey("imm_int_extra", specs[1]) {
		t.Errorf("did not expect imm_int_extra to match specs[1]")
	}
	if !MatchNullableFieldKey("flat_str", specs[2]) {
		t.Errorf("expected flat_str to match flattened spec")
	}
	if !MatchNullableFieldKey("rules.12.opt_int", specs[4]) {
		t.Errorf("expected rules.12.opt_int to match array spec")
	}
	if !MatchNullableFieldKey(`envs["prod.\"us\""].enabled`, specs[5]) {
		t.Errorf("expected escaped map key to match map spec")
	}
	if MatchNullableFieldKey(`envs["prod"].other`, specs[5]) {
		t.Errorf("did not expect mismatched leaf to match map spec")
	}
}

func TestEvaluateNullableConfigPresenceUnknownForceNew(t *testing.T) {
	t.Parallel()
	specs := testNullableSpecs()

	cfgUnknownNonForceNew := cty.ObjectVal(map[string]cty.Value{
		"opt_bool": cty.UnknownVal(cty.Bool),
		"imm_int":  cty.NullVal(cty.Number),
	})
	_, hasUnk, hasUnkFN := EvaluateNullableConfigPresence(cfgUnknownNonForceNew, specs)
	if !hasUnk || hasUnkFN {
		t.Fatalf("expected hasUnknown=true, hasUnknownForceNew=false; got %v, %v", hasUnk, hasUnkFN)
	}

	cfgUnknownForceNew := cty.ObjectVal(map[string]cty.Value{
		"opt_bool": cty.False,
		"imm_int":  cty.UnknownVal(cty.Number),
	})
	_, hasUnk, hasUnkFN = EvaluateNullableConfigPresence(cfgUnknownForceNew, specs)
	if !hasUnk || !hasUnkFN {
		t.Fatalf("expected hasUnknown=true, hasUnknownForceNew=true; got %v, %v", hasUnk, hasUnkFN)
	}
}

// TestSDKv2PlanResourceChangeNullZeroTransitions verifies end-to-end through
// schema.GRPCProviderServer.PlanResourceChange that standalone transitions
// between null and zero (false, "", 0) produce real planned diffs instead of
// being swallowed by hcl2shim.ValuesSDKEquivalent, and trigger RequiresReplace
// when ForceNew is set.
func TestSDKv2PlanResourceChangeNullZeroTransitions(t *testing.T) {
	t.Parallel()

	specs := []NullableFieldSpec{
		{
			Segments: []NullablePathSegment{
				{TFName: "opt_bool", APIName: "optBool", Kind: NullableSegmentAttr},
			},
		},
		{
			Segments: []NullablePathSegment{
				{TFName: "opt_str", APIName: "optStr", Kind: NullableSegmentAttr},
			},
		},
		{
			Segments: []NullablePathSegment{
				{TFName: "imm_int", APIName: "immInt", Kind: NullableSegmentAttr},
			},
			ForceNew: true,
		},
		{
			Segments: []NullablePathSegment{
				{TFName: "config", APIName: "config", Kind: NullableSegmentNestedObject},
				{TFName: "nested_bool", APIName: "nestedBool", Kind: NullableSegmentAttr},
			},
		},
	}

	res := &schema.Resource{
		CustomizeDiff: SetNullableFieldPresenceDiff(specs),
		Create:        func(d *schema.ResourceData, _ interface{}) error { d.SetId("id-1"); return nil },
		Read:          func(d *schema.ResourceData, _ interface{}) error { return nil },
		Update:        func(d *schema.ResourceData, _ interface{}) error { return nil },
		Delete:        func(d *schema.ResourceData, _ interface{}) error { return nil },
		Schema: map[string]*schema.Schema{
			"opt_bool": {Type: schema.TypeBool, Optional: true},
			"opt_str":  {Type: schema.TypeString, Optional: true},
			"imm_int":  {Type: schema.TypeInt, Optional: true, ForceNew: true},
			"config": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"nested_bool": {Type: schema.TypeBool, Optional: true},
					},
				},
			},
			NullableFieldPresenceAttr: {
				Type:     schema.TypeMap,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeBool},
			},
		},
	}

	p := &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"google_test": res,
		},
	}
	srv := schema.NewGRPCProviderServer(p)

	configObjType := tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"nested_bool": tftypes.Bool,
		},
	}
	rootType := tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"id":                      tftypes.String,
			"opt_bool":                tftypes.Bool,
			"opt_str":                 tftypes.String,
			"imm_int":                 tftypes.Number,
			"config":                  tftypes.List{ElementType: configObjType},
			NullableFieldPresenceAttr: tftypes.Map{ElementType: tftypes.Bool},
		},
	}

	encodeVal := func(vals map[string]tftypes.Value) *tfprotov5.DynamicValue {
		dv, err := tfprotov5.NewDynamicValue(rootType, tftypes.NewValue(rootType, vals))
		if err != nil {
			t.Fatalf("NewDynamicValue failed: %v", err)
		}
		return &dv
	}

	decodePresence := func(dv *tfprotov5.DynamicValue) map[string]tftypes.Value {
		v, err := dv.Unmarshal(rootType)
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		var m map[string]tftypes.Value
		if err := v.As(&m); err != nil {
			t.Fatalf("As root map failed: %v", err)
		}
		var pm map[string]tftypes.Value
		if !m[NullableFieldPresenceAttr].IsNull() && m[NullableFieldPresenceAttr].IsKnown() {
			if err := m[NullableFieldPresenceAttr].As(&pm); err != nil {
				t.Fatalf("As presence map failed: %v", err)
			}
		}
		return pm
	}

	// 1. Standalone null -> false / "" transition on existing resource
	priorUnset := encodeVal(map[string]tftypes.Value{
		"id":                      tftypes.NewValue(tftypes.String, "id-1"),
		"opt_bool":                tftypes.NewValue(tftypes.Bool, false),
		"opt_str":                 tftypes.NewValue(tftypes.String, ""),
		"imm_int":                 tftypes.NewValue(tftypes.Number, 0),
		"config":                  tftypes.NewValue(tftypes.List{ElementType: configObjType}, []tftypes.Value{}),
		NullableFieldPresenceAttr: tftypes.NewValue(tftypes.Map{ElementType: tftypes.Bool}, map[string]tftypes.Value{}),
	})
	cfgZero := encodeVal(map[string]tftypes.Value{
		"id":       tftypes.NewValue(tftypes.String, nil),
		"opt_bool": tftypes.NewValue(tftypes.Bool, false),
		"opt_str":  tftypes.NewValue(tftypes.String, ""),
		"imm_int":  tftypes.NewValue(tftypes.Number, nil),
		"config": tftypes.NewValue(tftypes.List{ElementType: configObjType}, []tftypes.Value{
			tftypes.NewValue(configObjType, map[string]tftypes.Value{
				"nested_bool": tftypes.NewValue(tftypes.Bool, false),
			}),
		}),
		NullableFieldPresenceAttr: tftypes.NewValue(tftypes.Map{ElementType: tftypes.Bool}, nil),
	})
	proposedZero := encodeVal(map[string]tftypes.Value{
		"id":       tftypes.NewValue(tftypes.String, "id-1"),
		"opt_bool": tftypes.NewValue(tftypes.Bool, false),
		"opt_str":  tftypes.NewValue(tftypes.String, ""),
		"imm_int":  tftypes.NewValue(tftypes.Number, 0),
		"config": tftypes.NewValue(tftypes.List{ElementType: configObjType}, []tftypes.Value{
			tftypes.NewValue(configObjType, map[string]tftypes.Value{
				"nested_bool": tftypes.NewValue(tftypes.Bool, false),
			}),
		}),
		NullableFieldPresenceAttr: tftypes.NewValue(tftypes.Map{ElementType: tftypes.Bool}, map[string]tftypes.Value{}),
	})

	planResp, err := srv.PlanResourceChange(context.Background(), &tfprotov5.PlanResourceChangeRequest{
		TypeName:         "google_test",
		PriorState:       priorUnset,
		Config:           cfgZero,
		ProposedNewState: proposedZero,
	})
	if err != nil || len(planResp.Diagnostics) > 0 {
		t.Fatalf("PlanResourceChange(null -> zero) failed: err=%v diags=%v", err, planResp.Diagnostics)
	}
	plannedPresence := decodePresence(planResp.PlannedState)
	if len(plannedPresence) != 3 {
		t.Fatalf("expected 3 present keys in planned state (diff not swallowed), got: %#v", plannedPresence)
	}
	if len(planResp.RequiresReplace) != 0 {
		t.Errorf("expected no RequiresReplace when non-ForceNew fields changed, got: %v", planResp.RequiresReplace)
	}

	// 2. Standalone zero -> null transition (clearing explicit false / "" back to unset)
	priorZeroPresent := planResp.PlannedState
	cfgUnset := encodeVal(map[string]tftypes.Value{
		"id":                      tftypes.NewValue(tftypes.String, nil),
		"opt_bool":                tftypes.NewValue(tftypes.Bool, nil),
		"opt_str":                 tftypes.NewValue(tftypes.String, nil),
		"imm_int":                 tftypes.NewValue(tftypes.Number, nil),
		"config":                  tftypes.NewValue(tftypes.List{ElementType: configObjType}, []tftypes.Value{}),
		NullableFieldPresenceAttr: tftypes.NewValue(tftypes.Map{ElementType: tftypes.Bool}, nil),
	})
	proposedUnset := encodeVal(map[string]tftypes.Value{
		"id":                      tftypes.NewValue(tftypes.String, "id-1"),
		"opt_bool":                tftypes.NewValue(tftypes.Bool, false),
		"opt_str":                 tftypes.NewValue(tftypes.String, ""),
		"imm_int":                 tftypes.NewValue(tftypes.Number, 0),
		"config":                  tftypes.NewValue(tftypes.List{ElementType: configObjType}, []tftypes.Value{}),
		NullableFieldPresenceAttr: decodePresenceDynamic(priorZeroPresent, rootType, t),
	})
	clearResp, err := srv.PlanResourceChange(context.Background(), &tfprotov5.PlanResourceChangeRequest{
		TypeName:         "google_test",
		PriorState:       priorZeroPresent,
		Config:           cfgUnset,
		ProposedNewState: proposedUnset,
	})
	if err != nil || len(clearResp.Diagnostics) > 0 {
		t.Fatalf("PlanResourceChange(zero -> null) failed: err=%v diags=%v", err, clearResp.Diagnostics)
	}
	clearedPresence := decodePresence(clearResp.PlannedState)
	if len(clearedPresence) != 0 {
		t.Fatalf("expected 0 present keys after clearing zero -> null, got: %#v", clearedPresence)
	}

	// 3. ForceNew immutable OptionalInteger null -> 0 triggers RequiresReplace
	cfgImmZero := encodeVal(map[string]tftypes.Value{
		"id":                      tftypes.NewValue(tftypes.String, nil),
		"opt_bool":                tftypes.NewValue(tftypes.Bool, nil),
		"opt_str":                 tftypes.NewValue(tftypes.String, nil),
		"imm_int":                 tftypes.NewValue(tftypes.Number, 0),
		"config":                  tftypes.NewValue(tftypes.List{ElementType: configObjType}, []tftypes.Value{}),
		NullableFieldPresenceAttr: tftypes.NewValue(tftypes.Map{ElementType: tftypes.Bool}, nil),
	})
	fnResp, err := srv.PlanResourceChange(context.Background(), &tfprotov5.PlanResourceChangeRequest{
		TypeName:         "google_test",
		PriorState:       priorUnset,
		Config:           cfgImmZero,
		ProposedNewState: priorUnset,
	})
	if err != nil || len(fnResp.Diagnostics) > 0 {
		t.Fatalf("PlanResourceChange(immutable null -> 0) failed: err=%v diags=%v", err, fnResp.Diagnostics)
	}
	if len(fnResp.RequiresReplace) == 0 {
		t.Fatalf("expected RequiresReplace when ForceNew OptionalInteger changed null -> 0")
	}
}

func decodePresenceDynamic(dv *tfprotov5.DynamicValue, rootType tftypes.Type, t *testing.T) tftypes.Value {
	t.Helper()
	v, err := dv.Unmarshal(rootType)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	var m map[string]tftypes.Value
	if err := v.As(&m); err != nil {
		t.Fatalf("As map failed: %v", err)
	}
	return m[NullableFieldPresenceAttr]
}

func TestNullableResourceDataHelpers(t *testing.T) {
	t.Parallel()
	specs := testNullableSpecs()

	r := &schema.Resource{
		Schema: map[string]*schema.Schema{
			"opt_bool": {Type: schema.TypeBool, Optional: true},
			"config": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"opt_double": {Type: schema.TypeFloat, Optional: true},
					},
				},
			},
			NullableFieldPresenceAttr: {
				Type:     schema.TypeMap,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeBool},
			},
		},
	}

	// Simulate an existing resource transitioning opt_bool: false -> unset,
	// and config.0.opt_double: 0.0 -> unset.
	rd := r.Data(nil)
	rd.SetId("existing-1")
	if err := rd.Set(NullableFieldPresenceAttr, map[string]interface{}{
		"opt_bool":            true,
		"config.0.opt_double": true,
	}); err != nil {
		t.Fatalf("rd.Set prior failed: %v", err)
	}
	// Mark state clean as prior state, then simulate planned update to empty presence:
	// On a real ResourceData without RawConfig (e.g. TGC / unit tests), CurrentNullableFieldPresence
	// reads d.Get(NullableFieldPresenceAttr).
	if !IsNullableFieldPresent(rd, "opt_bool", specs) {
		t.Errorf("expected IsNullableFieldPresent(opt_bool) == true via fallback")
	}

	// Verify GetChange / WasNullableFieldPresent / HasNullableFieldPresenceChange via TerraformResourceDataChange
	diffData := &mockResourceDataChange{
		ResourceData: rd,
		oldPresence: map[string]interface{}{
			"opt_bool":                true,
			"config.0.opt_double":     true,
			`envs["prod.us"].enabled`: true,
		},
		newPresence: map[string]interface{}{},
	}
	if !WasNullableFieldPresent(diffData, "opt_bool") {
		t.Errorf("expected WasNullableFieldPresent(opt_bool) == true")
	}
	if !WasNullableFieldPresentUnderPrefix(diffData, "config") {
		t.Errorf("expected WasNullableFieldPresentUnderPrefix(config) == true")
	}
	if !WasNullableFieldPresentUnderPrefix(diffData, "envs") {
		t.Errorf("expected WasNullableFieldPresentUnderPrefix(envs) == true")
	}
	if WasNullableFieldPresentUnderPrefix(diffData, "rules") {
		t.Errorf("expected WasNullableFieldPresentUnderPrefix(rules) == false")
	}
	if !HasNullableFieldPresenceChange(diffData, "opt_bool", specs) {
		t.Errorf("expected HasNullableFieldPresenceChange(opt_bool) == true")
	}
	if !HasNullableFieldPresenceChange(diffData, "config", specs) {
		t.Errorf("expected HasNullableFieldPresenceChange(config) == true")
	}
	if HasNullableFieldPresenceChange(diffData, "rules", specs) {
		t.Errorf("expected HasNullableFieldPresenceChange(rules) == false")
	}
}

type mockResourceDataChange struct {
	*schema.ResourceData
	oldPresence map[string]interface{}
	newPresence map[string]interface{}
}

func (m *mockResourceDataChange) Get(key string) interface{} {
	if key == NullableFieldPresenceAttr {
		return m.newPresence
	}
	return m.ResourceData.Get(key)
}

func (m *mockResourceDataChange) GetChange(key string) (interface{}, interface{}) {
	if key == NullableFieldPresenceAttr {
		return m.oldPresence, m.newPresence
	}
	return m.ResourceData.GetChange(key)
}

func (m *mockResourceDataChange) GetRawConfig() cty.Value {
	return cty.NilVal
}
