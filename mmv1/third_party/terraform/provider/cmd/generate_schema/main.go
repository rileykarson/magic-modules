package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"reflect"

	"github.com/hashicorp/terraform-provider-google/google/provider"
)

func main() {
	outputPath := flag.String("output", "provider_schema.gz", "output path for the compressed embedded provider schema")
	flag.Parse()

	if err := os.Setenv("GOOGLE_TERRAFORM_LIVE_SCHEMA", "1"); err != nil {
		fmt.Fprintf(os.Stderr, "error setting GOOGLE_TERRAFORM_LIVE_SCHEMA: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	schemas, err := provider.ComputeLiveEmbeddedSchemas(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error computing live provider schemas: %v\n", err)
		os.Exit(1)
	}

	encoded, err := provider.EncodeEmbeddedSchemas(schemas)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error encoding provider schemas: %v\n", err)
		os.Exit(1)
	}

	decoded, err := provider.DecodeEmbeddedSchemas(encoded)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error verifying encoded provider schemas: %v\n", err)
		os.Exit(1)
	}
	if !reflect.DeepEqual(decoded, schemas) {
		fmt.Fprintf(os.Stderr, "round-trip verification failed: decoded schemas do not match live schemas\n")
		os.Exit(1)
	}

	tmpPath := *outputPath + ".tmp"
	if err := os.WriteFile(tmpPath, encoded, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", tmpPath, err)
		os.Exit(1)
	}
	if err := os.Rename(tmpPath, *outputPath); err != nil {
		_ = os.Remove(tmpPath)
		fmt.Fprintf(os.Stderr, "error renaming %s to %s: %v\n", tmpPath, *outputPath, err)
		os.Exit(1)
	}
}
