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

func TestLazyProvider_resolvesOnDemand(t *testing.T) {
	ctx := context.Background()

	lazyP := provider.LazyProvider()
	lazyP.ResourcesMap["google_compute_network"] = nil
	fastServer := provider.NewFastValidateProviderServer(lazyP)

	metaResp, err := fastServer.GetMetadata(ctx, &tfprotov5.GetMetadataRequest{})
	if err != nil {
		t.Fatalf("lazy fastServer GetMetadata failed: %s", err)
	}
	if len(metaResp.Resources) == 0 {
		t.Fatal("expected non-empty resources metadata from LazyProvider")
	}

	// Obtain the object type for google_compute_network via a separate server
	// while lazyP.ResourcesMap["google_compute_network"] remains nil.
	schemaHelper := provider.NewFastValidateProviderServer(provider.LazyProvider())
	schemaResp, err := schemaHelper.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("lazy schemaHelper GetProviderSchema failed: %s", err)
	}
	netSchema, ok := schemaResp.ResourceSchemas["google_compute_network"]
	if !ok {
		t.Fatal("expected google_compute_network schema in GetProviderSchema response")
	}
	netObjType, ok := netSchema.ValueType().(tftypes.Object)
	if !ok {
		t.Fatalf("google_compute_network ValueType is %T, want tftypes.Object", netSchema.ValueType())
	}

	valResp, err := fastServer.ValidateResourceTypeConfig(ctx, &tfprotov5.ValidateResourceTypeConfigRequest{
		TypeName: "google_compute_network",
		Config:   providerConfigValue(t, netObjType, map[string]tftypes.Value{"name": tftypes.NewValue(tftypes.String, "my-net")}),
	})
	if err != nil {
		t.Fatalf("ValidateResourceTypeConfig failed: %s", err)
	}
	if len(valResp.Diagnostics) != 0 {
		t.Errorf("unexpected diagnostics from ValidateResourceTypeConfig: %v", valResp.Diagnostics)
	}
}

// TestFastValidateProviderServer_matchesSDKServer checks that the
// PrepareProviderConfig responses of the fast validation server are
// indistinguishable from the stock SDK server's, for both accepted and
// rejected provider configurations.
func TestFastValidateProviderServer_matchesSDKServer(t *testing.T) {
	ctx := context.Background()

	sdkServer := provider.Provider().GRPCProvider()
	fastServer := provider.NewFastValidateProviderServer(provider.Provider())

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
			got, err := fastServer.PrepareProviderConfig(ctx, req)
			if err != nil {
				t.Fatalf("fast validate server PrepareProviderConfig returned an error: %s", err)
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
		})
	}
}

// TestFastValidateProviderServer_servesRealSchema makes sure the wrapper does
// not accidentally serve the resource-less provider on any other RPC.
func TestFastValidateProviderServer_servesRealSchema(t *testing.T) {
	ctx := context.Background()

	sdkServer := provider.Provider().GRPCProvider()
	fastServer := provider.NewFastValidateProviderServer(provider.Provider())

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
}

func TestFastFrameworkProviderServer_matchesMuxServer(t *testing.T) {
	ctx := context.Background()

	stockPrimary := provider.Provider()
	stockMux, err := tf5muxserver.NewMuxServer(ctx,
		stockPrimary.GRPCProvider,
		providerserver.NewProtocol5(fwprovider.New(stockPrimary)),
	)
	if err != nil {
		t.Fatalf("NewMuxServer failed: %s", err)
	}

	fastPrimary := provider.Provider()
	fastMux, err := tf5muxserver.NewMuxServer(ctx,
		func() tfprotov5.ProviderServer { return provider.NewFastValidateProviderServer(fastPrimary) },
		provider.NewFastFrameworkProviderServer(providerserver.NewProtocol5(fwprovider.New(fastPrimary))),
	)
	if err != nil {
		t.Fatalf("NewMuxServer failed: %s", err)
	}

	wantMeta, err := stockMux.ProviderServer().GetMetadata(ctx, &tfprotov5.GetMetadataRequest{})
	if err != nil {
		t.Fatalf("stock mux GetMetadata failed: %s", err)
	}
	gotMeta, err := fastMux.ProviderServer().GetMetadata(ctx, &tfprotov5.GetMetadataRequest{})
	if err != nil {
		t.Fatalf("fast mux GetMetadata failed: %s", err)
	}
	sortMetadata(wantMeta)
	sortMetadata(gotMeta)
	if !reflect.DeepEqual(gotMeta, wantMeta) {
		t.Errorf("GetMetadata differs between fastMux and stockMux:\ngot:  %#v\nwant: %#v", gotMeta, wantMeta)
	}

	providerType := providerConfigType(ctx, t, stockMux.ProviderServer())

	cases := map[string]map[string]tftypes.Value{
		"empty config":                {},
		"valid config":                {"project": tftypes.NewValue(tftypes.String, "my-project"), "request_timeout": tftypes.NewValue(tftypes.String, "30s")},
		"invalid credentials":         {"credentials": tftypes.NewValue(tftypes.String, "this is not json")},
		"empty credentials":           {"credentials": tftypes.NewValue(tftypes.String, "")},
		"invalid custom endpoint":     {"compute_custom_endpoint": tftypes.NewValue(tftypes.String, "https://example.com/no-trailing-slash")},
		"invalid access token & cred": {"access_token": tftypes.NewValue(tftypes.String, ""), "credentials": tftypes.NewValue(tftypes.String, "{}")},
		"empty request_timeout":       {"request_timeout": tftypes.NewValue(tftypes.String, "")},
		"invalid request_timeout":     {"request_timeout": tftypes.NewValue(tftypes.String, "not-a-duration")},
		"negative request_timeout":    {"request_timeout": tftypes.NewValue(tftypes.String, "-5s")},
		"invalid poll_interval":       {"poll_interval": tftypes.NewValue(tftypes.String, "bad-interval")},
	}

	for name, attrs := range cases {
		t.Run(name, func(t *testing.T) {
			req := &tfprotov5.PrepareProviderConfigRequest{
				Config: providerConfigValue(t, providerType, attrs),
			}

			want, err := stockMux.ProviderServer().PrepareProviderConfig(ctx, req)
			if err != nil {
				t.Fatalf("stock mux server PrepareProviderConfig returned an error: %s", err)
			}
			got, err := fastMux.ProviderServer().PrepareProviderConfig(ctx, req)
			if err != nil {
				t.Fatalf("fast mux server PrepareProviderConfig returned an error: %s", err)
			}

			if diff := diagnosticsDiff(want.Diagnostics, got.Diagnostics); diff != "" {
				t.Errorf("diagnostics differ from the stock mux server: %s", diff)
			}
		})
	}

	// Verify ConfigureProvider on the fast Framework server succeeds without
	// panicking even when Primary.Meta() is nil or when Config is non-nil.
	fwSrv := provider.NewFastFrameworkProviderServer(providerserver.NewProtocol5(fwprovider.New(fastPrimary)))()
	cfgResp, err := fwSrv.ConfigureProvider(ctx, &tfprotov5.ConfigureProviderRequest{
		Config: providerConfigValue(t, providerType, map[string]tftypes.Value{
			"project": tftypes.NewValue(tftypes.String, "my-project"),
		}),
	})
	if err != nil {
		t.Fatalf("fast framework ConfigureProvider returned an error: %s", err)
	}
	if len(cfgResp.Diagnostics) != 0 {
		t.Errorf("fast framework ConfigureProvider returned unexpected diagnostics: %v", cfgResp.Diagnostics)
	}
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

// diagnosticsDiff returns a human readable description of the difference
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
