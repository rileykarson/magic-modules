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

const schemaCodecMagic = "TPGSCHM1"

// EmbeddedSchemas holds the pre-computed protocol v5 schema and resource
// identity schema responses for both the SDKv2 provider server and the Plugin
// Framework provider server.
type EmbeddedSchemas struct {
	SDKProviderSchema                *tfprotov5.GetProviderSchemaResponse
	SDKResourceIdentitySchemas       *tfprotov5.GetResourceIdentitySchemasResponse
	FrameworkProviderSchema          *tfprotov5.GetProviderSchemaResponse
	FrameworkResourceIdentitySchemas *tfprotov5.GetResourceIdentitySchemasResponse
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
	s, err := getEmbeddedSchemasWithError()
	return err == nil && s != nil
}

func getEmbeddedSchemasWithError() (*EmbeddedSchemas, error) {
	embeddedSchemasOnce.Do(func() {
		if len(embeddedSchemaBlob) == 0 {
			return
		}
		embeddedSchemasVal, embeddedSchemasErr = DecodeEmbeddedSchemas(embeddedSchemaBlob)
	})
	return embeddedSchemasVal, embeddedSchemasErr
}

// getEmbeddedSchemas returns the decoded embedded schemas, or nil if no
// embedded schema blob is available or GOOGLE_TERRAFORM_LIVE_SCHEMA is set.
func getEmbeddedSchemas() *EmbeddedSchemas {
	if os.Getenv("GOOGLE_TERRAFORM_LIVE_SCHEMA") != "" {
		return nil
	}
	s, err := getEmbeddedSchemasWithError()
	if err != nil {
		return nil
	}
	return s
}

func cloneProviderSchemaResponse(src *tfprotov5.GetProviderSchemaResponse) *tfprotov5.GetProviderSchemaResponse {
	if src == nil {
		return nil
	}
	cp := *src
	if src.ResourceSchemas != nil {
		cp.ResourceSchemas = maps.Clone(src.ResourceSchemas)
	}
	if src.DataSourceSchemas != nil {
		cp.DataSourceSchemas = maps.Clone(src.DataSourceSchemas)
	}
	if src.EphemeralResourceSchemas != nil {
		cp.EphemeralResourceSchemas = maps.Clone(src.EphemeralResourceSchemas)
	}
	if src.Functions != nil {
		cp.Functions = maps.Clone(src.Functions)
	}
	if src.ListResourceSchemas != nil {
		cp.ListResourceSchemas = maps.Clone(src.ListResourceSchemas)
	}
	if src.ActionSchemas != nil {
		cp.ActionSchemas = maps.Clone(src.ActionSchemas)
	}
	if src.Diagnostics != nil {
		cp.Diagnostics = slices.Clone(src.Diagnostics)
	}
	return &cp
}

func cloneResourceIdentitySchemasResponse(src *tfprotov5.GetResourceIdentitySchemasResponse) *tfprotov5.GetResourceIdentitySchemasResponse {
	if src == nil {
		return nil
	}
	cp := *src
	if src.IdentitySchemas != nil {
		cp.IdentitySchemas = maps.Clone(src.IdentitySchemas)
	}
	if src.Diagnostics != nil {
		cp.Diagnostics = slices.Clone(src.Diagnostics)
	}
	return &cp
}

// ComputeLiveEmbeddedSchemas dynamically constructs the full SDKv2 and Plugin
// Framework providers from source (invoking all SchemaFunc registrations) and
// returns their live GetProviderSchema and GetResourceIdentitySchemas responses.
func ComputeLiveEmbeddedSchemas(ctx context.Context) (*EmbeddedSchemas, error) {
	stockPrimary := Provider()
	sdkServer := stockPrimary.GRPCProvider()
	fwServerFactory := providerserver.NewProtocol5(fwprovider.New(stockPrimary))
	fwServer := fwServerFactory()

	// Validate that the two live servers mux cleanly without conflicts.
	if _, err := tf5muxserver.NewMuxServer(ctx,
		func() tfprotov5.ProviderServer { return sdkServer },
		func() tfprotov5.ProviderServer { return fwServer },
	); err != nil {
		return nil, fmt.Errorf("muxing live SDKv2 and Framework servers failed: %w", err)
	}

	sdkSchema, err := sdkServer.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{})
	if err != nil {
		return nil, fmt.Errorf("live SDK GetProviderSchema failed: %w", err)
	}
	if len(sdkSchema.Diagnostics) > 0 {
		return nil, fmt.Errorf("live SDK GetProviderSchema returned diagnostics: %v", sdkSchema.Diagnostics)
	}

	sdkIdentity, err := sdkServer.GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{})
	if err != nil {
		return nil, fmt.Errorf("live SDK GetResourceIdentitySchemas failed: %w", err)
	}
	if len(sdkIdentity.Diagnostics) > 0 {
		return nil, fmt.Errorf("live SDK GetResourceIdentitySchemas returned diagnostics: %v", sdkIdentity.Diagnostics)
	}

	fwSchema, err := fwServer.GetProviderSchema(ctx, &tfprotov5.GetProviderSchemaRequest{})
	if err != nil {
		return nil, fmt.Errorf("live Framework GetProviderSchema failed: %w", err)
	}
	if len(fwSchema.Diagnostics) > 0 {
		return nil, fmt.Errorf("live Framework GetProviderSchema returned diagnostics: %v", fwSchema.Diagnostics)
	}

	fwIdentity, err := fwServer.GetResourceIdentitySchemas(ctx, &tfprotov5.GetResourceIdentitySchemasRequest{})
	if err != nil {
		return nil, fmt.Errorf("live Framework GetResourceIdentitySchemas failed: %w", err)
	}
	if len(fwIdentity.Diagnostics) > 0 {
		return nil, fmt.Errorf("live Framework GetResourceIdentitySchemas returned diagnostics: %v", fwIdentity.Diagnostics)
	}

	return &EmbeddedSchemas{
		SDKProviderSchema:                sdkSchema,
		SDKResourceIdentitySchemas:       sdkIdentity,
		FrameworkProviderSchema:          fwSchema,
		FrameworkResourceIdentitySchemas: fwIdentity,
	}, nil
}

// Type tag constants for the binary tftypes.Type codec.
const (
	typeTagNil uint8 = iota
	typeTagString
	typeTagNumber
	typeTagBool
	typeTagDynamicPseudoType
	typeTagList
	typeTagSet
	typeTagMap
	typeTagTuple
	typeTagObject
	typeTagListString
	typeTagSetString
	typeTagMapString
)

// Pre-boxed tftypes.Type singletons to avoid interface boxing allocations on decode.
var (
	singletonStringType     tftypes.Type = tftypes.String
	singletonNumberType     tftypes.Type = tftypes.Number
	singletonBoolType       tftypes.Type = tftypes.Bool
	singletonDynamicType    tftypes.Type = tftypes.DynamicPseudoType
	singletonListStringType tftypes.Type = tftypes.List{ElementType: tftypes.String}
	singletonSetStringType  tftypes.Type = tftypes.Set{ElementType: tftypes.String}
	singletonMapStringType  tftypes.Type = tftypes.Map{ElementType: tftypes.String}

	emptySchemaAttributes   = []*tfprotov5.SchemaAttribute{}
	emptySchemaNestedBlocks = []*tfprotov5.SchemaNestedBlock{}
	emptyIdentityAttributes = []*tfprotov5.ResourceIdentitySchemaAttribute{}
	emptyFunctionParameters = []*tfprotov5.FunctionParameter{}
	emptyDiagnostics        = []*tfprotov5.Diagnostic{}
)

