package provider

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/gob"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func gzipRaw(b []byte) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write(b)
	_ = zw.Close()
	return buf.Bytes()
}

func encodeGobPayload(p gobPayload, extra ...byte) []byte {
	var raw bytes.Buffer
	raw.WriteString(schemaCodecMagic)
	_ = gob.NewEncoder(&raw).Encode(&p)
	raw.Write(extra)
	return gzipRaw(raw.Bytes())
}

func FuzzDecodeEmbeddedSchemas(f *testing.F) {
	flipped := bytes.Clone(embeddedSchemaBlob)
	if len(flipped) > 0 {
		flipped[len(flipped)/2] ^= 0xff
	}
	for _, s := range [][]byte{
		nil, []byte("not-gzip"), embeddedSchemaBlob[:min(len(embeddedSchemaBlob), 64)], flipped,
		gzipRaw([]byte("BADMAGIC")), gzipRaw([]byte(schemaCodecMagic)),
		gzipRaw(append([]byte(schemaCodecMagic), "corrupt-gob"...)),
		encodeGobPayload(gobPayload{HasProvider: true, Types: [][]byte{[]byte("bad-json")}}),
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = DecodeEmbeddedSchemas(data) })
}

func FuzzSchemaCodecRoundTrip(f *testing.F) {
	for i := byte(0); i < 16; i++ {
		f.Add([]byte{i, i ^ 0x5a, i * 17, i + 3, 0xff - i, i * 31, 0, 7, 8})
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		orig := buildFuzzSchemas(data)
		blob, err := EncodeEmbeddedSchemas(orig)
		if err != nil {
			t.Fatalf("EncodeEmbeddedSchemas: %v", err)
		}
		got, err := DecodeEmbeddedSchemas(blob)
		if err != nil {
			t.Fatalf("DecodeEmbeddedSchemas: %v", err)
		}
		if diff := cmp.Diff(orig, got, cmpopts.EquateEmpty()); diff != "" {
			t.Fatalf("round-trip mismatch:\n%s", diff)
		}
	})
}

