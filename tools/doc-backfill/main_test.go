package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCLI(t *testing.T) {
	tmp := t.TempDir()
	docsDir := filepath.Join(tmp, "docs")
	servicesDir := filepath.Join(tmp, "services")
	svcDir := filepath.Join(servicesDir, "demo")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(svcDir, 0755); err != nil {
		t.Fatal(err)
	}

	md := `---
subcategory: "Demo"
description: |-
  Demo resource.
---

# google_demo_item

## Argument Reference

* ` + "`title`" + ` - (Required) Human-readable title for the demo item.
`
	if err := os.WriteFile(filepath.Join(docsDir, "demo_item.html.markdown"), []byte(md), 0644); err != nil {
		t.Fatal(err)
	}

	src := `package demo

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
)

func init() {
	registry.Schema{
		Name:        "google_demo_item",
		ProductName: "demo",
		Type:        registry.SchemaTypeResource,
		Schema:      ResourceDemoItem(),
	}.Register()
}

func ResourceDemoItem() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"title": {
				Type:     schema.TypeString,
				Required: true,
			},
		},
	}
}
`
	if err := os.WriteFile(filepath.Join(svcDir, "resource_demo_item.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	// 1. --check exits 1 when backfill diffs are pending.
	var out, errBuf bytes.Buffer
	code := runCLI([]string{"--docs-dir", docsDir, "--services-dir", servicesDir, "--check"}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("expected exit code 1 from --check with pending diffs, got %d (stderr: %s)", code, errBuf.String())
	}

	// 2. --write + --report-json exits 0 and writes both file and JSON report.
	out.Reset()
	errBuf.Reset()
	reportFile := filepath.Join(tmp, "out", "report.json")
	code = runCLI([]string{"--docs-dir", docsDir, "--services-dir", servicesDir, "--service", "demo", "--resource", "google_demo_item", "--write", "--report-json", reportFile}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit code 0 from --write, got %d (stderr: %s)", code, errBuf.String())
	}
	if _, err := os.Stat(reportFile); err != nil {
		t.Fatalf("expected report.json to exist: %v", err)
	}

	// 3. --check now exits 0!
	out.Reset()
	errBuf.Reset()
	code = runCLI([]string{"--docs-dir", docsDir, "--services-dir", servicesDir, "--check"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit code 0 from --check after --write, got %d (stderr: %s)", code, errBuf.String())
	}

	// 4. Invalid --mode exits 2.
	out.Reset()
	errBuf.Reset()
	code = runCLI([]string{"--docs-dir", docsDir, "--services-dir", servicesDir, "--mode", "bogus"}, &out, &errBuf)
	if code != 2 || !strings.Contains(errBuf.String(), "invalid --mode") {
		t.Fatalf("expected exit code 2 on invalid --mode, got %d (stderr: %s)", code, errBuf.String())
	}
}
