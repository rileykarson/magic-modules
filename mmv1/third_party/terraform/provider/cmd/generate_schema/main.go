package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"reflect"

	"github.com/hashicorp/terraform-provider-google/google/provider"
)

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func main() {
	outputPath := flag.String("output", "provider_schema.gz", "output path for the compressed embedded provider schema")
	flag.Parse()

	if err := os.Setenv("GOOGLE_TERRAFORM_LIVE_SCHEMA", "1"); err != nil {
		fatalf("setting GOOGLE_TERRAFORM_LIVE_SCHEMA: %v", err)
	}
	schemas, err := provider.ComputeLiveEmbeddedSchemas(context.Background())
	if err != nil {
		fatalf("computing live provider schemas: %v", err)
	}
	encoded, err := provider.EncodeEmbeddedSchemas(schemas)
	if err != nil {
		fatalf("encoding provider schemas: %v", err)
	}
	decoded, err := provider.DecodeEmbeddedSchemas(encoded)
	if err != nil || !reflect.DeepEqual(decoded, schemas) {
		fatalf("round-trip verification failed: %v", err)
	}
	tmpPath := *outputPath + ".tmp"
	if err := os.WriteFile(tmpPath, encoded, 0644); err != nil {
		fatalf("writing %s: %v", tmpPath, err)
	}
	if err := os.Rename(tmpPath, *outputPath); err != nil {
		_ = os.Remove(tmpPath)
		fatalf("renaming %s to %s: %v", tmpPath, *outputPath, err)
	}
}
