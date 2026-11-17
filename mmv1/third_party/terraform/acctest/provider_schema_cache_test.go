package acctest

import (
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dnaeon/go-vcr/cassette"
	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-go/tfprotov5/tf5server"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func isBetaProviderPkg() bool {
	pkgPath := reflect.TypeOf(transport_tpg.Config{}).PkgPath()
	return strings.Contains(pkgPath, "google-beta")
}

func newObjectDynamicValueErr(s *tfprotov5.Schema, overrides map[string]tftypes.Value) (*tfprotov5.DynamicValue, error) {
	if s == nil {
		return nil, fmt.Errorf("schema is nil")
	}
	objType, ok := s.ValueType().(tftypes.Object)
	if !ok {
		return nil, fmt.Errorf("expected tftypes.Object schema ValueType, got %T", s.ValueType())
	}
	vals := make(map[string]tftypes.Value, len(objType.AttributeTypes))
	for attrName, attrType := range objType.AttributeTypes {
		if v, exists := overrides[attrName]; exists {
			vals[attrName] = v
		} else {
			vals[attrName] = tftypes.NewValue(attrType, nil)
		}
	}
	dv, err := tfprotov5.NewDynamicValue(objType, tftypes.NewValue(objType, vals))
	if err != nil {
		return nil, fmt.Errorf("NewDynamicValue failed: %w", err)
	}
	return &dv, nil
}

func newObjectDynamicValue(tb testing.TB, s *tfprotov5.Schema, overrides map[string]tftypes.Value) *tfprotov5.DynamicValue {
	tb.Helper()
	dv, err := newObjectDynamicValueErr(s, overrides)
	if err != nil {
		tb.Fatal(err)
	}
	return dv
}

func unmarshalObjectMapErr(s *tfprotov5.Schema, dv *tfprotov5.DynamicValue) (map[string]tftypes.Value, error) {
	if dv == nil {
		return nil, fmt.Errorf("DynamicValue is nil")
	}
	val, err := dv.Unmarshal(s.ValueType())
	if err != nil {
		return nil, fmt.Errorf("DynamicValue.Unmarshal failed: %w", err)
	}
	var m map[string]tftypes.Value
	if err := val.As(&m); err != nil {
		return nil, fmt.Errorf("tftypes.Value.As(map) failed: %w", err)
	}
	return m, nil
}

func unmarshalObjectMap(tb testing.TB, s *tfprotov5.Schema, dv *tfprotov5.DynamicValue) map[string]tftypes.Value {
	tb.Helper()
	m, err := unmarshalObjectMapErr(s, dv)
	if err != nil {
		tb.Fatal(err)
	}
	return m
}

func unmarshalStringAttrErr(m map[string]tftypes.Value, key string) (string, error) {
	v, ok := m[key]
	if !ok {
		return "", fmt.Errorf("attribute %q missing from object map", key)
	}
	if v.IsNull() || !v.IsKnown() {
		return "", nil
	}
	var s string
	if err := v.As(&s); err != nil {
		return "", fmt.Errorf("attribute %q As(string) failed: %w", key, err)
	}
	return s, nil
}

func unmarshalStringAttr(tb testing.TB, m map[string]tftypes.Value, key string) string {
	tb.Helper()
	s, err := unmarshalStringAttrErr(m, key)
	if err != nil {
		tb.Fatal(err)
	}
	return s
}

func configureServerWithFakeCredsErr(srv tfprotov5.ProviderServer, providerSchema *tfprotov5.Schema, credsID, project, region, zone string) error {
	ctx := context.Background()
	rawCfg, err := newObjectDynamicValueErr(providerSchema, map[string]tftypes.Value{
		"credentials": tftypes.NewValue(tftypes.String, GenerateFakeCredentialsJson(credsID)),
		"project":     tftypes.NewValue(tftypes.String, project),
		"region":      tftypes.NewValue(tftypes.String, region),
		"zone":        tftypes.NewValue(tftypes.String, zone),
	})
	if err != nil {
		return err
	}

	prepResp, err := srv.PrepareProviderConfig(ctx, &tfprotov5.PrepareProviderConfigRequest{
		Config: rawCfg,
	})
	if err != nil {
		return fmt.Errorf("PrepareProviderConfig failed: %w", err)
	}
	for _, d := range prepResp.Diagnostics {
		if d != nil && d.Severity == tfprotov5.DiagnosticSeverityError {
			return fmt.Errorf("PrepareProviderConfig error diagnostic: %s: %s", d.Summary, d.Detail)
		}
	}

	cfgResp, err := srv.ConfigureProvider(ctx, &tfprotov5.ConfigureProviderRequest{
		TerraformVersion: "1.14.3",
		Config:           prepResp.PreparedConfig,
	})
	if err != nil {
		return fmt.Errorf("ConfigureProvider failed: %w", err)
	}
	for _, d := range cfgResp.Diagnostics {
		if d != nil && d.Severity == tfprotov5.DiagnosticSeverityError {
			return fmt.Errorf("ConfigureProvider error diagnostic: %s: %s", d.Summary, d.Detail)
		}
	}
	return nil
}

func configureServerWithFakeCreds(tb testing.TB, srv tfprotov5.ProviderServer, providerSchema *tfprotov5.Schema, credsID, project, region, zone string) {
	tb.Helper()
	if err := configureServerWithFakeCredsErr(srv, providerSchema, credsID, project, region, zone); err != nil {
		tb.Fatal(err)
	}
}

func assertAllProtoV5Interfaces(t *testing.T, srv tfprotov5.ProviderServer) {
	t.Helper()
	if _, ok := srv.(tfprotov5.ProviderServerWithListResource); !ok {
		t.Fatalf("%T does not implement tfprotov5.ProviderServerWithListResource", srv)
	}
	if _, ok := srv.(tfprotov5.ProviderServerWithActions); !ok {
		t.Fatalf("%T does not implement tfprotov5.ProviderServerWithActions", srv)
	}
	if _, ok := srv.(tfprotov5.ResourceServer); !ok {
		t.Fatalf("%T does not implement tfprotov5.ResourceServer", srv)
	}
	if _, ok := srv.(tfprotov5.DataSourceServer); !ok {
		t.Fatalf("%T does not implement tfprotov5.DataSourceServer", srv)
	}
	if _, ok := srv.(tfprotov5.FunctionServer); !ok {
		t.Fatalf("%T does not implement tfprotov5.FunctionServer", srv)
	}
	if _, ok := srv.(tfprotov5.EphemeralResourceServer); !ok {
		t.Fatalf("%T does not implement tfprotov5.EphemeralResourceServer", srv)
	}
	if _, ok := srv.(tfprotov5.ListResourceServer); !ok {
		t.Fatalf("%T does not implement tfprotov5.ListResourceServer", srv)
	}
	if _, ok := srv.(tfprotov5.ActionServer); !ok {
		t.Fatalf("%T does not implement tfprotov5.ActionServer", srv)
	}
	if grpcSrv := tf5server.New("registry.terraform.io/hashicorp/google", srv); grpcSrv == nil {
		t.Fatal("tf5server.New returned nil")
	}
}

