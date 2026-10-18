// Copyright (c) IBM Corp. 2014, 2026
// SPDX-License-Identifier: MPL-2.0

package tpgresource

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// TestListResourceMetadataRawV5Schemas checks that the memoized protocol
// schemas are the ones the SDK would have built, that a second call reuses the
// first result across both distinct *schema.Resource pointers sharing a
// TypeName and identical pointers when TypeName is empty, and that two
// different resources do not share an entry.
func TestListResourceMetadataRawV5Schemas(t *testing.T) {
	ctx := context.Background()

	firstA := testListSDKv2Resource("first_field")
	firstB := testListSDKv2Resource("first_field")
	second := testListSDKv2Resource("second_field")

	wantSchema := firstA.ProtoSchema(ctx)()
	wantIdentity := firstA.ProtoIdentitySchema(ctx)()

	firstResp := list.RawV5SchemaResponse{}
	(&ListResourceMetadata{
		TypeName:      "google_test_first",
		SDKv2Resource: firstA,
	}).RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &firstResp)

	if !reflect.DeepEqual(firstResp.ProtoV5Schema, wantSchema) {
		t.Errorf("ProtoV5Schema = %#v, want %#v", firstResp.ProtoV5Schema, wantSchema)
	}
	if !reflect.DeepEqual(firstResp.ProtoV5IdentitySchema, wantIdentity) {
		t.Errorf("ProtoV5IdentitySchema = %#v, want %#v", firstResp.ProtoV5IdentitySchema, wantIdentity)
	}

	// A second lookup with the same TypeName but a newly allocated
	// *schema.Resource pointer (as generated NewXListResource constructors
	// produce) must be served from the cache rather than rebuilt.
	cachedByTypeResp := list.RawV5SchemaResponse{}
	(&ListResourceMetadata{
		TypeName:      "google_test_first",
		SDKv2Resource: firstB,
	}).RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &cachedByTypeResp)

	if cachedByTypeResp.ProtoV5Schema != firstResp.ProtoV5Schema {
		t.Error("ProtoV5Schema was rebuilt for a second instance with the same TypeName, want the memoized value")
	}
	if cachedByTypeResp.ProtoV5IdentitySchema != firstResp.ProtoV5IdentitySchema {
		t.Error("ProtoV5IdentitySchema was rebuilt for a second instance with the same TypeName, want the memoized value")
	}

	// Fallback when TypeName is empty: cache by *schema.Resource pointer.
	unnamedFirst := list.RawV5SchemaResponse{}
	(&ListResourceMetadata{SDKv2Resource: firstA}).RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &unnamedFirst)
	unnamedCached := list.RawV5SchemaResponse{}
	(&ListResourceMetadata{SDKv2Resource: firstA}).RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &unnamedCached)
	if unnamedCached.ProtoV5Schema != unnamedFirst.ProtoV5Schema {
		t.Error("ProtoV5Schema was rebuilt when TypeName was empty, want the pointer-memoized value")
	}

	// A different resource must not pick up the first resource's entry.
	secondResp := list.RawV5SchemaResponse{}
	(&ListResourceMetadata{
		TypeName:      "google_test_second",
		SDKv2Resource: second,
	}).RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &secondResp)

	if !reflect.DeepEqual(secondResp.ProtoV5Schema, second.ProtoSchema(ctx)()) {
		t.Error("a second resource was served the first resource's schema")
	}
}