// Attribute boolean flag bitmask constants.
const (
	attrFlagRequired   uint8 = 1 << 0
	attrFlagOptional   uint8 = 1 << 1
	attrFlagComputed   uint8 = 1 << 2
	attrFlagSensitive  uint8 = 1 << 3
	attrFlagDeprecated uint8 = 1 << 4
	attrFlagWriteOnly  uint8 = 1 << 5
)

type schemaEncoder struct {
	buf       bytes.Buffer
	strings   []string
	stringIdx map[string]uint32
}

func newSchemaEncoder() *schemaEncoder {
	e := &schemaEncoder{
		strings:   []string{""},
		stringIdx: map[string]uint32{"": 0},
	}
	return e
}

func (e *schemaEncoder) intern(s string) uint32 {
	if idx, ok := e.stringIdx[s]; ok {
		return idx
	}
	idx := uint32(len(e.strings))
	e.strings = append(e.strings, s)
	e.stringIdx[s] = idx
	return idx
}

func (e *schemaEncoder) writeByte(b uint8) {
	e.buf.WriteByte(b)
}

func (e *schemaEncoder) writeBool(v bool) {
	if v {
		e.buf.WriteByte(1)
	} else {
		e.buf.WriteByte(0)
	}
}

func (e *schemaEncoder) writeUvarint(v uint64) {
	var tmp [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(tmp[:], v)
	e.buf.Write(tmp[:n])
}

func (e *schemaEncoder) writeVarint(v int64) {
	var tmp [binary.MaxVarintLen64]byte
	n := binary.PutVarint(tmp[:], v)
	e.buf.Write(tmp[:n])
}

func (e *schemaEncoder) writeString(s string) {
	e.writeUvarint(uint64(e.intern(s)))
}

// writeLen writes -1 for a nil slice/map, or len >= 0 for non-nil.
func (e *schemaEncoder) writeLen(isNil bool, length int) {
	if isNil {
		e.writeVarint(-1)
		return
	}
	e.writeVarint(int64(length))
}

func (e *schemaEncoder) writeType(t tftypes.Type) error {
	if t == nil {
		e.writeByte(typeTagNil)
		return nil
	}
	switch {
	case t.Is(tftypes.String):
		e.writeByte(typeTagString)
		return nil
	case t.Is(tftypes.Number):
		e.writeByte(typeTagNumber)
		return nil
	case t.Is(tftypes.Bool):
		e.writeByte(typeTagBool)
		return nil
	case t.Is(tftypes.DynamicPseudoType):
		e.writeByte(typeTagDynamicPseudoType)
		return nil
	}

	switch v := t.(type) {
	case tftypes.List:
		if v.ElementType != nil && v.ElementType.Is(tftypes.String) {
			e.writeByte(typeTagListString)
			return nil
		}
		e.writeByte(typeTagList)
		return e.writeType(v.ElementType)
	case tftypes.Set:
		if v.ElementType != nil && v.ElementType.Is(tftypes.String) {
			e.writeByte(typeTagSetString)
			return nil
		}
		e.writeByte(typeTagSet)
		return e.writeType(v.ElementType)
	case tftypes.Map:
		if v.ElementType != nil && v.ElementType.Is(tftypes.String) {
			e.writeByte(typeTagMapString)
			return nil
		}
		e.writeByte(typeTagMap)
		return e.writeType(v.ElementType)
	case tftypes.Tuple:
		e.writeByte(typeTagTuple)
		e.writeLen(v.ElementTypes == nil, len(v.ElementTypes))
		for _, elem := range v.ElementTypes {
			if err := e.writeType(elem); err != nil {
				return err
			}
		}
		return nil
	case tftypes.Object:
		e.writeByte(typeTagObject)
		e.writeLen(v.AttributeTypes == nil, len(v.AttributeTypes))
		if len(v.AttributeTypes) > 0 {
			keys := slices.Sorted(maps.Keys(v.AttributeTypes))
			for _, k := range keys {
				e.writeString(k)
				if err := e.writeType(v.AttributeTypes[k]); err != nil {
					return err
				}
			}
		}
		e.writeLen(v.OptionalAttributes == nil, len(v.OptionalAttributes))
		if len(v.OptionalAttributes) > 0 {
			optKeys := slices.Sorted(maps.Keys(v.OptionalAttributes))
			for _, k := range optKeys {
				e.writeString(k)
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported tftypes.Type: %T (%v)", t, t)
	}
}

func (e *schemaEncoder) writeSchema(s *tfprotov5.Schema) error {
	if s == nil {
		e.writeBool(false)
		return nil
	}
	e.writeBool(true)
	e.writeVarint(s.Version)
	return e.writeSchemaBlock(s.Block)
}

func (e *schemaEncoder) writeSchemaBlock(b *tfprotov5.SchemaBlock) error {
	if b == nil {
		e.writeBool(false)
		return nil
	}
	e.writeBool(true)
	e.writeVarint(b.Version)
	e.writeString(b.Description)
	e.writeVarint(int64(b.DescriptionKind))
	e.writeBool(b.Deprecated)
	e.writeString(b.DeprecationMessage)

	e.writeLen(b.Attributes == nil, len(b.Attributes))
	for _, attr := range b.Attributes {
		if err := e.writeSchemaAttribute(attr); err != nil {
			return err
		}
	}

	e.writeLen(b.BlockTypes == nil, len(b.BlockTypes))
	for _, blk := range b.BlockTypes {
		if err := e.writeSchemaNestedBlock(blk); err != nil {
			return err
		}
	}
	return nil
}

func (e *schemaEncoder) writeSchemaAttribute(a *tfprotov5.SchemaAttribute) error {
	if a == nil {
		e.writeBool(false)
		return nil
	}
	e.writeBool(true)
	e.writeString(a.Name)
	if err := e.writeType(a.Type); err != nil {
		return err
	}
	e.writeString(a.Description)
	var flags uint8
	if a.Required {
		flags |= attrFlagRequired
	}
	if a.Optional {
		flags |= attrFlagOptional
	}
	if a.Computed {
		flags |= attrFlagComputed
	}
	if a.Sensitive {
		flags |= attrFlagSensitive
	}
	if a.Deprecated {
		flags |= attrFlagDeprecated
	}
	if a.WriteOnly {
		flags |= attrFlagWriteOnly
	}
	e.writeByte(flags)
	e.writeVarint(int64(a.DescriptionKind))
	e.writeString(a.DeprecationMessage)
	return nil
}

func (e *schemaEncoder) writeSchemaNestedBlock(nb *tfprotov5.SchemaNestedBlock) error {
	if nb == nil {
		e.writeBool(false)
		return nil
	}
	e.writeBool(true)
	e.writeString(nb.TypeName)
	if err := e.writeSchemaBlock(nb.Block); err != nil {
		return err
	}
	e.writeVarint(int64(nb.Nesting))
	e.writeVarint(nb.MinItems)
	e.writeVarint(nb.MaxItems)
	return nil
}

func (e *schemaEncoder) writeSchemaMap(m map[string]*tfprotov5.Schema) error {
	e.writeLen(m == nil, len(m))
	if len(m) == 0 {
		return nil
	}
	keys := slices.Sorted(maps.Keys(m))
	for _, k := range keys {
		e.writeString(k)
		if err := e.writeSchema(m[k]); err != nil {
			return err
		}
	}
	return nil
}

func (e *schemaEncoder) writeFunctionParameter(p *tfprotov5.FunctionParameter) error {
	if p == nil {
		e.writeBool(false)
		return nil
	}
	e.writeBool(true)
	e.writeBool(p.AllowNullValue)
	e.writeBool(p.AllowUnknownValues)
	e.writeString(p.Description)
	e.writeVarint(int64(p.DescriptionKind))
	e.writeString(p.Name)
	return e.writeType(p.Type)
}

func (e *schemaEncoder) writeFunction(fn *tfprotov5.Function) error {
	if fn == nil {
		e.writeBool(false)
		return nil
	}
	e.writeBool(true)
	e.writeLen(fn.Parameters == nil, len(fn.Parameters))
	for _, p := range fn.Parameters {
		if err := e.writeFunctionParameter(p); err != nil {
			return err
		}
	}
	if err := e.writeFunctionParameter(fn.VariadicParameter); err != nil {
		return err
	}
	if fn.Return == nil {
		e.writeBool(false)
	} else {
		e.writeBool(true)
		if err := e.writeType(fn.Return.Type); err != nil {
			return err
		}
	}
	e.writeString(fn.Summary)
	e.writeString(fn.Description)
	e.writeVarint(int64(fn.DescriptionKind))
	e.writeString(fn.DeprecationMessage)
	return nil
}

func (e *schemaEncoder) writeFunctionMap(m map[string]*tfprotov5.Function) error {
	e.writeLen(m == nil, len(m))
	if len(m) == 0 {
		return nil
	}
	keys := slices.Sorted(maps.Keys(m))
	for _, k := range keys {
		e.writeString(k)
		if err := e.writeFunction(m[k]); err != nil {
			return err
		}
	}
	return nil
}

func (e *schemaEncoder) writeActionSchema(a *tfprotov5.ActionSchema) error {
	if a == nil {
		e.writeBool(false)
		return nil
	}
	e.writeBool(true)
	return e.writeSchema(a.Schema)
}

func (e *schemaEncoder) writeActionSchemaMap(m map[string]*tfprotov5.ActionSchema) error {
	e.writeLen(m == nil, len(m))
	if len(m) == 0 {
		return nil
	}
	keys := slices.Sorted(maps.Keys(m))
	for _, k := range keys {
		e.writeString(k)
		if err := e.writeActionSchema(m[k]); err != nil {
			return err
		}
	}
	return nil
}

func (e *schemaEncoder) writeDiagnostics(diags []*tfprotov5.Diagnostic) {
	e.writeLen(diags == nil, len(diags))
	for _, d := range diags {
		if d == nil {
			e.writeBool(false)
			continue
		}
		e.writeBool(true)
		e.writeVarint(int64(d.Severity))
		e.writeString(d.Summary)
		e.writeString(d.Detail)
	}
}

func (e *schemaEncoder) writeServerCapabilities(sc *tfprotov5.ServerCapabilities) {
	if sc == nil {
		e.writeBool(false)
		return
	}
	e.writeBool(true)
	e.writeBool(sc.PlanDestroy)
	e.writeBool(sc.GetProviderSchemaOptional)
	e.writeBool(sc.MoveResourceState)
	e.writeBool(sc.GenerateResourceConfig)
}

func (e *schemaEncoder) writeProviderSchemaResponse(r *tfprotov5.GetProviderSchemaResponse) error {
	if r == nil {
		e.writeBool(false)
		return nil
	}
	e.writeBool(true)
	if err := e.writeSchema(r.Provider); err != nil {
		return err
	}
	if err := e.writeSchema(r.ProviderMeta); err != nil {
		return err
	}
	if err := e.writeSchemaMap(r.ResourceSchemas); err != nil {
		return err
	}
	if err := e.writeSchemaMap(r.DataSourceSchemas); err != nil {
		return err
	}
	if err := e.writeSchemaMap(r.EphemeralResourceSchemas); err != nil {
		return err
	}
	if err := e.writeFunctionMap(r.Functions); err != nil {
		return err
	}
	if err := e.writeSchemaMap(r.ListResourceSchemas); err != nil {
		return err
	}
	if err := e.writeActionSchemaMap(r.ActionSchemas); err != nil {
		return err
	}
	e.writeDiagnostics(r.Diagnostics)
	e.writeServerCapabilities(r.ServerCapabilities)
	return nil
}

func (e *schemaEncoder) writeResourceIdentitySchema(ris *tfprotov5.ResourceIdentitySchema) error {
	if ris == nil {
		e.writeBool(false)
		return nil
	}
	e.writeBool(true)
	e.writeVarint(ris.Version)
	e.writeLen(ris.IdentityAttributes == nil, len(ris.IdentityAttributes))
	for _, a := range ris.IdentityAttributes {
		if a == nil {
			e.writeBool(false)
			continue
		}
		e.writeBool(true)
		e.writeString(a.Name)
		if err := e.writeType(a.Type); err != nil {
			return err
		}
		e.writeBool(a.RequiredForImport)
		e.writeBool(a.OptionalForImport)
		e.writeString(a.Description)
	}
	return nil
}

func (e *schemaEncoder) writeResourceIdentitySchemasResponse(r *tfprotov5.GetResourceIdentitySchemasResponse) error {
	if r == nil {
		e.writeBool(false)
		return nil
	}
	e.writeBool(true)
	e.writeLen(r.IdentitySchemas == nil, len(r.IdentitySchemas))
	if len(r.IdentitySchemas) > 0 {
		keys := slices.Sorted(maps.Keys(r.IdentitySchemas))
		for _, k := range keys {
			e.writeString(k)
			if err := e.writeResourceIdentitySchema(r.IdentitySchemas[k]); err != nil {
				return err
			}
		}
	}
	e.writeDiagnostics(r.Diagnostics)
	return nil
}

// EncodeEmbeddedSchemas serializes and gzip-compresses s into a deterministic byte slice.
func EncodeEmbeddedSchemas(s *EmbeddedSchemas) ([]byte, error) {
	if s == nil {
		return nil, errors.New("cannot encode nil EmbeddedSchemas")
	}
	enc := newSchemaEncoder()
	if err := enc.writeProviderSchemaResponse(s.SDKProviderSchema); err != nil {
		return nil, fmt.Errorf("encoding SDKProviderSchema: %w", err)
	}
	if err := enc.writeResourceIdentitySchemasResponse(s.SDKResourceIdentitySchemas); err != nil {
		return nil, fmt.Errorf("encoding SDKResourceIdentitySchemas: %w", err)
	}
	if err := enc.writeProviderSchemaResponse(s.FrameworkProviderSchema); err != nil {
		return nil, fmt.Errorf("encoding FrameworkProviderSchema: %w", err)
	}
	if err := enc.writeResourceIdentitySchemasResponse(s.FrameworkResourceIdentitySchemas); err != nil {
		return nil, fmt.Errorf("encoding FrameworkResourceIdentitySchemas: %w", err)
	}

	var raw bytes.Buffer
	raw.WriteString(schemaCodecMagic)

	var varintBuf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(varintBuf[:], uint64(len(enc.strings)))
	raw.Write(varintBuf[:n])

	totalStringBytes := 0
	for _, str := range enc.strings {
		totalStringBytes += len(str)
	}
	n = binary.PutUvarint(varintBuf[:], uint64(totalStringBytes))
	raw.Write(varintBuf[:n])

	for _, str := range enc.strings {
		n = binary.PutUvarint(varintBuf[:], uint64(len(str)))
		raw.Write(varintBuf[:n])
	}
	for _, str := range enc.strings {
		raw.WriteString(str)
	}
	raw.Write(enc.buf.Bytes())

	var out bytes.Buffer
	zw, err := gzip.NewWriterLevel(&out, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(raw.Bytes()); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

type schemaDecoder struct {
	data    []byte
	pos     int
	strings []string
}

func (d *schemaDecoder) readByte() (uint8, error) {
	if d.pos >= len(d.data) {
		return 0, io.ErrUnexpectedEOF
	}
	b := d.data[d.pos]
	d.pos++
	return b, nil
}

func (d *schemaDecoder) readBool() (bool, error) {
	b, err := d.readByte()
	if err != nil {
		return false, err
	}
	return b != 0, nil
}

func (d *schemaDecoder) readUvarint() (uint64, error) {
	v, n := binary.Uvarint(d.data[d.pos:])
	if n <= 0 {
		return 0, io.ErrUnexpectedEOF
	}
	d.pos += n
	return v, nil
}

func (d *schemaDecoder) readVarint() (int64, error) {
	v, n := binary.Varint(d.data[d.pos:])
	if n <= 0 {
		return 0, io.ErrUnexpectedEOF
	}
	d.pos += n
	return v, nil
}

func (d *schemaDecoder) readString() (string, error) {
	idx, err := d.readUvarint()
	if err != nil {
		return "", err
	}
	if idx >= uint64(len(d.strings)) {
		return "", fmt.Errorf("string table index %d out of bounds (%d)", idx, len(d.strings))
	}
	return d.strings[idx], nil
}

func (d *schemaDecoder) readType() (tftypes.Type, error) {
	tag, err := d.readByte()
	if err != nil {
		return nil, err
	}
	switch tag {
	case typeTagNil:
		return nil, nil
	case typeTagString:
		return singletonStringType, nil
	case typeTagNumber:
		return singletonNumberType, nil
	case typeTagBool:
		return singletonBoolType, nil
	case typeTagDynamicPseudoType:
		return singletonDynamicType, nil
	case typeTagListString:
		return singletonListStringType, nil
	case typeTagSetString:
		return singletonSetStringType, nil
	case typeTagMapString:
		return singletonMapStringType, nil
	case typeTagList:
		elem, err := d.readType()
		if err != nil {
			return nil, err
		}
		return tftypes.List{ElementType: elem}, nil
	case typeTagSet:
		elem, err := d.readType()
		if err != nil {
			return nil, err
		}
		return tftypes.Set{ElementType: elem}, nil
	case typeTagMap:
		elem, err := d.readType()
		if err != nil {
			return nil, err
		}
		return tftypes.Map{ElementType: elem}, nil
	case typeTagTuple:
		n, err := d.readVarint()
		if err != nil {
			return nil, err
		}
		var elems []tftypes.Type
		if n >= 0 {
			elems = make([]tftypes.Type, n)
			for i := int64(0); i < n; i++ {
				elem, err := d.readType()
				if err != nil {
					return nil, err
				}
				elems[i] = elem
			}
		}
		return tftypes.Tuple{ElementTypes: elems}, nil
	case typeTagObject:
		numAttrs, err := d.readVarint()
		if err != nil {
			return nil, err
		}
		var attrTypes map[string]tftypes.Type
		if numAttrs >= 0 {
			attrTypes = make(map[string]tftypes.Type, numAttrs)
			for i := int64(0); i < numAttrs; i++ {
				k, err := d.readString()
				if err != nil {
					return nil, err
				}
				t, err := d.readType()
				if err != nil {
					return nil, err
				}
				attrTypes[k] = t
			}
		}
		numOpt, err := d.readVarint()
		if err != nil {
			return nil, err
		}
		var optAttrs map[string]struct{}
		if numOpt >= 0 {
			optAttrs = make(map[string]struct{}, numOpt)
			for i := int64(0); i < numOpt; i++ {
				k, err := d.readString()
				if err != nil {
					return nil, err
				}
				optAttrs[k] = struct{}{}
			}
		}
		return tftypes.Object{
			AttributeTypes:     attrTypes,
			OptionalAttributes: optAttrs,
		}, nil
	default:
		return nil, fmt.Errorf("unknown tftypes.Type tag: %d", tag)
	}
}

func (d *schemaDecoder) readSchema() (*tfprotov5.Schema, error) {
	present, err := d.readBool()
	if err != nil || !present {
		return nil, err
	}
	ver, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	blk, err := d.readSchemaBlock()
	if err != nil {
		return nil, err
	}
	return &tfprotov5.Schema{
		Version: ver,
		Block:   blk,
	}, nil
}

func (d *schemaDecoder) readSchemaBlock() (*tfprotov5.SchemaBlock, error) {
	present, err := d.readBool()
	if err != nil || !present {
		return nil, err
	}
	ver, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	desc, err := d.readString()
	if err != nil {
		return nil, err
	}
	descKind, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	deprecated, err := d.readBool()
	if err != nil {
		return nil, err
	}
	depMsg, err := d.readString()
	if err != nil {
		return nil, err
	}

	numAttrs, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	var attrs []*tfprotov5.SchemaAttribute
	if numAttrs == 0 {
		attrs = emptySchemaAttributes
	} else if numAttrs > 0 {
		attrsVal := make([]tfprotov5.SchemaAttribute, numAttrs)
		attrs = make([]*tfprotov5.SchemaAttribute, numAttrs)
		for i := int64(0); i < numAttrs; i++ {
			a, err := d.readSchemaAttributeInto(&attrsVal[i])
			if err != nil {
				return nil, err
			}
			attrs[i] = a
		}
	}

	numBlocks, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	var blocks []*tfprotov5.SchemaNestedBlock
	if numBlocks == 0 {
		blocks = emptySchemaNestedBlocks
	} else if numBlocks > 0 {
		blocksVal := make([]tfprotov5.SchemaNestedBlock, numBlocks)
		blocks = make([]*tfprotov5.SchemaNestedBlock, numBlocks)
		for i := int64(0); i < numBlocks; i++ {
			nb, err := d.readSchemaNestedBlockInto(&blocksVal[i])
			if err != nil {
				return nil, err
			}
			blocks[i] = nb
		}
	}

	return &tfprotov5.SchemaBlock{
		Version:            ver,
		Attributes:         attrs,
		BlockTypes:         blocks,
		Description:        desc,
		DescriptionKind:    tfprotov5.StringKind(descKind),
		Deprecated:         deprecated,
		DeprecationMessage: depMsg,
	}, nil
}

func (d *schemaDecoder) readSchemaAttributeInto(dst *tfprotov5.SchemaAttribute) (*tfprotov5.SchemaAttribute, error) {
	present, err := d.readBool()
	if err != nil || !present {
		return nil, err
	}
	name, err := d.readString()
	if err != nil {
		return nil, err
	}
	typ, err := d.readType()
	if err != nil {
		return nil, err
	}
	desc, err := d.readString()
	if err != nil {
		return nil, err
	}
	flags, err := d.readByte()
	if err != nil {
		return nil, err
	}
	descKind, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	depMsg, err := d.readString()
	if err != nil {
		return nil, err
	}
	*dst = tfprotov5.SchemaAttribute{
		Name:               name,
		Type:               typ,
		Description:        desc,
		Required:           flags&attrFlagRequired != 0,
		Optional:           flags&attrFlagOptional != 0,
		Computed:           flags&attrFlagComputed != 0,
		Sensitive:          flags&attrFlagSensitive != 0,
		Deprecated:         flags&attrFlagDeprecated != 0,
		WriteOnly:          flags&attrFlagWriteOnly != 0,
		DescriptionKind:    tfprotov5.StringKind(descKind),
		DeprecationMessage: depMsg,
	}
	return dst, nil
}

func (d *schemaDecoder) readSchemaNestedBlockInto(dst *tfprotov5.SchemaNestedBlock) (*tfprotov5.SchemaNestedBlock, error) {
	present, err := d.readBool()
	if err != nil || !present {
		return nil, err
	}
	typeName, err := d.readString()
	if err != nil {
		return nil, err
	}
	blk, err := d.readSchemaBlock()
	if err != nil {
		return nil, err
	}
	nesting, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	minItems, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	maxItems, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	*dst = tfprotov5.SchemaNestedBlock{
		TypeName: typeName,
		Block:    blk,
		Nesting:  tfprotov5.SchemaNestedBlockNestingMode(nesting),
		MinItems: minItems,
		MaxItems: maxItems,
	}
	return dst, nil
}

func (d *schemaDecoder) readSchemaMap() (map[string]*tfprotov5.Schema, error) {
	n, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, nil
	}
	m := make(map[string]*tfprotov5.Schema, n)
	for i := int64(0); i < n; i++ {
		k, err := d.readString()
		if err != nil {
			return nil, err
		}
		s, err := d.readSchema()
		if err != nil {
			return nil, err
		}
		m[k] = s
	}
	return m, nil
}

func (d *schemaDecoder) readFunctionParameter() (*tfprotov5.FunctionParameter, error) {
	present, err := d.readBool()
	if err != nil || !present {
		return nil, err
	}
	allowNull, err := d.readBool()
	if err != nil {
		return nil, err
	}
	allowUnknown, err := d.readBool()
	if err != nil {
		return nil, err
	}
	desc, err := d.readString()
	if err != nil {
		return nil, err
	}
	descKind, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	name, err := d.readString()
	if err != nil {
		return nil, err
	}
	typ, err := d.readType()
	if err != nil {
		return nil, err
	}
	return &tfprotov5.FunctionParameter{
		AllowNullValue:     allowNull,
		AllowUnknownValues: allowUnknown,
		Description:        desc,
		DescriptionKind:    tfprotov5.StringKind(descKind),
		Name:               name,
		Type:               typ,
	}, nil
}

func (d *schemaDecoder) readFunction() (*tfprotov5.Function, error) {
	present, err := d.readBool()
	if err != nil || !present {
		return nil, err
	}
	numParams, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	var params []*tfprotov5.FunctionParameter
	if numParams == 0 {
		params = emptyFunctionParameters
	} else if numParams > 0 {
		params = make([]*tfprotov5.FunctionParameter, numParams)
		for i := int64(0); i < numParams; i++ {
			p, err := d.readFunctionParameter()
			if err != nil {
				return nil, err
			}
			params[i] = p
		}
	}
	variadic, err := d.readFunctionParameter()
	if err != nil {
		return nil, err
	}
	retPresent, err := d.readBool()
	if err != nil {
		return nil, err
	}
	var ret *tfprotov5.FunctionReturn
	if retPresent {
		retType, err := d.readType()
		if err != nil {
			return nil, err
		}
		ret = &tfprotov5.FunctionReturn{Type: retType}
	}
	summary, err := d.readString()
	if err != nil {
		return nil, err
	}
	desc, err := d.readString()
	if err != nil {
		return nil, err
	}
	descKind, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	depMsg, err := d.readString()
	if err != nil {
		return nil, err
	}
	return &tfprotov5.Function{
		Parameters:         params,
		VariadicParameter:  variadic,
		Return:             ret,
		Summary:            summary,
		Description:        desc,
		DescriptionKind:    tfprotov5.StringKind(descKind),
		DeprecationMessage: depMsg,
	}, nil
}

func (d *schemaDecoder) readFunctionMap() (map[string]*tfprotov5.Function, error) {
	n, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, nil
	}
	m := make(map[string]*tfprotov5.Function, n)
	for i := int64(0); i < n; i++ {
		k, err := d.readString()
		if err != nil {
			return nil, err
		}
		fn, err := d.readFunction()
		if err != nil {
			return nil, err
		}
		m[k] = fn
	}
	return m, nil
}

func (d *schemaDecoder) readActionSchema() (*tfprotov5.ActionSchema, error) {
	present, err := d.readBool()
	if err != nil || !present {
		return nil, err
	}
	s, err := d.readSchema()
	if err != nil {
		return nil, err
	}
	return &tfprotov5.ActionSchema{
		Schema: s,
	}, nil
}

func (d *schemaDecoder) readActionSchemaMap() (map[string]*tfprotov5.ActionSchema, error) {
	n, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, nil
	}
	m := make(map[string]*tfprotov5.ActionSchema, n)
	for i := int64(0); i < n; i++ {
		k, err := d.readString()
		if err != nil {
			return nil, err
		}
		as, err := d.readActionSchema()
		if err != nil {
			return nil, err
		}
		m[k] = as
	}
	return m, nil
}

func (d *schemaDecoder) readDiagnostics() ([]*tfprotov5.Diagnostic, error) {
	n, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, nil
	}
	if n == 0 {
		return emptyDiagnostics, nil
	}
	diags := make([]*tfprotov5.Diagnostic, n)
	for i := int64(0); i < n; i++ {
		present, err := d.readBool()
		if err != nil {
			return nil, err
		}
		if !present {
			continue
		}
		sev, err := d.readVarint()
		if err != nil {
			return nil, err
		}
		summary, err := d.readString()
		if err != nil {
			return nil, err
		}
		detail, err := d.readString()
		if err != nil {
			return nil, err
		}
		diags[i] = &tfprotov5.Diagnostic{
			Severity: tfprotov5.DiagnosticSeverity(sev),
			Summary:  summary,
			Detail:   detail,
		}
	}
	return diags, nil
}

func (d *schemaDecoder) readServerCapabilities() (*tfprotov5.ServerCapabilities, error) {
	present, err := d.readBool()
	if err != nil || !present {
		return nil, err
	}
	planDestroy, err := d.readBool()
	if err != nil {
		return nil, err
	}
	schemaOpt, err := d.readBool()
	if err != nil {
		return nil, err
	}
	moveState, err := d.readBool()
	if err != nil {
		return nil, err
	}
	genConfig, err := d.readBool()
	if err != nil {
		return nil, err
	}
	return &tfprotov5.ServerCapabilities{
		PlanDestroy:               planDestroy,
		GetProviderSchemaOptional: schemaOpt,
		MoveResourceState:         moveState,
		GenerateResourceConfig:    genConfig,
	}, nil
}

func (d *schemaDecoder) readProviderSchemaResponse() (*tfprotov5.GetProviderSchemaResponse, error) {
	present, err := d.readBool()
	if err != nil || !present {
		return nil, err
	}
	prov, err := d.readSchema()
	if err != nil {
		return nil, err
	}
	meta, err := d.readSchema()
	if err != nil {
		return nil, err
	}
	resSchemas, err := d.readSchemaMap()
	if err != nil {
		return nil, err
	}
	dsSchemas, err := d.readSchemaMap()
	if err != nil {
		return nil, err
	}
	ephSchemas, err := d.readSchemaMap()
	if err != nil {
		return nil, err
	}
	funcs, err := d.readFunctionMap()
	if err != nil {
		return nil, err
	}
	listSchemas, err := d.readSchemaMap()
	if err != nil {
		return nil, err
	}
	actionSchemas, err := d.readActionSchemaMap()
	if err != nil {
		return nil, err
	}
	diags, err := d.readDiagnostics()
	if err != nil {
		return nil, err
	}
	caps, err := d.readServerCapabilities()
	if err != nil {
		return nil, err
	}
	return &tfprotov5.GetProviderSchemaResponse{
		Provider:                 prov,
		ProviderMeta:             meta,
		ResourceSchemas:          resSchemas,
		DataSourceSchemas:        dsSchemas,
		EphemeralResourceSchemas: ephSchemas,
		Functions:                funcs,
		ListResourceSchemas:      listSchemas,
		ActionSchemas:            actionSchemas,
		Diagnostics:              diags,
		ServerCapabilities:       caps,
	}, nil
}

func (d *schemaDecoder) readResourceIdentitySchema() (*tfprotov5.ResourceIdentitySchema, error) {
	present, err := d.readBool()
	if err != nil || !present {
		return nil, err
	}
	ver, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	numAttrs, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	var attrs []*tfprotov5.ResourceIdentitySchemaAttribute
	if numAttrs == 0 {
		attrs = emptyIdentityAttributes
	} else if numAttrs > 0 {
		attrsVal := make([]tfprotov5.ResourceIdentitySchemaAttribute, numAttrs)
		attrs = make([]*tfprotov5.ResourceIdentitySchemaAttribute, numAttrs)
		for i := int64(0); i < numAttrs; i++ {
			p, err := d.readBool()
			if err != nil {
				return nil, err
			}
			if !p {
				continue
			}
			name, err := d.readString()
			if err != nil {
				return nil, err
			}
			typ, err := d.readType()
			if err != nil {
				return nil, err
			}
			reqImport, err := d.readBool()
			if err != nil {
				return nil, err
			}
			optImport, err := d.readBool()
			if err != nil {
				return nil, err
			}
			desc, err := d.readString()
			if err != nil {
				return nil, err
			}
			attrsVal[i] = tfprotov5.ResourceIdentitySchemaAttribute{
				Name:              name,
				Type:              typ,
				RequiredForImport: reqImport,
				OptionalForImport: optImport,
				Description:       desc,
			}
			attrs[i] = &attrsVal[i]
		}
	}
	return &tfprotov5.ResourceIdentitySchema{
		Version:            ver,
		IdentityAttributes: attrs,
	}, nil
}

func (d *schemaDecoder) readResourceIdentitySchemasResponse() (*tfprotov5.GetResourceIdentitySchemasResponse, error) {
	present, err := d.readBool()
	if err != nil || !present {
		return nil, err
	}
	n, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	var idSchemas map[string]*tfprotov5.ResourceIdentitySchema
	if n >= 0 {
		idSchemas = make(map[string]*tfprotov5.ResourceIdentitySchema, n)
		for i := int64(0); i < n; i++ {
			k, err := d.readString()
			if err != nil {
				return nil, err
			}
			ris, err := d.readResourceIdentitySchema()
			if err != nil {
				return nil, err
			}
			idSchemas[k] = ris
		}
	}
	diags, err := d.readDiagnostics()
	if err != nil {
		return nil, err
	}
	return &tfprotov5.GetResourceIdentitySchemasResponse{
		IdentitySchemas: idSchemas,
		Diagnostics:     diags,
	}, nil
}

// DecodeEmbeddedSchemas gunzips and deserializes an EmbeddedSchemas payload.
func DecodeEmbeddedSchemas(compressed []byte) (*EmbeddedSchemas, error) {
	zr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer zr.Close()

	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("reading gunzipped schema blob: %w", err)
	}
	if len(raw) < len(schemaCodecMagic) || string(raw[:len(schemaCodecMagic)]) != schemaCodecMagic {
		return nil, errors.New("invalid embedded schema magic header")
	}

	dec := &schemaDecoder{
		data: raw,
		pos:  len(schemaCodecMagic),
	}

	numStrings, err := dec.readUvarint()
	if err != nil {
		return nil, fmt.Errorf("reading string table count: %w", err)
	}
	totalStringBytes, err := dec.readUvarint()
	if err != nil {
		return nil, fmt.Errorf("reading string table byte size: %w", err)
	}
	lengths := make([]uint64, numStrings)
	for i := uint64(0); i < numStrings; i++ {
		l, err := dec.readUvarint()
		if err != nil {
			return nil, fmt.Errorf("reading string length %d: %w", i, err)
		}
		lengths[i] = l
	}
	if uint64(len(dec.data)-dec.pos) < totalStringBytes {
		return nil, io.ErrUnexpectedEOF
	}
	allStrings := string(dec.data[dec.pos : dec.pos+int(totalStringBytes)])
	dec.pos += int(totalStringBytes)

	dec.strings = make([]string, numStrings)
	offset := 0
	for i, l := range lengths {
		end := offset + int(l)
		if end > len(allStrings) {
			return nil, io.ErrUnexpectedEOF
		}
		dec.strings[i] = allStrings[offset:end]
		offset = end
	}

	sdkSchema, err := dec.readProviderSchemaResponse()
	if err != nil {
		return nil, fmt.Errorf("decoding SDKProviderSchema: %w", err)
	}
	sdkIdentity, err := dec.readResourceIdentitySchemasResponse()
	if err != nil {
		return nil, fmt.Errorf("decoding SDKResourceIdentitySchemas: %w", err)
	}
	fwSchema, err := dec.readProviderSchemaResponse()
	if err != nil {
		return nil, fmt.Errorf("decoding FrameworkProviderSchema: %w", err)
	}
	fwIdentity, err := dec.readResourceIdentitySchemasResponse()
	if err != nil {
		return nil, fmt.Errorf("decoding FrameworkResourceIdentitySchemas: %w", err)
	}

	return &EmbeddedSchemas{
		SDKProviderSchema:                sdkSchema,
		SDKResourceIdentitySchemas:       sdkIdentity,
		FrameworkProviderSchema:          fwSchema,
		FrameworkResourceIdentitySchemas: fwIdentity,
	}, nil
}

