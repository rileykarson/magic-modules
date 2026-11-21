package backfill_test

import (
	"bytes"
	"encoding/json"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/GoogleCloudPlatform/magic-modules/tools/doc-backfill/backfill"
)

func TestBackfill_ModesCheckTemplateAndIdempotency(t *testing.T) {
	tmp := t.TempDir()
	docsDir := filepath.Join(tmp, "mmv1", "third_party", "terraform", "website", "docs", "r")
	servicesDir := filepath.Join(tmp, "mmv1", "third_party", "terraform", "services")
	sampleSvcDir := filepath.Join(servicesDir, "sample")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sampleSvcDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 1. SDKv2 .go.tmpl resource with version guards, backticks, {{placeholder}}, non-literal Description, and empty doc field.
	widgetDoc := `---
subcategory: "Sample"
description: |-
  Manages a sample widget.
---

# google_sample_widget

Manages a sample widget in GCP.

## Argument Reference

The following arguments are supported:

* ` + "`name`" + ` - (Required) The unique widget name.

* ` + "`stale_field`" + ` - (Optional) Updated description with ` + "`code_span`" + ` and placeholder ` + "`/dev/disk/google-{{device_name}}`" + `.

* ` + "`already_current`" + ` - (Optional) Exact matching description.

* ` + "`dynamic_desc`" + ` - (Optional) Markdown prose for dynamic Go description.

* ` + "`empty_doc_field`" + ` - (Optional)

* ` + "`beta_flag`" + ` - (Optional, [Beta](../guides/provider_versions.html.markdown)) Beta-only toggle for widgets.

## Attributes Reference

In addition to the arguments listed above, the following computed attributes are exported:

* ` + "`id`" + ` - an identifier for the resource with format ` + "`projects/{{project}}/widgets/{{name}}`" + `
`
	if err := os.WriteFile(filepath.Join(docsDir, "sample_widget.html.markdown"), []byte(widgetDoc), 0644); err != nil {
		t.Fatal(err)
	}

	widgetGoTmpl := `package sample

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
)

func init() {
	registry.Schema{
		Name:        "google_sample_widget",
		ProductName: "sample",
		Type:        registry.SchemaTypeResource,
		Schema:      ResourceSampleWidget(),
	}.Register()
}

func ResourceSampleWidget() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"stale_field": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Old stale description.",
			},
			"already_current": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Exact matching description.",
			},
			"dynamic_desc": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: fmt.Sprintf("computed %s", "at runtime"),
			},
			"empty_doc_field": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Keep existing non-empty Go description!",
			},
{{- if ne $.TargetVersionName "ga" }}
			"beta_flag": {
				Type:     schema.TypeBool,
				Optional: true,
			},
{{- end }}
		},
	}
}
`
	widgetGoPath := filepath.Join(sampleSvcDir, "resource_sample_widget.go.tmpl")
	if err := os.WriteFile(widgetGoPath, []byte(widgetGoTmpl), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Plugin Framework resource + IAM parent schema.
	fwDoc := `---
subcategory: "Sample"
description: |-
  Short page description.
---

# google_sample_fw_thing

Rich overview description for Plugin Framework resource.

## Argument Reference

* ` + "`bucket`" + ` - (Required) Bucket hosting the thing.
`
	if err := os.WriteFile(filepath.Join(docsDir, "sample_fw_thing.html.markdown"), []byte(fwDoc), 0644); err != nil {
		t.Fatal(err)
	}

	fwGo := `package sample

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
)

func init() {
	registry.FrameworkResource{
		Name:        "google_sample_fw_thing",
		ProductName: "sample",
		Func:        NewSampleFWThingResource,
	}.Register()
}

func NewSampleFWThingResource() resource.Resource {
	return &sampleFWThingResource{}
}

type sampleFWThingResource struct{}

func (r *sampleFWThingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Old framework overview.",
		Attributes: map[string]schema.Attribute{
			"bucket": schema.StringAttribute{
				Required: true,
			},
			"id": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}
`
	fwGoPath := filepath.Join(sampleSvcDir, "fw_resource_sample_thing.go")
	if err := os.WriteFile(fwGoPath, []byte(fwGo), 0644); err != nil {
		t.Fatal(err)
	}

	iamDoc := `---
subcategory: "Sample"
description: |-
  IAM policy for Sample Widget
---

# IAM policy for Sample Widget

## Argument Reference

* ` + "`widget`" + ` - (Required) Used to find the parent widget to bind the IAM policy to.
* ` + "`member/members`" + ` - (Required) Identities that will be granted the privilege in ` + "`role`" + `.
`
	if err := os.WriteFile(filepath.Join(docsDir, "sample_widget_iam.html.markdown"), []byte(iamDoc), 0644); err != nil {
		t.Fatal(err)
	}

	iamGo := `package sample

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgiamresource"
)

var SampleWidgetIamSchema = map[string]*schema.Schema{
	"widget": {
		Type:     schema.TypeString,
		Required: true,
		ForceNew: true,
	},
}

func init() {
	registry.Schema{
		Name:        "google_sample_widget_iam_binding",
		ProductName: "sample",
		Type:        registry.SchemaTypeIAMResource,
		Schema:      tpgiamresource.ResourceIamBinding(SampleWidgetIamSchema, nil, nil),
	}.Register()
}
`
	iamGoPath := filepath.Join(sampleSvcDir, "iam_sample_widget.go")
	if err := os.WriteFile(iamGoPath, []byte(iamGo), 0644); err != nil {
		t.Fatal(err)
	}

	// Phase A: --mode=missing with IncludeIAM=false should populate missing fields on widget + fw_thing,
	// leave stale_field and fw resp.Schema.Description untouched, and skip IAM!
	missingRes, err := backfill.Run(backfill.Options{
		RepoRoot:                   tmp,
		Mode:                       backfill.ModeMissing,
		IncludeIAM:                 false,
		IncludeResourceDescription: true,
		Write:                      true,
	})
	if err != nil {
		t.Fatalf("missing mode Run failed: %v", err)
	}
	if missingRes.Report.FieldsMissingPopulated != 3 {
		t.Fatalf("expected 3 missing fields populated (name, beta_flag, bucket), got %d", missingRes.Report.FieldsMissingPopulated)
	}
	if missingRes.Report.FieldsUpdated != 0 {
		t.Fatalf("expected 0 updated fields in missing mode, got %d", missingRes.Report.FieldsUpdated)
	}
	if missingRes.Report.ResourceDescriptionsPatched != 0 {
		t.Fatalf("expected 0 resource descriptions patched in missing mode when non-empty, got %d", missingRes.Report.ResourceDescriptionsPatched)
	}

	afterMissingWidget, err := os.ReadFile(widgetGoPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(afterMissingWidget), "Old stale description.") {
		t.Fatalf("missing mode unexpectedly overwrote stale_field:\n%s", afterMissingWidget)
	}
	if !strings.Contains(string(afterMissingWidget), "Keep existing non-empty Go description!") {
		t.Fatalf("empty_doc_field was overwritten:\n%s", afterMissingWidget)
	}
	if !strings.Contains(string(afterMissingWidget), `{{- if ne $.TargetVersionName "ga" }}`) {
		t.Fatalf(".go.tmpl guard was corrupted:\n%s", afterMissingWidget)
	}

	// Phase B: --mode=sync dry-run / --check should still detect pending updates (stale_field, fw overview, iam widget).
	checkPending, err := backfill.Run(backfill.Options{
		RepoRoot:                   tmp,
		Mode:                       backfill.ModeSync,
		IncludeIAM:                 true,
		IncludeResourceDescription: true,
		Check:                      true,
	})
	if err != nil {
		t.Fatalf("sync check Run failed: %v", err)
	}
	if !checkPending.HasPendingChanges() {
		t.Fatalf("expected pending sync changes after missing-only pass")
	}
	if len(checkPending.Report.SkippedNonLiteral) != 1 || checkPending.Report.SkippedNonLiteral[0].FieldPath != "dynamic_desc" {
		t.Fatalf("expected dynamic_desc in SkippedNonLiteral, got %+v", checkPending.Report.SkippedNonLiteral)
	}

	// Phase C: --mode=sync with Write=true applies remaining updates.
	reportPath := filepath.Join(tmp, "report.json")
	syncWrite, err := backfill.Run(backfill.Options{
		RepoRoot:                   tmp,
		Mode:                       backfill.ModeSync,
		IncludeIAM:                 true,
		IncludeResourceDescription: true,
		Write:                      true,
		ReportJSON:                 reportPath,
	})
	if err != nil {
		t.Fatalf("sync write Run failed: %v", err)
	}
	if syncWrite.Report.FieldsMissingPopulated != 1 {
		t.Fatalf("expected 1 missing IAM field populated (widget), got %d", syncWrite.Report.FieldsMissingPopulated)
	}
	if syncWrite.Report.FieldsUpdated != 1 {
		t.Fatalf("expected 1 updated field (stale_field), got %d", syncWrite.Report.FieldsUpdated)
	}
	if syncWrite.Report.ResourceDescriptionsPatched != 1 {
		t.Fatalf("expected 1 resource description patched, got %d", syncWrite.Report.ResourceDescriptionsPatched)
	}

	// Verify patched .go.tmpl still parses and executes cleanly under Go text/template (MMv1 template engine)!
	finalWidgetTmpl, err := os.ReadFile(widgetGoPath)
	if err != nil {
		t.Fatal(err)
	}
	parsedTmpl, err := template.New("widget").Parse(string(finalWidgetTmpl))
	if err != nil {
		t.Fatalf("patched .go.tmpl failed text/template.Parse (unescaped {{...}}?): %v\n%s", err, finalWidgetTmpl)
	}
	var rendered bytes.Buffer
	if err := parsedTmpl.Execute(&rendered, map[string]string{"TargetVersionName": "beta"}); err != nil {
		t.Fatalf("patched .go.tmpl failed text/template.Execute: %v", err)
	}

	// Phase D: Second run (--check and --write) MUST be 100% idempotent with 0 diffs!
	secondRun, err := backfill.Run(backfill.Options{
		RepoRoot:                   tmp,
		Mode:                       backfill.ModeSync,
		IncludeIAM:                 true,
		IncludeResourceDescription: true,
		Check:                      true,
	})
	if err != nil {
		t.Fatalf("idempotency check Run failed: %v", err)
	}
	if secondRun.HasPendingChanges() || secondRun.Report.TotalEditsApplied != 0 || secondRun.Report.FilesModified != 0 {
		t.Fatalf("expected 0 edits on idempotent second run, got files=%d edits=%d", secondRun.Report.FilesModified, secondRun.Report.TotalEditsApplied)
	}
}

func TestBackfill_SharedHelperConflictAndIdempotency(t *testing.T) {
	tmp := t.TempDir()
	docsDir := filepath.Join(tmp, "mmv1", "third_party", "terraform", "website", "docs", "r")
	svcDir := filepath.Join(tmp, "mmv1", "third_party", "terraform", "services", "sharedsvc")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(svcDir, 0755); err != nil {
		t.Fatal(err)
	}

	docAlpha := `---
subcategory: "Shared"
description: |-
  Alpha resource.
---

# google_sharedsvc_alpha

## Argument Reference

* ` + "`shared_block`" + ` - (Optional) Alpha block doc. Structure is [documented below](#nested_shared_block).

<a name="nested_shared_block"></a>The ` + "`shared_block`" + ` block supports:

* ` + "`mode`" + ` - (Optional) Alpha description for shared mode.
`
	docBeta := `---
subcategory: "Shared"
description: |-
  Beta resource.
---

# google_sharedsvc_beta

## Argument Reference

* ` + "`shared_block`" + ` - (Optional) Beta block doc. Structure is [documented below](#nested_shared_block).

<a name="nested_shared_block"></a>The ` + "`shared_block`" + ` block supports:

* ` + "`mode`" + ` - (Optional) Beta conflicting description for shared mode.
`
	if err := os.WriteFile(filepath.Join(docsDir, "sharedsvc_alpha.html.markdown"), []byte(docAlpha), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "sharedsvc_beta.html.markdown"), []byte(docBeta), 0644); err != nil {
		t.Fatal(err)
	}

	sharedGo := `package sharedsvc

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
)

func init() {
	registry.Schema{
		Name:        "google_sharedsvc_alpha",
		ProductName: "sharedsvc",
		Type:        registry.SchemaTypeResource,
		Schema:      ResourceAlpha(),
	}.Register()
	registry.Schema{
		Name:        "google_sharedsvc_beta",
		ProductName: "sharedsvc",
		Type:        registry.SchemaTypeResource,
		Schema:      ResourceBeta(),
	}.Register()
}

func sharedBlockSchema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"mode": {
				Type:     schema.TypeString,
				Optional: true,
			},
		},
	}
}

func ResourceAlpha() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"shared_block": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem:     sharedBlockSchema(),
			},
		},
	}
}

func ResourceBeta() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"shared_block": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem:     sharedBlockSchema(),
			},
		},
	}
}
`
	goFile := filepath.Join(svcDir, "resources.go")
	if err := os.WriteFile(goFile, []byte(sharedGo), 0644); err != nil {
		t.Fatal(err)
	}

	res1, err := backfill.Run(backfill.Options{
		RepoRoot:   tmp,
		Mode:       backfill.ModeSync,
		IncludeIAM: true,
		Write:      true,
	})
	if err != nil {
		t.Fatalf("shared helper Run failed: %v", err)
	}
	if len(res1.Report.SharedHelperConflicts) != 1 {
		t.Fatalf("expected 1 SharedHelperConflict, got %+v", res1.Report.SharedHelperConflicts)
	}
	conflict := res1.Report.SharedHelperConflicts[0]
	if conflict.ChosenResource != "google_sharedsvc_alpha" || conflict.ConflictingResource != "google_sharedsvc_beta" {
		t.Fatalf("unexpected conflict winner/loser: %+v", conflict)
	}

	patchedBytes, err := os.ReadFile(goFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(patchedBytes), "Alpha description for shared mode.") {
		t.Fatalf("expected winning Alpha description in shared helper:\n%s", patchedBytes)
	}
	if strings.Contains(string(patchedBytes), "Beta conflicting description for shared mode.") {
		t.Fatalf("unexpectedly wrote losing Beta description in shared helper:\n%s", patchedBytes)
	}

	// Second run MUST be 100% idempotent even in the presence of a shared helper conflict!
	res2, err := backfill.Run(backfill.Options{
		RepoRoot:   tmp,
		Mode:       backfill.ModeSync,
		IncludeIAM: true,
		Check:      true,
	})
	if err != nil {
		t.Fatalf("second shared helper Run failed: %v", err)
	}
	if res2.HasPendingChanges() {
		t.Fatalf("expected 0 pending changes on second run, got %d edits", res2.Report.TotalEditsApplied)
	}
}