// TestListResourceMetadataRawV5Schemas_metadataProbe checks that the
// Metadata() -> RawV5Schemas() probe sequence used by
// fwserver.Server.ListResourceFuncs returns non-nil stub schemas without
// populating the cache with stubs, and that a subsequent direct RawV5Schemas()
// call returns the real protocol schemas.
func TestListResourceMetadataRawV5Schemas_metadataProbe(t *testing.T) {
	ctx := context.Background()
	sdkRes := testListSDKv2Resource("probed_field")
	wantSchema := sdkRes.ProtoSchema(ctx)()
	wantIdentity := sdkRes.ProtoIdentitySchema(ctx)()

	meta := &ListResourceMetadata{
		TypeName:      "google_test_probed",
		SDKv2Resource: sdkRes,
	}

	var metaResp resource.MetadataResponse
	meta.Metadata(ctx, resource.MetadataRequest{}, &metaResp)
	if metaResp.TypeName != "google_test_probed" {
		t.Fatalf("Metadata TypeName = %q, want %q", metaResp.TypeName, "google_test_probed")
	}

	var probeResp list.RawV5SchemaResponse
	meta.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &probeResp)
	if probeResp.ProtoV5Schema == nil || probeResp.ProtoV5IdentitySchema == nil {
		t.Fatalf("probe RawV5Schemas returned nil schema (%v) or identity (%v)", probeResp.ProtoV5Schema, probeResp.ProtoV5IdentitySchema)
	}
	if probeResp.ProtoV5Schema != stubProtoV5Schema {
		t.Errorf("probe RawV5Schemas did not return stubProtoV5Schema")
	}
	if probeResp.ProtoV5IdentitySchema != stubProtoV5IdentitySchema {
		t.Errorf("probe RawV5Schemas did not return stubProtoV5IdentitySchema")
	}

	// A subsequent RawV5Schemas call on the same instance (without a preceding
	// Metadata call) must build and return the full schema, not the stub.
	var realResp list.RawV5SchemaResponse
	meta.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &realResp)
	if !reflect.DeepEqual(realResp.ProtoV5Schema, wantSchema) {
		t.Errorf("subsequent RawV5Schemas ProtoV5Schema = %#v, want %#v", realResp.ProtoV5Schema, wantSchema)
	}
	if !reflect.DeepEqual(realResp.ProtoV5IdentitySchema, wantIdentity) {
		t.Errorf("subsequent RawV5Schemas ProtoV5IdentitySchema = %#v, want %#v", realResp.ProtoV5IdentitySchema, wantIdentity)
	}
}

// TestListResourceMetadataRawV5Schemas_lifecycleReset checks that intervening
// lifecycle calls (Configure, Defaults, ListResourceConfigSchema) after
// Metadata() clear the probe flag so a later RawV5Schemas() call on the same
// instance returns the full schema rather than the stub.
func TestListResourceMetadataRawV5Schemas_lifecycleReset(t *testing.T) {
	ctx := context.Background()
	sdkRes := testListSDKv2Resource("lifecycle_field")
	wantSchema := sdkRes.ProtoSchema(ctx)()
	wantIdentity := sdkRes.ProtoIdentitySchema(ctx)()

	cases := []struct {
		name string
		step func(*ListResourceMetadata)
	}{
		{
			name: "Configure",
			step: func(m *ListResourceMetadata) {
				var resp resource.ConfigureResponse
				m.Configure(ctx, resource.ConfigureRequest{}, &resp)
			},
		},
		{
			name: "Defaults",
			step: func(m *ListResourceMetadata) {
				var resp resource.ConfigureResponse
				m.Defaults(resource.ConfigureRequest{}, &resp)
			},
		},
		{
			name: "ListResourceConfigSchema",
			step: func(m *ListResourceMetadata) {
				var resp list.ListResourceSchemaResponse
				m.ListResourceConfigSchema(ctx, list.ListResourceSchemaRequest{}, &resp)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			meta := &ListResourceMetadata{
				TypeName:      "google_test_lifecycle_" + tc.name,
				SDKv2Resource: sdkRes,
				ListConfigFields: []ListConfigField{
					{Name: "project", Kind: ListConfigKindString, Optional: true},
				},
			}
			var metaResp resource.MetadataResponse
			meta.Metadata(ctx, resource.MetadataRequest{}, &metaResp)

			tc.step(meta)

			var rawResp list.RawV5SchemaResponse
			meta.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &rawResp)
			if rawResp.ProtoV5Schema == stubProtoV5Schema {
				t.Fatalf("RawV5Schemas after Metadata -> %s returned stubProtoV5Schema, want full schema", tc.name)
			}
			if !reflect.DeepEqual(rawResp.ProtoV5Schema, wantSchema) {
				t.Errorf("ProtoV5Schema = %#v, want %#v", rawResp.ProtoV5Schema, wantSchema)
			}
			if !reflect.DeepEqual(rawResp.ProtoV5IdentitySchema, wantIdentity) {
				t.Errorf("ProtoV5IdentitySchema = %#v, want %#v", rawResp.ProtoV5IdentitySchema, wantIdentity)
			}
		})
	}
}