func TestCachedProviderServer_Interfaces(t *testing.T) {
	factories := ProtoV5ProviderFactories(t)
	googleFactory, ok := factories["google"]
	if !ok {
		t.Fatal("expected 'google' key in ProtoV5ProviderFactories")
	}
	srv, err := googleFactory()
	if err != nil {
		t.Fatalf("ProtoV5ProviderFactories['google']() failed: %v", err)
	}
	assertAllProtoV5Interfaces(t, srv)

	betaFactories := ProtoV5ProviderBetaFactories(t)
	if isBetaProviderPkg() {
		betaFactory, ok := betaFactories["google-beta"]
		if !ok {
			t.Fatal("expected 'google-beta' key in ProtoV5ProviderBetaFactories in beta package")
		}
		betaSrv, err := betaFactory()
		if err != nil {
			t.Fatalf("ProtoV5ProviderBetaFactories['google-beta']() failed: %v", err)
		}
		assertAllProtoV5Interfaces(t, betaSrv)
	} else if len(betaFactories) != 0 {
		t.Fatalf("expected empty ProtoV5ProviderBetaFactories in GA package, got keys: %v", betaFactories)
	}
}

func TestCachedProviderServer_SchemaParityAndLazyDownstream(t *testing.T) {
	ctx := context.Background()
	factory := ProtoV5ProviderFactories(t)["google"]

	srv1, err := factory()
	if err != nil {
		t.Fatalf("factory() #1 failed: %v", err)
	}
	srv2, err := factory()
	if err != nil {
		t.Fatalf("factory() #2 failed: %v", err)
	}
	uncached, err := newUncachedMuxServer(t.Name())
	if err != nil {
		t.Fatalf("newUncachedMuxServer failed: %v", err)
	}

	meta1, err := srv1.GetMetadata(ctx, &tfprotov5.GetMetadataRequest{})
	if err != nil {
		t.Fatalf("cached GetMetadata failed: %v", err)
	}
	meta2, err := srv2.GetMetadata(ctx, &tfprotov5.GetMetadataRequest{})
	if err != nil {
		t.Fatalf("cached GetMetadata #2 failed: %v", err)
	}
	if meta1 != meta2 {
		t.Fatal("expected GetMetadata to return identical cached pointer across server instances")
	}
	uncachedMeta, err := uncached.GetMetadata(ctx, &tfprotov5.GetMetadataRequest{})
	if err != nil {
		t.Fatalf("uncached GetMetadata failed: %v", err)
	}
	if len(meta1.Resources) != len(uncachedMeta.Resources) ||
		len(meta1.DataSources) != len(uncachedMeta.DataSources) ||
		len(meta1.EphemeralResources) != len(uncachedMeta.EphemeralResources) ||
		len(meta1.ListResources) != len(uncachedMeta.ListResources) ||
		len(meta1.Functions) != len(uncachedMeta.Functions) ||
		len(meta1.Actions) != len(uncachedMeta.Actions) {
		t.Fatalf("metadata counts mismatch: cached=(res=%d ds=%d ephem=%d list=%d fn=%d act=%d) vs uncached=(res=%d ds=%d ephem=%d list=%d fn=%d act=%d)",
			len(meta1.Resources), len(meta1.DataSources), len(meta1.EphemeralResources), len(meta1.ListResources), len(meta1.Functions), len(meta1.Actions),
			len(uncachedMeta.Resources), len(uncachedMeta.DataSources), len(uncachedMeta.EphemeralResources), len(uncachedMeta.ListResources), len(uncachedMeta.Functions), len(uncachedMeta.Actions))
	}

	schema1, err := srv1.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("cached GetProviderSchema failed: %v", err)
	}
	schema2, err := srv2.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("cached GetProviderSchema #2 failed: %v", err)
	}
	if schema1 != schema2 {
		t.Fatal("expected GetProviderSchema to return identical cached pointer across server instances")
	}
	uncachedSchema, err := uncached.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("uncached GetProviderSchema failed: %v", err)
	}

	cmpOpts := cmp.Comparer(func(a, b tftypes.Type) bool {
		if a == nil || b == nil {
			return a == nil && b == nil
		}
		return a.Equal(b)
	})
	if diff := cmp.Diff(uncachedSchema, schema1, cmpOpts); diff != "" {
		t.Fatalf("GetProviderSchema mismatch (-uncached +cached):\n%s", diff)
	}

	// Verify test-only datasources and modern capability schemas are present in cached schema.
	if _, ok := schema1.DataSourceSchemas["google_provider_config_sdk"]; !ok {
		t.Fatal("missing test datasource google_provider_config_sdk in cached schema")
	}
	if _, ok := schema1.DataSourceSchemas["google_provider_config_plugin_framework"]; !ok {
		t.Fatal("missing test datasource google_provider_config_plugin_framework in cached schema")
	}
	if _, ok := schema1.EphemeralResourceSchemas["google_client_config"]; !ok {
		t.Fatal("missing ephemeral resource google_client_config in cached schema")
	}
	if _, ok := schema1.ListResourceSchemas["google_pubsub_topic"]; !ok {
		t.Fatal("missing list resource google_pubsub_topic in cached schema")
	}
	if _, ok := schema1.ActionSchemas["google_compute_instance_reset"]; !ok {
		t.Fatal("missing action google_compute_instance_reset in cached schema")
	}
	if _, ok := schema1.Functions["name_from_id"]; !ok {
		t.Fatal("missing function name_from_id in cached schema")
	}

	id1, err := srv1.GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{})
	if err != nil {
		t.Fatalf("cached GetResourceIdentitySchemas failed: %v", err)
	}
	id2, err := srv2.GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{})
	if err != nil {
		t.Fatalf("cached GetResourceIdentitySchemas #2 failed: %v", err)
	}
	if id1 != id2 {
		t.Fatal("expected GetResourceIdentitySchemas to return identical cached pointer across server instances")
	}
	uncachedID, err := uncached.GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{})
	if err != nil {
		t.Fatalf("uncached GetResourceIdentitySchemas failed: %v", err)
	}
	if diff := cmp.Diff(uncachedID, id1, cmpOpts); diff != "" {
		t.Fatalf("GetResourceIdentitySchemas mismatch (-uncached +cached):\n%s", diff)
	}
	if _, ok := id1.IdentitySchemas["google_pubsub_topic"]; !ok {
		t.Fatal("missing google_pubsub_topic in cached GetResourceIdentitySchemas")
	}

	fn1, err := srv1.GetFunctions(ctx, &tfprotov5.GetFunctionsRequest{})
	if err != nil {
		t.Fatalf("cached GetFunctions failed: %v", err)
	}
	fn2, err := srv2.GetFunctions(ctx, &tfprotov5.GetFunctionsRequest{})
	if err != nil {
		t.Fatalf("cached GetFunctions #2 failed: %v", err)
	}
	if fn1 != fn2 {
		t.Fatal("expected GetFunctions to return identical cached pointer across server instances")
	}
	uncachedFn, err := uncached.GetFunctions(ctx, &tfprotov5.GetFunctionsRequest{})
	if err != nil {
		t.Fatalf("uncached GetFunctions failed: %v", err)
	}
	if diff := cmp.Diff(uncachedFn, fn1, cmpOpts); diff != "" {
		t.Fatalf("GetFunctions mismatch (-uncached +cached):\n%s", diff)
	}

	// Schema-only + StopProvider calls must NOT allocate an underlying per-call muxServer.
	if _, err := srv1.StopProvider(ctx, &tfprotov5.StopProviderRequest{}); err != nil {
		t.Fatalf("StopProvider on uninitialized wrapper failed: %v", err)
	}
	if srv1.(*cachedProviderServer).initialized.Load() {
		t.Fatal("expected underlying muxServer to remain uninitialized when only schema/metadata/StopProvider RPCs are called")
	}
}

