package tpgresource_test

import (
	"testing"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
)

type testSensitivityMark string

func testRawConfigCtyObject() cty.Value {
	cdnElemType := cty.Object(map[string]cty.Type{
		"default_ttl": cty.Number,
		"enabled":     cty.Bool,
		"secret_wo":   cty.String,
	})
	emptyClusterElemType := cty.Object(map[string]cty.Type{
		"secondary_worker": cty.List(cty.Object(map[string]cty.Type{
			"core_count": cty.Number,
		})),
	})

	return cty.ObjectVal(map[string]cty.Value{
		"project":           cty.StringVal("my-project"),
		"empty_str":         cty.StringVal(""),
		"zero_bool":         cty.False,
		"null_str":          cty.NullVal(cty.String),
		"unknown_str":       cty.UnknownVal(cty.String),
		"marked_secret_wo":  cty.StringVal("top-secret").Mark(testSensitivityMark("sensitive")),
		"marked_unknown":    cty.UnknownVal(cty.String).Mark(testSensitivityMark("sensitive")),
		"labels":            cty.MapVal(map[string]cty.Value{"env": cty.StringVal("prod")}),
		"unknown_map":       cty.UnknownVal(cty.Map(cty.String)),
		"tags_set":          cty.SetVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
		"empty_set":         cty.SetValEmpty(cty.String),
		"null_list":         cty.NullVal(cty.List(cty.String)),
		"unknown_list":      cty.UnknownVal(cty.List(cdnElemType)),
		"empty_cluster_cfg": cty.ListValEmpty(emptyClusterElemType),
		"cdn_policy": cty.ListVal([]cty.Value{
			cty.ObjectVal(map[string]cty.Value{
				"default_ttl": cty.NumberIntVal(0),
				"enabled":     cty.False.Mark(testSensitivityMark("sensitive")),
				"secret_wo":   cty.StringVal("nested-secret").Mark(testSensitivityMark("wo")),
			}),
		}).Mark(testSensitivityMark("block")),
		"tuple_block": cty.TupleVal([]cty.Value{
			cty.ObjectVal(map[string]cty.Value{
				"flag": cty.True,
			}),
		}),
	})
}

func TestGetRawConfigAtKey_UnavailableAndNilReaders(t *testing.T) {
	t.Parallel()

	var typedNilRD *schema.ResourceData
	var typedNilDiff *schema.ResourceDiff

	emptySchema := map[string]*schema.Schema{
		"project": {Type: schema.TypeString, Optional: true},
	}
	testRD := schema.TestResourceDataRaw(t, emptySchema, map[string]interface{}{})

	readers := map[string]tpgresource.RawConfigReader{
		"untyped nil":            nil,
		"typed nil ResourceData": typedNilRD,
		"typed nil ResourceDiff": typedNilDiff,
		"ResourceDataMock unset": &tpgresource.ResourceDataMock{},
		"ResourceDiffMock unset": &tpgresource.ResourceDiffMock{},
		"TestResourceDataRaw":    testRD,
	}

	for name, r := range readers {
		t.Run(name, func(t *testing.T) {
			if val, ok := tpgresource.GetRawConfigAtKey(r, "project"); ok || val != cty.NilVal {
				t.Fatalf("GetRawConfigAtKey(%s) = (%#v, %v), want (cty.NilVal, false)", name, val, ok)
			}
			if elems, ok := tpgresource.GetRawConfigSlice(r, "project"); ok || elems != nil {
				t.Fatalf("GetRawConfigSlice(%s) = (%#v, %v), want (nil, false)", name, elems, ok)
			}
			if got := tpgresource.GetRawConfigAttributeAsString(r, "project"); got != "" {
				t.Fatalf("GetRawConfigAttributeAsString(%s) = %q, want \"\"", name, got)
			}
		})
	}
}

