package provider_test

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-mux/tf5muxserver"

	"github.com/hashicorp/terraform-provider-google/google/fwprovider"
	"github.com/hashicorp/terraform-provider-google/google/provider"
	_ "github.com/hashicorp/terraform-provider-google/google/services/compute"
)

func TestEmbeddedSDKProviderServer_matchesSDKServer(t *testing.T) {
	ctx := context.Background()

	primary := provider.Provider()
	sdkServer := primary.GRPCProvider()
	embeddedServer := provider.NewEmbeddedSDKProviderServer(primary.GRPCProvider)()

	wantSchema, err := sdkServer.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("SDK server GetProviderSchema returned an error: %s", err)
	}
	gotSchema, err := embeddedServer.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("embedded SDK server GetProviderSchema returned an error: %s", err)
	}
	if len(wantSchema.ResourceSchemas) == 0 {
		t.Fatal("SDK server returned no resource schemas")
	}
	if !reflect.DeepEqual(gotSchema, wantSchema) {
		t.Fatalf("embedded SDK server GetProviderSchema differs from live SDK server")
	}

	wantIdentity, err := sdkServer.GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{})
	if err != nil {
		t.Fatalf("SDK server GetResourceIdentitySchemas returned an error: %s", err)
	}
	gotIdentity, err := embeddedServer.GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{})
	if err != nil {
		t.Fatalf("embedded SDK server GetResourceIdentitySchemas returned an error: %s", err)
	}
	if !reflect.DeepEqual(gotIdentity, wantIdentity) {
		t.Fatalf("embedded SDK server GetResourceIdentitySchemas differs from live SDK server")
	}

	providerType := providerConfigType(ctx, t, sdkServer)

	cases := map[string]map[string]tftypes.Value{
		"empty config":                {},
		"valid config":                {"project": tftypes.NewValue(tftypes.String, "my-project")},
		"invalid credentials":         {"credentials": tftypes.NewValue(tftypes.String, "this is not json")},
		"empty credentials":           {"credentials": tftypes.NewValue(tftypes.String, "")},
		"invalid custom endpoint":     {"compute_custom_endpoint": tftypes.NewValue(tftypes.String, "https://example.com/no-trailing-slash")},
		"invalid access token & cred": {"access_token": tftypes.NewValue(tftypes.String, ""), "credentials": tftypes.NewValue(tftypes.String, "{}")},
	}

	for name, attrs := range cases {
		t.Run(name, func(t *testing.T) {
			req := &tfprotov5.PrepareProviderConfigRequest{
				Config: providerConfigValue(t, providerType, attrs),
			}

			want, err := sdkServer.PrepareProviderConfig(ctx, req)
			if err != nil {
				t.Fatalf("SDK server PrepareProviderConfig returned an error: %s", err)
			}
			got, err := embeddedServer.PrepareProviderConfig(ctx, req)
			if err != nil {
				t.Fatalf("embedded SDK server PrepareProviderConfig returned an error: %s", err)
			}

			if diff := diagnosticsDiff(want.Diagnostics, got.Diagnostics); diff != "" {
				t.Errorf("diagnostics differ from the SDK server: %s", diff)
			}

			if want.PreparedConfig == nil || got.PreparedConfig == nil {
				if want.PreparedConfig != got.PreparedConfig {
					t.Errorf("PreparedConfig presence differs: SDK server %v, embedded SDK server %v", want.PreparedConfig, got.PreparedConfig)
				}
				return
			}
			if !bytes.Equal(want.PreparedConfig.MsgPack, got.PreparedConfig.MsgPack) {
				t.Errorf("PreparedConfig differs from the SDK server")
			}
		})
	}
}

