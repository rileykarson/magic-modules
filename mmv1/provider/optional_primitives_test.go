// Copyright 2026 Google LLC. All Rights Reserved.
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
	"bytes"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/magic-modules/mmv1/api"
	"github.com/GoogleCloudPlatform/magic-modules/mmv1/api/product"
)

func buildOptionalPrimitiveTestResource(t *testing.T, updateVerb string, updateMask bool, framework bool, compiler string) (*api.Product, *api.Resource) {
	t.Helper()

	var props []*api.Type
	if framework {
		props = []*api.Type{
			{
				Name:        "name",
				Type:        "String",
				Required:    true,
				Immutable:   true,
				Description: "Resource name.",
			},
			{
				Name:        "optBool",
				Type:        "OptionalBoolean",
				Immutable:   true,
				Description: "Optional boolean with explicit presence.",
			},
			{
				Name:        "optStr",
				Type:        "OptionalString",
				Description: "Optional string with explicit presence.",
			},
			{
				Name:        "optInt",
				Type:        "OptionalInteger",
				Description: "Optional integer with explicit presence.",
			},
			{
				Name:        "optDouble",
				Type:        "OptionalDouble",
				Description: "Optional double with explicit presence.",
			},
		}
	} else {
		props = []*api.Type{
			{
				Name:        "name",
				Type:        "String",
				Required:    true,
				Immutable:   true,
				Description: "Resource name.",
			},
			{
				Name:        "optBool",
				Type:        "OptionalBoolean",
				Immutable:   true,
				Description: "Optional boolean with explicit presence.",
			},
			{
				Name:        "optStr",
				Type:        "OptionalString",
				Description: "Optional string with explicit presence.",
			},
			{
				Name:        "optInt",
				Type:        "OptionalInteger",
				Description: "Optional integer with explicit presence.",
			},
			{
				Name:        "optDouble",
				Type:        "OptionalDouble",
				Description: "Optional double with explicit presence.",
			},
			{
				Name:        "customOptInt",
				Type:        "OptionalInteger",
				UpdateUrl:   "projects/{{project}}/widgets/{{name}}:setCustomOptInt",
				UpdateVerb:  "PATCH",
				Description: "Optional integer with dedicated PATCH endpoint.",
			},
			{
				Name:          "flatSettings",
				Type:          "NestedObject",
				FlattenObject: true,
				Description:   "Flattened container.",
				Properties: []*api.Type{
					{
						Name:        "flatBool",
						Type:        "OptionalBoolean",
						Description: "Flattened optional bool.",
					},
					{
						Name:          "innerFlat",
						Type:          "NestedObject",
						FlattenObject: true,
						Description:   "Nested flattened container.",
						Properties: []*api.Type{
							{
								Name:        "deepFlatStr",
								Type:        "OptionalString",
								Description: "Deeply flattened optional string.",
							},
						},
					},
				},
			},
			{
				Name:        "config",
				Type:        "NestedObject",
				Description: "Nested config block.",
				Properties: []*api.Type{
					{
						Name:        "nestedStr",
						Type:        "OptionalString",
						Description: "Nested optional string.",
					},
					{
						Name:        "nestedInt",
						Type:        "OptionalInteger",
						Description: "Nested optional int.",
					},
				},
			},
			{
				Name:        "rules",
				Type:        "Array",
				Description: "Ordered list of rules.",
				ItemType: &api.Type{
					Type: "NestedObject",
					Properties: []*api.Type{
						{
							Name:        "threshold",
							Type:        "OptionalDouble",
							Description: "Rule threshold.",
						},
					},
				},
			},
			{
				Name:        "envs",
				Type:        "Map",
				KeyName:     "env_id",
				Description: "Environment map.",
				ValueType: &api.Type{
					Type: "NestedObject",
					Properties: []*api.Type{
						{
							Name:        "enabled",
							Type:        "OptionalBoolean",
							Description: "Whether environment is enabled.",
						},
					},
				},
			},
		}
	}

	res := &api.Resource{
		Name:                 "Widget",
		BaseUrl:              "projects/{{project}}/widgets",
		CreateUrl:            "projects/{{project}}/widgets?widgetId={{name}}",
		SelfLink:             "projects/{{project}}/widgets/{{name}}",
		UpdateVerb:           updateVerb,
		UpdateMask:           updateMask,
		FrameworkResource:    framework,
		GenerateListResource: !framework,
		Identity:             []string{"name"},
		Properties:           props,
	}

	prod := &api.Product{
		Name:     "Pubsub",
		ApiName:  "pubsub",
		Compiler: compiler,
		Versions: []*product.Version{
			{Name: "ga", BaseUrl: "https://pubsub.googleapis.com/v1/"},
			{Name: "beta", BaseUrl: "https://pubsub.googleapis.com/v1/"},
		},
		Objects:     []*api.Resource{res},
		PackagePath: "products/pubsub",
	}
	prod.Version = prod.Versions[0]
	prod.ImportPath = ImportPathFromVersion("ga")
	res.ImportPath = prod.ImportPath
	res.SetDefault(prod)

	for _, prop := range res.Properties {
		if errs := prop.Validate(res.Name); len(errs) > 0 {
			t.Fatalf("unexpected validation errors on %s: %v", prop.Name, errs)
		}
	}
	return prod, res
}