// TestListResourceMetadataRawV5Schemas_nilSafety checks that nil SDKv2Resource
// or nil SDKv2Resource.Identity does not panic on either the post-Metadata
// probe path or a direct RawV5Schemas call.
func TestListResourceMetadataRawV5Schemas_nilSafety(t *testing.T) {
	ctx := context.Background()

	// 1. Nil SDKv2Resource (post-Metadata and direct).
	nilResMeta := &ListResourceMetadata{TypeName: "google_test_nil_resource"}
	var metaResp resource.MetadataResponse
	nilResMeta.Metadata(ctx, resource.MetadataRequest{}, &metaResp)

	var nilProbeResp list.RawV5SchemaResponse
	nilResMeta.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &nilProbeResp)
	if nilProbeResp.ProtoV5Schema != nil || nilProbeResp.ProtoV5IdentitySchema != nil {
		t.Errorf("nil SDKv2Resource probe returned non-nil schemas: %#v", nilProbeResp)
	}

	var nilDirectResp list.RawV5SchemaResponse
	nilResMeta.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &nilDirectResp)
	if nilDirectResp.ProtoV5Schema != nil || nilDirectResp.ProtoV5IdentitySchema != nil {
		t.Errorf("nil SDKv2Resource direct call returned non-nil schemas: %#v", nilDirectResp)
	}

	// 2. Non-nil SDKv2Resource with nil Identity (post-Metadata and direct).
	noIdentityRes := &schema.Resource{
		SchemaVersion: 1,
		Schema: map[string]*schema.Schema{
			"name": {Type: schema.TypeString, Required: true},
		},
	}
	noIdMeta := &ListResourceMetadata{
		TypeName:      "google_test_no_identity",
		SDKv2Resource: noIdentityRes,
	}
	noIdMeta.Metadata(ctx, resource.MetadataRequest{}, &metaResp)

	var noIdProbeResp list.RawV5SchemaResponse
	noIdMeta.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &noIdProbeResp)
	if noIdProbeResp.ProtoV5IdentitySchema != nil {
		t.Errorf("nil Identity probe returned non-nil ProtoV5IdentitySchema: %#v", noIdProbeResp.ProtoV5IdentitySchema)
	}

	var noIdDirectResp list.RawV5SchemaResponse
	noIdMeta.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &noIdDirectResp)
	if noIdDirectResp.ProtoV5Schema == nil {
		t.Error("nil Identity direct call returned nil ProtoV5Schema, want valid resource schema")
	}
	if noIdDirectResp.ProtoV5IdentitySchema != nil {
		t.Errorf("nil Identity direct call returned non-nil ProtoV5IdentitySchema: %#v", noIdDirectResp.ProtoV5IdentitySchema)
	}
}

func BenchmarkListResourceMetadataRawV5Schemas_MetadataProbe(b *testing.B) {
	ctx := context.Background()
	sdkRes := testListSDKv2Resource("bench_probe_field")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		meta := ListResourceMetadata{
			TypeName:      "google_bench_probe",
			SDKv2Resource: sdkRes,
		}
		var metaResp resource.MetadataResponse
		meta.Metadata(ctx, resource.MetadataRequest{}, &metaResp)
		var rawResp list.RawV5SchemaResponse
		meta.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &rawResp)
	}
}

func BenchmarkListResourceMetadataRawV5Schemas_Cached(b *testing.B) {
	ctx := context.Background()
	sdkRes := testListSDKv2Resource("bench_cached_field")
	meta := &ListResourceMetadata{
		TypeName:      "google_bench_cached",
		SDKv2Resource: sdkRes,
	}
	var warmResp list.RawV5SchemaResponse
	meta.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &warmResp)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var rawResp list.RawV5SchemaResponse
		meta.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &rawResp)
	}
}

func BenchmarkListResourceMetadataRawV5Schemas_Uncached(b *testing.B) {
	ctx := context.Background()
	sdkRes := testListSDKv2Resource("bench_uncached_field")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		typeName := fmt.Sprintf("google_bench_uncached_%d", i)
		meta := ListResourceMetadata{
			TypeName:      typeName,
			SDKv2Resource: sdkRes,
		}
		var rawResp list.RawV5SchemaResponse
		meta.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &rawResp)
		rawV5SchemaCache.Delete(typeName)
	}
}

// testListSDKv2Resource builds a minimal SDKv2 resource with an identity, which
// RawV5Schemas requires.
func testListSDKv2Resource(fieldName string) *schema.Resource {
	return &schema.Resource{
		SchemaVersion: 1,
		Schema: map[string]*schema.Schema{
			fieldName: {Type: schema.TypeString, Required: true},
		},
		Identity: &schema.ResourceIdentity{
			Version: 1,
			SchemaFunc: func() map[string]*schema.Schema {
				return map[string]*schema.Schema{
					"name": {Type: schema.TypeString, RequiredForImport: true},
				}
			},
		},
	}
}