func TestProviderSchemaCache_LazyInitAndVariantIsolation(t *testing.T) {
	ctx := context.Background()

	var callsA atomic.Int32
	var seenNamesA sync.Map
	cacheA := newProviderSchemaCache(func(testName string) (tfprotov5.ProviderServer, error) {
		callsA.Add(1)
		seenNamesA.Store(testName, true)
		return &stubVariantProviderServer{variantResource: "google_variant_a"}, nil
	})

	var callsB atomic.Int32
	cacheB := newProviderSchemaCache(func(testName string) (tfprotov5.ProviderServer, error) {
		callsB.Add(1)
		return &stubVariantProviderServer{variantResource: "google_variant_b"}, nil
	})

	// Simulate 25 schema-only CLI invocations (init / show -json / state rm).
	wrappers := make([]*cachedProviderServer, 25)
	for i := range wrappers {
		w := cacheA.providerServer(fmt.Sprintf("TestLazy_%d", i))
		wrappers[i] = w
		if _, err := w.GetMetadata(ctx, &tfprotov5.GetMetadataRequest{}); err != nil {
			t.Fatalf("GetMetadata failed: %v", err)
		}
		sResp, err := w.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{})
		if err != nil {
			t.Fatalf("GetProviderSchema failed: %v", err)
		}
		if _, ok := sResp.ResourceSchemas["google_variant_a"]; !ok {
			t.Fatalf("expected google_variant_a in cacheA schema, got: %v", sResp.ResourceSchemas)
		}
		if _, ok := sResp.ResourceSchemas["google_variant_b"]; ok {
			t.Fatal("unexpected cross-variant schema bleed from cacheB into cacheA")
		}
		if _, err := w.GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{}); err != nil {
			t.Fatalf("GetResourceIdentitySchemas failed: %v", err)
		}
		if _, err := w.GetFunctions(ctx, &tfprotov5.GetFunctionsRequest{}); err != nil {
			t.Fatalf("GetFunctions failed: %v", err)
		}
		if _, err := w.StopProvider(ctx, &tfprotov5.StopProviderRequest{}); err != nil {
			t.Fatalf("StopProvider failed: %v", err)
		}
	}

	if got := callsA.Load(); got != 1 {
		t.Fatalf("expected exactly 1 factory call for schema cache population across 25 schema-only wrappers, got %d", got)
	}

	// Verify cacheB stays completely isolated from cacheA.
	wB := cacheB.providerServer("TestVariantB")
	sRespB, err := wB.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("cacheB GetProviderSchema failed: %v", err)
	}
	if _, ok := sRespB.ResourceSchemas["google_variant_b"]; !ok {
		t.Fatalf("expected google_variant_b in cacheB schema, got: %v", sRespB.ResourceSchemas)
	}
	if _, ok := sRespB.ResourceSchemas["google_variant_a"]; ok {
		t.Fatal("unexpected cross-variant schema bleed from cacheA into cacheB")
	}
	if got := callsB.Load(); got != 1 {
		t.Fatalf("expected 1 factory call on cacheB, got %d", got)
	}

	// Trigger stateful RPC on 2 specific wrappers; only those 2 should lazily instantiate downstream servers.
	if _, err := wrappers[3].ConfigureProvider(ctx, &tfprotov5.ConfigureProviderRequest{}); err != nil {
		t.Fatalf("ConfigureProvider on wrapper 3 failed: %v", err)
	}
	if _, err := wrappers[3].ValidateResourceTypeConfig(ctx, &tfprotov5.ValidateResourceTypeConfigRequest{}); err != nil {
		t.Fatalf("ValidateResourceTypeConfig on wrapper 3 failed: %v", err)
	}
	if _, err := wrappers[17].PrepareProviderConfig(ctx, &tfprotov5.PrepareProviderConfigRequest{}); err != nil {
		t.Fatalf("PrepareProviderConfig on wrapper 17 failed: %v", err)
	}

	if got := callsA.Load(); got != 3 {
		t.Fatalf("expected 3 total factory calls (1 schema + 2 stateful wrappers), got %d", got)
	}
	if _, ok := seenNamesA.Load("TestLazy_3"); !ok {
		t.Fatal("expected TestLazy_3 to be passed to factory")
	}
	if _, ok := seenNamesA.Load("TestLazy_17"); !ok {
		t.Fatal("expected TestLazy_17 to be passed to factory")
	}
	if _, ok := seenNamesA.Load("TestLazy_0"); ok {
		t.Fatal("did not expect schema-only wrapper TestLazy_0 to invoke factory")
	}
}

