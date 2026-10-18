// Copyright (c) IBM Corp. 2014, 2026
// SPDX-License-Identifier: MPL-2.0

package tpgiamresource

import (
	"context"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func testIamListParentSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"project": {
			Type:     schema.TypeString,
			Optional: true,
			Computed: true,
			ForceNew: true,
		},
		"instance": {
			Type:     schema.TypeString,
			Required: true,
			ForceNew: true,
		},
	}
}

func testIamParentIdentityParser(d *schema.ResourceData, _ *schema.IdentityData, _ *transport_tpg.Config) (string, error) {
	return d.Id(), nil
}

func TestIamListResource_MetadataAndRawV5Schemas(t *testing.T) {
	ctx := context.Background()

	parentSchema := testIamListParentSchema()
	callCfg := IamListCallConfig{
		ParentResourceField: "instance",
		EnableRoleFilter:    true,
		EnableMemberFilter:  true,
	}

	memberResA := ResourceIamMember(parentSchema, nil, nil, IamWithParentResourceIdentity(testIamParentIdentityParser))
	memberResB := ResourceIamMember(parentSchema, nil, nil, IamWithParentResourceIdentity(testIamParentIdentityParser))
	bindingResA := ResourceIamBinding(parentSchema, nil, nil, IamWithParentResourceIdentity(testIamParentIdentityParser))
	bindingResB := ResourceIamBinding(parentSchema, nil, nil, IamWithParentResourceIdentity(testIamParentIdentityParser))

	cases := []struct {
		name     string
		typeName string
		resA     *schema.Resource
		resB     *schema.Resource
		factory  func(string, *schema.Resource) list.ListResource
	}{
		{
			name:     "IamMemberListResource",
			typeName: "google_test_instance_iam_member",
			resA:     memberResA,
			resB:     memberResB,
			factory: func(typeName string, r *schema.Resource) list.ListResource {
				return NewIamMemberListResource(typeName, r, nil, callCfg)
			},
		},
		{
			name:     "IamBindingListResource",
			typeName: "google_test_instance_iam_binding",
			resA:     bindingResA,
			resB:     bindingResB,
			factory: func(typeName string, r *schema.Resource) list.ListResource {
				return NewIamBindingListResource(typeName, r, nil, callCfg)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantSchema := tc.resA.ProtoSchema(ctx)()
			wantIdentity := tc.resA.ProtoIdentitySchema(ctx)()

			// 1. Metadata() -> RawV5Schemas() probe sequence (as used by fwserver.Server.ListResourceFuncs)
			// must return non-nil stub schemas without populating the cache with stubs.
			probed := tc.factory(tc.typeName, tc.resA)
			var metaResp resource.MetadataResponse
			probed.Metadata(ctx, resource.MetadataRequest{}, &metaResp)
			if metaResp.TypeName != tc.typeName {
				t.Fatalf("Metadata TypeName = %q, want %q", metaResp.TypeName, tc.typeName)
			}

			probedWithRaw, ok := probed.(list.ListResourceWithRawV5Schemas)
			if !ok {
				t.Fatalf("%T does not implement list.ListResourceWithRawV5Schemas", probed)
			}

			var probeResp list.RawV5SchemaResponse
			probedWithRaw.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &probeResp)
			if probeResp.ProtoV5Schema == nil || probeResp.ProtoV5IdentitySchema == nil {
				t.Fatalf("probe RawV5Schemas returned nil schema (%v) or identity (%v)", probeResp.ProtoV5Schema, probeResp.ProtoV5IdentitySchema)
			}
			if probeResp.ProtoV5Schema.Block != nil {
				t.Errorf("probe RawV5Schemas built a full Block instead of returning the lightweight stub schema")
			}

			// 2. Direct RawV5Schemas() on a fresh instance (or subsequent call on the same instance)
			// must return the full protocol schemas and memoize them across distinct instances.
			instA := tc.factory(tc.typeName, tc.resA).(tpgresource.ListResource)
			var respA list.RawV5SchemaResponse
			instA.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &respA)
			if !reflect.DeepEqual(respA.ProtoV5Schema, wantSchema) {
				t.Errorf("ProtoV5Schema = %#v, want %#v", respA.ProtoV5Schema, wantSchema)
			}
			if !reflect.DeepEqual(respA.ProtoV5IdentitySchema, wantIdentity) {
				t.Errorf("ProtoV5IdentitySchema = %#v, want %#v", respA.ProtoV5IdentitySchema, wantIdentity)
			}

			instB := tc.factory(tc.typeName, tc.resB).(tpgresource.ListResource)
			var respB list.RawV5SchemaResponse
			instB.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &respB)
			if respB.ProtoV5Schema != respA.ProtoV5Schema {
				t.Error("ProtoV5Schema was rebuilt for second IAM list resource instance with same TypeName, want memoized pointer")
			}
			if respB.ProtoV5IdentitySchema != respA.ProtoV5IdentitySchema {
				t.Error("ProtoV5IdentitySchema was rebuilt for second IAM list resource instance with same TypeName, want memoized pointer")
			}
		})
	}
}
