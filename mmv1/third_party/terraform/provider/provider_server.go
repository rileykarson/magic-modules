package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
)

type fullProviderServer interface {
	tfprotov5.ProviderServer
	tfprotov5.ProviderServerWithListResource
	tfprotov5.ProviderServerWithActions
}

type embeddedSDKProviderServer struct {
	fullProviderServer
}

// NewEmbeddedSDKProviderServer wraps the SDKv2 protocol v5 provider server so
// that GetProviderSchema and GetResourceIdentitySchemas serve the pre-computed
// embedded schema from provider_schema.gz when available, while delegating all
// other RPCs directly to the underlying SDKv2 server.
func NewEmbeddedSDKProviderServer(next func() tfprotov5.ProviderServer) func() tfprotov5.ProviderServer {
	return func() tfprotov5.ProviderServer {
		srv := next()
		if full, ok := srv.(fullProviderServer); ok {
			return &embeddedSDKProviderServer{fullProviderServer: full}
		}
		return srv
	}
}

func (s *embeddedSDKProviderServer) GetProviderSchema(ctx context.Context, req *tfprotov5.GetProviderSchemaRequest) (*tfprotov5.GetProviderSchemaResponse, error) {
	if embedded := getEmbeddedSchemas(); embedded != nil && embedded.SDKProviderSchema != nil {
		return cloneProviderSchemaResponse(embedded.SDKProviderSchema), nil
	}
	return s.fullProviderServer.GetProviderSchema(ctx, req)
}

func (s *embeddedSDKProviderServer) GetResourceIdentitySchemas(ctx context.Context, req *tfprotov5.GetResourceIdentitySchemasRequest) (*tfprotov5.GetResourceIdentitySchemasResponse, error) {
	if embedded := getEmbeddedSchemas(); embedded != nil && embedded.SDKResourceIdentitySchemas != nil {
		return cloneResourceIdentitySchemasResponse(embedded.SDKResourceIdentitySchemas), nil
	}
	return s.fullProviderServer.GetResourceIdentitySchemas(ctx, req)
}

type embeddedFrameworkProviderServer struct {
	fullProviderServer
}

// NewEmbeddedFrameworkProviderServer wraps the Plugin Framework protocol v5
// provider server so that GetProviderSchema and GetResourceIdentitySchemas
// serve the pre-computed embedded schema from provider_schema.gz when
// available, while delegating all other RPCs directly to the underlying
// Framework server.
func NewEmbeddedFrameworkProviderServer(next func() tfprotov5.ProviderServer) func() tfprotov5.ProviderServer {
	return func() tfprotov5.ProviderServer {
		srv := next()
		if full, ok := srv.(fullProviderServer); ok {
			return &embeddedFrameworkProviderServer{fullProviderServer: full}
		}
		return srv
	}
}

func (s *embeddedFrameworkProviderServer) GetProviderSchema(ctx context.Context, req *tfprotov5.GetProviderSchemaRequest) (*tfprotov5.GetProviderSchemaResponse, error) {
	if embedded := getEmbeddedSchemas(); embedded != nil && embedded.FrameworkProviderSchema != nil {
		return cloneProviderSchemaResponse(embedded.FrameworkProviderSchema), nil
	}
	return s.fullProviderServer.GetProviderSchema(ctx, req)
}

func (s *embeddedFrameworkProviderServer) GetResourceIdentitySchemas(ctx context.Context, req *tfprotov5.GetResourceIdentitySchemasRequest) (*tfprotov5.GetResourceIdentitySchemasResponse, error) {
	if embedded := getEmbeddedSchemas(); embedded != nil && embedded.FrameworkResourceIdentitySchemas != nil {
		return cloneResourceIdentitySchemasResponse(embedded.FrameworkResourceIdentitySchemas), nil
	}
	return s.fullProviderServer.GetResourceIdentitySchemas(ctx, req)
}

var (
	_ tfprotov5.ProviderServer                 = (*embeddedSDKProviderServer)(nil)
	_ tfprotov5.ProviderServerWithListResource = (*embeddedSDKProviderServer)(nil)
	_ tfprotov5.ProviderServerWithActions      = (*embeddedSDKProviderServer)(nil)
	_ tfprotov5.ProviderServer                 = (*embeddedFrameworkProviderServer)(nil)
	_ tfprotov5.ProviderServerWithListResource = (*embeddedFrameworkProviderServer)(nil)
	_ tfprotov5.ProviderServerWithActions      = (*embeddedFrameworkProviderServer)(nil)
)