func buildFuzzSchemas(data []byte) *EmbeddedSchemas {
	idx := 0
	next := func() byte {
		idx++
		if len(data) == 0 {
			return byte(idx)
		}
		return data[(idx-1)%len(data)] ^ byte(idx*13)
	}
	bit := func() bool { return next()&1 == 1 }
	str := func() string { return string([]byte{'a' + next()%26, 'a' + next()%26}[:next()%3]) }
	prims := []tftypes.Type{tftypes.String, tftypes.Number, tftypes.Bool, tftypes.DynamicPseudoType}
	var genType func(int) tftypes.Type
	genType = func(d int) tftypes.Type {
		if d >= 2 {
			return prims[int(next())%len(prims)]
		}
		switch int(next()) % 9 {
		case 4:
			return tftypes.List{ElementType: genType(d + 1)}
		case 5:
			return tftypes.Set{ElementType: genType(d + 1)}
		case 6:
			return tftypes.Map{ElementType: genType(d + 1)}
		case 7:
			return tftypes.Tuple{ElementTypes: []tftypes.Type{genType(d + 1)}}
		case 8:
			k := str() + "k"
			var opts map[string]struct{}
			if bit() {
				opts = map[string]struct{}{k: {}}
			}
			return tftypes.Object{AttributeTypes: map[string]tftypes.Type{k: genType(d + 1)}, OptionalAttributes: opts}
		default:
			return prims[int(next())%len(prims)]
		}
	}
	var genBlock func(int) *tfprotov5.SchemaBlock
	genBlock = func(d int) *tfprotov5.SchemaBlock {
		if next()%8 == 0 {
			return nil
		}
		b := &tfprotov5.SchemaBlock{Version: int64(next() % 3), Description: str(), DescriptionKind: tfprotov5.StringKind(next() % 2), Deprecated: bit(), DeprecationMessage: str()}
		if bit() {
			b.Attributes = []*tfprotov5.SchemaAttribute{{Name: str(), Type: genType(0), Description: str(), Required: bit(), Optional: bit(), Computed: bit(), Sensitive: bit(), Deprecated: bit(), WriteOnly: bit(), DescriptionKind: tfprotov5.StringKind(next() % 2), DeprecationMessage: str()}}
		}
		if d < 2 && bit() {
			b.BlockTypes = []*tfprotov5.SchemaNestedBlock{{TypeName: str(), Block: genBlock(d + 1), Nesting: tfprotov5.SchemaNestedBlockNestingMode(next() % 6), MinItems: int64(next() % 2), MaxItems: int64(next() % 4)}}
		}
		return b
	}
	genSchema := func() *tfprotov5.Schema { return &tfprotov5.Schema{Version: int64(next() % 3), Block: genBlock(0)} }
	genMap := func() map[string]*tfprotov5.Schema {
		if next()%5 == 0 {
			return nil
		}
		return map[string]*tfprotov5.Schema{"s_" + str(): genSchema()}
	}
	return &EmbeddedSchemas{
		ProviderSchema: &tfprotov5.GetProviderSchemaResponse{
			Provider: genSchema(), ProviderMeta: genSchema(),
			ResourceSchemas: genMap(), DataSourceSchemas: genMap(), EphemeralResourceSchemas: genMap(), ListResourceSchemas: genMap(),
			Functions:          map[string]*tfprotov5.Function{"fn": {Parameters: []*tfprotov5.FunctionParameter{{Name: str(), Type: genType(0), AllowNullValue: bit(), AllowUnknownValues: bit(), Description: str(), DescriptionKind: tfprotov5.StringKind(next() % 2)}}, VariadicParameter: &tfprotov5.FunctionParameter{Name: str(), Type: genType(0)}, Return: &tfprotov5.FunctionReturn{Type: genType(0)}, Summary: str(), Description: str(), DescriptionKind: tfprotov5.StringKind(next() % 2), DeprecationMessage: str()}},
			ActionSchemas:      map[string]*tfprotov5.ActionSchema{"act": {Schema: genSchema()}},
			ServerCapabilities: &tfprotov5.ServerCapabilities{PlanDestroy: bit(), GetProviderSchemaOptional: bit(), MoveResourceState: bit(), GenerateResourceConfig: bit()},
		},
		ResourceIdentitySchemas: &tfprotov5.GetResourceIdentitySchemasResponse{
			IdentitySchemas: map[string]*tfprotov5.ResourceIdentitySchema{"r": {Version: int64(next() % 3), IdentityAttributes: []*tfprotov5.ResourceIdentitySchemaAttribute{{Name: str(), Type: genType(0), RequiredForImport: bit(), OptionalForImport: bit(), Description: str()}}}},
		},
	}
}