// CompareProviderSchemas compares two GetProviderSchemaResponse values and
// returns human-readable descriptions of any differences found. An empty slice
// indicates the two responses are identical.
func CompareProviderSchemas(got, want *tfprotov5.GetProviderSchemaResponse) []string {
	var diffs []string
	if got == nil || want == nil {
		if got != want {
			diffs = append(diffs, fmt.Sprintf("GetProviderSchemaResponse nil mismatch: got=%v, want=%v", got == nil, want == nil))
		}
		return diffs
	}

	diffs = append(diffs, compareSingleSchema("Provider", got.Provider, want.Provider)...)
	diffs = append(diffs, compareSingleSchema("ProviderMeta", got.ProviderMeta, want.ProviderMeta)...)
	diffs = append(diffs, compareSchemaMaps("ResourceSchemas", got.ResourceSchemas, want.ResourceSchemas)...)
	diffs = append(diffs, compareSchemaMaps("DataSourceSchemas", got.DataSourceSchemas, want.DataSourceSchemas)...)
	diffs = append(diffs, compareSchemaMaps("EphemeralResourceSchemas", got.EphemeralResourceSchemas, want.EphemeralResourceSchemas)...)
	diffs = append(diffs, compareSchemaMaps("ListResourceSchemas", got.ListResourceSchemas, want.ListResourceSchemas)...)
	diffs = append(diffs, compareFunctionMaps("Functions", got.Functions, want.Functions)...)
	diffs = append(diffs, compareActionSchemaMaps("ActionSchemas", got.ActionSchemas, want.ActionSchemas)...)

	if !reflect.DeepEqual(got.ServerCapabilities, want.ServerCapabilities) {
		diffs = append(diffs, fmt.Sprintf("ServerCapabilities mismatch: embedded=%+v, live=%+v", got.ServerCapabilities, want.ServerCapabilities))
	}
	if !reflect.DeepEqual(got.Diagnostics, want.Diagnostics) {
		diffs = append(diffs, fmt.Sprintf("Diagnostics mismatch: embedded=%+v, live=%+v", got.Diagnostics, want.Diagnostics))
	}
	return diffs
}