func TestGetRawConfigAtKey_LookupSemantics(t *testing.T) {
	t.Parallel()

	mock := &tpgresource.ResourceDataMock{
		RawConfig: testRawConfigCtyObject(),
	}

	t.Run("root empty key returns unmarked root object", func(t *testing.T) {
		val, ok := tpgresource.GetRawConfigAtKey(mock, "")
		if !ok || val.IsNull() || !val.IsKnown() {
			t.Fatalf("expected valid known root object, got (%#v, %v)", val, ok)
		}
		if val.ContainsMarked() {
			t.Fatalf("expected unmarked root object, but ContainsMarked() is true")
		}
	})

	t.Run("known zero values and nested paths", func(t *testing.T) {
		val, ok := tpgresource.GetRawConfigAtKey(mock, "empty_str")
		if !ok || val.IsNull() || !val.IsKnown() || val.AsString() != "" {
			t.Fatalf("empty_str: got (%#v, %v)", val, ok)
		}

		val, ok = tpgresource.GetRawConfigAtKey(mock, "zero_bool")
		if !ok || val.IsNull() || !val.IsKnown() || !val.False() {
			t.Fatalf("zero_bool: got (%#v, %v)", val, ok)
		}

		val, ok = tpgresource.GetRawConfigAtKey(mock, "cdn_policy.0.default_ttl")
		if !ok || val.IsNull() || !val.IsKnown() || !val.RawEquals(cty.NumberIntVal(0)) {
			t.Fatalf("cdn_policy.0.default_ttl: got (%#v, %v)", val, ok)
		}

		val, ok = tpgresource.GetRawConfigAtKey(mock, "cdn_policy.0.enabled")
		if !ok || val.IsNull() || !val.IsKnown() || val.IsMarked() || !val.False() {
			t.Fatalf("marked bool cdn_policy.0.enabled: got (%#v, %v)", val, ok)
		}

		val, ok = tpgresource.GetRawConfigAtKey(mock, "labels.env")
		if !ok || val.IsNull() || !val.IsKnown() || val.AsString() != "prod" {
			t.Fatalf("map key labels.env: got (%#v, %v)", val, ok)
		}

		val, ok = tpgresource.GetRawConfigAtKey(mock, "tuple_block.0.flag")
		if !ok || val.IsNull() || !val.IsKnown() || !val.True() {
			t.Fatalf("tuple key tuple_block.0.flag: got (%#v, %v)", val, ok)
		}
	})

	t.Run("null leaf, omitted parent blocks, and partial objects return null with ok=true", func(t *testing.T) {
		for _, tc := range []struct {
			key      string
			wantType cty.Type
		}{
			{"null_str", cty.String},
			{"cdn_policy.5.default_ttl", cty.Number},
			{"empty_cluster_cfg.0.secondary_worker.0.core_count", cty.Number},
			{"labels.missing_key", cty.String},
			{"omitted_in_partial_object", cty.DynamicPseudoType},
			{"tuple_block.0.omitted_nested_block.0.field", cty.DynamicPseudoType},
			{"tuple_block.1.flag", cty.DynamicPseudoType},
		} {
			val, ok := tpgresource.GetRawConfigAtKey(mock, tc.key)
			if !ok {
				t.Fatalf("%s: expected ok=true, got false", tc.key)
			}
			if !val.IsNull() || !val.IsKnown() {
				t.Fatalf("%s: expected known null value, got %#v", tc.key, val)
			}
			if !val.Type().Equals(tc.wantType) {
				t.Fatalf("%s: got type %s, want %s", tc.key, val.Type().FriendlyName(), tc.wantType.FriendlyName())
			}
		}
	})

	t.Run("unknown leaf and unknown enclosing collections return typed unknown with ok=true", func(t *testing.T) {
		for _, tc := range []struct {
			key      string
			wantType cty.Type
		}{
			{"unknown_str", cty.String},
			{"marked_unknown", cty.String},
			{"unknown_list.0.default_ttl", cty.Number},
			{"unknown_map.any_key", cty.String},
		} {
			val, ok := tpgresource.GetRawConfigAtKey(mock, tc.key)
			if !ok {
				t.Fatalf("%s: expected ok=true, got false", tc.key)
			}
			if val.IsNull() || val.IsKnown() || val.IsMarked() {
				t.Fatalf("%s: expected unmarked unknown value, got %#v", tc.key, val)
			}
			if !val.Type().Equals(tc.wantType) {
				t.Fatalf("%s: got type %s, want %s", tc.key, val.Type().FriendlyName(), tc.wantType.FriendlyName())
			}
		}
	})

	t.Run("structurally invalid paths return ok=false", func(t *testing.T) {
		invalidKeys := []string{
			"cdn_policy.-1.default_ttl",
			"cdn_policy.not_an_int.default_ttl",
			"tags_set.0",
			"project.sub",
			"cdn_policy..0",
			"cdn_policy.",
			".project",
		}
		for _, key := range invalidKeys {
			if val, ok := tpgresource.GetRawConfigAtKey(mock, key); ok || val != cty.NilVal {
				t.Fatalf("GetRawConfigAtKey(%q) = (%#v, %v), want (cty.NilVal, false)", key, val, ok)
			}
		}
	})
}