func TestCachedProviderServer_OptionalCapabilitiesParity(t *testing.T) {
	UnsetTestProviderConfigEnvs(t)
	ctx := context.Background()

	cachedSrv, err := ProtoV5ProviderFactories(t)["google"]()
	if err != nil {
		t.Fatalf("cached factory failed: %v", err)
	}
	uncachedSrv, err := newUncachedMuxServer(t.Name())
	if err != nil {
		t.Fatalf("newUncachedMuxServer failed: %v", err)
	}

	schemaResp, err := cachedSrv.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("GetProviderSchema failed: %v", err)
	}

	// 1. Provider Functions (CallFunction)
	argDV, err := tfprotov5.NewDynamicValue(tftypes.String, tftypes.NewValue(tftypes.String, "projects/my-proj/topics/my-topic"))
	if err != nil {
		t.Fatalf("NewDynamicValue failed: %v", err)
	}
	callReq := &tfprotov5.CallFunctionRequest{
		Name:      "name_from_id",
		Arguments: []*tfprotov5.DynamicValue{&argDV},
	}
	cachedFnResp, err := cachedSrv.CallFunction(ctx, callReq)
	if err != nil {
		t.Fatalf("cached CallFunction failed: %v", err)
	}
	uncachedFnResp, err := uncachedSrv.CallFunction(ctx, callReq)
	if err != nil {
		t.Fatalf("uncached CallFunction failed: %v", err)
	}
	if cachedFnResp.Error != nil || uncachedFnResp.Error != nil {
		t.Fatalf("unexpected CallFunction error: cached=%v uncached=%v", cachedFnResp.Error, uncachedFnResp.Error)
	}
	cachedFnVal, err := cachedFnResp.Result.Unmarshal(tftypes.String)
	if err != nil {
		t.Fatalf("Unmarshal CallFunction result failed: %v", err)
	}
	var gotTopicName string
	if err := cachedFnVal.As(&gotTopicName); err != nil || gotTopicName != "my-topic" {
		t.Fatalf("CallFunction(name_from_id) = %q (err=%v), want %q", gotTopicName, err, "my-topic")
	}
	if !reflect.DeepEqual(cachedFnResp, uncachedFnResp) {
		t.Fatalf("CallFunction response mismatch: cached=%+v uncached=%+v", cachedFnResp, uncachedFnResp)
	}

	// 2. Resource Identity (UpgradeResourceIdentity)
	upgradeIDReq := &tfprotov5.UpgradeResourceIdentityRequest{
		TypeName: "google_pubsub_topic",
		Version:  0,
		RawIdentity: &tfprotov5.RawState{
			JSON: []byte(`{"project":"my-proj","name":"my-topic"}`),
		},
	}
	cachedIDResp, err := cachedSrv.UpgradeResourceIdentity(ctx, upgradeIDReq)
	if err != nil {
		t.Fatalf("cached UpgradeResourceIdentity failed: %v", err)
	}
	uncachedIDResp, err := uncachedSrv.UpgradeResourceIdentity(ctx, upgradeIDReq)
	if err != nil {
		t.Fatalf("uncached UpgradeResourceIdentity failed: %v", err)
	}
	if !reflect.DeepEqual(cachedIDResp, uncachedIDResp) {
		t.Fatalf("UpgradeResourceIdentity mismatch: cached=%+v uncached=%+v", cachedIDResp, uncachedIDResp)
	}

	// 3. GenerateResourceConfig
	topicSchema := schemaResp.ResourceSchemas["google_pubsub_topic"]
	topicState := newObjectDynamicValue(t, topicSchema, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "projects/my-proj/topics/my-topic"),
		"project": tftypes.NewValue(tftypes.String, "my-proj"),
		"name":    tftypes.NewValue(tftypes.String, "my-topic"),
	})
	genCfgReq := &tfprotov5.GenerateResourceConfigRequest{
		TypeName: "google_pubsub_topic",
		State:    topicState,
	}
	cachedGenResp, err := cachedSrv.GenerateResourceConfig(ctx, genCfgReq)
	if err != nil {
		t.Fatalf("cached GenerateResourceConfig failed: %v", err)
	}
	uncachedGenResp, err := uncachedSrv.GenerateResourceConfig(ctx, genCfgReq)
	if err != nil {
		t.Fatalf("uncached GenerateResourceConfig failed: %v", err)
	}
	if !reflect.DeepEqual(cachedGenResp, uncachedGenResp) {
		t.Fatalf("GenerateResourceConfig mismatch: cached=%+v uncached=%+v", cachedGenResp, uncachedGenResp)
	}
	genMap := unmarshalObjectMap(t, topicSchema, cachedGenResp.Config)
	if got := unmarshalStringAttr(t, genMap, "name"); got != "my-topic" {
		t.Fatalf("GenerateResourceConfig generated name = %q, want %q", got, "my-topic")
	}

	// 4. List Resources (ProviderServerWithListResource)
	cachedListSrv := cachedSrv.(tfprotov5.ProviderServerWithListResource)
	uncachedListSrv := uncachedSrv.(tfprotov5.ProviderServerWithListResource)
	listSchema := schemaResp.ListResourceSchemas["google_pubsub_topic"]
	listCfg := newObjectDynamicValue(t, listSchema, map[string]tftypes.Value{
		"project": tftypes.NewValue(tftypes.String, "my-proj"),
	})
	incResDV, err := tfprotov5.NewDynamicValue(tftypes.Bool, tftypes.NewValue(tftypes.Bool, false))
	if err != nil {
		t.Fatalf("NewDynamicValue(Bool) failed: %v", err)
	}
	limitDV, err := tfprotov5.NewDynamicValue(tftypes.Number, tftypes.NewValue(tftypes.Number, 10))
	if err != nil {
		t.Fatalf("NewDynamicValue(Number) failed: %v", err)
	}
	valListReq := &tfprotov5.ValidateListResourceConfigRequest{
		TypeName:              "google_pubsub_topic",
		Config:                listCfg,
		IncludeResourceObject: &incResDV,
		Limit:                 &limitDV,
	}
	cachedListValResp, err := cachedListSrv.ValidateListResourceConfig(ctx, valListReq)
	if err != nil {
		t.Fatalf("cached ValidateListResourceConfig failed: %v", err)
	}
	uncachedListValResp, err := uncachedListSrv.ValidateListResourceConfig(ctx, valListReq)
	if err != nil {
		t.Fatalf("uncached ValidateListResourceConfig failed: %v", err)
	}
	if !reflect.DeepEqual(cachedListValResp, uncachedListValResp) {
		t.Fatalf("ValidateListResourceConfig mismatch: cached=%+v uncached=%+v", cachedListValResp, uncachedListValResp)
	}
	if len(cachedListValResp.Diagnostics) != 0 {
		t.Fatalf("unexpected ValidateListResourceConfig diagnostics: %+v", cachedListValResp.Diagnostics)
	}

	// 5. Actions (ProviderServerWithActions)
	cachedActSrv := cachedSrv.(tfprotov5.ProviderServerWithActions)
	uncachedActSrv := uncachedSrv.(tfprotov5.ProviderServerWithActions)
	actSchema := schemaResp.ActionSchemas["google_compute_instance_reset"].Schema

	validActCfg := newObjectDynamicValue(t, actSchema, map[string]tftypes.Value{
		"instance": tftypes.NewValue(tftypes.String, "projects/my-proj/zones/us-central1-a/instances/my-vm"),
		"timeout":  tftypes.NewValue(tftypes.String, "5m"),
	})
	cachedValidAct, err := cachedActSrv.ValidateActionConfig(ctx, &tfprotov5.ValidateActionConfigRequest{
		ActionType: "google_compute_instance_reset",
		Config:     validActCfg,
	})
	if err != nil {
		t.Fatalf("cached ValidateActionConfig (valid) failed: %v", err)
	}
	uncachedValidAct, err := uncachedActSrv.ValidateActionConfig(ctx, &tfprotov5.ValidateActionConfigRequest{
		ActionType: "google_compute_instance_reset",
		Config:     validActCfg,
	})
	if err != nil {
		t.Fatalf("uncached ValidateActionConfig (valid) failed: %v", err)
	}
	if !reflect.DeepEqual(cachedValidAct, uncachedValidAct) || len(cachedValidAct.Diagnostics) != 0 {
		t.Fatalf("ValidateActionConfig (valid) mismatch or unexpected diags: cached=%+v uncached=%+v", cachedValidAct, uncachedValidAct)
	}

	invalidActCfg := newObjectDynamicValue(t, actSchema, map[string]tftypes.Value{
		"instance": tftypes.NewValue(tftypes.String, "projects/my-proj/zones/us-central1-a/instances/my-vm"),
		"timeout":  tftypes.NewValue(tftypes.String, "99h"),
	})
	cachedInvalidAct, err := cachedActSrv.ValidateActionConfig(ctx, &tfprotov5.ValidateActionConfigRequest{
		ActionType: "google_compute_instance_reset",
		Config:     invalidActCfg,
	})
	if err != nil {
		t.Fatalf("cached ValidateActionConfig (invalid) failed: %v", err)
	}
	uncachedInvalidAct, err := uncachedActSrv.ValidateActionConfig(ctx, &tfprotov5.ValidateActionConfigRequest{
		ActionType: "google_compute_instance_reset",
		Config:     invalidActCfg,
	})
	if err != nil {
		t.Fatalf("uncached ValidateActionConfig (invalid) failed: %v", err)
	}
	if len(cachedInvalidAct.Diagnostics) == 0 {
		t.Fatal("expected error diagnostic for 99h timeout on google_compute_instance_reset")
	}
	if !reflect.DeepEqual(cachedInvalidAct, uncachedInvalidAct) {
		t.Fatalf("ValidateActionConfig (invalid) mismatch: cached=%+v uncached=%+v", cachedInvalidAct, uncachedInvalidAct)
	}

	// Configure both servers to test PlanAction and EphemeralResources.
	configureServerWithFakeCreds(t, cachedSrv, schemaResp.Provider, "ephem", "ephem-proj", "us-east1", "us-east1-b")
	configureServerWithFakeCreds(t, uncachedSrv, schemaResp.Provider, "ephem", "ephem-proj", "us-east1", "us-east1-b")

	cachedPlanAct, err := cachedActSrv.PlanAction(ctx, &tfprotov5.PlanActionRequest{
		ActionType: "google_compute_instance_reset",
		Config:     validActCfg,
	})
	if err != nil {
		t.Fatalf("cached PlanAction failed: %v", err)
	}
	uncachedPlanAct, err := uncachedActSrv.PlanAction(ctx, &tfprotov5.PlanActionRequest{
		ActionType: "google_compute_instance_reset",
		Config:     validActCfg,
	})
	if err != nil {
		t.Fatalf("uncached PlanAction failed: %v", err)
	}
	if !reflect.DeepEqual(cachedPlanAct, uncachedPlanAct) {
		t.Fatalf("PlanAction mismatch: cached=%+v uncached=%+v", cachedPlanAct, uncachedPlanAct)
	}

	// 6. Ephemeral Resources (Validate + Open + Close on google_client_config)
	ephemSchema := schemaResp.EphemeralResourceSchemas["google_client_config"]
	ephemCfg := newObjectDynamicValue(t, ephemSchema, nil)
	valEphemResp, err := cachedSrv.ValidateEphemeralResourceConfig(ctx, &tfprotov5.ValidateEphemeralResourceConfigRequest{
		TypeName: "google_client_config",
		Config:   ephemCfg,
	})
	if err != nil || len(valEphemResp.Diagnostics) != 0 {
		t.Fatalf("ValidateEphemeralResourceConfig failed: err=%v diags=%+v", err, valEphemResp.Diagnostics)
	}

	cachedOpenResp, err := cachedSrv.OpenEphemeralResource(ctx, &tfprotov5.OpenEphemeralResourceRequest{
		TypeName: "google_client_config",
		Config:   ephemCfg,
	})
	if err != nil {
		t.Fatalf("cached OpenEphemeralResource failed: %v", err)
	}
	uncachedOpenResp, err := uncachedSrv.OpenEphemeralResource(ctx, &tfprotov5.OpenEphemeralResourceRequest{
		TypeName: "google_client_config",
		Config:   ephemCfg,
	})
	if err != nil {
		t.Fatalf("uncached OpenEphemeralResource failed: %v", err)
	}
	// Note: google_client_config returns an access_token diagnostic warning/error if fake creds cannot mint OAuth token,
	// or sets project/region/zone; compare cached vs uncached for exact behavioral parity!
	if !reflect.DeepEqual(cachedOpenResp, uncachedOpenResp) {
		t.Fatalf("OpenEphemeralResource mismatch: cached=%+v uncached=%+v", cachedOpenResp, uncachedOpenResp)
	}
	if closeResp, err := cachedSrv.CloseEphemeralResource(ctx, &tfprotov5.CloseEphemeralResourceRequest{
		TypeName: "google_client_config",
		Private:  cachedOpenResp.Private,
	}); err != nil || len(closeResp.Diagnostics) != 0 {
		t.Fatalf("CloseEphemeralResource failed: err=%v diags=%+v", err, closeResp.Diagnostics)
	}
}

