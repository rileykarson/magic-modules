package provider

import (
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-mux/tf5muxserver"
	"github.com/hashicorp/terraform-provider-google/google/fwprovider"
)

//go:generate go run ./cmd/generate_schema -output provider_schema.gz

//go:embed provider_schema.gz
var embeddedSchemaBlob []byte

const (
	schemaCodecMagic          = "TPGSCHM3"
	maxDecompressedSchemaSize = 64 << 20
	maxCodecNestingDepth      = 64
)

// EmbeddedSchemas holds the pre-computed protocol v5 schema and resource
// identity schema responses for the muxed provider server.
type EmbeddedSchemas struct {
	ProviderSchema          *tfprotov5.GetProviderSchemaResponse
	ResourceIdentitySchemas *tfprotov5.GetResourceIdentitySchemasResponse
}

var (
	embeddedSchemasOnce sync.Once
	embeddedSchemasVal  *EmbeddedSchemas
	embeddedSchemasErr  error
)

// EmbeddedSchemaLoaded reports whether a valid non-empty embedded schema blob
// is present and decodes without error.
func EmbeddedSchemaLoaded() bool {
	if len(embeddedSchemaBlob) == 0 {
		return false
	}
	embeddedSchemasOnce.Do(func() {
		embeddedSchemasVal, embeddedSchemasErr = DecodeEmbeddedSchemas(embeddedSchemaBlob)
	})
	return embeddedSchemasErr == nil && embeddedSchemasVal != nil
}

func getEmbeddedSchemas() *EmbeddedSchemas {
	if os.Getenv("GOOGLE_TERRAFORM_LIVE_SCHEMA") == "1" || !EmbeddedSchemaLoaded() {
		return nil
	}
	return embeddedSchemasVal
}

type fullProviderServer interface {
	tfprotov5.ProviderServer
	tfprotov5.ProviderServerWithListResource
	tfprotov5.ProviderServerWithActions
}

type embeddedProviderServer struct{ fullProviderServer }

// NewEmbeddedProviderServer wraps a protocol v5 provider server so that
// GetProviderSchema and GetResourceIdentitySchemas serve the pre-computed
// embedded schemas from provider_schema.gz when available.
func NewEmbeddedProviderServer(next func() tfprotov5.ProviderServer) func() tfprotov5.ProviderServer {
	return func() tfprotov5.ProviderServer {
		srv := next()
		if full, ok := srv.(fullProviderServer); ok {
			return &embeddedProviderServer{fullProviderServer: full}
		}
		return srv
	}
}

func (s *embeddedProviderServer) GetProviderSchema(ctx context.Context, req *tfprotov5.GetProviderSchemaRequest) (*tfprotov5.GetProviderSchemaResponse, error) {
	if embedded := getEmbeddedSchemas(); embedded != nil && embedded.ProviderSchema != nil {
		cp := *embedded.ProviderSchema
		cp.ResourceSchemas, cp.DataSourceSchemas = maps.Clone(cp.ResourceSchemas), maps.Clone(cp.DataSourceSchemas)
		cp.EphemeralResourceSchemas, cp.ListResourceSchemas = maps.Clone(cp.EphemeralResourceSchemas), maps.Clone(cp.ListResourceSchemas)
		cp.Functions, cp.ActionSchemas, cp.Diagnostics = maps.Clone(cp.Functions), maps.Clone(cp.ActionSchemas), slices.Clone(cp.Diagnostics)
		return &cp, nil
	}
	return s.fullProviderServer.GetProviderSchema(ctx, req)
}

func (s *embeddedProviderServer) GetResourceIdentitySchemas(ctx context.Context, req *tfprotov5.GetResourceIdentitySchemasRequest) (*tfprotov5.GetResourceIdentitySchemasResponse, error) {
	if embedded := getEmbeddedSchemas(); embedded != nil && embedded.ResourceIdentitySchemas != nil {
		cp := *embedded.ResourceIdentitySchemas
		cp.IdentitySchemas, cp.Diagnostics = maps.Clone(cp.IdentitySchemas), slices.Clone(cp.Diagnostics)
		return &cp, nil
	}
	return s.fullProviderServer.GetResourceIdentitySchemas(ctx, req)
}