func TestGetRawConfigSlice(t *testing.T) {
	t.Parallel()

	mock := &tpgresource.ResourceDataMock{
		RawConfig: testRawConfigCtyObject(),
	}

	t.Run("non-empty list, set, and tuple return unmarked elements", func(t *testing.T) {
		elems, ok := tpgresource.GetRawConfigSlice(mock, "cdn_policy")
		if !ok || len(elems) != 1 {
			t.Fatalf("cdn_policy: got (len=%d, ok=%v), want (1, true)", len(elems), ok)
		}
		if elems[0].ContainsMarked() {
			t.Fatalf("cdn_policy[0] still contains marks")
		}
		if sec, ok := tpgresource.GetCtyValueAtKey(elems[0], "secret_wo"); !ok || sec.AsString() != "nested-secret" {
			t.Fatalf("GetCtyValueAtKey on slice element failed: (%#v, %v)", sec, ok)
		}

		setElems, ok := tpgresource.GetRawConfigSlice(mock, "tags_set")
		if !ok || len(setElems) != 2 {
			t.Fatalf("tags_set: got (len=%d, ok=%v), want (2, true)", len(setElems), ok)
		}

		tupleElems, ok := tpgresource.GetRawConfigSlice(mock, "tuple_block")
		if !ok || len(tupleElems) != 1 {
			t.Fatalf("tuple_block: got (len=%d, ok=%v), want (1, true)", len(tupleElems), ok)
		}
	})

	t.Run("null, empty, and omitted-parent collections return (nil, true)", func(t *testing.T) {
		for _, key := range []string{
			"null_list",
			"empty_set",
			"empty_cluster_cfg",
			"empty_cluster_cfg.0.secondary_worker",
			"tuple_block.0.omitted_collection_in_partial_object",
		} {
			elems, ok := tpgresource.GetRawConfigSlice(mock, key)
			if !ok || len(elems) != 0 {
				t.Fatalf("GetRawConfigSlice(%q) = (%#v, %v), want (empty, true)", key, elems, ok)
			}
		}
	})

	t.Run("unknown collection, non-collection, and structurally invalid key return (nil, false)", func(t *testing.T) {
		for _, key := range []string{
			"unknown_list",
			"project",
			"null_str",
			"labels",
			"project.sub",
		} {
			elems, ok := tpgresource.GetRawConfigSlice(mock, key)
			if ok || elems != nil {
				t.Fatalf("GetRawConfigSlice(%q) = (%#v, %v), want (nil, false)", key, elems, ok)
			}
		}
	})
}

func TestGetRawConfigAttributeAsString(t *testing.T) {
	t.Parallel()

	rawCfg := testRawConfigCtyObject()
	dataMock := &tpgresource.ResourceDataMock{RawConfig: rawCfg}
	diffMock := &tpgresource.ResourceDiffMock{RawConfig: rawCfg}

	for name, reader := range map[string]tpgresource.RawConfigReader{
		"ResourceDataMock": dataMock,
		"ResourceDiffMock": diffMock,
	} {
		t.Run(name, func(t *testing.T) {
			if got := tpgresource.GetRawConfigAttributeAsString(reader, "project"); got != "my-project" {
				t.Fatalf("project: got %q, want %q", got, "my-project")
			}
			if got := tpgresource.GetRawConfigAttributeAsString(reader, "marked_secret_wo"); got != "top-secret" {
				t.Fatalf("marked_secret_wo: got %q, want %q", got, "top-secret")
			}
			if got := tpgresource.GetRawConfigAttributeAsString(reader, "cdn_policy.0.secret_wo"); got != "nested-secret" {
				t.Fatalf("cdn_policy.0.secret_wo: got %q, want %q", got, "nested-secret")
			}
			for _, emptyCase := range []string{
				"null_str",
				"unknown_str",
				"marked_unknown",
				"zero_bool",
				"cdn_policy.3.secret_wo",
				"nonexistent",
			} {
				if got := tpgresource.GetRawConfigAttributeAsString(reader, emptyCase); got != "" {
					t.Fatalf("%s: got %q, want \"\"", emptyCase, got)
				}
			}
		})
	}
}
