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

type fastValidateProviderServer struct {
	*schema.GRPCProviderServer

	provider         *schema.Provider
	validate         *schema.GRPCProviderServer
	mu               sync.RWMutex
	resMaps          sync.Map
	dsMaps           sync.Map
	allOnce          sync.Once
	allResMap        map[string]*schema.Resource
	allDsMap         map[string]*schema.Resource
	fwValidateNeeded atomic.Bool
}

// NewFastValidateProviderServer returns a tfprotov5.ProviderServer for p that
// answers PrepareProviderConfig without walking every resource schema, and
// lazily resolves per-resource schemas when p was created via LazyProvider().
func NewFastValidateProviderServer(p *schema.Provider) tfprotov5.ProviderServer {
	srv := &fastValidateProviderServer{
		GRPCProviderServer: schema.NewGRPCProviderServer(p),
		provider:           p,
	}

	validateSchema := maps.Clone(p.Schema)
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

func (s *fastValidateProviderServer) serverForResource(typeName string) *schema.GRPCProviderServer {
	if typeName == "" {
		return s.validate
	}
	res, ok := s.provider.ResourcesMap[typeName]
	if !ok {
		return s.validate
	}

	var m map[string]*schema.Resource
	if res != nil {
		m = map[string]*schema.Resource{typeName: res}
	} else if cached, ok := s.resMaps.Load(typeName); ok {
		m = cached.(map[string]*schema.Resource)
	} else {
		built := registry.Resource(typeName)
		m = map[string]*schema.Resource{typeName: built}
		actual, _ := s.resMaps.LoadOrStore(typeName, m)
		m = actual.(map[string]*schema.Resource)
	}

	s.mu.RLock()
	pCopy := *s.provider
	s.mu.RUnlock()
	pCopy.ResourcesMap = m
	pCopy.DataSourcesMap = nil
	return schema.NewGRPCProviderServer(&pCopy)
}

func (s *fastValidateProviderServer) serverForDataSource(typeName string) *schema.GRPCProviderServer {
	if typeName == "" {
		return s.validate
	}
	ds, ok := s.provider.DataSourcesMap[typeName]
	if !ok {
		return s.validate
	}

	var m map[string]*schema.Resource
	if ds != nil {
		m = map[string]*schema.Resource{typeName: ds}
	} else if cached, ok := s.dsMaps.Load(typeName); ok {
		m = cached.(map[string]*schema.Resource)
	} else {
		built := registry.DataSource(typeName)
		m = map[string]*schema.Resource{typeName: built}
		actual, _ := s.dsMaps.LoadOrStore(typeName, m)
		m = actual.(map[string]*schema.Resource)
	}

	s.mu.RLock()
	pCopy := *s.provider
	s.mu.RUnlock()
	pCopy.ResourcesMap = nil
	pCopy.DataSourcesMap = m
	return schema.NewGRPCProviderServer(&pCopy)
}

func (s *fastValidateProviderServer) allSchemasServer() *schema.GRPCProviderServer {
	s.allOnce.Do(func() {
		s.allResMap = registry.ResourceMap()
		s.allDsMap = registry.DatasourceMap()
	})
	s.mu.RLock()
	pCopy := *s.provider
	s.mu.RUnlock()
	pCopy.ResourcesMap = s.allResMap
	pCopy.DataSourcesMap = s.allDsMap
	return schema.NewGRPCProviderServer(&pCopy)
}

func (s *fastValidateProviderServer) ConfigureProvider(ctx context.Context, req *tfprotov5.ConfigureProviderRequest) (*tfprotov5.ConfigureProviderResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.GRPCProviderServer.ConfigureProvider(ctx, req)
}

func (s *fastValidateProviderServer) GetProviderSchema(ctx context.Context, req *tfprotov5.GetProviderSchemaRequest) (*tfprotov5.GetProviderSchemaResponse, error) {
	if embedded := getEmbeddedSchemas(); embedded != nil && embedded.SDKProviderSchema != nil {
		return cloneProviderSchemaResponse(embedded.SDKProviderSchema), nil
	}
	return s.allSchemasServer().GetProviderSchema(ctx, req)
}

func (s *fastValidateProviderServer) GetResourceIdentitySchemas(ctx context.Context, req *tfprotov5.GetResourceIdentitySchemasRequest) (*tfprotov5.GetResourceIdentitySchemasResponse, error) {
	if embedded := getEmbeddedSchemas(); embedded != nil && embedded.SDKResourceIdentitySchemas != nil {
		return cloneResourceIdentitySchemasResponse(embedded.SDKResourceIdentitySchemas), nil
	}
	return s.allSchemasServer().GetResourceIdentitySchemas(ctx, req)
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

func (s *fastValidateProviderServer) ValidateResourceTypeConfig(ctx context.Context, req *tfprotov5.ValidateResourceTypeConfigRequest) (*tfprotov5.ValidateResourceTypeConfigResponse, error) {
	if req == nil {
		return s.GRPCProviderServer.ValidateResourceTypeConfig(ctx, req)
	}
	return s.serverForResource(req.TypeName).ValidateResourceTypeConfig(ctx, req)
}

func (s *fastValidateProviderServer) ValidateDataSourceConfig(ctx context.Context, req *tfprotov5.ValidateDataSourceConfigRequest) (*tfprotov5.ValidateDataSourceConfigResponse, error) {
	if req == nil {
		return s.GRPCProviderServer.ValidateDataSourceConfig(ctx, req)
	}
	return s.serverForDataSource(req.TypeName).ValidateDataSourceConfig(ctx, req)
}

func (s *fastValidateProviderServer) UpgradeResourceState(ctx context.Context, req *tfprotov5.UpgradeResourceStateRequest) (*tfprotov5.UpgradeResourceStateResponse, error) {
	if req == nil {
		return s.GRPCProviderServer.UpgradeResourceState(ctx, req)
	}
	return s.serverForResource(req.TypeName).UpgradeResourceState(ctx, req)
}

func (s *fastValidateProviderServer) UpgradeResourceIdentity(ctx context.Context, req *tfprotov5.UpgradeResourceIdentityRequest) (*tfprotov5.UpgradeResourceIdentityResponse, error) {
	if req == nil {
		return s.GRPCProviderServer.UpgradeResourceIdentity(ctx, req)
	}
	return s.serverForResource(req.TypeName).UpgradeResourceIdentity(ctx, req)
}

func (s *fastValidateProviderServer) ReadResource(ctx context.Context, req *tfprotov5.ReadResourceRequest) (*tfprotov5.ReadResourceResponse, error) {
	if req == nil {
		return s.GRPCProviderServer.ReadResource(ctx, req)
	}
	return s.serverForResource(req.TypeName).ReadResource(ctx, req)
}

func (s *fastValidateProviderServer) PlanResourceChange(ctx context.Context, req *tfprotov5.PlanResourceChangeRequest) (*tfprotov5.PlanResourceChangeResponse, error) {
	if req == nil {
		return s.GRPCProviderServer.PlanResourceChange(ctx, req)
	}
	return s.serverForResource(req.TypeName).PlanResourceChange(ctx, req)
}

func (s *fastValidateProviderServer) ApplyResourceChange(ctx context.Context, req *tfprotov5.ApplyResourceChangeRequest) (*tfprotov5.ApplyResourceChangeResponse, error) {
	if req == nil {
		return s.GRPCProviderServer.ApplyResourceChange(ctx, req)
	}
	return s.serverForResource(req.TypeName).ApplyResourceChange(ctx, req)
}

func (s *fastValidateProviderServer) ImportResourceState(ctx context.Context, req *tfprotov5.ImportResourceStateRequest) (*tfprotov5.ImportResourceStateResponse, error) {
	if req == nil {
		return s.GRPCProviderServer.ImportResourceState(ctx, req)
	}
	return s.serverForResource(req.TypeName).ImportResourceState(ctx, req)
}

func (s *fastValidateProviderServer) GenerateResourceConfig(ctx context.Context, req *tfprotov5.GenerateResourceConfigRequest) (*tfprotov5.GenerateResourceConfigResponse, error) {
	if req == nil {
		return s.GRPCProviderServer.GenerateResourceConfig(ctx, req)
	}
	return s.serverForResource(req.TypeName).GenerateResourceConfig(ctx, req)
}

func (s *fastValidateProviderServer) ReadDataSource(ctx context.Context, req *tfprotov5.ReadDataSourceRequest) (*tfprotov5.ReadDataSourceResponse, error) {
	if req == nil {
		return s.GRPCProviderServer.ReadDataSource(ctx, req)
	}
	return s.serverForDataSource(req.TypeName).ReadDataSource(ctx, req)
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
// so that GetMetadata answers directly from the registry without instantiating
// all 152 list resources and their underlying SDKv2 resource schemas.
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

func (s *fastFrameworkProviderServer) GetProviderSchema(ctx context.Context, req *tfprotov5.GetProviderSchemaRequest) (*tfprotov5.GetProviderSchemaResponse, error) {
	if embedded := getEmbeddedSchemas(); embedded != nil && embedded.FrameworkProviderSchema != nil {
		return cloneProviderSchemaResponse(embedded.FrameworkProviderSchema), nil
	}
	return s.fullProviderServer.GetProviderSchema(ctx, req)
}

func (s *fastFrameworkProviderServer) GetResourceIdentitySchemas(ctx context.Context, req *tfprotov5.GetResourceIdentitySchemasRequest) (*tfprotov5.GetResourceIdentitySchemasResponse, error) {
	if embedded := getEmbeddedSchemas(); embedded != nil && embedded.FrameworkResourceIdentitySchemas != nil {
		return cloneResourceIdentitySchemasResponse(embedded.FrameworkResourceIdentitySchemas), nil
	}
	return s.fullProviderServer.GetResourceIdentitySchemas(ctx, req)
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