func assertValidGoFile(t *testing.T, path string) string {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, path, src, parser.AllErrors); err != nil {
		t.Fatalf("generated Go file %s failed syntax check: %v\n--- SOURCE ---\n%s", path, err, string(src))
	}
	formatted, err := format.Source(src)
	if err != nil {
		t.Fatalf("gofmt failed on %s: %v", path, err)
	}
	if !bytes.Equal(src, formatted) {
		t.Errorf("generated Go file %s was not gofmt-clean", path)
	}
	return string(src)
}

func TestGenerateOptionalPrimitives_SDKv2MaskedAndUnmasked(t *testing.T) {
	fsys := os.DirFS("..")
	tempDir := t.TempDir()
	td := NewTemplateData(tempDir, "ga", fsys)

	// 1. Masked PATCH resource
	_, maskedRes := buildOptionalPrimitiveTestResource(t, "PATCH", true, false, "")
	maskedResPath := filepath.Join(tempDir, "resource_pubsub_widget_masked.go")
	td.GenerateResourceFile(maskedResPath, *maskedRes)
	maskedSrc := assertValidGoFile(t, maskedResPath)

	for _, want := range []string{
		`tpgresource.NullableFieldPresenceAttr: {`,
		`tpgresource.SetNullableFieldPresenceDiff(resourcePubsubWidgetNullableFields),`,
		`var resourcePubsubWidgetNullableFields = []tpgresource.NullableFieldSpec{`,
		`tpgresource.IsNullableFieldPresent(d, "opt_bool", resourcePubsubWidgetNullableFields)`,
		`tpgresource.IsNullableFieldPresent(d, "opt_str", resourcePubsubWidgetNullableFields)`,
		`tpgresource.IsNullableFieldPresent(d, "opt_int", resourcePubsubWidgetNullableFields)`,
		`tpgresource.IsNullableFieldPresent(d, "opt_double", resourcePubsubWidgetNullableFields)`,
		`tpgresource.HasNullableFieldPresenceChange(d, "opt_str", resourcePubsubWidgetNullableFields)`,
		`tpgresource.HasNullableFieldPresenceChange(d, "flat_bool", resourcePubsubWidgetNullableFields)`,
		`tpgresource.HasNullableFieldPresenceChange(d, "deep_flat_str", resourcePubsubWidgetNullableFields)`,
		`tpgresource.HasNullableFieldPresenceChange(d, "config", resourcePubsubWidgetNullableFields)`,
		`tpgresource.HasNullableFieldPresenceChange(d, "rules", resourcePubsubWidgetNullableFields)`,
		`tpgresource.HasNullableFieldPresenceChange(d, "envs", resourcePubsubWidgetNullableFields)`,
		`tpgresource.HasNullableFieldPresenceChange(d, "custom_opt_int", resourcePubsubWidgetNullableFields)`,
		`tpgresource.WasNullableFieldPresent(d, "custom_opt_int")`,
		`expandPubsubWidgetConfigWithPrefix(`,
		`expandPubsubWidgetRulesWithPrefix(`,
		`expandPubsubWidgetEnvsWithPrefix(`,
		`expandPubsubWidgetFlatSettingsWithPrefix(`,
		`tpgresource.ExtractNullableFieldPresenceFromAPIResponse(res, resourcePubsubWidgetNullableFields)`,
		`d.Set("flat_bool", nil)`,
		`d.Set("deep_flat_str", nil)`,
	} {
		if !strings.Contains(maskedSrc, want) {
			t.Errorf("masked SDKv2 resource missing expected snippet %q", want)
		}
	}
	if strings.Contains(maskedSrc, `WasNullableFieldPresent(d, "opt_str")`) {
		t.Errorf("masked PATCH resource must omit cleared fields rather than checking WasNullableFieldPresent for main Update")
	}

	// Verify singular DataSource, ListResource, Docs, and Metadata
	dsPath := filepath.Join(tempDir, "data_source_pubsub_widget.go")
	td.GenerateDataSourceFile(dsPath, *maskedRes)
	assertValidGoFile(t, dsPath)

	listPath := filepath.Join(tempDir, "list_pubsub_widget.go")
	td.GenerateFile(listPath, "templates/terraform/list_resource.go.tmpl", *maskedRes, true,
		"templates/terraform/list_resource.go.tmpl",
		"templates/terraform/list_resource_method.go.tmpl",
	)
	assertValidGoFile(t, listPath)

	docPath := filepath.Join(tempDir, "pubsub_widget.html.markdown")
	td.GenerateDocumentationFile(docPath, *maskedRes)
	docBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("reading doc file: %v", err)
	}
	if !strings.Contains(string(docBytes), "* `nullable_field_presence` -") {
		t.Errorf("generated documentation missing nullable_field_presence attribute reference")
	}

	metaPath := filepath.Join(tempDir, "metadata.yaml")
	td.GenerateMetadataFile(metaPath, *maskedRes)
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("reading metadata file: %v", err)
	}
	if !strings.Contains(string(metaBytes), "field: nullable_field_presence") {
		t.Errorf("generated metadata.yaml missing nullable_field_presence entry:\n%s", string(metaBytes))
	}

	// 2. Unmasked PATCH resource
	_, unmaskedRes := buildOptionalPrimitiveTestResource(t, "PATCH", false, false, "")
	unmaskedResPath := filepath.Join(tempDir, "resource_pubsub_widget_unmasked.go")
	td.GenerateResourceFile(unmaskedResPath, *unmaskedRes)
	unmaskedSrc := assertValidGoFile(t, unmaskedResPath)

	for _, want := range []string{
		`tpgresource.WasNullableFieldPresent(d, "opt_str")`,
		`obj["optStr"] = nil`,
		`tpgresource.WasNullableFieldPresentUnderPrefix(d, "config")`,
		`obj["config"] = nil`,
		`transformed["nestedStr"] = nil`,
	} {
		if !strings.Contains(unmaskedSrc, want) {
			t.Errorf("unmasked PATCH SDKv2 resource missing expected snippet %q", want)
		}
	}
}

