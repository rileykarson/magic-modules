package tpgresource_test

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func testDeletionPolicyResource() *schema.Resource {
	return &schema.Resource{
		CustomizeDiff: tpgresource.DefaultProviderDeletionPolicy("DELETE"),
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"deletion_policy": tpgresource.DeletionPolicySchemaEntry("DELETE"),
		},
	}
}

func TestDefaultProviderDeletionPolicy(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	res := testDeletionPolicyResource()
	schemaBlock := res.CoreConfigSchema()
	impliedType := schemaBlock.ImpliedType()

	makeState := func(t *testing.T, prior cty.Value, rawCfg cty.Value) *terraform.InstanceState {
		t.Helper()
		st, err := res.ShimInstanceStateFromValue(prior)
		if err != nil {
			t.Fatalf("ShimInstanceStateFromValue failed: %v", err)
		}
		st.RawState = prior
		st.RawConfig = rawCfg
		st.RawPlan = rawCfg
		return st
	}

	t.Run("omitted deletion_policy falls back to resource default and provider default", func(t *testing.T) {
		t.Parallel()

		rawCfg := cty.ObjectVal(map[string]cty.Value{
			"id":              cty.NullVal(cty.String),
			"name":            cty.StringVal("example"),
			"deletion_policy": cty.NullVal(cty.String),
		})
		st := makeState(t, cty.NullVal(impliedType), rawCfg)
		cfg := terraform.NewResourceConfigShimmed(rawCfg, schemaBlock)

		diffDefault, err := res.SimpleDiff(ctx, st, cfg, &transport_tpg.Config{})
		if err != nil {
			t.Fatalf("SimpleDiff error: %v", err)
		}
		if got := diffDefault.Attributes["deletion_policy"]; got == nil || got.New != "DELETE" {
			t.Fatalf("expected deletion_policy New=DELETE, got %#v", got)
		}

		stProvider := makeState(t, cty.NullVal(impliedType), rawCfg)
		diffProvider, err := res.SimpleDiff(ctx, stProvider, cfg, &transport_tpg.Config{DeletionPolicy: "ABANDON"})
		if err != nil {
			t.Fatalf("SimpleDiff error: %v", err)
		}
		if got := diffProvider.Attributes["deletion_policy"]; got == nil || got.New != "ABANDON" {
			t.Fatalf("expected deletion_policy New=ABANDON, got %#v", got)
		}
	})

	t.Run("removing deletion_policy from config resets to default", func(t *testing.T) {
		t.Parallel()

		priorVal := cty.ObjectVal(map[string]cty.Value{
			"id":              cty.StringVal("id-1"),
			"name":            cty.StringVal("example"),
			"deletion_policy": cty.StringVal("PREVENT"),
		})
		rawCfg := cty.ObjectVal(map[string]cty.Value{
			"id":              cty.StringVal("id-1"),
			"name":            cty.StringVal("example"),
			"deletion_policy": cty.NullVal(cty.String),
		})
		st := makeState(t, priorVal, rawCfg)
		cfg := terraform.NewResourceConfigShimmed(rawCfg, schemaBlock)

		diff, err := res.SimpleDiff(ctx, st, cfg, &transport_tpg.Config{})
		if err != nil {
			t.Fatalf("SimpleDiff error: %v", err)
		}
		if got := diff.Attributes["deletion_policy"]; got == nil || got.Old != "PREVENT" || got.New != "DELETE" {
			t.Fatalf("expected deletion_policy PREVENT -> DELETE, got %#v", got)
		}
	})

	t.Run("explicit empty string deletion_policy returns error instead of being swallowed", func(t *testing.T) {
		t.Parallel()

		rawCfg := cty.ObjectVal(map[string]cty.Value{
			"id":              cty.NullVal(cty.String),
			"name":            cty.StringVal("example"),
			"deletion_policy": cty.StringVal(""),
		})
		stCreate := makeState(t, cty.NullVal(impliedType), rawCfg)
		cfg := terraform.NewResourceConfigShimmed(rawCfg, schemaBlock)

		if _, err := res.SimpleDiff(ctx, stCreate, cfg, &transport_tpg.Config{}); err == nil || !strings.Contains(err.Error(), "deletion_policy") {
			t.Fatalf("expected error on create with empty deletion_policy, got: %v", err)
		}

		priorVal := cty.ObjectVal(map[string]cty.Value{
			"id":              cty.StringVal("id-1"),
			"name":            cty.StringVal("example"),
			"deletion_policy": cty.StringVal("PREVENT"),
		})
		stUpdate := makeState(t, priorVal, rawCfg)
		if _, err := res.SimpleDiff(ctx, stUpdate, cfg, &transport_tpg.Config{}); err == nil || !strings.Contains(err.Error(), "deletion_policy") {
			t.Fatalf("expected error on update with empty deletion_policy, got: %v", err)
		}
	})

	t.Run("unknown deletion_policy at plan time marks NewComputed without error", func(t *testing.T) {
		t.Parallel()

		rawCfg := cty.ObjectVal(map[string]cty.Value{
			"id":              cty.StringVal("id-1"),
			"name":            cty.StringVal("example"),
			"deletion_policy": cty.UnknownVal(cty.String),
		})
		priorVal := cty.ObjectVal(map[string]cty.Value{
			"id":              cty.StringVal("id-1"),
			"name":            cty.StringVal("example"),
			"deletion_policy": cty.StringVal("PREVENT"),
		})
		st := makeState(t, priorVal, rawCfg)
		cfg := terraform.NewResourceConfigShimmed(rawCfg, schemaBlock)

		diff, err := res.SimpleDiff(ctx, st, cfg, &transport_tpg.Config{})
		if err != nil {
			t.Fatalf("unexpected error for unknown deletion_policy: %v", err)
		}
		if got := diff.Attributes["deletion_policy"]; got == nil || !got.NewComputed {
			t.Fatalf("expected deletion_policy NewComputed=true, got %#v", got)
		}
	})

	t.Run("nil meta and unset RawConfig in unit tests do not panic", func(t *testing.T) {
		t.Parallel()

		state := &terraform.InstanceState{
			ID: "id-1",
			Attributes: map[string]string{
				"id":   "id-1",
				"name": "example",
			},
		}
		cfg := terraform.NewResourceConfigRaw(map[string]interface{}{
			"name": "example-updated",
		})

		diff, err := res.SimpleDiff(ctx, state, cfg, nil)
		if err != nil {
			t.Fatalf("SimpleDiff with nil meta and nil RawConfig failed: %v", err)
		}
		if got := diff.Attributes["deletion_policy"]; got == nil || got.New != "DELETE" {
			t.Fatalf("expected deletion_policy New=DELETE with nil meta/RawConfig, got %#v", got)
		}
	})

	t.Run("marked sensitive deletion_policy cty.Value does not panic", func(t *testing.T) {
		t.Parallel()

		// 1. Valid marked sensitive string ("ABANDON")
		unmarkedCfg := cty.ObjectVal(map[string]cty.Value{
			"id":              cty.NullVal(cty.String),
			"name":            cty.StringVal("example"),
			"deletion_policy": cty.StringVal("ABANDON"),
		})
		markedRawCfg := cty.ObjectVal(map[string]cty.Value{
			"id":              cty.NullVal(cty.String),
			"name":            cty.StringVal("example"),
			"deletion_policy": cty.StringVal("ABANDON").Mark("sensitive"),
		})
		st := makeState(t, cty.NullVal(impliedType), markedRawCfg)
		cfg := terraform.NewResourceConfigShimmed(unmarkedCfg, schemaBlock)

		diff, err := res.SimpleDiff(ctx, st, cfg, &transport_tpg.Config{})
		if err != nil {
			t.Fatalf("unexpected error for marked sensitive deletion_policy: %v", err)
		}
		if got := diff.Attributes["deletion_policy"]; got == nil || got.New != "ABANDON" {
			t.Fatalf("expected deletion_policy New=ABANDON for marked value, got %#v", got)
		}

		// 2. Marked empty string ("") returns error cleanly without panicking
		unmarkedEmptyCfg := cty.ObjectVal(map[string]cty.Value{
			"id":              cty.NullVal(cty.String),
			"name":            cty.StringVal("example"),
			"deletion_policy": cty.StringVal(""),
		})
		markedEmptyRawCfg := cty.ObjectVal(map[string]cty.Value{
			"id":              cty.NullVal(cty.String),
			"name":            cty.StringVal("example"),
			"deletion_policy": cty.StringVal("").Mark("sensitive"),
		})
		stEmpty := makeState(t, cty.NullVal(impliedType), markedEmptyRawCfg)
		cfgEmpty := terraform.NewResourceConfigShimmed(unmarkedEmptyCfg, schemaBlock)

		if _, err := res.SimpleDiff(ctx, stEmpty, cfgEmpty, &transport_tpg.Config{}); err == nil || !strings.Contains(err.Error(), "deletion_policy") {
			t.Fatalf("expected error for marked empty deletion_policy, got: %v", err)
		}

		// 3. Marked unknown string marks NewComputed without panicking
		unmarkedUnknownCfg := cty.ObjectVal(map[string]cty.Value{
			"id":              cty.StringVal("id-1"),
			"name":            cty.StringVal("example"),
			"deletion_policy": cty.UnknownVal(cty.String),
		})
		markedUnknownRawCfg := cty.ObjectVal(map[string]cty.Value{
			"id":              cty.StringVal("id-1"),
			"name":            cty.StringVal("example"),
			"deletion_policy": cty.UnknownVal(cty.String).Mark("sensitive"),
		})
		priorVal := cty.ObjectVal(map[string]cty.Value{
			"id":              cty.StringVal("id-1"),
			"name":            cty.StringVal("example"),
			"deletion_policy": cty.StringVal("PREVENT"),
		})
		stUnknown := makeState(t, priorVal, markedUnknownRawCfg)
		cfgUnknown := terraform.NewResourceConfigShimmed(unmarkedUnknownCfg, schemaBlock)

		diffUnknown, err := res.SimpleDiff(ctx, stUnknown, cfgUnknown, &transport_tpg.Config{})
		if err != nil {
			t.Fatalf("unexpected error for marked unknown deletion_policy: %v", err)
		}
		if got := diffUnknown.Attributes["deletion_policy"]; got == nil || !got.NewComputed {
			t.Fatalf("expected deletion_policy NewComputed=true for marked unknown value, got %#v", got)
		}
	})
}

