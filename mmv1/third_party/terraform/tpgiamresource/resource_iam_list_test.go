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
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

type Server struct{}

//go:noinline
func (*Server) ListResourceFuncs(ctx context.Context, r list.ListResourceWithRawV5Schemas) (resource.MetadataResponse, list.RawV5SchemaResponse) {
	var metaResp resource.MetadataResponse
	r.Metadata(ctx, resource.MetadataRequest{}, &metaResp)
	var schemaResp list.RawV5SchemaResponse
	r.RawV5Schemas(ctx, list.RawV5SchemaRequest{}, &schemaResp)
	return metaResp, schemaResp
}

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
	srv := &Server{}

	parentSchema := testIamListParentSchema()
	callCfg := IamListCallConfig{
		ParentResourceField: "instance",
		EnableRoleFilter:    true,
		EnableMemberFilter:  true,
	}

	memberRes := ResourceIamMember(parentSchema, nil, nil, IamWithParentResourceIdentity(testIamParentIdentityParser))
	bindingRes := ResourceIamBinding(parentSchema, nil, nil, IamWithParentResourceIdentity(testIamParentIdentityParser))

	for _, tc := range []struct {
		name     string
		typeName string
		res      *schema.Resource
		factory  func(string, *schema.Resource) list.ListResource
	}{
		{
			name:     "IamMemberListResource",
			typeName: "google_test_instance_iam_member",
			res:      memberRes,
			factory: func(typeName string, r *schema.Resource) list.ListResource {
				return NewIamMemberListResource(typeName, r, nil, callCfg)
			},
		},
		{
			name:     "IamBindingListResource",
			typeName: "google_test_instance_iam_binding",
			res:      bindingRes,
			factory: func(typeName string, r *schema.Resource) list.ListResource {
				return NewIamBindingListResource(typeName, r, nil, callCfg)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantSchema := tc.res.ProtoSchema(ctx)()
			wantIdentity := tc.res.ProtoIdentitySchema(ctx)()

			probed := tc.factory(tc.typeName, tc.res).(list.ListResourceWithRawV5Schemas)
			metaResp, probeResp := srv.ListResourceFuncs(ctx, probed)
			if metaResp.TypeName != tc.typeName {
				t.Fatalf("Metadata TypeName = %q, want %q", metaResp.TypeName, tc.typeName)
			}
			if probeResp.ProtoV5Schema == nil || probeResp.ProtoV5IdentitySchema == nil || probeResp.ProtoV5Schema.Block != nil {
				t.Fatalf("probe RawV5Schemas = (%#v, %#v), want lightweight non-nil stub schemas", probeResp.ProtoV5Schema, probeResp.ProtoV5IdentitySchema)
			}

			inst := tc.factory(tc.typeName, tc.res).(list.ListResourceWithRawV5Schemas)
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
}