// CompareResourceIdentitySchemas compares two GetResourceIdentitySchemasResponse
// values and returns human-readable descriptions of any differences found.
func CompareResourceIdentitySchemas(got, want *tfprotov5.GetResourceIdentitySchemasResponse) []string {
	var diffs []string
	if got == nil || want == nil {
		if got != want {
			diffs = append(diffs, fmt.Sprintf("GetResourceIdentitySchemasResponse nil mismatch: got=%v, want=%v", got == nil, want == nil))
		}
		return diffs
	}

	gotKeys := slices.Sorted(maps.Keys(got.IdentitySchemas))
	wantKeys := slices.Sorted(maps.Keys(want.IdentitySchemas))

	for _, k := range wantKeys {
		if _, ok := got.IdentitySchemas[k]; !ok {
			diffs = append(diffs, fmt.Sprintf("IdentitySchemas[%q]: missing from embedded schema (present in live provider)", k))
		}
	}
	for _, k := range gotKeys {
		if _, ok := want.IdentitySchemas[k]; !ok {
			diffs = append(diffs, fmt.Sprintf("IdentitySchemas[%q]: extra in embedded schema (not in live provider)", k))
		}
	}
	for _, k := range wantKeys {
		g, ok := got.IdentitySchemas[k]
		if !ok {
			continue
		}
		w := want.IdentitySchemas[k]
		if !reflect.DeepEqual(g, w) {
			diffs = append(diffs, compareResourceIdentitySchema(fmt.Sprintf("IdentitySchemas[%q]", k), g, w)...)
		}
	}

	if !reflect.DeepEqual(got.Diagnostics, want.Diagnostics) {
		diffs = append(diffs, fmt.Sprintf("IdentitySchemas Diagnostics mismatch: embedded=%+v, live=%+v", got.Diagnostics, want.Diagnostics))
	}
	return diffs
}

