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

	// 1. Metadata() -> RawV5Schemas() probe sequence (used by fwserver.Server.ListResourceFuncs)
	// returns non-nil stub schemas without converting the full SDKv2 schema.
	meta := &ListResourceMetadata{
		TypeName:      "google_test_resource",
		SDKv2Resource: sdkRes,
	}
	var metaResp resource.MetadataResponse
	meta.Metadata(ctx, resource.MetadataRequest{}, &metaResp)
	if metaResp.TypeName != "google_test_resource" {
		t.Fatalf("Metadata TypeName = %q, want %q", metaResp.TypeName, "google_test_resource")
	}

	var probeResp list.RawV5SchemaResponse
	meta.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &probeResp)
	if probeResp.ProtoV5Schema != stubProtoV5Schema || probeResp.ProtoV5IdentitySchema != stubProtoV5IdentitySchema {
		t.Fatalf("probe RawV5Schemas = (%#v, %#v), want stub schemas", probeResp.ProtoV5Schema, probeResp.ProtoV5IdentitySchema)
	}

	// 2. Subsequent RawV5Schemas() on the same instance (and direct RawV5Schemas()
	// on a fresh instance, as during ListResource RPC execution) returns the full schemas.
	for _, tc := range []struct {
		name string
		inst *ListResourceMetadata
	}{
		{name: "after_probe", inst: meta},
		{name: "direct", inst: &ListResourceMetadata{TypeName: "google_test_resource", SDKv2Resource: sdkRes}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var resp list.RawV5SchemaResponse
			tc.inst.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &resp)
			if !reflect.DeepEqual(resp.ProtoV5Schema, wantSchema) {
				t.Errorf("ProtoV5Schema = %#v, want %#v", resp.ProtoV5Schema, wantSchema)
			}
			if !reflect.DeepEqual(resp.ProtoV5IdentitySchema, wantIdentity) {
				t.Errorf("ProtoV5IdentitySchema = %#v, want %#v", resp.ProtoV5IdentitySchema, wantIdentity)
			}
		})
	}
}