func verifyProviderConfigReadbackErr(srv tfprotov5.ProviderServer, schemaResp *tfprotov5.GetProviderSchemaResponse, wantProject, wantRegion, wantZone string) error {
	ctx := context.Background()

	for _, dsType := range []string{"google_provider_config_sdk", "google_provider_config_plugin_framework"} {
		dsSchema := schemaResp.DataSourceSchemas[dsType]
		cfg, err := newObjectDynamicValueErr(dsSchema, nil)
		if err != nil {
			return err
		}
		resp, err := srv.ReadDataSource(ctx, &tfprotov5.ReadDataSourceRequest{
			TypeName: dsType,
			Config:   cfg,
		})
		if err != nil {
			return fmt.Errorf("ReadDataSource(%s) failed: %w", dsType, err)
		}
		for _, d := range resp.Diagnostics {
			if d != nil && d.Severity == tfprotov5.DiagnosticSeverityError {
				return fmt.Errorf("ReadDataSource(%s) error diag: %s: %s", dsType, d.Summary, d.Detail)
			}
		}
		attrs, err := unmarshalObjectMapErr(dsSchema, resp.State)
		if err != nil {
			return err
		}
		for attr, want := range map[string]string{"project": wantProject, "region": wantRegion, "zone": wantZone} {
			got, err := unmarshalStringAttrErr(attrs, attr)
			if err != nil {
				return err
			}
			if got != want {
				return fmt.Errorf("%s %s = %q, want %q", dsType, attr, got, want)
			}
		}
	}
	return nil
}

func assertProviderConfigReadback(tb testing.TB, srv tfprotov5.ProviderServer, schemaResp *tfprotov5.GetProviderSchemaResponse, wantProject, wantRegion, wantZone string) {
	tb.Helper()
	if err := verifyProviderConfigReadbackErr(srv, schemaResp, wantProject, wantRegion, wantZone); err != nil {
		tb.Fatal(err)
	}
}

