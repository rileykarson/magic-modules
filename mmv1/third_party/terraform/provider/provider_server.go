package provider

import (
	"context"
	"maps"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
)

var (
	lastValidatedConfigMu sync.Mutex
	lastValidatedConfig   *tfprotov5.DynamicValue
)

// fastValidateProviderServer serves every RPC from the real SDKv2 provider
// server except PrepareProviderConfig (the protocol v5 name for
// ValidateProviderConfig), which it serves from a second provider server built
// over a provider that has the same top level Schema and ProviderMetaSchema but
// no resources or data sources.
//
// helper/schema.Provider.Validate, which PrepareProviderConfig calls, runs
// Provider.InternalValidate first, and that walks every registered resource and
// data source schema. The provider has ~1500 of them, so the walk dominates the
// RPC, and Terraform issues the RPC once per provider instance on every plan
// and apply. PrepareProviderConfig itself only ever reads Provider.Schema (for
// the config block) and Provider.ProviderMetaSchema, so a resource-less
// provider returns the same PreparedConfig and the same user facing
// diagnostics.
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

	validate         *schema.GRPCProviderServer
	fwValidateNeeded atomic.Bool
}

// NewFastValidateProviderServer returns a tfprotov5.ProviderServer for p that
// answers PrepareProviderConfig without walking every resource schema. It is a
// drop-in replacement for p.GRPCProvider().
func NewFastValidateProviderServer(p *schema.Provider) tfprotov5.ProviderServer {
	srv := &fastValidateProviderServer{
		GRPCProviderServer: schema.NewGRPCProviderServer(p),
	}

	validateSchema := maps.Clone(p.Schema)
	// request_timeout and poll_interval have schema-level duration validators on
	// the Framework provider but are validated during Configure on the SDKv2
	// provider. Probe them during SDKv2 PrepareProviderConfig without altering
	// SDKv2 diagnostics so the Framework server wrapper knows whether it needs
	// to run its own schema validation to emit duration diagnostics.
	checkDuration := func(v interface{}, _ string) ([]string, []error) {
		s, ok := v.(string)
		if !ok || s == "" {
			srv.fwValidateNeeded.Store(true)
			return nil, nil
		}
		dur, err := time.ParseDuration(s)
		if err != nil || dur < 0 {
			srv.fwValidateNeeded.Store(true)
		}
		return nil, nil
	}
	for _, field := range []string{"request_timeout", "poll_interval"} {
		if orig, ok := validateSchema[field]; ok && orig != nil && orig.ValidateFunc == nil {
			cp := *orig
			cp.ValidateFunc = checkDuration
			validateSchema[field] = &cp
		}
	}

	configOnly := &schema.Provider{
		Schema:             validateSchema,
		ProviderMetaSchema: p.ProviderMetaSchema,
	}
	srv.validate = schema.NewGRPCProviderServer(configOnly)
	return srv
}

func (s *fastValidateProviderServer) PrepareProviderConfig(ctx context.Context, req *tfprotov5.PrepareProviderConfigRequest) (*tfprotov5.PrepareProviderConfigResponse, error) {
	s.fwValidateNeeded.Store(false)
	resp, err := s.validate.PrepareProviderConfig(ctx, req)
	if err == nil && resp != nil && len(resp.Diagnostics) == 0 && !s.fwValidateNeeded.Load() && req != nil && req.Config != nil {
		lastValidatedConfigMu.Lock()
		lastValidatedConfig = req.Config
		lastValidatedConfigMu.Unlock()
	} else {
		lastValidatedConfigMu.Lock()
		lastValidatedConfig = nil
		lastValidatedConfigMu.Unlock()
	}
	return resp, err
}

type fullProviderServer interface {
	tfprotov5.ProviderServer
	tfprotov5.ProviderServerWithListResource
	tfprotov5.ProviderServerWithActions
}

type fastFrameworkProviderServer struct {
	fullProviderServer
}

var frameworkFunctionNames = []string{
	"location_from_id",
	"name_from_id",
	"project_from_id",
	"region_from_id",
	"region_from_zone",
	"zone_from_id",
}

