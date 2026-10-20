package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// fastValidateProviderServer serves every RPC from the real SDKv2 provider
// server except PrepareProviderConfig (the protocol v5 name for
// ValidateProviderConfig), which it serves from a second provider server built
// over a provider that has the same top-level Schema and ProviderMetaSchema but
// no resources or data sources.
//
// helper/schema.Provider.Validate, which PrepareProviderConfig calls, runs
// Provider.InternalValidate first, and that walks every registered resource and
// data source schema. The provider has ~1,950 of them, so the walk dominates
// the RPC, and Terraform issues the RPC once per provider instance on every
// plan and apply. PrepareProviderConfig itself only ever reads Provider.Schema
// (to unmarshal, apply defaults, and validate the config block) and
// Provider.ProviderMetaSchema (which InternalValidate checks for write-only
// attributes), so a resource-less provider returns the same PreparedConfig and
// the same user-facing diagnostics.
//
// The resource and data source half of InternalValidate is not lost: it is
// checked for every resource by TestProvider in provider_test.go, which is the
// documented place for that check ("This should be called in a unit test for
// any provider to verify before release that a provider is properly configured
// for use with this library").
type fastValidateProviderServer struct {
	// Embedding the concrete SDK server type, rather than the
	// tfprotov5.ProviderServer interface, promotes every method it has. That
	// keeps the optional protocol interfaces (list resources, actions, and
	// anything the SDK adds later) satisfied without having to enumerate them.
	*schema.GRPCProviderServer

	validate *schema.GRPCProviderServer
}

// NewFastValidateProviderServer returns a tfprotov5.ProviderServer for p that
// answers PrepareProviderConfig without walking every resource schema. It is a
// drop-in replacement for p.GRPCProvider().
func NewFastValidateProviderServer(p *schema.Provider) tfprotov5.ProviderServer {
	var configOnly *schema.Provider
	if p != nil {
		configOnly = &schema.Provider{
			Schema:             p.Schema,
			ProviderMetaSchema: p.ProviderMetaSchema,
		}
	}

	return &fastValidateProviderServer{
		GRPCProviderServer: schema.NewGRPCProviderServer(p),
		validate:           schema.NewGRPCProviderServer(configOnly),
	}
}

func (s *fastValidateProviderServer) PrepareProviderConfig(ctx context.Context, req *tfprotov5.PrepareProviderConfigRequest) (*tfprotov5.PrepareProviderConfigResponse, error) {
	return s.validate.PrepareProviderConfig(ctx, req)
}

var (
	_ tfprotov5.ProviderServer                 = (*fastValidateProviderServer)(nil)
	_ tfprotov5.ListResourceServer             = (*fastValidateProviderServer)(nil)
	_ tfprotov5.ActionServer                   = (*fastValidateProviderServer)(nil)
	_ tfprotov5.ProviderServerWithListResource = (*fastValidateProviderServer)(nil)
	_ tfprotov5.ProviderServerWithActions      = (*fastValidateProviderServer)(nil)
)