func TestEmbeddedMuxServer_matchesStockMuxServer(t *testing.T) {
	ctx := context.Background()

	stockPrimary := provider.Provider()
	stockMux, err := tf5muxserver.NewMuxServer(ctx,
		stockPrimary.GRPCProvider,
		providerserver.NewProtocol5(fwprovider.New(stockPrimary)),
	)
	if err != nil {
		t.Fatalf("NewMuxServer failed: %s", err)
	}

	embeddedPrimary := provider.Provider()
	embeddedMux, err := tf5muxserver.NewMuxServer(ctx,
		provider.NewEmbeddedSDKProviderServer(embeddedPrimary.GRPCProvider),
		provider.NewEmbeddedFrameworkProviderServer(providerserver.NewProtocol5(fwprovider.New(embeddedPrimary))),
	)
	if err != nil {
		t.Fatalf("NewMuxServer failed: %s", err)
	}

	wantMeta, err := stockMux.ProviderServer().GetMetadata(ctx, &tfprotov5.GetMetadataRequest{})
	if err != nil {
		t.Fatalf("stock mux GetMetadata failed: %s", err)
	}
	gotMeta, err := embeddedMux.ProviderServer().GetMetadata(ctx, &tfprotov5.GetMetadataRequest{})
	if err != nil {
		t.Fatalf("embedded mux GetMetadata failed: %s", err)
	}
	sortMetadata(wantMeta)
	sortMetadata(gotMeta)
	if !reflect.DeepEqual(gotMeta, wantMeta) {
		t.Errorf("GetMetadata differs between embeddedMux and stockMux:\ngot:  %#v\nwant: %#v", gotMeta, wantMeta)
	}

	t.Run("GOOGLE_TERRAFORM_LIVE_SCHEMA fallback", func(t *testing.T) {
		t.Setenv("GOOGLE_TERRAFORM_LIVE_SCHEMA", "1")
		liveSchema, err := embeddedMux.ProviderServer().GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{})
		if err != nil {
			t.Fatalf("GetProviderSchema with GOOGLE_TERRAFORM_LIVE_SCHEMA=1 failed: %s", err)
		}
		wantSchema, err := stockMux.ProviderServer().GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{})
		if err != nil {
			t.Fatalf("stock GetProviderSchema failed: %s", err)
		}
		if !reflect.DeepEqual(liveSchema, wantSchema) {
			t.Errorf("GetProviderSchema with GOOGLE_TERRAFORM_LIVE_SCHEMA=1 differs from stock mux")
		}
	})
}

// providerConfigType returns the tftypes.Type of the provider config block.
func providerConfigType(ctx context.Context, t *testing.T, server tfprotov5.ProviderServer) tftypes.Object {
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

// providerConfigValue builds a provider config where every attribute is null
// except the ones in attrs.
func providerConfigValue(t *testing.T, providerType tftypes.Object, attrs map[string]tftypes.Value) *tfprotov5.DynamicValue {
	t.Helper()

	values := make(map[string]tftypes.Value, len(providerType.AttributeTypes))
	for name, attrType := range providerType.AttributeTypes {
		if v, ok := attrs[name]; ok {
			values[name] = v
			continue
		}
		values[name] = tftypes.NewValue(attrType, nil)
	}
	for name := range attrs {
		if _, ok := providerType.AttributeTypes[name]; !ok {
			t.Fatalf("test sets %q, which is not a provider schema attribute", name)
		}
	}

	dv, err := tfprotov5.NewDynamicValue(providerType, tftypes.NewValue(providerType, values))
	if err != nil {
		t.Fatalf("error building provider config value: %s", err)
	}
	return &dv
}

func diagnosticsDiff(want, got []*tfprotov5.Diagnostic) string {
	wantStrings := diagnosticStrings(want)
	gotStrings := diagnosticStrings(got)

	if !slices.Equal(wantStrings, gotStrings) {
		return "[" + strings.Join(wantStrings, ", ") + "] != [" + strings.Join(gotStrings, ", ") + "]"
	}
	return ""
}

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

func sortMetadata(resp *tfprotov5.GetMetadataResponse) {
	slices.SortFunc(resp.DataSources, func(a, b tfprotov5.DataSourceMetadata) int {
		return strings.Compare(a.TypeName, b.TypeName)
	})
	slices.SortFunc(resp.Resources, func(a, b tfprotov5.ResourceMetadata) int {
		return strings.Compare(a.TypeName, b.TypeName)
	})
	slices.SortFunc(resp.Functions, func(a, b tfprotov5.FunctionMetadata) int {
		return strings.Compare(a.Name, b.Name)
	})
	slices.SortFunc(resp.ListResources, func(a, b tfprotov5.ListResourceMetadata) int {
		return strings.Compare(a.TypeName, b.TypeName)
	})
	slices.SortFunc(resp.EphemeralResources, func(a, b tfprotov5.EphemeralResourceMetadata) int {
		return strings.Compare(a.TypeName, b.TypeName)
	})
}

func BenchmarkMuxedProviderSchema(b *testing.B) {
	ctx := context.Background()

	b.Run("Stock_LiveSchema", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			primary := provider.Provider()
			mux, err := tf5muxserver.NewMuxServer(ctx,
				primary.GRPCProvider,
				providerserver.NewProtocol5(fwprovider.New(primary)),
			)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := mux.ProviderServer().GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{}); err != nil {
				b.Fatal(err)
			}
			if _, err := mux.ProviderServer().GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{}); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Embedded_PrecomputedSchema", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			primary := provider.Provider()
			mux, err := tf5muxserver.NewMuxServer(ctx,
				provider.NewEmbeddedSDKProviderServer(primary.GRPCProvider),
				provider.NewEmbeddedFrameworkProviderServer(providerserver.NewProtocol5(fwprovider.New(primary))),
			)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := mux.ProviderServer().GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{}); err != nil {
				b.Fatal(err)
			}
			if _, err := mux.ProviderServer().GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{}); err != nil {
				b.Fatal(err)
			}
		}
	})
}