// ComputeLiveEmbeddedSchemas dynamically constructs the muxed SDKv2 and Plugin
// Framework provider server and returns its live schema and identity responses.
func ComputeLiveEmbeddedSchemas(ctx context.Context) (*EmbeddedSchemas, error) {
	primary := Provider()
	mux, err := tf5muxserver.NewMuxServer(ctx, primary.GRPCProvider, providerserver.NewProtocol5(fwprovider.New(primary)))
	if err != nil {
		return nil, fmt.Errorf("muxing live servers: %w", err)
	}
	srv := mux.ProviderServer()
	schema, err := srv.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{})
	if err != nil || len(schema.Diagnostics) > 0 {
		return nil, fmt.Errorf("live GetProviderSchema (err=%v, diags=%v)", err, schema.Diagnostics)
	}
	identity, err := srv.GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{})
	if err != nil || len(identity.Diagnostics) > 0 {
		return nil, fmt.Errorf("live GetResourceIdentitySchemas (err=%v, diags=%v)", err, identity.Diagnostics)
	}
	return &EmbeddedSchemas{ProviderSchema: schema, ResourceIdentitySchemas: identity}, nil
}

type gobPayload struct {
	HasProvider, HasIdentity                                                          bool
	ServerCapabilities                                                                *tfprotov5.ServerCapabilities
	Provider, ProviderMeta                                                            *tfprotov5.Schema
	ResourceSchemas, DataSourceSchemas, EphemeralResourceSchemas, ListResourceSchemas map[string]*tfprotov5.Schema
	Functions                                                                         map[string]*tfprotov5.Function
	ActionSchemas                                                                     map[string]*tfprotov5.ActionSchema
	IdentitySchemas                                                                   map[string]*tfprotov5.ResourceIdentitySchema
	Types                                                                             [][]byte
}

