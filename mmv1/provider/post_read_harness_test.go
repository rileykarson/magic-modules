// Copyright 2026 Google LLC.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provider

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/magic-modules/mmv1/api"
	"github.com/GoogleCloudPlatform/magic-modules/mmv1/api/product"
)

func renderTestResourceGo(t *testing.T, res *api.Resource, p *api.Product) (string, *ast.File) {
	t.Helper()
	fsys := os.DirFS("..")
	outDir := filepath.Join(t.TempDir(), "terraform-provider-google")
	if err := os.MkdirAll(filepath.Join(outDir, "google", "services", strings.ToLower(p.Name)), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	providerObj := NewTerraform(p, "ga", time.Now(), fsys)
	res.TargetVersionName = "ga"
	res.Properties = res.AddExtraFields(res.Properties, nil)
	goFile := filepath.Join(outDir, "google", "services", strings.ToLower(p.Name), "resource_"+p.TerraformName()+"_"+strings.ToLower(res.Name)+".go")
	providerObj.GenerateResourceFile(*res, goFile)
	contentBytes, err := os.ReadFile(goFile)
	if err != nil {
		t.Fatalf("failed to read generated resource file %s: %v", goFile, err)
	}
	content := string(contentBytes)
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, goFile, content, parser.AllErrors)
	if err != nil {
		t.Fatalf("generated Go file %s failed to parse:\n%v\n--- content ---\n%s", goFile, err, content)
	}
	return content, parsed
}

func extractFuncSource(t *testing.T, src string, file *ast.File, funcName string) string {
	t.Helper()
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == funcName {
			return src[fn.Pos()-1 : fn.End()-1]
		}
	}
	t.Fatalf("function %s not found in generated file", funcName)
	return ""
}