func TestCachedProviderServer_MetaAndVcrIsolation(t *testing.T) {
	UnsetTestProviderConfigEnvs(t)
	ctx := context.Background()

	// Part 1: Non-VCR per-test and per-CLI-call isolation of p.meta & Framework providerConfig.
	fnAlpha1, err := MuxedProviders("TestIsolationAlpha")
	if err != nil {
		t.Fatalf("MuxedProviders(Alpha #1) failed: %v", err)
	}
	srvAlpha1 := fnAlpha1()

	fnBeta, err := MuxedProviders("TestIsolationBeta")
	if err != nil {
		t.Fatalf("MuxedProviders(Beta) failed: %v", err)
	}
	srvBeta := fnBeta()

	fnAlpha2, err := MuxedProviders("TestIsolationAlpha")
	if err != nil {
		t.Fatalf("MuxedProviders(Alpha #2) failed: %v", err)
	}
	srvAlpha2 := fnAlpha2()

	schemaResp, err := srvAlpha1.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("GetProviderSchema failed: %v", err)
	}

	configureServerWithFakeCreds(t, srvAlpha1, schemaResp.Provider, "alpha1", "proj-alpha-1", "us-central1", "us-central1-a")
	configureServerWithFakeCreds(t, srvBeta, schemaResp.Provider, "beta", "proj-beta", "europe-west1", "europe-west1-b")
	configureServerWithFakeCreds(t, srvAlpha2, schemaResp.Provider, "alpha2", "proj-alpha-2", "asia-east1", "asia-east1-a")

	assertProviderConfigReadback(t, srvAlpha1, schemaResp, "proj-alpha-1", "us-central1", "us-central1-a")
	assertProviderConfigReadback(t, srvBeta, schemaResp, "proj-beta", "europe-west1", "europe-west1-b")
	assertProviderConfigReadback(t, srvAlpha2, schemaResp, "proj-alpha-2", "asia-east1", "asia-east1-a")

	// Part 2: VCR mode per-testName HTTP client & config isolation.
	vcrDir := t.TempDir()
	testOne := "TestVcrIsolationOne"
	testTwo := "TestVcrIsolationTwo"
	for _, name := range []string{testOne, testTwo} {
		c := cassette.New(filepath.Join(vcrDir, vcrFileName(name)))
		if err := c.Save(); err != nil {
			t.Fatalf("failed saving empty cassette for %s: %v", name, err)
		}
	}
	t.Cleanup(func() {
		configsLock.Lock()
		delete(configs, testOne)
		delete(configs, testTwo)
		configsLock.Unlock()
	})

	t.Setenv("VCR_PATH", vcrDir)
	t.Setenv("VCR_MODE", "REPLAYING")

	fnVcrOneA, err := MuxedProviders(testOne)
	if err != nil {
		t.Fatalf("MuxedProviders(%s A) failed: %v", testOne, err)
	}
	srvVcrOneA := fnVcrOneA()

	fnVcrTwo, err := MuxedProviders(testTwo)
	if err != nil {
		t.Fatalf("MuxedProviders(%s) failed: %v", testTwo, err)
	}
	srvVcrTwo := fnVcrTwo()

	fnVcrOneB, err := MuxedProviders(testOne)
	if err != nil {
		t.Fatalf("MuxedProviders(%s B) failed: %v", testOne, err)
	}
	srvVcrOneB := fnVcrOneB()

	configureServerWithFakeCreds(t, srvVcrOneA, schemaResp.Provider, "vcr1", "vcr-proj-one", "us-west1", "us-west1-a")
	configureServerWithFakeCreds(t, srvVcrTwo, schemaResp.Provider, "vcr2", "vcr-proj-two", "us-east4", "us-east4-b")
	configureServerWithFakeCreds(t, srvVcrOneB, schemaResp.Provider, "vcr1", "vcr-proj-one", "us-west1", "us-west1-a")

	assertProviderConfigReadback(t, srvVcrOneA, schemaResp, "vcr-proj-one", "us-west1", "us-west1-a")
	assertProviderConfigReadback(t, srvVcrTwo, schemaResp, "vcr-proj-two", "us-east4", "us-east4-b")
	assertProviderConfigReadback(t, srvVcrOneB, schemaResp, "vcr-proj-one", "us-west1", "us-west1-a")

	configsLock.RLock()
	cfgOne := configs[testOne]
	cfgTwo := configs[testTwo]
	configsLock.RUnlock()
	if cfgOne == nil || cfgTwo == nil {
		t.Fatalf("expected VCR configs to be populated for both test names: one=%v two=%v", cfgOne, cfgTwo)
	}
	if cfgOne == cfgTwo {
		t.Fatal("expected distinct *transport_tpg.Config pointers for different VCR test names")
	}
}

func TestCachedProviderServer_ConcurrentRaceSafety(t *testing.T) {
	UnsetTestProviderConfigEnvs(t)
	ctx := context.Background()

	const workers = 24
	var wg sync.WaitGroup
	errCh := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			testName := fmt.Sprintf("TestConcurrentRace_%d", id)
			fn, err := MuxedProviders(testName)
			if err != nil {
				errCh <- fmt.Errorf("worker %d MuxedProviders: %w", id, err)
				return
			}
			srv := fn()

			if _, err := srv.GetMetadata(ctx, &tfprotov5.GetMetadataRequest{}); err != nil {
				errCh <- fmt.Errorf("worker %d GetMetadata: %w", id, err)
				return
			}
			schemaResp, err := srv.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{})
			if err != nil {
				errCh <- fmt.Errorf("worker %d GetProviderSchema: %w", id, err)
				return
			}
			if _, err := srv.GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{}); err != nil {
				errCh <- fmt.Errorf("worker %d GetResourceIdentitySchemas: %w", id, err)
				return
			}
			if _, err := srv.GetFunctions(ctx, &tfprotov5.GetFunctionsRequest{}); err != nil {
				errCh <- fmt.Errorf("worker %d GetFunctions: %w", id, err)
				return
			}

			argDV, err := tfprotov5.NewDynamicValue(tftypes.String, tftypes.NewValue(tftypes.String, fmt.Sprintf("us-central1-%d", id)))
			if err != nil {
				errCh <- fmt.Errorf("worker %d NewDynamicValue: %w", id, err)
				return
			}
			if fnResp, err := srv.CallFunction(ctx, &tfprotov5.CallFunctionRequest{
				Name:      "region_from_zone",
				Arguments: []*tfprotov5.DynamicValue{&argDV},
			}); err != nil || fnResp.Error != nil {
				errCh <- fmt.Errorf("worker %d CallFunction: err=%v fnErr=%v", id, err, fnResp.Error)
				return
			}

			wantProj := fmt.Sprintf("race-proj-%d", id)
			wantRegion := fmt.Sprintf("us-central%d", id%9+1)
			wantZone := fmt.Sprintf("%s-a", wantRegion)
			if err := configureServerWithFakeCredsErr(srv, schemaResp.Provider, fmt.Sprintf("worker-%d", id), wantProj, wantRegion, wantZone); err != nil {
				errCh <- fmt.Errorf("worker %d configure: %w", id, err)
				return
			}
			if err := verifyProviderConfigReadbackErr(srv, schemaResp, wantProj, wantRegion, wantZone); err != nil {
				errCh <- fmt.Errorf("worker %d readback: %w", id, err)
				return
			}
		}(i)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

func TestCachedProviderServer_EndToEndUnitTestCLI(t *testing.T) {
	if _, err := exec.LookPath("terraform"); err != nil {
		t.Skip("skipping CLI integration subtest: terraform binary not in PATH")
	}
	UnsetTestProviderConfigEnvs(t)
	t.Setenv("GOOGLE_OAUTH_ACCESS_TOKEN", "fake-unit-test-access-token")

	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: ProtoV5ProviderFactories(t),
		ProtoV6ProviderFactories: ProtoV6ProviderFactories(t),
		Steps: []resource.TestStep{
			{
				Config: `
provider "google" {
  project = "cli-unit-proj"
  region  = "us-west1"
  zone    = "us-west1-b"
}

action "google_compute_instance_reset" "test" {
  config {
    instance = "projects/cli-unit-proj/zones/us-west1-b/instances/my-vm"
    timeout  = "0s"
  }
}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("Invalid Duration"),
			},
			{
				Config: `
provider "google" {
  project = "cli-unit-proj"
  region  = "us-west1"
  zone    = "us-west1-b"
}

data "google_provider_config_sdk" "sdk" {}

data "google_provider_config_plugin_framework" "fw" {}

ephemeral "google_client_config" "ephem" {}

provider "echo" {
  data = ephemeral.google_client_config.ephem
}

resource "echo" "test" {}

output "parsed_name" {
  value = provider::google::name_from_id("projects/cli-unit-proj/topics/my-topic")
}

output "parsed_region" {
  value = provider::google::region_from_zone("us-west1-b")
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.google_provider_config_sdk.sdk", "project", "cli-unit-proj"),
					resource.TestCheckResourceAttr("data.google_provider_config_sdk.sdk", "region", "us-west1"),
					resource.TestCheckResourceAttr("data.google_provider_config_sdk.sdk", "zone", "us-west1-b"),
					resource.TestCheckResourceAttr("data.google_provider_config_plugin_framework.fw", "project", "cli-unit-proj"),
					resource.TestCheckResourceAttr("data.google_provider_config_plugin_framework.fw", "region", "us-west1"),
					resource.TestCheckResourceAttr("data.google_provider_config_plugin_framework.fw", "zone", "us-west1-b"),
					resource.TestCheckResourceAttr(EchoResourceName, "data.project", "cli-unit-proj"),
					resource.TestCheckResourceAttr(EchoResourceName, "data.region", "us-west1"),
					resource.TestCheckResourceAttr(EchoResourceName, "data.zone", "us-west1-b"),
					resource.TestCheckOutput("parsed_name", "my-topic"),
					resource.TestCheckOutput("parsed_region", "us-west1"),
				),
			},
		},
	})
}