func TestEmbeddedSchemas_MutationsAndBoundaries(t *testing.T) {
	base := buildFuzzSchemas([]byte{1, 2, 3, 4})
	base.ProviderSchema.ResourceSchemas = map[string]*tfprotov5.Schema{"r": {Version: 1, Block: &tfprotov5.SchemaBlock{Attributes: []*tfprotov5.SchemaAttribute{{Name: "a", Type: tftypes.String, Required: true}}, BlockTypes: []*tfprotov5.SchemaNestedBlock{{TypeName: "b", Nesting: tfprotov5.SchemaNestedBlockNestingModeSingle}}}}}
	base.ProviderSchema.ListResourceSchemas = map[string]*tfprotov5.Schema{"lr": {Version: 1}}
	blob, err := EncodeEmbeddedSchemas(base)
	if err != nil {
		t.Fatalf("EncodeEmbeddedSchemas: %v", err)
	}
	decoded, err := DecodeEmbeddedSchemas(blob)
	if err != nil {
		t.Fatalf("DecodeEmbeddedSchemas: %v", err)
	}
	for i, mut := range []func(*EmbeddedSchemas){
		func(s *EmbeddedSchemas) { s.ProviderSchema.ResourceSchemas["new"] = &tfprotov5.Schema{Version: 1} },
		func(s *EmbeddedSchemas) { delete(s.ProviderSchema.ResourceSchemas, "r") },
		func(s *EmbeddedSchemas) {
			s.ProviderSchema.ResourceSchemas["r"].Block.Attributes[0].Type = tftypes.Number
		},
		func(s *EmbeddedSchemas) { s.ProviderSchema.ResourceSchemas["r"].Block.Attributes[0].Required = false },
		func(s *EmbeddedSchemas) { s.ProviderSchema.ResourceSchemas["r"].Block.Attributes[0].Optional = true },
		func(s *EmbeddedSchemas) { s.ProviderSchema.ResourceSchemas["r"].Block.Attributes[0].Computed = true },
		func(s *EmbeddedSchemas) { s.ProviderSchema.ResourceSchemas["r"].Block.Attributes[0].Sensitive = true },
		func(s *EmbeddedSchemas) { s.ProviderSchema.ResourceSchemas["r"].Block.Attributes[0].WriteOnly = true },
		func(s *EmbeddedSchemas) { s.ProviderSchema.ResourceSchemas["r"].Block.Attributes[0].Deprecated = true },
		func(s *EmbeddedSchemas) { s.ProviderSchema.ResourceSchemas["r"].Block.BlockTypes[0].MinItems = 1 },
		func(s *EmbeddedSchemas) { s.ResourceIdentitySchemas.IdentitySchemas["r"].Version = 99 },
		func(s *EmbeddedSchemas) { s.ProviderSchema.Functions["fn"].Return.Type = tftypes.Bool },
		func(s *EmbeddedSchemas) { s.ProviderSchema.ListResourceSchemas["lr"].Version = 2 },
		func(s *EmbeddedSchemas) { s.ProviderSchema.ActionSchemas["act"].Schema.Version = 9 },
	} {
		clone, _ := DecodeEmbeddedSchemas(blob)
		if mut(clone); cmp.Diff(decoded, clone, cmpopts.EquateEmpty()) == "" {
			t.Fatalf("mutant %d survived cmp.Diff", i)
		}
	}
	if _, err := EncodeEmbeddedSchemas(nil); err == nil {
		t.Fatal("expected error on nil EncodeEmbeddedSchemas")
	}
	if _, err := EncodeEmbeddedSchemas(&EmbeddedSchemas{ProviderSchema: &tfprotov5.GetProviderSchemaResponse{Diagnostics: []*tfprotov5.Diagnostic{{Summary: "err"}}}}); err == nil {
		t.Fatal("expected error on EncodeEmbeddedSchemas with diagnostics")
	}
	oneAttr := &tfprotov5.Schema{Block: &tfprotov5.SchemaBlock{Attributes: []*tfprotov5.SchemaAttribute{{Name: "a"}}}}
	for _, b := range [][]byte{
		nil,
		[]byte("bad"),
		gzipRaw([]byte("BADMAGIC")),
		gzipRaw([]byte(schemaCodecMagic)),
		encodeGobPayload(gobPayload{}, 0xff),
		encodeGobPayload(gobPayload{HasProvider: true, Provider: oneAttr}),
		encodeGobPayload(gobPayload{Types: [][]byte{[]byte(`"string"`)}}),
		encodeGobPayload(gobPayload{HasProvider: true, Provider: oneAttr, Types: [][]byte{[]byte("bad-json")}}),
	} {
		if _, err := DecodeEmbeddedSchemas(b); err == nil {
			t.Fatalf("expected DecodeEmbeddedSchemas error for %x", b)
		}
	}
	if EmbeddedSchemaLoaded() {
		srv := NewEmbeddedProviderServer(func() tfprotov5.ProviderServer { return struct{ fullProviderServer }{} })()
		r1, _ := srv.GetProviderSchema(context.Background(), &tfprotov5.GetProviderSchemaRequest{})
		delete(r1.ResourceSchemas, "google_compute_instance")
		r2, _ := srv.GetProviderSchema(context.Background(), &tfprotov5.GetProviderSchemaRequest{})
		i1, _ := srv.GetResourceIdentitySchemas(context.Background(), &tfprotov5.GetResourceIdentitySchemasRequest{})
		delete(i1.IdentitySchemas, "google_compute_instance")
		i2, _ := srv.GetResourceIdentitySchemas(context.Background(), &tfprotov5.GetResourceIdentitySchemasRequest{})
		if r2.ResourceSchemas["google_compute_instance"] == nil || i2.IdentitySchemas["google_compute_instance"] == nil {
			t.Fatal("cached embedded schema maps not cloned")
		}
	}
}
