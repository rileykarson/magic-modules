package provider_test

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/hashicorp/terraform-provider-google/google/provider"
	_ "github.com/hashicorp/terraform-provider-google/google/services/compute"
)

// TestFastValidateProviderServer_matchesSDKServer checks that the
// PrepareProviderConfig responses of the fast validation server are
// indistinguishable from the stock SDK server's, for both accepted and
// rejected provider configurations across top-level attributes, nested blocks,
// ConflictsWith constraints, default values, and unknown values during plan.
func TestFastValidateProviderServer_matchesSDKServer(t *testing.T) {
	ctx := context.Background()

	p := provider.Provider()
	sdkServer := p.GRPCProvider()
	fastServer := provider.NewFastValidateProviderServer(p)

	providerType := providerConfigType(ctx, t, sdkServer)

	validExternalCreds := map[string]tftypes.Value{
		"audience":              tftypes.NewValue(tftypes.String, "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/prov"),
		"service_account_email": tftypes.NewValue(tftypes.String, "my-sa@my-project.iam.gserviceaccount.com"),
		"identity_token":        tftypes.NewValue(tftypes.String, "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.c2lnbmF0dXJl"),
	}

	cases := []struct {
		name      string
		attrs     map[string]tftypes.Value
		wantDiags bool
	}{
		{
			name:      "empty config",
			attrs:     map[string]tftypes.Value{},
			wantDiags: false,
		},
		{
			name: "valid config",
			attrs: map[string]tftypes.Value{
				"project":                         tftypes.NewValue(tftypes.String, "my-project"),
				"region":                          tftypes.NewValue(tftypes.String, "us-central1"),
				"compute_custom_endpoint":         tftypes.NewValue(tftypes.String, "https://example.com/compute/v1/"),
				"add_terraform_attribution_label": tftypes.NewValue(tftypes.Bool, false),
			},
			wantDiags: false,
		},
		{
			name:      "invalid credentials",
			attrs:     map[string]tftypes.Value{"credentials": tftypes.NewValue(tftypes.String, "this is not json")},
			wantDiags: true,
		},
		{
			name:      "empty credentials",
			attrs:     map[string]tftypes.Value{"credentials": tftypes.NewValue(tftypes.String, "")},
			wantDiags: true,
		},
		{
			name:      "invalid custom endpoint",
			attrs:     map[string]tftypes.Value{"compute_custom_endpoint": tftypes.NewValue(tftypes.String, "https://example.com/no-trailing-slash")},
			wantDiags: true,
		},
		{
			name: "invalid access token & cred",
			attrs: map[string]tftypes.Value{
				"access_token": tftypes.NewValue(tftypes.String, ""),
				"credentials":  tftypes.NewValue(tftypes.String, "{}"),
			},
			wantDiags: true,
		},
		{
			name: "conflicts with access_token and external_credentials",
			attrs: map[string]tftypes.Value{
				"access_token":         tftypes.NewValue(tftypes.String, "fake-access-token"),
				"external_credentials": nestedBlockList(t, providerType, "external_credentials", validExternalCreds),
			},
			wantDiags: true,
		},
		{
			name: "valid batching block",
			attrs: map[string]tftypes.Value{
				"batching": nestedBlockList(t, providerType, "batching", map[string]tftypes.Value{
					"send_after":      tftypes.NewValue(tftypes.String, "10s"),
					"enable_batching": tftypes.NewValue(tftypes.Bool, true),
				}),
			},
			wantDiags: false,
		},
		{
			name: "invalid batching negative duration",
			attrs: map[string]tftypes.Value{
				"batching": nestedBlockList(t, providerType, "batching", map[string]tftypes.Value{
					"send_after": tftypes.NewValue(tftypes.String, "-5s"),
				}),
			},
			wantDiags: true,
		},
		{
			name: "invalid batching max items exceeded",
			attrs: map[string]tftypes.Value{
				"batching": nestedBlockList(t, providerType, "batching",
					map[string]tftypes.Value{"send_after": tftypes.NewValue(tftypes.String, "5s")},
					map[string]tftypes.Value{"send_after": tftypes.NewValue(tftypes.String, "10s")},
				),
			},
			wantDiags: true,
		},
		{
			name: "valid external_credentials block",
			attrs: map[string]tftypes.Value{
				"external_credentials": nestedBlockList(t, providerType, "external_credentials", validExternalCreds),
			},
			wantDiags: false,
		},
		{
			name: "invalid external_credentials missing required audience and bad email/jwt",
			attrs: map[string]tftypes.Value{
				"external_credentials": nestedBlockList(t, providerType, "external_credentials", map[string]tftypes.Value{
					"service_account_email": tftypes.NewValue(tftypes.String, "not-an-email"),
					"identity_token":        tftypes.NewValue(tftypes.String, "not-a-jwt"),
				}),
			},
			wantDiags: true,
		},
		{
			name: "null element in scopes list",
			attrs: map[string]tftypes.Value{
				"scopes": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{
					tftypes.NewValue(tftypes.String, "https://www.googleapis.com/auth/cloud-platform"),
					tftypes.NewValue(tftypes.String, nil),
				}),
			},
			wantDiags: true,
		},
		{
			name: "unknown top-level attributes skip validation and conflicts",
			attrs: map[string]tftypes.Value{
				"project":                         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
				"credentials":                     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
				"access_token":                    tftypes.NewValue(tftypes.String, "fake-access-token"),
				"compute_custom_endpoint":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
				"add_terraform_attribution_label": tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
			},
			wantDiags: false,
		},
		{
			name: "unknown nested blocks and map elements",
			attrs: map[string]tftypes.Value{
				"batching": tftypes.NewValue(providerType.AttributeTypes["batching"], tftypes.UnknownValue),
				"external_credentials": nestedBlockList(t, providerType, "external_credentials", map[string]tftypes.Value{
					"audience":              validExternalCreds["audience"],
					"service_account_email": validExternalCreds["service_account_email"],
					"identity_token":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
				}),
				"default_labels": tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, map[string]tftypes.Value{
					"env": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
				}),
			},
			wantDiags: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &tfprotov5.PrepareProviderConfigRequest{
				Config: providerConfigValue(t, providerType, tc.attrs),
			}

			want, err := sdkServer.PrepareProviderConfig(ctx, req)
			if err != nil {
				t.Fatalf("SDK server PrepareProviderConfig returned an error: %s", err)
			}
			got, err := fastServer.PrepareProviderConfig(ctx, req)
			if err != nil {
				t.Fatalf("fast validate server PrepareProviderConfig returned an error: %s", err)
			}

			if (len(got.Diagnostics) > 0) != tc.wantDiags {
				t.Errorf("got diagnostics %v, wantDiags=%v", diagnosticStrings(got.Diagnostics), tc.wantDiags)
			}
			if diff := diagnosticsDiff(want.Diagnostics, got.Diagnostics); diff != "" {
				t.Errorf("diagnostics differ from the SDK server: %s", diff)
			}

			if want.PreparedConfig == nil || got.PreparedConfig == nil {
				if want.PreparedConfig != got.PreparedConfig {
					t.Errorf("PreparedConfig presence differs: SDK server %v, fast validate server %v", want.PreparedConfig, got.PreparedConfig)
				}
				return
			}
			if !bytes.Equal(want.PreparedConfig.MsgPack, got.PreparedConfig.MsgPack) {
				t.Errorf("PreparedConfig differs from the SDK server")
			}

			// Verify schema default injection (add_terraform_attribution_label defaults to true).
			unmarshaled, err := got.PreparedConfig.Unmarshal(providerType)
			if err != nil {
				t.Fatalf("failed to unmarshal PreparedConfig: %s", err)
			}
			var vals map[string]tftypes.Value
			if err := unmarshaled.As(&vals); err != nil {
				t.Fatalf("failed to extract PreparedConfig map: %s", err)
			}
			attrVal := vals["add_terraform_attribution_label"]
			if configured, ok := tc.attrs["add_terraform_attribution_label"]; ok && !configured.IsKnown() {
				if attrVal.IsKnown() {
					t.Errorf("expected add_terraform_attribution_label to remain unknown, got %v", attrVal)
				}
			} else {
				wantBool := true
				if ok {
					_ = configured.As(&wantBool)
				}
				var gotBool bool
				if err := attrVal.As(&gotBool); err != nil || gotBool != wantBool {
					t.Errorf("PreparedConfig add_terraform_attribution_label = %v (err=%v), want %v", gotBool, err, wantBool)
				}
			}
		})
	}
}

