package tpgresource

import (
	"context"
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
