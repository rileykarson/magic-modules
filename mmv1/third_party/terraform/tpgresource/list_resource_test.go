// Copyright (c) IBM Corp. 2014, 2026
// SPDX-License-Identifier: MPL-2.0

package tpgresource

import (
	"context"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Server simulates fwserver.Server so ListResourceFuncs exercises the
// (*Server).ListResourceFuncs stack-frame check in RawV5Schemas under both
// current and future versions of terraform-plugin-framework.
type Server struct{}

//go:noinline
func (*Server) ListResourceFuncs(ctx context.Context, r list.ListResourceWithRawV5Schemas) (resource.MetadataResponse, list.RawV5SchemaResponse) {
	var metaResp resource.MetadataResponse
	r.Metadata(ctx, resource.MetadataRequest{}, &metaResp)
	var schemaResp list.RawV5SchemaResponse
	r.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &schemaResp)
	return metaResp, schemaResp
}

func TestListResourceMetadataRawV5Schemas(t *testing.T) {
	ctx := context.Background()
	sdkRes := &schema.Resource{
		SchemaVersion: 1,
		Schema: map[string]*schema.Schema{
			"name": {Type: schema.TypeString, Required: true},
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
	wantSchema := sdkRes.ProtoSchema(ctx)()
	wantIdentity := sdkRes.ProtoIdentitySchema(ctx)()
	srv := &Server{}

	// 1. Metadata() -> RawV5Schemas() called from (*Server).ListResourceFuncs
	// returns non-nil stub schemas without converting the full SDKv2 schema.
	meta := &ListResourceMetadata{
		TypeName:      "google_test_resource",
		SDKv2Resource: sdkRes,
	}
	metaResp, probeResp := srv.ListResourceFuncs(ctx, meta)
	if metaResp.TypeName != "google_test_resource" {
		t.Fatalf("Metadata TypeName = %q, want %q", metaResp.TypeName, "google_test_resource")
	}
	if probeResp.ProtoV5Schema != stubProtoV5Schema || probeResp.ProtoV5IdentitySchema != stubProtoV5IdentitySchema {
		t.Fatalf("probe RawV5Schemas = (%#v, %#v), want stub schemas", probeResp.ProtoV5Schema, probeResp.ProtoV5IdentitySchema)
	}

	// 2. Every other call pattern — subsequent RawV5Schemas() after probe, direct
	// RawV5Schemas() on a fresh instance, Metadata() -> RawV5Schemas() from a
	// non-ListResourceFuncs caller, or Metadata() -> Configure/Defaults/ListResourceConfigSchema -> RawV5Schemas()
	// — returns the full ProtoV5Schema and ProtoV5IdentitySchema.
	t.Run("after_probe", func(t *testing.T) {
		var resp list.RawV5SchemaResponse
		meta.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &resp)
		if !reflect.DeepEqual(resp.ProtoV5Schema, wantSchema) {
			t.Errorf("ProtoV5Schema = %#v, want %#v", resp.ProtoV5Schema, wantSchema)
		}
		if !reflect.DeepEqual(resp.ProtoV5IdentitySchema, wantIdentity) {
			t.Errorf("ProtoV5IdentitySchema = %#v, want %#v", resp.ProtoV5IdentitySchema, wantIdentity)
		}
	})

	t.Run("direct", func(t *testing.T) {
		inst := &ListResourceMetadata{TypeName: "google_test_resource", SDKv2Resource: sdkRes}
		var resp list.RawV5SchemaResponse
		inst.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &resp)
		if !reflect.DeepEqual(resp.ProtoV5Schema, wantSchema) {
			t.Errorf("ProtoV5Schema = %#v, want %#v", resp.ProtoV5Schema, wantSchema)
		}
		if !reflect.DeepEqual(resp.ProtoV5IdentitySchema, wantIdentity) {
			t.Errorf("ProtoV5IdentitySchema = %#v, want %#v", resp.ProtoV5IdentitySchema, wantIdentity)
		}
	})

	t.Run("metadata_then_raw_outside_listresourcefuncs", func(t *testing.T) {
		inst := &ListResourceMetadata{TypeName: "google_test_resource", SDKv2Resource: sdkRes}
		var mr resource.MetadataResponse
		inst.Metadata(ctx, resource.MetadataRequest{}, &mr)
		var resp list.RawV5SchemaResponse
		inst.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &resp)
		if !reflect.DeepEqual(resp.ProtoV5Schema, wantSchema) {
			t.Errorf("ProtoV5Schema = %#v, want %#v", resp.ProtoV5Schema, wantSchema)
		}
		if !reflect.DeepEqual(resp.ProtoV5IdentitySchema, wantIdentity) {
			t.Errorf("ProtoV5IdentitySchema = %#v, want %#v", resp.ProtoV5IdentitySchema, wantIdentity)
		}
	})

	t.Run("lifecycle_resets_metadata_called", func(t *testing.T) {
		for _, hook := range []struct {
			name string
			fn   func(*ListResourceMetadata)
		}{
			{
				name: "Configure",
				fn: func(m *ListResourceMetadata) {
					var cr resource.ConfigureResponse
					m.Configure(ctx, resource.ConfigureRequest{}, &cr)
				},
			},
			{
				name: "Defaults",
				fn: func(m *ListResourceMetadata) {
					var cr resource.ConfigureResponse
					m.Defaults(resource.ConfigureRequest{}, &cr)
				},
			},
			{
				name: "ListResourceConfigSchema",
				fn: func(m *ListResourceMetadata) {
					var sr list.ListResourceSchemaResponse
					m.ListResourceConfigSchema(ctx, list.ListResourceSchemaRequest{}, &sr)
				},
			},
		} {
			t.Run(hook.name, func(t *testing.T) {
				inst := &ListResourceMetadata{TypeName: "google_test_resource", SDKv2Resource: sdkRes}
				var mr resource.MetadataResponse
				inst.Metadata(ctx, resource.MetadataRequest{}, &mr)
				hook.fn(inst)
				if inst.metadataCalled {
					t.Fatalf("metadataCalled remained true after %s", hook.name)
				}
				var resp list.RawV5SchemaResponse
				inst.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &resp)
				if !reflect.DeepEqual(resp.ProtoV5Schema, wantSchema) {
					t.Errorf("ProtoV5Schema = %#v, want %#v", resp.ProtoV5Schema, wantSchema)
				}
				if !reflect.DeepEqual(resp.ProtoV5IdentitySchema, wantIdentity) {
					t.Errorf("ProtoV5IdentitySchema = %#v, want %#v", resp.ProtoV5IdentitySchema, wantIdentity)
				}
			})
		}
	})

	t.Run("nil_sdkv2_resource_and_nil_identity", func(t *testing.T) {
		nilRes := &ListResourceMetadata{TypeName: "google_nil_resource"}
		_, probeNilResp := srv.ListResourceFuncs(ctx, nilRes)
		if probeNilResp.ProtoV5Schema != nil || probeNilResp.ProtoV5IdentitySchema != nil {
			t.Errorf("probe with nil SDKv2Resource = (%#v, %#v), want (nil, nil)", probeNilResp.ProtoV5Schema, probeNilResp.ProtoV5IdentitySchema)
		}
		var directNilResp list.RawV5SchemaResponse
		nilRes.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &directNilResp)
		if directNilResp.ProtoV5Schema != nil || directNilResp.ProtoV5IdentitySchema != nil {
			t.Errorf("direct with nil SDKv2Resource = (%#v, %#v), want (nil, nil)", directNilResp.ProtoV5Schema, directNilResp.ProtoV5IdentitySchema)
		}

		noIdentitySDK := &schema.Resource{
			Schema: map[string]*schema.Schema{
				"name": {Type: schema.TypeString, Required: true},
			},
		}
		noIdentityRes := &ListResourceMetadata{
			TypeName:      "google_no_identity_resource",
			SDKv2Resource: noIdentitySDK,
		}
		_, probeNoIdResp := srv.ListResourceFuncs(ctx, noIdentityRes)
		if probeNoIdResp.ProtoV5Schema != nil || probeNoIdResp.ProtoV5IdentitySchema != nil {
			t.Errorf("probe with nil Identity = (%#v, %#v), want (nil, nil)", probeNoIdResp.ProtoV5Schema, probeNoIdResp.ProtoV5IdentitySchema)
		}
		var directNoIdResp list.RawV5SchemaResponse
		noIdentityRes.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &directNoIdResp)
		if directNoIdResp.ProtoV5Schema == nil || directNoIdResp.ProtoV5IdentitySchema != nil {
			t.Errorf("direct with nil Identity = (%#v, %#v), want (non-nil, nil)", directNoIdResp.ProtoV5Schema, directNoIdResp.ProtoV5IdentitySchema)
		}
	})
}