func TestBackfill_RealMagicModulesDryRunDeterminism(t *testing.T) {
	repoRoot, err := backfill.DetectRepoRoot()
	if err != nil {
		t.Fatalf("DetectRepoRoot failed: %v", err)
	}

	opts := backfill.Options{
		RepoRoot:                   repoRoot,
		Mode:                       backfill.ModeSync,
		IncludeIAM:                 true,
		IncludeResourceDescription: true,
		Write:                      false,
	}

	res1, err := backfill.Run(opts)
	if err != nil {
		t.Fatalf("real repo dry-run #1 failed: %v", err)
	}
	if res1.Report.DocsParsed < 130 {
		t.Fatalf("expected >= 130 parsed docs, got %d", res1.Report.DocsParsed)
	}
	if res1.Report.ResourcesIndexed < 100 {
		t.Fatalf("expected >= 100 indexed resources, got %d", res1.Report.ResourcesIndexed)
	}
	if res1.Report.ResourcesMatched < 90 {
		t.Fatalf("expected >= 90 matched resources, got %d", res1.Report.ResourcesMatched)
	}
	if res1.Report.FieldsEvaluated < 4000 {
		t.Fatalf("expected >= 4000 evaluated fields in dry-run, got %d", res1.Report.FieldsEvaluated)
	}

	// Verify all generated .go.tmpl diffs remain valid Go text/templates and all .go diffs are gofmt-clean!
	for _, fd := range res1.FileDiffs {
		if strings.HasSuffix(fd.FilePath, ".tmpl") {
			if _, err := template.New(filepath.Base(fd.FilePath)).Funcs(dummyMMv1Funcs()).Parse(string(fd.UpdatedBytes)); err != nil {
				t.Errorf("updated .go.tmpl %s failed text/template.Parse: %v", fd.FilePath, err)
			}
		} else if strings.HasSuffix(fd.FilePath, ".go") {
			formatted, err := format.Source(fd.UpdatedBytes)
			if err != nil {
				t.Errorf("updated .go %s failed format.Source: %v", fd.FilePath, err)
			} else if !bytes.Equal(formatted, fd.UpdatedBytes) {
				t.Errorf("updated .go %s is not gofmt-clean", fd.FilePath)
			}
		}
	}

	res2, err := backfill.Run(opts)
	if err != nil {
		t.Fatalf("real repo dry-run #2 failed: %v", err)
	}

	json1, err := json.Marshal(res1.Report)
	if err != nil {
		t.Fatal(err)
	}
	json2, err := json.Marshal(res2.Report)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(json1, json2) {
		t.Fatalf("dry-run JSON reports were not deterministic across two consecutive runs")
	}
}

func dummyMMv1Funcs() template.FuncMap {
	return template.FuncMap{
		"TemplatePath": func() string { return "" },
		"contains":     strings.Contains,
		"hasPrefix":    strings.HasPrefix,
		"hasSuffix":    strings.HasSuffix,
		"join":         strings.Join,
		"replace":      strings.Replace,
		"replaceAll":   strings.ReplaceAll,
		"split":        strings.Split,
		"toLower":      strings.ToLower,
		"toUpper":      strings.ToUpper,
		"trimPrefix":   strings.TrimPrefix,
		"trimSuffix":   strings.TrimSuffix,
		"trimSpace":    strings.TrimSpace,
		"add":          func(a, b int) int { return a + b },
		"sub":          func(a, b int) int { return a - b },
		"dict":         func(...any) map[string]any { return map[string]any{} },
		"list":         func(...any) []any { return nil },
	}
}
