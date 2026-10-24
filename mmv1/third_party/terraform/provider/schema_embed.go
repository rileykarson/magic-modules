package provider

import (
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"maps"
	"math/big"
	"os"
	"reflect"
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
	schemaCodecMagic          = "TPGSCHM2"
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
		cp.ResourceSchemas = maps.Clone(cp.ResourceSchemas)
		cp.DataSourceSchemas = maps.Clone(cp.DataSourceSchemas)
		cp.EphemeralResourceSchemas = maps.Clone(cp.EphemeralResourceSchemas)
		cp.Functions = maps.Clone(cp.Functions)
		cp.ListResourceSchemas = maps.Clone(cp.ListResourceSchemas)
		cp.ActionSchemas = maps.Clone(cp.ActionSchemas)
		cp.Diagnostics = slices.Clone(cp.Diagnostics)
		return &cp, nil
	}
	return s.fullProviderServer.GetProviderSchema(ctx, req)
}

func (s *embeddedProviderServer) GetResourceIdentitySchemas(ctx context.Context, req *tfprotov5.GetResourceIdentitySchemasRequest) (*tfprotov5.GetResourceIdentitySchemasResponse, error) {
	if embedded := getEmbeddedSchemas(); embedded != nil && embedded.ResourceIdentitySchemas != nil {
		cp := *embedded.ResourceIdentitySchemas
		cp.IdentitySchemas = maps.Clone(cp.IdentitySchemas)
		cp.Diagnostics = slices.Clone(cp.Diagnostics)
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

type codecErr struct{ err error }

func fail(err error) { panic(codecErr{err}) }

func catchCodecErr(errp *error) {
	if p := recover(); p != nil {
		if ce, ok := p.(codecErr); ok {
			*errp = ce.err
		} else {
			panic(p)
		}
	}
}

// EncodeEmbeddedSchemas serializes and gzip-compresses s into a deterministic byte slice.
func EncodeEmbeddedSchemas(s *EmbeddedSchemas) (_ []byte, err error) {
	if s == nil {
		return nil, errors.New("cannot encode nil EmbeddedSchemas")
	}
	defer catchCodecErr(&err)
	var w binWriter
	w.buf.WriteString(schemaCodecMagic)
	w.providerSchemaResp(s.ProviderSchema)
	w.identitySchemasResp(s.ResourceIdentitySchemas)

	var out bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&out, gzip.BestCompression)
	if _, err := zw.Write(w.buf.Bytes()); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// DecodeEmbeddedSchemas gunzips and deserializes an EmbeddedSchemas payload.
func DecodeEmbeddedSchemas(compressed []byte) (_ *EmbeddedSchemas, err error) {
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

	defer catchCodecErr(&err)
	r := &binReader{data: raw, pos: len(schemaCodecMagic)}
	out := &EmbeddedSchemas{
		ProviderSchema:          r.providerSchemaResp(),
		ResourceIdentitySchemas: r.identitySchemasResp(),
	}
	if r.pos != len(r.data) {
		return nil, errors.New("unexpected trailing bytes in embedded schema")
	}
	return out, nil
}

type binWriter struct{ buf bytes.Buffer }

func (w *binWriter) byte(b uint8) { w.buf.WriteByte(b) }
func (w *binWriter) bool(vs ...bool) {
	for _, v := range vs {
		if v {
			w.buf.WriteByte(1)
		} else {
			w.buf.WriteByte(0)
		}
	}
}
func (w *binWriter) int64(vs ...int64) {
	var tmp [binary.MaxVarintLen64]byte
	for _, v := range vs {
		w.buf.Write(tmp[:binary.PutVarint(tmp[:], v)])
	}
}
func (w *binWriter) str(ss ...string) {
	for _, s := range ss {
		w.int64(int64(len(s)))
		w.buf.WriteString(s)
	}
}

type binReader struct {
	data  []byte
	pos   int
	depth int
	inVal bool
}

func (r *binReader) byte() uint8 {
	if r.pos >= len(r.data) {
		fail(io.ErrUnexpectedEOF)
	}
	b := r.data[r.pos]
	r.pos++
	return b
}

func (r *binReader) bool() bool {
	b := r.byte()
	if b > 1 {
		fail(fmt.Errorf("invalid bool byte: %d", b))
	}
	return b == 1
}

func (r *binReader) int64() int64 {
	v, n := binary.Varint(r.data[r.pos:])
	if n <= 0 {
		fail(io.ErrUnexpectedEOF)
	}
	r.pos += n
	return v
}

func (r *binReader) str() string {
	n := r.int64()
	if n < 0 || n > int64(len(r.data)-r.pos) {
		fail(io.ErrUnexpectedEOF)
	}
	s := string(r.data[r.pos : r.pos+int(n)])
	r.pos += int(n)
	return s
}

func writePtr[T any](w *binWriter, p *T, fn func(*T)) {
	if w.bool(p != nil); p != nil {
		fn(p)
	}
}

func readPtr[T any](r *binReader, fn func() T) *T {
	if !r.bool() {
		return nil
	}
	v := fn()
	return &v
}

func writeSlice[T any](w *binWriter, s []T, fn func(T)) {
	if s == nil {
		w.int64(-1)
		return
	}
	w.int64(int64(len(s)))
	for _, v := range s {
		fn(v)
	}
}

func readSlice[T any](r *binReader, fn func() T) []T {
	n := r.int64()
	if n == -1 {
		return nil
	}
	if n < -1 || n > int64(len(r.data)-r.pos) {
		fail(io.ErrUnexpectedEOF)
	}
	out := make([]T, 0, min(n, 1024))
	for range n {
		out = append(out, fn())
	}
	return out
}

func writeMap[V any](w *binWriter, m map[string]V, fn func(V)) {
	if m == nil {
		w.int64(-1)
		return
	}
	w.int64(int64(len(m)))
	for _, k := range slices.Sorted(maps.Keys(m)) {
		w.str(k)
		fn(m[k])
	}
}

func readMap[V any](r *binReader, fn func() V) map[string]V {
	n := r.int64()
	if n == -1 {
		return nil
	}
	if n < -1 || n > int64(len(r.data)-r.pos) {
		fail(io.ErrUnexpectedEOF)
	}
	out := make(map[string]V, min(n, 1024))
	for range n {
		k, v := r.str(), fn()
		if _, dup := out[k]; dup {
			fail(fmt.Errorf("duplicate map key %q", k))
		}
		out[k] = v
	}
	return out
}

func (w *binWriter) typ(t tftypes.Type) {
	switch v := t.(type) {
	case nil:
		w.byte(0)
	case tftypes.List:
		w.byte(5)
		w.typ(v.ElementType)
	case tftypes.Set:
		w.byte(6)
		w.typ(v.ElementType)
	case tftypes.Map:
		w.byte(7)
		w.typ(v.ElementType)
	case tftypes.Tuple:
		w.byte(8)
		writeSlice(w, v.ElementTypes, w.typ)
	case tftypes.Object:
		w.byte(9)
		writeMap(w, v.AttributeTypes, w.typ)
		writeMap(w, v.OptionalAttributes, func(struct{}) {})
	default:
		switch {
		case t.Is(tftypes.String):
			w.byte(1)
		case t.Is(tftypes.Number):
			w.byte(2)
		case t.Is(tftypes.Bool):
			w.byte(3)
		case t.Is(tftypes.DynamicPseudoType):
			w.byte(4)
		default:
			fail(fmt.Errorf("unsupported tftypes.Type: %T", t))
		}
	}
}

func (r *binReader) nonNilTyp() tftypes.Type {
	t := r.typ()
	if t == nil {
		fail(errors.New("nil nested tftypes.Type"))
	}
	return t
}

func (r *binReader) typ() tftypes.Type {
	if r.depth++; r.depth > maxCodecNestingDepth {
		fail(errors.New("max type nesting depth exceeded"))
	}
	defer func() { r.depth-- }()
	switch tag := r.byte(); tag {
	case 0:
		return nil
	case 1:
		return tftypes.String
	case 2:
		return tftypes.Number
	case 3:
		return tftypes.Bool
	case 4:
		return tftypes.DynamicPseudoType
	case 5:
		return tftypes.List{ElementType: r.nonNilTyp()}
	case 6:
		return tftypes.Set{ElementType: r.nonNilTyp()}
	case 7:
		return tftypes.Map{ElementType: r.nonNilTyp()}
	case 8:
		elems := readSlice(r, r.nonNilTyp)
		if elems == nil {
			fail(errors.New("nil Tuple.ElementTypes"))
		}
		return tftypes.Tuple{ElementTypes: elems}
	case 9:
		attrs := readMap(r, r.nonNilTyp)
		if attrs == nil {
			fail(errors.New("nil Object.AttributeTypes"))
		}
		opts := readMap(r, func() struct{} { return struct{}{} })
		if r.inVal && len(opts) > 0 {
			fail(errors.New("value type cannot have OptionalAttributes"))
		}
		for k := range opts {
			if _, ok := attrs[k]; !ok {
				fail(fmt.Errorf("optional attribute %q not in AttributeTypes", k))
			}
		}
		return tftypes.Object{AttributeTypes: attrs, OptionalAttributes: opts}
	default:
		fail(fmt.Errorf("unknown tftypes.Type tag: %d", tag))
		return nil
	}
}

func (w *binWriter) val(v tftypes.Value) {
	t := v.Type()
	w.typ(t)
	switch {
	case t == nil:
	case !v.IsKnown():
		w.byte(0)
	case v.IsNull():
		w.byte(1)
	default:
		w.byte(2)
		switch t.(type) {
		case tftypes.List, tftypes.Set, tftypes.Tuple:
			var vs []tftypes.Value
			if err := v.As(&vs); err != nil {
				fail(err)
			}
			writeSlice(w, vs, w.val)
		case tftypes.Map, tftypes.Object:
			var vm map[string]tftypes.Value
			if err := v.As(&vm); err != nil {
				fail(err)
			}
			writeMap(w, vm, w.val)
		default:
			switch {
			case t.Is(tftypes.String):
				var s string
				if err := v.As(&s); err != nil {
					fail(err)
				}
				w.str(s)
			case t.Is(tftypes.Number):
				bf := new(big.Float)
				if err := v.As(&bf); err != nil {
					fail(err)
				}
				b, err := bf.GobEncode()
				if err != nil {
					fail(err)
				}
				w.str(string(b))
			case t.Is(tftypes.Bool):
				var b bool
				if err := v.As(&b); err != nil {
					fail(err)
				}
				w.bool(b)
			default:
				fail(fmt.Errorf("unsupported tftypes.Value type: %s", t))
			}
		}
	}
}

func (r *binReader) nonNilVal() tftypes.Value {
	v := r.val()
	if v.Type() == nil {
		fail(errors.New("nil nested tftypes.Value"))
	}
	return v
}

func (r *binReader) val() tftypes.Value {
	if r.depth++; r.depth > maxCodecNestingDepth {
		fail(errors.New("max value nesting depth exceeded"))
	}
	defer func() { r.depth-- }()
	r.inVal = true
	t := r.typ()
	r.inVal = false
	if t == nil {
		return tftypes.Value{}
	}
	var raw any
	switch state := r.byte(); state {
	case 0:
		raw = tftypes.UnknownValue
	case 1:
		raw = nil
	case 2:
		switch t.(type) {
		case tftypes.List, tftypes.Set, tftypes.Tuple:
			vs := readSlice(r, r.nonNilVal)
			if vs == nil {
				fail(errors.New("nil composite value slice"))
			}
			raw = vs
		case tftypes.Map, tftypes.Object:
			vm := readMap(r, r.nonNilVal)
			if vm == nil {
				fail(errors.New("nil composite value map"))
			}
			raw = vm
		default:
			switch {
			case t.Is(tftypes.String):
				raw = r.str()
			case t.Is(tftypes.Number):
				bf := new(big.Float)
				if err := bf.GobDecode([]byte(r.str())); err != nil {
					fail(err)
				}
				raw = bf
			case t.Is(tftypes.Bool):
				raw = r.bool()
			default:
				fail(fmt.Errorf("unsupported known value type: %s", t))
			}
		}
	default:
		fail(fmt.Errorf("unknown tftypes.Value state: %d", state))
	}
	if err := tftypes.ValidateValue(t, raw); err != nil {
		fail(err)
	}
	return tftypes.NewValue(t, raw)
}

func (w *binWriter) schema(s *tfprotov5.Schema) {
	writePtr(w, s, func(s *tfprotov5.Schema) { w.int64(s.Version); w.block(s.Block) })
}

func (r *binReader) schema() *tfprotov5.Schema {
	return readPtr(r, func() tfprotov5.Schema { return tfprotov5.Schema{Version: r.int64(), Block: r.block()} })
}

func (w *binWriter) block(b *tfprotov5.SchemaBlock) {
	writePtr(w, b, func(b *tfprotov5.SchemaBlock) {
		w.int64(b.Version, int64(b.DescriptionKind))
		w.bool(b.Deprecated)
		w.str(b.Description, b.DeprecationMessage)
		writeSlice(w, b.Attributes, func(a *tfprotov5.SchemaAttribute) {
			writePtr(w, a, func(a *tfprotov5.SchemaAttribute) {
				w.str(a.Name, a.Description, a.DeprecationMessage)
				w.typ(a.Type)
				w.int64(int64(a.DescriptionKind))
				w.bool(a.Required, a.Optional, a.Computed, a.Sensitive, a.Deprecated, a.WriteOnly)
			})
		})
		writeSlice(w, b.BlockTypes, func(nb *tfprotov5.SchemaNestedBlock) {
			writePtr(w, nb, func(nb *tfprotov5.SchemaNestedBlock) {
				w.str(nb.TypeName)
				w.block(nb.Block)
				w.int64(int64(nb.Nesting), nb.MinItems, nb.MaxItems)
			})
		})
	})
}

func (r *binReader) block() *tfprotov5.SchemaBlock {
	if r.depth++; r.depth > maxCodecNestingDepth {
		fail(errors.New("max block nesting depth exceeded"))
	}
	defer func() { r.depth-- }()
	return readPtr(r, func() (b tfprotov5.SchemaBlock) {
		b.Version, b.DescriptionKind, b.Deprecated, b.Description, b.DeprecationMessage = r.int64(), tfprotov5.StringKind(r.int64()), r.bool(), r.str(), r.str()
		b.Attributes = readSlice(r, func() *tfprotov5.SchemaAttribute {
			return readPtr(r, func() (a tfprotov5.SchemaAttribute) {
				a.Name, a.Description, a.DeprecationMessage, a.Type, a.DescriptionKind = r.str(), r.str(), r.str(), r.typ(), tfprotov5.StringKind(r.int64())
				a.Required, a.Optional, a.Computed, a.Sensitive, a.Deprecated, a.WriteOnly = r.bool(), r.bool(), r.bool(), r.bool(), r.bool(), r.bool()
				return a
			})
		})
		b.BlockTypes = readSlice(r, func() *tfprotov5.SchemaNestedBlock {
			return readPtr(r, func() (nb tfprotov5.SchemaNestedBlock) {
				nb.TypeName, nb.Block, nb.Nesting, nb.MinItems, nb.MaxItems = r.str(), r.block(), tfprotov5.SchemaNestedBlockNestingMode(r.int64()), r.int64(), r.int64()
				return nb
			})
		})
		return b
	})
}

func (w *binWriter) funcParam(p *tfprotov5.FunctionParameter) {
	writePtr(w, p, func(p *tfprotov5.FunctionParameter) {
		w.bool(p.AllowNullValue, p.AllowUnknownValues)
		w.str(p.Name, p.Description)
		w.int64(int64(p.DescriptionKind))
		w.typ(p.Type)
	})
}

func (r *binReader) funcParam() *tfprotov5.FunctionParameter {
	return readPtr(r, func() (p tfprotov5.FunctionParameter) {
		p.AllowNullValue, p.AllowUnknownValues, p.Name, p.Description, p.DescriptionKind, p.Type = r.bool(), r.bool(), r.str(), r.str(), tfprotov5.StringKind(r.int64()), r.typ()
		return p
	})
}

func (w *binWriter) function(fn *tfprotov5.Function) {
	writePtr(w, fn, func(fn *tfprotov5.Function) {
		writeSlice(w, fn.Parameters, w.funcParam)
		w.funcParam(fn.VariadicParameter)
		writePtr(w, fn.Return, func(ret *tfprotov5.FunctionReturn) { w.typ(ret.Type) })
		w.str(fn.Summary, fn.Description, fn.DeprecationMessage)
		w.int64(int64(fn.DescriptionKind))
	})
}

func (r *binReader) function() *tfprotov5.Function {
	return readPtr(r, func() (fn tfprotov5.Function) {
		fn.Parameters, fn.VariadicParameter = readSlice(r, r.funcParam), r.funcParam()
		fn.Return = readPtr(r, func() tfprotov5.FunctionReturn { return tfprotov5.FunctionReturn{Type: r.typ()} })
		fn.Summary, fn.Description, fn.DeprecationMessage, fn.DescriptionKind = r.str(), r.str(), r.str(), tfprotov5.StringKind(r.int64())
		return fn
	})
}

func (w *binWriter) diag(d *tfprotov5.Diagnostic) {
	writePtr(w, d, func(d *tfprotov5.Diagnostic) {
		w.int64(int64(d.Severity))
		w.str(d.Summary, d.Detail)
		writePtr(w, d.Attribute, func(ap *tftypes.AttributePath) {
			steps := ap.Steps()
			if len(steps) == 0 && reflect.ValueOf(*ap).Field(0).IsNil() {
				steps = nil
			}
			writeSlice(w, steps, func(step tftypes.AttributePathStep) {
				switch s := step.(type) {
				case nil:
					w.byte(0)
				case tftypes.AttributeName:
					w.byte(1)
					w.str(string(s))
				case tftypes.ElementKeyString:
					w.byte(2)
					w.str(string(s))
				case tftypes.ElementKeyInt:
					w.byte(3)
					w.int64(int64(s))
				case tftypes.ElementKeyValue:
					w.byte(4)
					w.val(tftypes.Value(s))
				default:
					fail(fmt.Errorf("unsupported AttributePathStep: %T", step))
				}
			})
		})
	})
}

func (r *binReader) diag() *tfprotov5.Diagnostic {
	return readPtr(r, func() (d tfprotov5.Diagnostic) {
		d.Severity, d.Summary, d.Detail = tfprotov5.DiagnosticSeverity(r.int64()), r.str(), r.str()
		d.Attribute = readPtr(r, func() tftypes.AttributePath {
			return *tftypes.NewAttributePathWithSteps(readSlice(r, func() tftypes.AttributePathStep {
				switch tag := r.byte(); tag {
				case 0:
					return nil
				case 1:
					return tftypes.AttributeName(r.str())
				case 2:
					return tftypes.ElementKeyString(r.str())
				case 3:
					return tftypes.ElementKeyInt(r.int64())
				case 4:
					return tftypes.ElementKeyValue(r.val())
				default:
					fail(fmt.Errorf("unknown AttributePathStep tag: %d", tag))
					return nil
				}
			}))
		})
		return d
	})
}

func (w *binWriter) providerSchemaResp(resp *tfprotov5.GetProviderSchemaResponse) {
	writePtr(w, resp, func(resp *tfprotov5.GetProviderSchemaResponse) {
		w.schema(resp.Provider)
		w.schema(resp.ProviderMeta)
		writeMap(w, resp.ResourceSchemas, w.schema)
		writeMap(w, resp.DataSourceSchemas, w.schema)
		writeMap(w, resp.EphemeralResourceSchemas, w.schema)
		writeMap(w, resp.ListResourceSchemas, w.schema)
		writeMap(w, resp.Functions, w.function)
		writeMap(w, resp.ActionSchemas, func(a *tfprotov5.ActionSchema) {
			writePtr(w, a, func(a *tfprotov5.ActionSchema) { w.schema(a.Schema) })
		})
		writeSlice(w, resp.Diagnostics, w.diag)
		writePtr(w, resp.ServerCapabilities, func(sc *tfprotov5.ServerCapabilities) {
			w.bool(sc.PlanDestroy, sc.GetProviderSchemaOptional, sc.MoveResourceState, sc.GenerateResourceConfig)
		})
	})
}

func (r *binReader) providerSchemaResp() *tfprotov5.GetProviderSchemaResponse {
	return readPtr(r, func() (resp tfprotov5.GetProviderSchemaResponse) {
		resp.Provider, resp.ProviderMeta = r.schema(), r.schema()
		resp.ResourceSchemas, resp.DataSourceSchemas = readMap(r, r.schema), readMap(r, r.schema)
		resp.EphemeralResourceSchemas, resp.ListResourceSchemas = readMap(r, r.schema), readMap(r, r.schema)
		resp.Functions = readMap(r, r.function)
		resp.ActionSchemas = readMap(r, func() *tfprotov5.ActionSchema {
			return readPtr(r, func() tfprotov5.ActionSchema { return tfprotov5.ActionSchema{Schema: r.schema()} })
		})
		resp.Diagnostics = readSlice(r, r.diag)
		resp.ServerCapabilities = readPtr(r, func() (sc tfprotov5.ServerCapabilities) {
			sc.PlanDestroy, sc.GetProviderSchemaOptional, sc.MoveResourceState, sc.GenerateResourceConfig = r.bool(), r.bool(), r.bool(), r.bool()
			return sc
		})
		return resp
	})
}

func (w *binWriter) identitySchemasResp(resp *tfprotov5.GetResourceIdentitySchemasResponse) {
	writePtr(w, resp, func(resp *tfprotov5.GetResourceIdentitySchemasResponse) {
		writeMap(w, resp.IdentitySchemas, func(ris *tfprotov5.ResourceIdentitySchema) {
			writePtr(w, ris, func(ris *tfprotov5.ResourceIdentitySchema) {
				w.int64(ris.Version)
				writeSlice(w, ris.IdentityAttributes, func(a *tfprotov5.ResourceIdentitySchemaAttribute) {
					writePtr(w, a, func(a *tfprotov5.ResourceIdentitySchemaAttribute) {
						w.str(a.Name, a.Description)
						w.typ(a.Type)
						w.bool(a.RequiredForImport, a.OptionalForImport)
					})
				})
			})
		})
		writeSlice(w, resp.Diagnostics, w.diag)
	})
}

func (r *binReader) identitySchemasResp() *tfprotov5.GetResourceIdentitySchemasResponse {
	return readPtr(r, func() (resp tfprotov5.GetResourceIdentitySchemasResponse) {
		resp.IdentitySchemas = readMap(r, func() *tfprotov5.ResourceIdentitySchema {
			return readPtr(r, func() (ris tfprotov5.ResourceIdentitySchema) {
				ris.Version = r.int64()
				ris.IdentityAttributes = readSlice(r, func() *tfprotov5.ResourceIdentitySchemaAttribute {
					return readPtr(r, func() (a tfprotov5.ResourceIdentitySchemaAttribute) {
						a.Name, a.Description, a.Type, a.RequiredForImport, a.OptionalForImport = r.str(), r.str(), r.typ(), r.bool(), r.bool()
						return a
					})
				})
				return ris
			})
		})
		resp.Diagnostics = readSlice(r, r.diag)
		return resp
	})
}