func TestGenerateOptionalPrimitives_PluginFramework(t *testing.T) {
	fsys := os.DirFS("..")
	tempDir := t.TempDir()
	td := NewTemplateData(tempDir, "ga", fsys)

	// Masked PATCH FW resource
	_, fwMasked := buildOptionalPrimitiveTestResource(t, "PATCH", true, true, "")
	fwMaskedPath := filepath.Join(tempDir, "resource_fw_pubsub_widget_masked.go")
	td.GenerateFWResourceFile(fwMaskedPath, *fwMasked)
	fwMaskedSrc := assertValidGoFile(t, fwMaskedPath)

	for _, want := range []string{
		"OptBool   types.Bool",
		"OptStr    types.String",
		"OptInt    types.Int64",
		"OptDouble types.Float64",
		`"opt_bool": schema.BoolAttribute{`,
		`"opt_str": schema.StringAttribute{`,
		`"opt_int": schema.Int64Attribute{`,
		`"opt_double": schema.Float64Attribute{`,
		`boolplanmodifier.RequiresReplace()`,
		`data.OptBool.ValueBool()`,
		`data.OptStr.ValueString()`,
		`data.OptInt.ValueInt64()`,
		`data.OptDouble.ValueFloat64()`,
		`fwresource.FlattenOptionalBool(res["optBool"], diags)`,
		`fwresource.FlattenOptionalString(res["optStr"], diags)`,
		`fwresource.FlattenOptionalInt64(res["optInt"], diags)`,
		`fwresource.FlattenOptionalFloat64(res["optDouble"], diags)`,
	} {
		if !strings.Contains(fwMaskedSrc, want) {
			t.Errorf("FW masked resource missing expected snippet %q", want)
		}
	}
	if strings.Contains(fwMaskedSrc, "nullable_field_presence") {
		t.Errorf("Plugin Framework resource must not emit SDKv2 nullable_field_presence")
	}
	if strings.Contains(fwMaskedSrc, `obj["optStr"] = nil`) {
		t.Errorf("masked FW resource must not emit explicit nil on clear")
	}

	// Unmasked PATCH FW resource
	_, fwUnmasked := buildOptionalPrimitiveTestResource(t, "PATCH", false, true, "")
	fwUnmaskedPath := filepath.Join(tempDir, "resource_fw_pubsub_widget_unmasked.go")
	td.GenerateFWResourceFile(fwUnmaskedPath, *fwUnmasked)
	fwUnmaskedSrc := assertValidGoFile(t, fwUnmaskedPath)
	if !strings.Contains(fwUnmaskedSrc, `obj["optStr"] = nil`) {
		t.Errorf("unmasked FW resource missing explicit nil clear for optStr")
	}
}