func silenceStdLog(b *testing.B) {
	b.Helper()
	orig := log.Writer()
	log.SetOutput(io.Discard)
	b.Cleanup(func() {
		log.SetOutput(orig)
	})
}

func BenchmarkProviderServer_SchemaOnly_Uncached(b *testing.B) {
	silenceStdLog(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		srv, err := newUncachedMuxServer("BenchmarkUncached")
		if err != nil {
			b.Fatal(err)
		}
		if _, err := srv.GetMetadata(ctx, &tfprotov5.GetMetadataRequest{}); err != nil {
			b.Fatal(err)
		}
		if _, err := srv.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{}); err != nil {
			b.Fatal(err)
		}
		if _, err := srv.GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProviderServer_SchemaOnly_Cached(b *testing.B) {
	silenceStdLog(b)
	ctx := context.Background()
	// Warm the process-wide sync.OnceValues cache once outside the timed loop.
	warmFn, err := MuxedProviders("BenchmarkWarm")
	if err != nil {
		b.Fatal(err)
	}
	warmSrv := warmFn()
	if _, err := warmSrv.GetMetadata(ctx, &tfprotov5.GetMetadataRequest{}); err != nil {
		b.Fatal(err)
	}
	if _, err := warmSrv.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{}); err != nil {
		b.Fatal(err)
	}
	if _, err := warmSrv.GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{}); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fn, err := MuxedProviders("BenchmarkCached")
		if err != nil {
			b.Fatal(err)
		}
		srv := fn()
		if _, err := srv.GetMetadata(ctx, &tfprotov5.GetMetadataRequest{}); err != nil {
			b.Fatal(err)
		}
		if _, err := srv.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{}); err != nil {
			b.Fatal(err)
		}
		if _, err := srv.GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{}); err != nil {
			b.Fatal(err)
		}
	}
}

func setupVcrBenchEnv(b *testing.B, testName string) (*tfprotov5.DynamicValue, *tfprotov5.DynamicValue) {
	b.Helper()
	ctx := context.Background()
	vcrDir := b.TempDir()
	c := cassette.New(filepath.Join(vcrDir, vcrFileName(testName)))
	if err := c.Save(); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		configsLock.Lock()
		delete(configs, testName)
		configsLock.Unlock()
	})
	b.Setenv("VCR_PATH", vcrDir)
	b.Setenv("VCR_MODE", "REPLAYING")

	warmFn, err := MuxedProviders(testName)
	if err != nil {
		b.Fatal(err)
	}
	warmSrv := warmFn()
	if _, err := warmSrv.GetMetadata(ctx, &tfprotov5.GetMetadataRequest{}); err != nil {
		b.Fatal(err)
	}
	schemaResp, err := warmSrv.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{})
	if err != nil {
		b.Fatal(err)
	}
	if _, err := warmSrv.GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{}); err != nil {
		b.Fatal(err)
	}
	providerCfg := newObjectDynamicValue(b, schemaResp.Provider, map[string]tftypes.Value{
		"credentials": tftypes.NewValue(tftypes.String, GenerateFakeCredentialsJson("bench")),
		"project":     tftypes.NewValue(tftypes.String, "bench-proj"),
		"region":      tftypes.NewValue(tftypes.String, "us-central1"),
		"zone":        tftypes.NewValue(tftypes.String, "us-central1-a"),
	})
	topicCfg := newObjectDynamicValue(b, schemaResp.ResourceSchemas["google_pubsub_topic"], map[string]tftypes.Value{
		"name": tftypes.NewValue(tftypes.String, "bench-topic"),
	})
	// Prime VCR's per-testName config cache (matching CLI calls 2..N in every VCR test).
	configureServerWithFakeCreds(b, warmSrv, schemaResp.Provider, "bench", "bench-proj", "us-central1", "us-central1-a")
	return providerCfg, topicCfg
}