func compareResourceIdentitySchema(prefix string, got, want *tfprotov5.ResourceIdentitySchema) []string {
	var diffs []string
	if got == nil || want == nil {
		if got != want {
			diffs = append(diffs, fmt.Sprintf("%s: nil mismatch (embedded=%v, live=%v)", prefix, got == nil, want == nil))
		}
		return diffs
	}
	if got.Version != want.Version {
		diffs = append(diffs, fmt.Sprintf("%s.Version: embedded=%d, live=%d", prefix, got.Version, want.Version))
	}
	gotAttrs := make(map[string]*tfprotov5.ResourceIdentitySchemaAttribute, len(got.IdentityAttributes))
	for _, a := range got.IdentityAttributes {
		if a != nil {
			gotAttrs[a.Name] = a
		}
	}
	wantAttrs := make(map[string]*tfprotov5.ResourceIdentitySchemaAttribute, len(want.IdentityAttributes))
	for _, a := range want.IdentityAttributes {
		if a != nil {
			wantAttrs[a.Name] = a
		}
	}
	for _, name := range slices.Sorted(maps.Keys(wantAttrs)) {
		if _, ok := gotAttrs[name]; !ok {
			diffs = append(diffs, fmt.Sprintf("%s.IdentityAttributes[%q]: missing from embedded schema", prefix, name))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(gotAttrs)) {
		if _, ok := wantAttrs[name]; !ok {
			diffs = append(diffs, fmt.Sprintf("%s.IdentityAttributes[%q]: extra in embedded schema", prefix, name))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(wantAttrs)) {
		ga, ok := gotAttrs[name]
		if !ok {
			continue
		}
		wa := wantAttrs[name]
		if !reflect.DeepEqual(ga, wa) {
			diffs = append(diffs, fmt.Sprintf("%s.IdentityAttributes[%q] differs: embedded=%+v, live=%+v", prefix, name, ga, wa))
		}
	}
	if len(diffs) == 0 && !reflect.DeepEqual(got, want) {
		diffs = append(diffs, fmt.Sprintf("%s differs: embedded=%+v, live=%+v", prefix, got, want))
	}
	return diffs
}

func compareSchemaMaps(label string, got, want map[string]*tfprotov5.Schema) []string {
	var diffs []string
	if (got == nil) != (want == nil) && len(got) == 0 && len(want) == 0 {
		diffs = append(diffs, fmt.Sprintf("%s nil map mismatch: embedded is nil=%v, live is nil=%v", label, got == nil, want == nil))
		return diffs
	}
	gotKeys := slices.Sorted(maps.Keys(got))
	wantKeys := slices.Sorted(maps.Keys(want))

	for _, k := range wantKeys {
		if _, ok := got[k]; !ok {
			diffs = append(diffs, fmt.Sprintf("%s[%q]: missing from embedded schema (present in live provider)", label, k))
		}
	}
	for _, k := range gotKeys {
		if _, ok := want[k]; !ok {
			diffs = append(diffs, fmt.Sprintf("%s[%q]: extra in embedded schema (not in live provider)", label, k))
		}
	}
	for _, k := range wantKeys {
		gs, ok := got[k]
		if !ok {
			continue
		}
		ws := want[k]
		if !reflect.DeepEqual(gs, ws) {
			diffs = append(diffs, compareSingleSchema(fmt.Sprintf("%s[%q]", label, k), gs, ws)...)
		}
	}
	return diffs
}

func compareSingleSchema(prefix string, got, want *tfprotov5.Schema) []string {
	var diffs []string
	if got == nil || want == nil {
		if got != want {
			diffs = append(diffs, fmt.Sprintf("%s: nil mismatch (embedded=%v, live=%v)", prefix, got == nil, want == nil))
		}
		return diffs
	}
	if got.Version != want.Version {
		diffs = append(diffs, fmt.Sprintf("%s.Version: embedded=%d, live=%d", prefix, got.Version, want.Version))
	}
	diffs = append(diffs, compareSchemaBlock(prefix+".Block", got.Block, want.Block)...)
	if len(diffs) == 0 && !reflect.DeepEqual(got, want) {
		diffs = append(diffs, fmt.Sprintf("%s differs: embedded=%+v, live=%+v", prefix, got, want))
	}
	return diffs
}

func compareSchemaBlock(prefix string, got, want *tfprotov5.SchemaBlock) []string {
	var diffs []string
	if got == nil || want == nil {
		if got != want {
			diffs = append(diffs, fmt.Sprintf("%s: nil mismatch (embedded=%v, live=%v)", prefix, got == nil, want == nil))
		}
		return diffs
	}
	if got.Version != want.Version {
		diffs = append(diffs, fmt.Sprintf("%s.Version: embedded=%d, live=%d", prefix, got.Version, want.Version))
	}
	if got.Description != want.Description || got.DescriptionKind != want.DescriptionKind || got.Deprecated != want.Deprecated || got.DeprecationMessage != want.DeprecationMessage {
		diffs = append(diffs, fmt.Sprintf("%s metadata differs: embedded=(desc=%q, kind=%v, deprecated=%v, depMsg=%q), live=(desc=%q, kind=%v, deprecated=%v, depMsg=%q)",
			prefix, got.Description, got.DescriptionKind, got.Deprecated, got.DeprecationMessage,
			want.Description, want.DescriptionKind, want.Deprecated, want.DeprecationMessage))
	}

	gotAttrs := make(map[string]*tfprotov5.SchemaAttribute, len(got.Attributes))
	for _, a := range got.Attributes {
		if a != nil {
			gotAttrs[a.Name] = a
		}
	}
	wantAttrs := make(map[string]*tfprotov5.SchemaAttribute, len(want.Attributes))
	for _, a := range want.Attributes {
		if a != nil {
			wantAttrs[a.Name] = a
		}
	}
	for _, name := range slices.Sorted(maps.Keys(wantAttrs)) {
		if _, ok := gotAttrs[name]; !ok {
			diffs = append(diffs, fmt.Sprintf("%s.Attributes[%q]: missing from embedded schema", prefix, name))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(gotAttrs)) {
		if _, ok := wantAttrs[name]; !ok {
			diffs = append(diffs, fmt.Sprintf("%s.Attributes[%q]: extra in embedded schema", prefix, name))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(wantAttrs)) {
		ga, ok := gotAttrs[name]
		if !ok {
			continue
		}
		wa := wantAttrs[name]
		if !reflect.DeepEqual(ga, wa) {
			diffs = append(diffs, fmt.Sprintf("%s.Attributes[%q] differs: embedded=%+v, live=%+v", prefix, name, ga, wa))
		}
	}

	gotBlocks := make(map[string]*tfprotov5.SchemaNestedBlock, len(got.BlockTypes))
	for _, b := range got.BlockTypes {
		if b != nil {
			gotBlocks[b.TypeName] = b
		}
	}
	wantBlocks := make(map[string]*tfprotov5.SchemaNestedBlock, len(want.BlockTypes))
	for _, b := range want.BlockTypes {
		if b != nil {
			wantBlocks[b.TypeName] = b
		}
	}
	for _, name := range slices.Sorted(maps.Keys(wantBlocks)) {
		if _, ok := gotBlocks[name]; !ok {
			diffs = append(diffs, fmt.Sprintf("%s.BlockTypes[%q]: missing from embedded schema", prefix, name))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(gotBlocks)) {
		if _, ok := wantBlocks[name]; !ok {
			diffs = append(diffs, fmt.Sprintf("%s.BlockTypes[%q]: extra in embedded schema", prefix, name))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(wantBlocks)) {
		gb, ok := gotBlocks[name]
		if !ok {
			continue
		}
		wb := wantBlocks[name]
		if !reflect.DeepEqual(gb, wb) {
			if gb.Nesting != wb.Nesting || gb.MinItems != wb.MinItems || gb.MaxItems != wb.MaxItems {
				diffs = append(diffs, fmt.Sprintf("%s.BlockTypes[%q] nesting/limits differ: embedded=(nesting=%v, min=%d, max=%d), live=(nesting=%v, min=%d, max=%d)",
					prefix, name, gb.Nesting, gb.MinItems, gb.MaxItems, wb.Nesting, wb.MinItems, wb.MaxItems))
			}
			diffs = append(diffs, compareSchemaBlock(fmt.Sprintf("%s.BlockTypes[%q].Block", prefix, name), gb.Block, wb.Block)...)
		}
	}

	if len(diffs) == 0 && !reflect.DeepEqual(got, want) {
		diffs = append(diffs, fmt.Sprintf("%s differs: embedded=%+v, live=%+v", prefix, got, want))
	}
	return diffs
}

func compareFunctionMaps(label string, got, want map[string]*tfprotov5.Function) []string {
	var diffs []string
	gotKeys := slices.Sorted(maps.Keys(got))
	wantKeys := slices.Sorted(maps.Keys(want))
	for _, k := range wantKeys {
		if _, ok := got[k]; !ok {
			diffs = append(diffs, fmt.Sprintf("%s[%q]: missing from embedded schema", label, k))
		}
	}
	for _, k := range gotKeys {
		if _, ok := want[k]; !ok {
			diffs = append(diffs, fmt.Sprintf("%s[%q]: extra in embedded schema", label, k))
		}
	}
	for _, k := range wantKeys {
		gf, ok := got[k]
		if !ok {
			continue
		}
		wf := want[k]
		if !reflect.DeepEqual(gf, wf) {
			diffs = append(diffs, fmt.Sprintf("%s[%q] differs: embedded=%+v, live=%+v", label, k, gf, wf))
		}
	}
	return diffs
}

func compareActionSchemaMaps(label string, got, want map[string]*tfprotov5.ActionSchema) []string {
	var diffs []string
	gotKeys := slices.Sorted(maps.Keys(got))
	wantKeys := slices.Sorted(maps.Keys(want))
	for _, k := range wantKeys {
		if _, ok := got[k]; !ok {
			diffs = append(diffs, fmt.Sprintf("%s[%q]: missing from embedded schema", label, k))
		}
	}
	for _, k := range gotKeys {
		if _, ok := want[k]; !ok {
			diffs = append(diffs, fmt.Sprintf("%s[%q]: extra in embedded schema", label, k))
		}
	}
	for _, k := range wantKeys {
		ga, ok := got[k]
		if !ok {
			continue
		}
		wa := want[k]
		if !reflect.DeepEqual(ga, wa) {
			diffs = append(diffs, fmt.Sprintf("%s[%q] differs: embedded=%+v, live=%+v", label, k, ga, wa))
		}
	}
	return diffs
}