func TestGenerateOptionalPrimitives_TGCAndTGCNext(t *testing.T) {
	fsys := os.DirFS("..")
	tempDir := t.TempDir()
	td := NewTemplateData(tempDir, "beta", fsys)

	// TGC v1
	_, tgcRes := buildOptionalPrimitiveTestResource(t, "PATCH", false, false, "terraformgoogleconversion-codegen")
	tgcPath := filepath.Join(tempDir, "pubsub_widget_tgc.go")
	td.GenerateTGCResourceFile("templates/tgc/resource_converter.go.tmpl", tgcPath, *tgcRes)
	tgcSrc := assertValidGoFile(t, tgcPath)
	if !strings.Contains(tgcSrc, "var resourcePubsubWidgetNullableFields = []tpgresource.NullableFieldSpec{") {
		t.Errorf("TGC v1 converter missing resourcePubsubWidgetNullableFields definition")
	}
	if !strings.Contains(tgcSrc, `tpgresource.IsNullableFieldPresent(d, "opt_str", resourcePubsubWidgetNullableFields)`) {
		t.Errorf("TGC v1 converter missing IsNullableFieldPresent check")
	}
	if strings.Contains(tgcSrc, "WasNullableFieldPresent") {
		t.Errorf("TGC v1 converter must not emit update-clear nil checks")
	}

	// TGC Next (services/resource.go, tfplan2cai, cai2hcl)
	_, tgcNextRes := buildOptionalPrimitiveTestResource(t, "PATCH", false, false, "terraformgoogleconversionnext-codegen")
	svcPath := filepath.Join(tempDir, "pubsub_widget.go")
	tfplan2caiPath := filepath.Join(tempDir, "pubsub_widget_tfplan2cai.go")
	cai2hclPath := filepath.Join(tempDir, "pubsub_widget_cai2hcl.go")

	td.GenerateTGCResourceFile("templates/tgc_next/services/resource.go.tmpl", svcPath, *tgcNextRes)
	td.GenerateTGCResourceFile("templates/tgc_next/tfplan2cai/resource_converter.go.tmpl", tfplan2caiPath, *tgcNextRes)
	td.GenerateTGCResourceFile("templates/tgc_next/cai2hcl/resource_converter.go.tmpl", cai2hclPath, *tgcNextRes)

	svcSrc := assertValidGoFile(t, svcPath)
	tfplan2caiSrc := assertValidGoFile(t, tfplan2caiPath)
	cai2hclSrc := assertValidGoFile(t, cai2hclPath)

	if !strings.Contains(svcSrc, "tpgresource.NullableFieldPresenceAttr: {") {
		t.Errorf("tgc_next service schema missing NullableFieldPresenceAttr")
	}
	if !strings.Contains(svcSrc, "var resourcePubsubWidgetNullableFields = []tpgresource.NullableFieldSpec{") {
		t.Errorf("tgc_next service file missing resourcePubsubWidgetNullableFields")
	}
	if strings.Contains(tfplan2caiSrc, "var resourcePubsubWidgetNullableFields =") {
		t.Errorf("tgc_next tfplan2cai file must not redeclare resourcePubsubWidgetNullableFields")
	}
	if !strings.Contains(tfplan2caiSrc, `tpgresource.IsNullableFieldPresent(d, "opt_str", resourcePubsubWidgetNullableFields)`) {
		t.Errorf("tgc_next tfplan2cai missing IsNullableFieldPresent check")
	}
	if strings.Contains(tfplan2caiSrc, "WasNullableFieldPresent") {
		t.Errorf("tgc_next tfplan2cai must not emit update-clear nil checks")
	}
	if !strings.Contains(cai2hclSrc, "StringToFixed64") {
		t.Errorf("tgc_next cai2hcl missing OptionalInteger StringToFixed64 handling")
	}
}