func TestDeletionPolicyPreDelete_NilSafe(t *testing.T) {
	t.Parallel()

	emptySchema := map[string]*schema.Schema{
		"name": {Type: schema.TypeString, Optional: true},
	}
	dMissing, err := schema.InternalMap(emptySchema).Data(&terraform.InstanceState{ID: "id-missing"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	skip, err := tpgresource.DeletionPolicyPreDelete(dMissing)
	if err != nil || skip {
		t.Fatalf("expected (false, nil) when deletion_policy is absent from schema, got (%v, %v)", skip, err)
	}

	resSchema := testDeletionPolicyResource().Schema
	dPrevent, _ := schema.InternalMap(resSchema).Data(&terraform.InstanceState{
		ID:         "id-prevent",
		Attributes: map[string]string{"deletion_policy": "PREVENT"},
	}, nil)
	if skip, err := tpgresource.DeletionPolicyPreDelete(dPrevent); !skip || err == nil {
		t.Fatalf("expected (true, error) for PREVENT, got (%v, %v)", skip, err)
	}

	dAbandon, _ := schema.InternalMap(resSchema).Data(&terraform.InstanceState{
		ID:         "id-abandon",
		Attributes: map[string]string{"deletion_policy": "ABANDON"},
	}, nil)
	if skip, err := tpgresource.DeletionPolicyPreDelete(dAbandon); !skip || err != nil {
		t.Fatalf("expected (true, nil) for ABANDON, got (%v, %v)", skip, err)
	}
}