func TestExcludePostCreateAndUpdateReadGeneration(t *testing.T) {
	t.Run("Sync Updatable Resource with UpdateMask, Project, Region, VirtualFields, DeletionPolicy", func(t *testing.T) {
		res := &api.Resource{
			Name:                  "Widget",
			BaseUrl:               "projects/{{project}}/regions/{{region}}/widgets",
			SelfLink:              "projects/{{project}}/regions/{{region}}/widgets/{{name}}",
			IdFormat:              "projects/{{project}}/regions/{{region}}/widgets/{{name}}",
			UpdateVerb:            "PATCH",
			UpdateMask:            true,
			ExcludePostCreateRead: true,
			ExcludePostUpdateRead: true,
			Parameters: []*api.Type{
				{Name: "region", Type: "String", IgnoreRead: true},
			},
			Properties: []*api.Type{
				{Name: "name", Type: "String", Output: true},
				{Name: "displayName", Type: "String"},
			},
			VirtualFields: []*api.Type{
				{Name: "force_destroy", Type: "Boolean", DefaultValue: false},
			},
		}
		p := &api.Product{
			Name:        " Acme",
			Versions:    []*product.Version{{Name: "ga", BaseUrl: "https://acme.googleapis.com/v1/"}},
			Objects:     []*api.Resource{res},
			PackagePath: "products/acme",
		}
		p.Name = "Acme"
		p.Version = p.Versions[0]
		res.SetDefault(p)
		for _, prop := range res.AllProperties() {
			prop.SetDefault(res)
		}

		src, parsed := renderTestResourceGo(t, res, p)
		createSrc := extractFuncSource(t, src, parsed, "resourceAcmeWidgetCreate")
		updateSrc := extractFuncSource(t, src, parsed, "resourceAcmeWidgetUpdate")

		if strings.Contains(createSrc, "resourceAcmeWidgetRead(") {
			t.Errorf("Create should not call resourceAcmeWidgetRead when ExcludePostCreateRead=true:\n%s", createSrc)
		}
		if !strings.Contains(createSrc, "ResourceAcmeWidgetFlatten(d, meta, res, config, project, userAgent, billingProject, url, headers)") {
			t.Errorf("Create should call ResourceAcmeWidgetFlatten with res:\n%s", createSrc)
		}
		// Verify project, region, virtual_fields, and deletion_policy are set before Flatten, and Identity is set after Flatten
		flattenIdx := strings.Index(createSrc, "ResourceAcmeWidgetFlatten(")
		projectIdx := strings.Index(createSrc, `d.Set("project", project)`)
		regionIdx := strings.Index(createSrc, `d.Set("region", region)`)
		delPolIdx := strings.Index(createSrc, `d.Set("deletion_policy"`)
		virtIdx := strings.Index(createSrc, `d.Set("force_destroy"`)
		identityIdx := strings.Index(createSrc, `d.Identity()`)
		if projectIdx == -1 || projectIdx > flattenIdx {
			t.Errorf("Create must call d.Set(\"project\", project) before ResourceAcmeWidgetFlatten")
		}
		if regionIdx == -1 || regionIdx > flattenIdx {
			t.Errorf("Create must call d.Set(\"region\", region) before ResourceAcmeWidgetFlatten")
		}
		if delPolIdx == -1 || delPolIdx > flattenIdx {
			t.Errorf("Create must set deletion_policy default before ResourceAcmeWidgetFlatten")
		}
		if virtIdx == -1 || virtIdx > flattenIdx {
			t.Errorf("Create must set virtual field default before ResourceAcmeWidgetFlatten")
		}
		if identityIdx == -1 || identityIdx < flattenIdx {
			t.Errorf("Create must populate d.Identity() AFTER ResourceAcmeWidgetFlatten so computed identity fields are present")
		}

		if strings.Contains(updateSrc, "resourceAcmeWidgetRead(") {
			t.Errorf("Update should not call resourceAcmeWidgetRead when ExcludePostUpdateRead=true:\n%s", updateSrc)
		}
		if !strings.Contains(updateSrc, "var updateRes map[string]interface{}") {
			t.Errorf("Update should declare updateRes:\n%s", updateSrc)
		}
		if !strings.Contains(updateSrc, "updateRes = res") {
			t.Errorf("Sync Update should assign updateRes = res:\n%s", updateSrc)
		}
		if !strings.Contains(updateSrc, "ResourceAcmeWidgetFlatten(d, meta, updateRes, config, project, userAgent, billingProject, url, headers)") {
			t.Errorf("Update should call ResourceAcmeWidgetFlatten with updateRes:\n%s", updateSrc)
		}
	})

	t.Run("OpAsync Resource with ResourceInsideResponse and ErrorRetryOnAsync", func(t *testing.T) {
		res := &api.Resource{
			Name:                  "Cluster",
			BaseUrl:               "projects/{{project}}/locations/{{location}}/clusters",
			SelfLink:              "projects/{{project}}/locations/{{location}}/clusters/{{name}}",
			IdFormat:              "projects/{{project}}/locations/{{location}}/clusters/{{name}}",
			UpdateVerb:            "PATCH",
			UpdateMask:            true,
			AutogenAsync:          true,
			ErrorRetryOnAsync:     true,
			ExcludePostCreateRead: true,
			ExcludePostUpdateRead: true,
			Async: &api.Async{
				Type:    "OpAsync",
				Actions: []string{"create", "update", "delete"},
				OpAsync: api.OpAsync{
					Result: api.OpAsyncResult{
						ResourceInsideResponse: true,
					},
				},
				Operation: &api.Operation{
					Timeouts: api.NewTimeouts(),
				},
			},
			Parameters: []*api.Type{
				{Name: "location", Type: "String", Required: true, Immutable: true, UrlParamOnly: true},
				{Name: "name", Type: "String", Required: true, Immutable: true},
			},
			Properties: []*api.Type{
				{Name: "description", Type: "String"},
				{Name: "state", Type: "String", Output: true},
			},
		}
		p := &api.Product{
			Name:        "Acme",
			Versions:    []*product.Version{{Name: "ga", BaseUrl: "https://acme.googleapis.com/v1/"}},
			Objects:     []*api.Resource{res},
			PackagePath: "products/acme",
		}
		p.Version = p.Versions[0]
		res.SetDefault(p)
		for _, prop := range res.AllProperties() {
			prop.SetDefault(res)
		}

		src, parsed := renderTestResourceGo(t, res, p)
		createSrc := extractFuncSource(t, src, parsed, "resourceAcmeClusterCreate")
		updateSrc := extractFuncSource(t, src, parsed, "resourceAcmeClusterUpdate")

		if strings.Contains(createSrc, "resourceAcmeClusterRead(") {
			t.Errorf("OpAsync Create should not call resourceAcmeClusterRead:\n%s", createSrc)
		}
		if !strings.Contains(createSrc, "AcmeOperationWaitTimeWithResponse(") || !strings.Contains(createSrc, "config, res, &opRes,") {
			t.Errorf("OpAsync Create should call AcmeOperationWaitTimeWithResponse even when HasPostCreateComputedFields is false:\n%s", createSrc)
		}
		if !strings.Contains(createSrc, "ResourceAcmeClusterFlatten(d, meta, opRes, config, project, userAgent, billingProject, url, headers)") {
			t.Errorf("OpAsync Create should flatten opRes:\n%s", createSrc)
		}

		if strings.Contains(updateSrc, "resourceAcmeClusterRead(") {
			t.Errorf("OpAsync Update should not call resourceAcmeClusterRead:\n%s", updateSrc)
		}
		if !strings.Contains(updateSrc, "AcmeOperationWaitTimeWithResponse(") || !strings.Contains(updateSrc, "config, res, &updateRes,") {
			t.Errorf("OpAsync Update should call AcmeOperationWaitTimeWithResponse:\n%s", updateSrc)
		}
		if !strings.Contains(updateSrc, "ResourceAcmeClusterFlatten(d, meta, updateRes, config, project, userAgent, billingProject, url, headers)") {
			t.Errorf("OpAsync Update should flatten updateRes:\n%s", updateSrc)
		}
	})

	t.Run("Immutable Resource with ClientSide Update", func(t *testing.T) {
		res := &api.Resource{
			Name:                  "Snapshot",
			BaseUrl:               "projects/{{project}}/snapshots",
			SelfLink:              "projects/{{project}}/snapshots/{{name}}",
			IdFormat:              "projects/{{project}}/snapshots/{{name}}",
			Immutable:             true,
			ExcludePostCreateRead: true,
			ExcludePostUpdateRead: true,
			Properties: []*api.Type{
				{Name: "name", Type: "String", Required: true, Immutable: true},
			},
		}
		p := &api.Product{
			Name:        "Acme",
			Versions:    []*product.Version{{Name: "ga", BaseUrl: "https://acme.googleapis.com/v1/"}},
			Objects:     []*api.Resource{res},
			PackagePath: "products/acme",
		}
		p.Version = p.Versions[0]
		res.SetDefault(p)
		for _, prop := range res.AllProperties() {
			prop.SetDefault(res)
		}

		src, parsed := renderTestResourceGo(t, res, p)
		updateSrc := extractFuncSource(t, src, parsed, "resourceAcmeSnapshotUpdate")

		if strings.Contains(updateSrc, "resourceAcmeSnapshotRead(") {
			t.Errorf("Immutable Update should not call resourceAcmeSnapshotRead:\n%s", updateSrc)
		}
		if !strings.Contains(updateSrc, `d.GetOkExists("deletion_policy")`) {
			t.Errorf("Immutable Update should preserve deletion_policy default logic:\n%s", updateSrc)
		}
	})
}