// NewFastFrameworkProviderServer wraps the Plugin Framework protocol v5 server
// so that:
//   - GetMetadata answers directly from the registry without instantiating all
//     152 list resources and their underlying SDKv2 resource schemas.
//   - PrepareProviderConfig short-circuits when the preceding SDKv2
//     PrepareProviderConfig pass on the same request Config produced zero
//     diagnostics and valid duration fields.
//   - ConfigureProvider passes a nil Config to avoid unmarshaling and
//     nullifying the ~220-attribute config MsgPack a second time when
//     FrameworkProvider.Configure reads primary.Meta() directly.
func NewFastFrameworkProviderServer(next func() tfprotov5.ProviderServer) func() tfprotov5.ProviderServer {
	return func() tfprotov5.ProviderServer {
		srv := next()
		if full, ok := srv.(fullProviderServer); ok {
			return &fastFrameworkProviderServer{fullProviderServer: full}
		}
		return srv
	}
}

func (s *fastFrameworkProviderServer) GetMetadata(_ context.Context, _ *tfprotov5.GetMetadataRequest) (*tfprotov5.GetMetadataResponse, error) {
	dsNames := registry.FrameworkDataSourceNames()
	resNames := registry.FrameworkResourceNames()
	ephNames := registry.FrameworkEphemeralResourceNames()
	listNames := registry.FrameworkListResourceNames()

	dataSources := make([]tfprotov5.DataSourceMetadata, len(dsNames))
	for i, name := range dsNames {
		dataSources[i] = tfprotov5.DataSourceMetadata{TypeName: name}
	}
	resources := make([]tfprotov5.ResourceMetadata, len(resNames))
	for i, name := range resNames {
		resources[i] = tfprotov5.ResourceMetadata{TypeName: name}
	}
	ephemeralResources := make([]tfprotov5.EphemeralResourceMetadata, len(ephNames))
	for i, name := range ephNames {
		ephemeralResources[i] = tfprotov5.EphemeralResourceMetadata{TypeName: name}
	}
	listResources := make([]tfprotov5.ListResourceMetadata, len(listNames))
	for i, name := range listNames {
		listResources[i] = tfprotov5.ListResourceMetadata{TypeName: name}
	}
	functions := make([]tfprotov5.FunctionMetadata, len(frameworkFunctionNames))
	for i, name := range frameworkFunctionNames {
		functions[i] = tfprotov5.FunctionMetadata{Name: name}
	}

	return &tfprotov5.GetMetadataResponse{
		Actions:            []tfprotov5.ActionMetadata{},
		DataSources:        dataSources,
		EphemeralResources: ephemeralResources,
		Functions:          functions,
		ListResources:      listResources,
		Resources:          resources,
		ServerCapabilities: &tfprotov5.ServerCapabilities{
			PlanDestroy:               true,
			GetProviderSchemaOptional: true,
			MoveResourceState:         true,
		},
	}, nil
}

func (s *fastFrameworkProviderServer) PrepareProviderConfig(ctx context.Context, req *tfprotov5.PrepareProviderConfigRequest) (*tfprotov5.PrepareProviderConfigResponse, error) {
	if req != nil && req.Config != nil {
		lastValidatedConfigMu.Lock()
		valid := lastValidatedConfig == req.Config
		lastValidatedConfigMu.Unlock()
		if valid {
			return &tfprotov5.PrepareProviderConfigResponse{
				PreparedConfig: req.Config,
			}, nil
		}
	}
	return s.fullProviderServer.PrepareProviderConfig(ctx, req)
}

func (s *fastFrameworkProviderServer) ConfigureProvider(ctx context.Context, req *tfprotov5.ConfigureProviderRequest) (*tfprotov5.ConfigureProviderResponse, error) {
	if req != nil && req.Config != nil {
		reqNoConfig := *req
		reqNoConfig.Config = nil
		return s.fullProviderServer.ConfigureProvider(ctx, &reqNoConfig)
	}
	return s.fullProviderServer.ConfigureProvider(ctx, req)
}

var (
	_ tfprotov5.ProviderServer                 = (*fastValidateProviderServer)(nil)
	_ tfprotov5.ProviderServerWithListResource = (*fastValidateProviderServer)(nil)
	_ tfprotov5.ProviderServerWithActions      = (*fastValidateProviderServer)(nil)
	_ tfprotov5.ProviderServer                 = (*fastFrameworkProviderServer)(nil)
	_ tfprotov5.ProviderServerWithListResource = (*fastFrameworkProviderServer)(nil)
	_ tfprotov5.ProviderServerWithActions      = (*fastFrameworkProviderServer)(nil)
)