func BenchmarkProviderServer_StatefulVCR_Uncached(b *testing.B) {
	silenceStdLog(b)
	ctx := context.Background()
	const testName = "TestBenchStatefulVcrUncached"
	providerCfg, topicCfg := setupVcrBenchEnv(b, testName)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		srv, err := newUncachedMuxServer(testName)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := srv.GetMetadata(ctx, &tfprotov5.GetMetadataRequest{}); err != nil {
			b.Fatal(err)
		}
		if _, err := srv.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{}); err != nil {
			b.Fatal(err)
		}
		if _, err := srv.GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{}); err != nil {
			b.Fatal(err)
		}
		prepResp, err := srv.PrepareProviderConfig(ctx, &tfprotov5.PrepareProviderConfigRequest{Config: providerCfg})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := srv.ConfigureProvider(ctx, &tfprotov5.ConfigureProviderRequest{
			TerraformVersion: "1.14.3",
			Config:           prepResp.PreparedConfig,
		}); err != nil {
			b.Fatal(err)
		}
		if _, err := srv.ValidateResourceTypeConfig(ctx, &tfprotov5.ValidateResourceTypeConfigRequest{
			TypeName: "google_pubsub_topic",
			Config:   topicCfg,
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProviderServer_StatefulVCR_Cached(b *testing.B) {
	silenceStdLog(b)
	ctx := context.Background()
	const testName = "TestBenchStatefulVcrCached"
	providerCfg, topicCfg := setupVcrBenchEnv(b, testName)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fn, err := MuxedProviders(testName)
		if err != nil {
			b.Fatal(err)
		}
		srv := fn()
		if _, err := srv.GetMetadata(ctx, &tfprotov5.GetMetadataRequest{}); err != nil {
			b.Fatal(err)
		}
		if _, err := srv.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{}); err != nil {
			b.Fatal(err)
		}
		if _, err := srv.GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{}); err != nil {
			b.Fatal(err)
		}
		prepResp, err := srv.PrepareProviderConfig(ctx, &tfprotov5.PrepareProviderConfigRequest{Config: providerCfg})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := srv.ConfigureProvider(ctx, &tfprotov5.ConfigureProviderRequest{
			TerraformVersion: "1.14.3",
			Config:           prepResp.PreparedConfig,
		}); err != nil {
			b.Fatal(err)
		}
		if _, err := srv.ValidateResourceTypeConfig(ctx, &tfprotov5.ValidateResourceTypeConfigRequest{
			TypeName: "google_pubsub_topic",
			Config:   topicCfg,
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProviderServer_ConfigureAndValidate_Uncached(b *testing.B) {
	silenceStdLog(b)
	ctx := context.Background()
	warmFn, err := MuxedProviders("BenchmarkSetup")
	if err != nil {
		b.Fatal(err)
	}
	schemaResp, err := warmFn().GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{})
	if err != nil {
		b.Fatal(err)
	}
	providerCfg := newObjectDynamicValue(b, schemaResp.Provider, map[string]tftypes.Value{
		"credentials": tftypes.NewValue(tftypes.String, GenerateFakeCredentialsJson("bench")),
		"project":     tftypes.NewValue(tftypes.String, "bench-proj"),
		"region":      tftypes.NewValue(tftypes.String, "us-central1"),
		"zone":        tftypes.NewValue(tftypes.String, "us-central1-a"),
	})
	topicCfg := newObjectDynamicValue(b, schemaResp.ResourceSchemas["google_pubsub_topic"], map[string]tftypes.Value{
		"name": tftypes.NewValue(tftypes.String, "bench-topic"),
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		srv, err := newUncachedMuxServer("BenchmarkUncachedConfigure")
		if err != nil {
			b.Fatal(err)
		}
		if _, err := srv.GetMetadata(ctx, &tfprotov5.GetMetadataRequest{}); err != nil {
			b.Fatal(err)
		}
		if _, err := srv.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{}); err != nil {
			b.Fatal(err)
		}
		if _, err := srv.GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{}); err != nil {
			b.Fatal(err)
		}
		prepResp, err := srv.PrepareProviderConfig(ctx, &tfprotov5.PrepareProviderConfigRequest{Config: providerCfg})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := srv.ConfigureProvider(ctx, &tfprotov5.ConfigureProviderRequest{
			TerraformVersion: "1.14.3",
			Config:           prepResp.PreparedConfig,
		}); err != nil {
			b.Fatal(err)
		}
		if _, err := srv.ValidateResourceTypeConfig(ctx, &tfprotov5.ValidateResourceTypeConfigRequest{
			TypeName: "google_pubsub_topic",
			Config:   topicCfg,
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProviderServer_ConfigureAndValidate_Cached(b *testing.B) {
	silenceStdLog(b)
	ctx := context.Background()
	warmFn, err := MuxedProviders("BenchmarkWarm")
	if err != nil {
		b.Fatal(err)
	}
	warmSrv := warmFn()
	if _, err := warmSrv.GetMetadata(ctx, &tfprotov5.GetMetadataRequest{}); err != nil {
		b.Fatal(err)
	}
	schemaResp, err := warmSrv.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{})
	if err != nil {
		b.Fatal(err)
	}
	if _, err := warmSrv.GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{}); err != nil {
		b.Fatal(err)
	}
	providerCfg := newObjectDynamicValue(b, schemaResp.Provider, map[string]tftypes.Value{
		"credentials": tftypes.NewValue(tftypes.String, GenerateFakeCredentialsJson("bench")),
		"project":     tftypes.NewValue(tftypes.String, "bench-proj"),
		"region":      tftypes.NewValue(tftypes.String, "us-central1"),
		"zone":        tftypes.NewValue(tftypes.String, "us-central1-a"),
	})
	topicCfg := newObjectDynamicValue(b, schemaResp.ResourceSchemas["google_pubsub_topic"], map[string]tftypes.Value{
		"name": tftypes.NewValue(tftypes.String, "bench-topic"),
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fn, err := MuxedProviders("BenchmarkCachedConfigure")
		if err != nil {
			b.Fatal(err)
		}
		srv := fn()
		if _, err := srv.GetMetadata(ctx, &tfprotov5.GetMetadataRequest{}); err != nil {
			b.Fatal(err)
		}
		if _, err := srv.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{}); err != nil {
			b.Fatal(err)
		}
		if _, err := srv.GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{}); err != nil {
			b.Fatal(err)
		}
		prepResp, err := srv.PrepareProviderConfig(ctx, &tfprotov5.PrepareProviderConfigRequest{Config: providerCfg})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := srv.ConfigureProvider(ctx, &tfprotov5.ConfigureProviderRequest{
			TerraformVersion: "1.14.3",
			Config:           prepResp.PreparedConfig,
		}); err != nil {
			b.Fatal(err)
		}
		if _, err := srv.ValidateResourceTypeConfig(ctx, &tfprotov5.ValidateResourceTypeConfigRequest{
			TypeName: "google_pubsub_topic",
			Config:   topicCfg,
		}); err != nil {
			b.Fatal(err)
		}
	}
}

type stubVariantProviderServer struct {
	tfprotov5.ProviderServer
	variantResource string
}

func (s *stubVariantProviderServer) GetMetadata(_ context.Context, _ *tfprotov5.GetMetadataRequest) (*tfprotov5.GetMetadataResponse, error) {
	return &tfprotov5.GetMetadataResponse{
		Resources: []tfprotov5.ResourceMetadata{{TypeName: s.variantResource}},
	}, nil
}

func (s *stubVariantProviderServer) GetProviderSchema(_ context.Context, _ *tfprotov5.GetProviderSchemaRequest) (*tfprotov5.GetProviderSchemaResponse, error) {
	return &tfprotov5.GetProviderSchemaResponse{
		ResourceSchemas: map[string]*tfprotov5.Schema{
			s.variantResource: {Version: 1, Block: &tfprotov5.SchemaBlock{}},
		},
	}, nil
}

func (s *stubVariantProviderServer) GetResourceIdentitySchemas(_ context.Context, _ *tfprotov5.GetResourceIdentitySchemasRequest) (*tfprotov5.GetResourceIdentitySchemasResponse, error) {
	return &tfprotov5.GetResourceIdentitySchemasResponse{
		IdentitySchemas: map[string]*tfprotov5.ResourceIdentitySchema{},
	}, nil
}

func (s *stubVariantProviderServer) GetFunctions(_ context.Context, _ *tfprotov5.GetFunctionsRequest) (*tfprotov5.GetFunctionsResponse, error) {
	return &tfprotov5.GetFunctionsResponse{
		Functions: map[string]*tfprotov5.Function{},
	}, nil
}

func (s *stubVariantProviderServer) PrepareProviderConfig(_ context.Context, _ *tfprotov5.PrepareProviderConfigRequest) (*tfprotov5.PrepareProviderConfigResponse, error) {
	return &tfprotov5.PrepareProviderConfigResponse{}, nil
}

func (s *stubVariantProviderServer) ConfigureProvider(_ context.Context, _ *tfprotov5.ConfigureProviderRequest) (*tfprotov5.ConfigureProviderResponse, error) {
	return &tfprotov5.ConfigureProviderResponse{}, nil
}

func (s *stubVariantProviderServer) ValidateResourceTypeConfig(_ context.Context, _ *tfprotov5.ValidateResourceTypeConfigRequest) (*tfprotov5.ValidateResourceTypeConfigResponse, error) {
	return &tfprotov5.ValidateResourceTypeConfigResponse{}, nil
}

func (s *stubVariantProviderServer) StopProvider(_ context.Context, _ *tfprotov5.StopProviderRequest) (*tfprotov5.StopProviderResponse, error) {
	return &tfprotov5.StopProviderResponse{}, nil
}