// TestFastValidateProviderServer_servesRealSchema makes sure the wrapper does
// not accidentally serve the resource-less provider on any other RPC.
func TestFastValidateProviderServer_servesRealSchema(t *testing.T) {
	ctx := context.Background()

	if provider.NewFastValidateProviderServer(nil) == nil {
		t.Fatal("expected non-nil server wrapper for nil provider")
	}

	p := provider.Provider()
	sdkServer := p.GRPCProvider()
	fastServer := provider.NewFastValidateProviderServer(p)

	want, err := sdkServer.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("SDK server GetProviderSchema returned an error: %s", err)
	}
	got, err := fastServer.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("fast validate server GetProviderSchema returned an error: %s", err)
	}

	if len(want.ResourceSchemas) == 0 {
		t.Fatal("SDK server returned no resource schemas, the test cannot detect a regression")
	}
	if len(got.ResourceSchemas) != len(want.ResourceSchemas) {
		t.Errorf("fast validate server returned %d resource schemas, want %d", len(got.ResourceSchemas), len(want.ResourceSchemas))
	}
	if len(got.DataSourceSchemas) != len(want.DataSourceSchemas) {
		t.Errorf("fast validate server returned %d data source schemas, want %d", len(got.DataSourceSchemas), len(want.DataSourceSchemas))
	}
	if got.ProviderMeta == nil {
		t.Errorf("fast validate server returned nil ProviderMeta schema")
	}

	wantMeta, err := sdkServer.GetMetadata(ctx, &tfprotov5.GetMetadataRequest{})
	if err != nil {
		t.Fatalf("SDK server GetMetadata returned an error: %s", err)
	}
	gotMeta, err := fastServer.GetMetadata(ctx, &tfprotov5.GetMetadataRequest{})
	if err != nil {
		t.Fatalf("fast validate server GetMetadata returned an error: %s", err)
	}
	if len(gotMeta.Resources) != len(wantMeta.Resources) || len(gotMeta.Resources) == 0 {
		t.Errorf("fast validate server GetMetadata returned %d resources, want %d", len(gotMeta.Resources), len(wantMeta.Resources))
	}
	if len(gotMeta.DataSources) != len(wantMeta.DataSources) || len(gotMeta.DataSources) == 0 {
		t.Errorf("fast validate server GetMetadata returned %d data sources, want %d", len(gotMeta.DataSources), len(wantMeta.DataSources))
	}

	wantIdent, err := sdkServer.GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{})
	if err != nil {
		t.Fatalf("SDK server GetResourceIdentitySchemas returned an error: %s", err)
	}
	gotIdent, err := fastServer.GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{})
	if err != nil {
		t.Fatalf("fast validate server GetResourceIdentitySchemas returned an error: %s", err)
	}
	if len(gotIdent.IdentitySchemas) != len(wantIdent.IdentitySchemas) || len(gotIdent.IdentitySchemas) == 0 {
		t.Errorf("fast validate server GetResourceIdentitySchemas returned %d schemas, want %d", len(gotIdent.IdentitySchemas), len(wantIdent.IdentitySchemas))
	}

	// Verify ValidateResourceTypeConfig and ValidateDataSourceConfig delegate to
	// the full provider rather than panicking on a nil schema lookup in configOnly.
	addrResType := want.ResourceSchemas["google_compute_address"].ValueType().(tftypes.Object)
	resReq := &tfprotov5.ValidateResourceTypeConfigRequest{
		TypeName: "google_compute_address",
		Config:   providerConfigValue(t, addrResType, nil),
	}
	wantResVal, err := sdkServer.ValidateResourceTypeConfig(ctx, resReq)
	if err != nil {
		t.Fatalf("SDK server ValidateResourceTypeConfig error: %s", err)
	}
	gotResVal, err := fastServer.ValidateResourceTypeConfig(ctx, resReq)
	if err != nil {
		t.Fatalf("fast server ValidateResourceTypeConfig error: %s", err)
	}
	if len(gotResVal.Diagnostics) == 0 || diagnosticsDiff(wantResVal.Diagnostics, gotResVal.Diagnostics) != "" {
		t.Errorf("ValidateResourceTypeConfig diagnostics mismatch: got %v, want %v", diagnosticStrings(gotResVal.Diagnostics), diagnosticStrings(wantResVal.Diagnostics))
	}

	addrDataType := want.DataSourceSchemas["google_compute_address"].ValueType().(tftypes.Object)
	dsReq := &tfprotov5.ValidateDataSourceConfigRequest{
		TypeName: "google_compute_address",
		Config:   providerConfigValue(t, addrDataType, nil),
	}
	wantDSVal, err := sdkServer.ValidateDataSourceConfig(ctx, dsReq)
	if err != nil {
		t.Fatalf("SDK server ValidateDataSourceConfig error: %s", err)
	}
	gotDSVal, err := fastServer.ValidateDataSourceConfig(ctx, dsReq)
	if err != nil {
		t.Fatalf("fast server ValidateDataSourceConfig error: %s", err)
	}
	if len(gotDSVal.Diagnostics) == 0 || diagnosticsDiff(wantDSVal.Diagnostics, gotDSVal.Diagnostics) != "" {
		t.Errorf("ValidateDataSourceConfig diagnostics mismatch: got %v, want %v", diagnosticStrings(gotDSVal.Diagnostics), diagnosticStrings(wantDSVal.Diagnostics))
	}
}

