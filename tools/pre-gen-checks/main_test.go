package main

import (
	"testing"
)

func TestHasPathPrefix(t *testing.T) {
	files := []string{
		"mmv1/products/compute/resource.yaml",
		"tools/template-check/main.go",
		".ci/magician/main.go",
	}

	if !hasPathPrefix(files, "tools/template-check/") {
		t.Errorf("expected true for tools/template-check/")
	}
	if !hasPathPrefix(files, ".ci/") {
		t.Errorf("expected true for .ci/")
	}
	if hasPathPrefix(files, "tools/go-changelog/") {
		t.Errorf("expected false for tools/go-changelog/")
	}
}

func TestFilterFiles(t *testing.T) {
	files := []string{"foo.tmpl", "bar.go", "baz.tmpl", "qux.yaml"}
	tmpls := filterFiles(files, func(p string) bool {
		return len(p) > 5 && p[len(p)-5:] == ".tmpl"
	})
	if len(tmpls) != 2 {
		t.Fatalf("expected 2 tmpl files, got %d", len(tmpls))
	}
	if tmpls[0] != "foo.tmpl" || tmpls[1] != "baz.tmpl" {
		t.Errorf("unexpected tmpls: %v", tmpls)
	}
}