// EncodeEmbeddedSchemas serializes and gzip-compresses s.
func EncodeEmbeddedSchemas(s *EmbeddedSchemas) ([]byte, error) {
	if s == nil {
		return nil, errors.New("cannot encode nil EmbeddedSchemas")
	}
	if (s.ProviderSchema != nil && len(s.ProviderSchema.Diagnostics) > 0) ||
		(s.ResourceIdentitySchemas != nil && len(s.ResourceIdentitySchemas.Diagnostics) > 0) {
		return nil, errors.New("cannot encode schema response with diagnostics")
	}

	var ptrs []*tftypes.Type
	var saved []tftypes.Type
	var types [][]byte
	defer func() {
		for i := len(ptrs) - 1; i >= 0; i-- {
			*ptrs[i] = saved[i]
		}
	}()

	if err := walkTypes(s, func(tp *tftypes.Type) error {
		ptrs, saved = append(ptrs, tp), append(saved, *tp)
		if *tp == nil {
			types = append(types, nil)
			return nil
		}
		b, err := (*tp).MarshalJSON()
		types, *tp = append(types, b), nil
		return err
	}); err != nil {
		return nil, err
	}

	p := gobPayload{Types: types}
	if ps := s.ProviderSchema; ps != nil {
		p.HasProvider, p.ServerCapabilities, p.Provider, p.ProviderMeta = true, ps.ServerCapabilities, ps.Provider, ps.ProviderMeta
		p.ResourceSchemas, p.DataSourceSchemas = ps.ResourceSchemas, ps.DataSourceSchemas
		p.EphemeralResourceSchemas, p.ListResourceSchemas = ps.EphemeralResourceSchemas, ps.ListResourceSchemas
		p.Functions, p.ActionSchemas = ps.Functions, ps.ActionSchemas
	}
	if ris := s.ResourceIdentitySchemas; ris != nil {
		p.HasIdentity, p.IdentitySchemas = true, ris.IdentitySchemas
	}

	var raw, out bytes.Buffer
	raw.WriteString(schemaCodecMagic)
	if err := gob.NewEncoder(&raw).Encode(&p); err != nil {
		return nil, err
	}
	zw, _ := gzip.NewWriterLevel(&out, gzip.BestCompression)
	if _, err := zw.Write(raw.Bytes()); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// DecodeEmbeddedSchemas gunzips and deserializes an EmbeddedSchemas payload.
func DecodeEmbeddedSchemas(compressed []byte) (*EmbeddedSchemas, error) {
	zr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	raw, err := io.ReadAll(io.LimitReader(zr, maxDecompressedSchemaSize+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxDecompressedSchemaSize {
		return nil, errors.New("decompressed schema exceeds maximum allowed size")
	}
	if err := zr.Close(); err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(raw, []byte(schemaCodecMagic)) {
		return nil, errors.New("invalid embedded schema magic header")
	}

	r := bytes.NewReader(raw[len(schemaCodecMagic):])
	var p gobPayload
	if err := gob.NewDecoder(r).Decode(&p); err != nil {
		return nil, err
	}
	if r.Len() != 0 {
		return nil, errors.New("unexpected trailing bytes in embedded schema")
	}

	out := &EmbeddedSchemas{}
	if p.HasProvider {
		out.ProviderSchema = &tfprotov5.GetProviderSchemaResponse{
			ServerCapabilities: p.ServerCapabilities, Provider: p.Provider, ProviderMeta: p.ProviderMeta,
			ResourceSchemas: p.ResourceSchemas, DataSourceSchemas: p.DataSourceSchemas,
			EphemeralResourceSchemas: p.EphemeralResourceSchemas, ListResourceSchemas: p.ListResourceSchemas,
			Functions: p.Functions, ActionSchemas: p.ActionSchemas,
		}
	}
	if p.HasIdentity {
		out.ResourceIdentitySchemas = &tfprotov5.GetResourceIdentitySchemasResponse{IdentitySchemas: p.IdentitySchemas}
	}

	idx := 0
	if err := walkTypes(out, func(tp *tftypes.Type) error {
		if idx >= len(p.Types) {
			return io.ErrUnexpectedEOF
		}
		rawType := p.Types[idx]
		idx++
		if len(rawType) == 0 {
			*tp = nil
			return nil
		}
		*tp, err = tftypes.ParseJSONType(rawType)
		return err
	}); err != nil {
		return nil, err
	}
	if idx != len(p.Types) {
		return nil, errors.New("type count mismatch in embedded schema")
	}
	return out, nil
}

func nonNilMapVals[V any](m map[string]*V) []*V {
	var out []*V
	for _, k := range slices.Sorted(maps.Keys(m)) {
		if v := m[k]; v != nil {
			out = append(out, v)
		}
	}
	return out
}

func walkTypes(s *EmbeddedSchemas, fn func(*tftypes.Type) error) (err error) {
	visit := func(tp *tftypes.Type) {
		if err == nil {
			err = fn(tp)
		}
	}
	var walkBlock func(*tfprotov5.SchemaBlock, int)
	walkBlock = func(b *tfprotov5.SchemaBlock, depth int) {
		if b == nil || err != nil {
			return
		}
		if depth > maxCodecNestingDepth {
			err = errors.New("max block nesting depth exceeded")
			return
		}
		for _, a := range b.Attributes {
			if a != nil {
				visit(&a.Type)
			}
		}
		for _, nb := range b.BlockTypes {
			if nb != nil {
				walkBlock(nb.Block, depth+1)
			}
		}
	}
	walkSchema := func(sc *tfprotov5.Schema) {
		if sc != nil {
			walkBlock(sc.Block, 0)
		}
	}
	if ps := s.ProviderSchema; ps != nil {
		for _, sc := range slices.Concat([]*tfprotov5.Schema{ps.Provider, ps.ProviderMeta}, nonNilMapVals(ps.ResourceSchemas), nonNilMapVals(ps.DataSourceSchemas), nonNilMapVals(ps.EphemeralResourceSchemas), nonNilMapVals(ps.ListResourceSchemas)) {
			walkSchema(sc)
		}
		for _, act := range nonNilMapVals(ps.ActionSchemas) {
			walkSchema(act.Schema)
		}
		for _, f := range nonNilMapVals(ps.Functions) {
			for _, p := range slices.Concat(f.Parameters, []*tfprotov5.FunctionParameter{f.VariadicParameter}) {
				if p != nil {
					visit(&p.Type)
				}
			}
			if f.Return != nil {
				visit(&f.Return.Type)
			}
		}
	}
	if ris := s.ResourceIdentitySchemas; ris != nil {
		for _, id := range nonNilMapVals(ris.IdentitySchemas) {
			for _, a := range id.IdentityAttributes {
				if a != nil {
					visit(&a.Type)
				}
			}
		}
	}
	return err
}