func BenchmarkPrepareProviderConfig(b *testing.B) {
	ctx := context.Background()

	p := provider.Provider()
	sdkServer := p.GRPCProvider()
	fastServer := provider.NewFastValidateProviderServer(p)

	providerType := providerConfigType(ctx, b, sdkServer)
	req := &tfprotov5.PrepareProviderConfigRequest{
		Config: providerConfigValue(b, providerType, map[string]tftypes.Value{
			"project": tftypes.NewValue(tftypes.String, "my-project"),
			"region":  tftypes.NewValue(tftypes.String, "us-central1"),
		}),
	}

	b.Run("SDKServer", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			resp, err := sdkServer.PrepareProviderConfig(ctx, req)
			if err != nil {
				b.Fatalf("PrepareProviderConfig returned an error: %s", err)
			}
			if len(resp.Diagnostics) > 0 {
				b.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
			}
		}
	})

	b.Run("FastValidateServer", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			resp, err := fastServer.PrepareProviderConfig(ctx, req)
			if err != nil {
				b.Fatalf("PrepareProviderConfig returned an error: %s", err)
			}
			if len(resp.Diagnostics) > 0 {
				b.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
			}
		}
	})
}

// providerConfigType returns the tftypes.Type of the provider config block.
func providerConfigType(ctx context.Context, t testing.TB, server tfprotov5.ProviderServer) tftypes.Object {
	t.Helper()

	resp, err := server.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("GetProviderSchema returned an error: %s", err)
	}

	providerType, ok := resp.Provider.ValueType().(tftypes.Object)
	if !ok {
		t.Fatalf("provider schema type is %T, want tftypes.Object", resp.Provider.ValueType())
	}
	return providerType
}

// providerConfigValue builds a config DynamicValue where every attribute in
// objType is null except the ones in attrs.
func providerConfigValue(t testing.TB, objType tftypes.Object, attrs map[string]tftypes.Value) *tfprotov5.DynamicValue {
	t.Helper()

	dv, err := tfprotov5.NewDynamicValue(objType, objectValue(t, objType, attrs))
	if err != nil {
		t.Fatalf("error building config value: %s", err)
	}
	return &dv
}

// nestedBlockList builds a tftypes.List value for a nested block attribute
// (such as "batching" or "external_credentials") from a slice of attribute maps,
// filling any omitted attributes in each block with null.
func nestedBlockList(t testing.TB, providerType tftypes.Object, blockName string, blocks ...map[string]tftypes.Value) tftypes.Value {
	t.Helper()

	listType, ok := providerType.AttributeTypes[blockName].(tftypes.List)
	if !ok {
		t.Fatalf("provider attribute %q has type %T, want tftypes.List", blockName, providerType.AttributeTypes[blockName])
	}
	elemObjType, ok := listType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatalf("provider attribute %q element has type %T, want tftypes.Object", blockName, listType.ElementType)
	}

	elems := make([]tftypes.Value, 0, len(blocks))
	for _, block := range blocks {
		elems = append(elems, objectValue(t, elemObjType, block))
	}
	return tftypes.NewValue(listType, elems)
}

// objectValue builds a tftypes.Object value where every attribute is null
// except the ones in attrs.
func objectValue(t testing.TB, objType tftypes.Object, attrs map[string]tftypes.Value) tftypes.Value {
	t.Helper()

	values := make(map[string]tftypes.Value, len(objType.AttributeTypes))
	for name, attrType := range objType.AttributeTypes {
		if v, ok := attrs[name]; ok {
			values[name] = v
			continue
		}
		values[name] = tftypes.NewValue(attrType, nil)
	}
	for name := range attrs {
		if _, ok := objType.AttributeTypes[name]; !ok {
			t.Fatalf("test sets %q, which is not a schema attribute", name)
		}
	}
	return tftypes.NewValue(objType, values)
}

// diagnosticsDiff returns a human-readable description of the difference
// between two diagnostic slices, or "" when they hold the same diagnostics.
//
// The comparison ignores ordering: helper/schema walks the provider schema
// map to produce these, so two calls against the same server can return the
// same diagnostics in a different order.
func diagnosticsDiff(want, got []*tfprotov5.Diagnostic) string {
	wantStrings := diagnosticStrings(want)
	gotStrings := diagnosticStrings(got)

	if !slices.Equal(wantStrings, gotStrings) {
		return "[" + strings.Join(wantStrings, ", ") + "] != [" + strings.Join(gotStrings, ", ") + "]"
	}
	return ""
}

// diagnosticStrings renders diagnostics as a sorted, comparable list.
func diagnosticStrings(diags []*tfprotov5.Diagnostic) []string {
	out := make([]string, 0, len(diags))
	for _, d := range diags {
		if d == nil {
			out = append(out, "<nil>")
			continue
		}
		attribute := "<nil>"
		if d.Attribute != nil {
			attribute = d.Attribute.String()
		}
		out = append(out, fmt.Sprintf("%s: %s / %s / %s", d.Severity, d.Summary, d.Detail, attribute))
	}
	slices.Sort(out)
	return out
}
