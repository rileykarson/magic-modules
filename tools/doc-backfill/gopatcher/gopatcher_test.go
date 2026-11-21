package gopatcher

import (
	"bytes"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"text/template"
)

func TestFormatGoStringLiteral(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantRaw  bool
		expected string
	}{
		{
			name:     "simple prose uses raw string",
			input:    "The name of the bucket.",
			wantRaw:  true,
			expected: "`The name of the bucket.`",
		},
		{
			name:     "double quotes and backslashes stay unescaped in raw string",
			input:    `One of "JSON_API_V1" or "NONE" under path C:\temp\dir.`,
			wantRaw:  true,
			expected: "`One of \"JSON_API_V1\" or \"NONE\" under path C:\\temp\\dir.`",
		},
		{
			name:    "multiline and tabs use raw string",
			input:   "First paragraph.\n\n\tIndented note with \"quotes\".",
			wantRaw: true,
		},
		{
			name:    "backticks use strconv.Quote so backticks survive verbatim",
			input:   "Refer to `effective_labels` and [`instance.reset`](https://example.com).",
			wantRaw: false,
		},
		{
			name:    "multiline with backticks uses strconv.Quote",
			input:   "Line one with `code`.\n\nLine two with \"quotes\".",
			wantRaw: false,
		},
		{
			name:    "carriage return falls back to strconv.Quote",
			input:   "has\rcarriage return",
			wantRaw: false,
		},
		{
			name:    "null control char falls back to strconv.Quote",
			input:   "null\x00byte",
			wantRaw: false,
		},
		{
			name:     "empty string",
			input:    "",
			wantRaw:  true,
			expected: "``",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatGoStringLiteral(tc.input)
			if tc.expected != "" && got != tc.expected {
				t.Fatalf("FormatGoStringLiteral(%q) = %s, want %s", tc.input, got, tc.expected)
			}
			if tc.wantRaw && !strings.HasPrefix(got, "`") {
				t.Fatalf("expected raw string literal for %q, got %s", tc.input, got)
			}
			if !tc.wantRaw && !strings.HasPrefix(got, "\"") {
				t.Fatalf("expected quoted string literal for %q, got %s", tc.input, got)
			}
			unquoted, err := strconv.Unquote(got)
			if err != nil {
				t.Fatalf("strconv.Unquote(%s) failed: %v", got, err)
			}
			if unquoted != tc.input {
				t.Fatalf("round-trip mismatch:\ngot:  %q\nwant: %q", unquoted, tc.input)
			}
		})
	}
}

func TestIndexServicesRoot_RealRepo(t *testing.T) {
	servicesRoot := filepath.Join("..", "..", "..", "mmv1", "third_party", "terraform", "services")
	targets, err := IndexServicesRoot(servicesRoot)
	if err != nil {
		t.Fatalf("IndexServicesRoot failed: %v", err)
	}

	var sdkCount, fwCount, iamCount, totalFieldPaths, totalFieldTargets int
	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		rt := targets[name]
		if rt.Service == "" || rt.PrimaryFile == "" {
			t.Errorf("resource %s missing Service or PrimaryFile: %+v", name, rt)
		}
		switch rt.Kind {
		case SDKv2:
			sdkCount++
		case PluginFramework:
			fwCount++
		case IAM:
			iamCount++
		default:
			t.Errorf("unexpected SDKKind %q on %s", rt.Kind, name)
		}
		if len(rt.Fields) == 0 {
			t.Errorf("resource %s (%s) indexed with 0 fields", name, rt.PrimaryFile)
		}
		totalFieldPaths += len(rt.Fields)
		for fpath, slice := range rt.Fields {
			if len(slice) == 0 {
				t.Errorf("resource %s field %s has empty target slice", name, fpath)
			}
			for _, ft := range slice {
				totalFieldTargets++
				if ft.Path != fpath {
					t.Errorf("field path mismatch: key=%q vs ft.Path=%q", fpath, ft.Path)
				}
				if ft.TargetID == "" || ft.FilePath == "" {
					t.Errorf("missing TargetID or FilePath on %s.%s", name, fpath)
				}
			}
		}
	}

	t.Logf("Indexed %d total resources (SDKv2=%d, Framework=%d, IAM=%d), %d field paths, %d field targets",
		len(targets), sdkCount, fwCount, iamCount, totalFieldPaths, totalFieldTargets)

	if sdkCount+fwCount < 114 {
		t.Fatalf("expected at least 114 non-IAM resources, got %d (SDKv2=%d, Framework=%d)", sdkCount+fwCount, sdkCount, fwCount)
	}
	if fwCount < 2 {
		t.Fatalf("expected at least 2 Plugin Framework resources, got %d", fwCount)
	}
	if iamCount < 20 {
		t.Fatalf("expected at least 20 grouped IAM resources, got %d", iamCount)
	}

	// 1. Verify compute_instance & cloned schema safety on compute_instance_from_template / from_machine_image.
	ci := targets["google_compute_instance"]
	if ci == nil {
		t.Fatal("missing google_compute_instance")
	}
	for _, wantPath := range []string{
		"boot_disk",
		"boot_disk.initialize_params.size",
		"network_interface.alias_ipv6_range",
		"partner_metadata",
	} {
		if len(ci.Fields[wantPath]) == 0 {
			t.Errorf("google_compute_instance missing expected field %q", wantPath)
		}
	}

	cift := targets["google_compute_instance_from_template"]
	if cift == nil {
		t.Fatal("missing google_compute_instance_from_template")
	}
	if len(cift.Fields["source_instance_template"]) != 1 {
		t.Errorf("google_compute_instance_from_template missing source_instance_template: %v", cift.Fields)
	}
	if _, leaked := cift.Fields["boot_disk"]; leaked {
		t.Errorf("google_compute_instance_from_template leaked cloned ResourceComputeInstance() field boot_disk")
	}

	cifmi := targets["google_compute_instance_from_machine_image"]
	if cifmi == nil {
		t.Fatal("missing google_compute_instance_from_machine_image")
	}
	if len(cifmi.Fields["source_machine_image_encryption_key.raw_key"]) != 1 {
		t.Errorf("google_compute_instance_from_machine_image missing nested encryption key field")
	}
	if _, leaked := cifmi.Fields["boot_disk"]; leaked {
		t.Errorf("google_compute_instance_from_machine_image leaked cloned ResourceComputeInstance() field boot_disk")
	}

	// 2. Verify storage_bucket non-literal fmt.Sprintf vs literal shared helper TargetID.
	sb := targets["google_storage_bucket"]
	if sb == nil {
		t.Fatal("missing google_storage_bucket")
	}
	gmekMode := sb.Fields["encryption.google_managed_encryption_enforcement_config.restriction_mode"]
	if len(gmekMode) != 1 || gmekMode[0].IsLiteralOrMissing || !gmekMode[0].HasDescription {
		t.Errorf("expected non-literal fmt.Sprintf Description on restriction_mode, got %+v", gmekMode)
	}
	gmekEff := sb.Fields["encryption.google_managed_encryption_enforcement_config.effective_time"]
	cmekEff := sb.Fields["encryption.customer_managed_encryption_enforcement_config.effective_time"]
	if len(gmekEff) != 1 || len(cmekEff) != 1 {
		t.Fatalf("missing effective_time targets on storage_bucket: %v / %v", gmekEff, cmekEff)
	}
	if gmekEff[0].TargetID != cmekEff[0].TargetID {
		t.Errorf("expected identical shared helper TargetID, got %q vs %q", gmekEff[0].TargetID, cmekEff[0].TargetID)
	}
	if gmekEff[0].SharedHelper != "buckeEncryptionEnforcementConfigSchema" {
		t.Errorf("expected SharedHelper buckeEncryptionEnforcementConfigSchema, got %q", gmekEff[0].SharedHelper)
	}

	// 3. Verify dataplex_zone *Schema() helpers.
	dz := targets["google_dataplex_zone"]
	if dz == nil {
		t.Fatal("missing google_dataplex_zone")
	}
	delim := dz.Fields["discovery_spec.csv_options.delimiter"]
	if len(delim) != 1 || delim[0].SharedHelper != "DataplexZoneDiscoverySpecCsvOptionsSchema" {
		t.Errorf("unexpected dataplex delimiter target: %+v", delim)
	}

	// 4. Verify shared TargetID across google_container_cluster and google_container_node_pool.
	cc := targets["google_container_cluster"]
	cnp := targets["google_container_node_pool"]
	if cc == nil || cnp == nil {
		t.Fatalf("missing container cluster (%v) or node pool (%v)", cc, cnp)
	}
	ccMachine := cc.Fields["node_config.machine_type"]
	cnpMachine := cnp.Fields["node_config.machine_type"]
	if len(ccMachine) != 1 || len(cnpMachine) != 1 || ccMachine[0].TargetID != cnpMachine[0].TargetID {
		t.Errorf("expected shared TargetID for node_config.machine_type across cluster & node_pool: %+v vs %+v", ccMachine, cnpMachine)
	}

	// 5. Verify Plugin Framework resources.
	sn := targets["google_storage_notification"]
	if sn == nil || sn.Kind != PluginFramework || !sn.CanPatchResDesc || !sn.HasResDescription {
		t.Fatalf("unexpected google_storage_notification: %+v", sn)
	}
	if len(sn.Fields["id"]) != 1 || !sn.Fields["id"][0].IsLiteralOrMissing {
		t.Errorf("expected literal-or-missing Description on google_storage_notification.id, got %+v", sn.Fields["id"])
	}

	// 6. Verify IAM parent schemas.
	sbIam := targets["google_storage_bucket_iam"]
	if sbIam == nil || sbIam.Kind != IAM || len(sbIam.Fields["bucket"]) != 1 || !sbIam.Fields["bucket"][0].IsLiteralOrMissing {
		t.Fatalf("unexpected google_storage_bucket_iam: %+v", sbIam)
	}
	bqIam := targets["google_bigquery_dataset_iam"]
	if bqIam == nil || len(bqIam.Fields["dataset_id"]) != 2 {
		t.Fatalf("expected 2 targets for google_bigquery_dataset_iam.dataset_id, got %+v", bqIam)
	}

	// 7. Determinism check: second full run matches identically.
	targets2, err := IndexServicesRoot(servicesRoot)
	if err != nil {
		t.Fatalf("second IndexServicesRoot failed: %v", err)
	}
	if !reflect.DeepEqual(targets, targets2) {
		t.Fatal("IndexServicesRoot is not 100% deterministic across runs")
	}
}

func TestApplyPatches_SDKv2_GoAndTmpl_RoundTripAndIdempotency(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a .go.tmpl resource file with:
	// - if/else defining the same field "mode" in both branches (both missing Description)
	// - conditional field flags inside a single field "flag_field" (missing Description)
	// - existing literal Description to update with backticks & {{project}} template braces
	// - non-literal fmt.Sprintf Description that must be left untouched
	// - shared helper in a sibling .go file
	tmplPath := filepath.Join(tmpDir, "resource_widget.go.tmpl")
	tmplSrc := `package widget

import (
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
)

func init() {
	registry.Schema{
		Name:        "google_widget_item",
		ProductName: "widget",
		Type:        registry.SchemaTypeResource,
		Schema:      ResourceWidgetItem(),
	}.Register()
}

func ResourceWidgetItem() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Old name description.",
			},
{{- if ne $.TargetVersionName "ga" }}
			"mode": {
				Type:     schema.TypeString,
				Optional: true,
			},
{{- else }}
			"mode": {
				Type:     schema.TypeString,
				Computed: true,
			},
{{- end }}
			"flag_field": {
				Type: schema.TypeBool,
{{- if ne $.TargetVersionName "ga" }}
				Optional: true,
{{- else }}
				Computed: true,
{{- end }}
				ForceNew: true,
			},
			"computed_dynamic": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: fmt.Sprintf("dynamic %s", "val"),
			},
			"nested": {
				Type:     schema.TypeList,
				Optional: true,
				Elem:     widgetNestedSchema(),
			},
		},
	}
}
`
	helperPath := filepath.Join(tmpDir, "widget_helpers.go")
	helperSrc := `package widget

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func widgetNestedSchema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"child_count": {
				Type:     schema.TypeInt,
				Optional: true,
			},
			"inline_single": {Type: schema.TypeString, Computed: true},
		},
	}
}
`
	if err := os.WriteFile(tmplPath, []byte(tmplSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(helperPath, []byte(helperSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	resources, err := IndexServiceDir(tmpDir)
	if err != nil {
		t.Fatalf("IndexServiceDir failed: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	rt := resources[0]

	if len(rt.Fields["mode"]) != 2 {
		t.Fatalf("expected 2 branch targets for mode, got %d", len(rt.Fields["mode"]))
	}
	if rt.Fields["computed_dynamic"][0].IsLiteralOrMissing {
		t.Fatal("expected computed_dynamic IsLiteralOrMissing == false")
	}

	wantDescs := map[string]string{
		"name":                 "Identifier in format `projects/{{project}}/widgets/{{name}}`.\n\nMust be unique.",
		"mode":                 "Operating mode of the widget.",
		"flag_field":           "Whether the flag is enabled.",
		"computed_dynamic":     "Should be ignored because non-literal.",
		"nested":               "Nested widget configuration.",
		"nested.child_count":   "Number of child elements with `backticks`.",
		"nested.inline_single": "Single-line inline field.",
	}

	var patches []PatchRequest
	for path, desc := range wantDescs {
		for _, ft := range rt.Fields[path] {
			patches = append(patches, PatchRequest{
				Target:         ft,
				NewDescription: desc,
			})
		}
	}
	// Duplicate identical PatchRequest for nested.child_count to test TargetID deduplication.
	patches = append(patches, PatchRequest{
		Target:         rt.Fields["nested.child_count"][0],
		NewDescription: wantDescs["nested.child_count"],
	})

	diffs, err := ApplyPatches(patches, nil)
	if err != nil {
		t.Fatalf("ApplyPatches failed: %v", err)
	}
	if len(diffs) != 2 {
		t.Fatalf("expected 2 FileDiffs, got %d", len(diffs))
	}

	for _, d := range diffs {
		if err := os.WriteFile(d.FilePath, d.UpdatedBytes, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Verify .go.tmpl still contains template guards AND renders valid Go in BOTH ga and beta!
	patchedTmplBytes, err := os.ReadFile(tmplPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(patchedTmplBytes, []byte(`{{- if ne $.TargetVersionName "ga" }}`)) {
		t.Fatal("template guard was lost in .go.tmpl")
	}
	assertValidRenderedGoTemplate(t, tmplPath, patchedTmplBytes)

	// Re-index and verify exact round-trip descriptions!
	resourcesAfter, err := IndexServiceDir(tmpDir)
	if err != nil {
		t.Fatalf("re-indexing after patch failed: %v", err)
	}
	rtAfter := resourcesAfter[0]
	for path, want := range wantDescs {
		if path == "computed_dynamic" {
			if rtAfter.Fields[path][0].IsLiteralOrMissing {
				t.Errorf("computed_dynamic unexpectedly became literal")
			}
			continue
		}
		for idx, ft := range rtAfter.Fields[path] {
			if !ft.HasDescription || !ft.IsLiteralOrMissing || ft.CurrentDescription != want {
				t.Errorf("field %s[%d] mismatch:\ngot:  %q (has=%v lit=%v)\nwant: %q",
					path, idx, ft.CurrentDescription, ft.HasDescription, ft.IsLiteralOrMissing, want)
			}
		}
	}

	// Idempotency check: applying the exact same patches again produces 0 FileDiffs!
	var secondPatches []PatchRequest
	for path, desc := range wantDescs {
		for _, ft := range rtAfter.Fields[path] {
			secondPatches = append(secondPatches, PatchRequest{
				Target:         ft,
				NewDescription: desc,
			})
		}
	}
	diffs2, err := ApplyPatches(secondPatches, nil)
	if err != nil {
		t.Fatalf("second ApplyPatches failed: %v", err)
	}
	if len(diffs2) != 0 {
		t.Fatalf("expected 0 FileDiffs on idempotent re-run, got %d", len(diffs2))
	}

	// Conflicting patches for the same TargetID must return an error.
	_, err = ApplyPatches([]PatchRequest{
		{Target: rtAfter.Fields["nested.child_count"][0], NewDescription: "desc A"},
		{Target: rtAfter.Fields["nested.child_count"][0], NewDescription: "desc B"},
	}, nil)
	if err == nil {
		t.Fatal("expected error for conflicting descriptions on same TargetID")
	}
}

func TestApplyPatches_PluginFramework_NestedAndResourceDesc(t *testing.T) {
	tmpDir := t.TempDir()
	fwPath := filepath.Join(tmpDir, "fw_resource_sample.go")
	fwSrc := `package sample

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
)

func init() {
	registry.FrameworkResource{
		Name:        "google_sample_fw",
		ProductName: "sample",
		Func:        NewSampleFWResource,
	}.Register()
}

func NewSampleFWResource() resource.Resource {
	return &sampleFWResource{}
}

type sampleFWResource struct{}

func (r *sampleFWResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"single_nested": schema.SingleNestedAttribute{
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"inner": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "old markdown desc",
					},
				},
			},
			"list_nested": schema.ListNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"item_key": schema.StringAttribute{
							Optional: true,
						},
					},
				},
			},
		},
		Blocks: map[string]schema.Block{
			"rule": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"priority": schema.Int64Attribute{
							Optional: true,
						},
					},
					Blocks: map[string]schema.Block{
						"match": schema.SingleNestedBlock{
							Attributes: map[string]schema.Attribute{
								"expr": schema.StringAttribute{
									Required: true,
								},
							},
						},
					},
				},
			},
		},
	}
}
`
	if err := os.WriteFile(fwPath, []byte(fwSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	targets, err := IndexServiceDir(tmpDir)
	if err != nil {
		t.Fatalf("IndexServiceDir failed: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 framework resource, got %d", len(targets))
	}
	rt := targets[0]
	if !rt.CanPatchResDesc || rt.HasResDescription {
		t.Fatalf("expected missing patchable resource description, got %+v", rt)
	}

	wantFields := []string{
		"id",
		"single_nested",
		"single_nested.inner",
		"list_nested",
		"list_nested.item_key",
		"rule",
		"rule.priority",
		"rule.match",
		"rule.match.expr",
	}
	var patches []PatchRequest
	for _, p := range wantFields {
		if len(rt.Fields[p]) != 1 {
			t.Fatalf("missing expected framework field %q", p)
		}
		patches = append(patches, PatchRequest{
			Target:         rt.Fields[p][0],
			NewDescription: "Doc for `" + p + "`.",
		})
	}
	resPatches := []ResourceDescPatch{
		{
			Resource:       rt,
			NewDescription: "Resource-level overview for `google_sample_fw`.",
		},
	}

	diffs, err := ApplyPatches(patches, resPatches)
	if err != nil {
		t.Fatalf("ApplyPatches failed: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 FileDiff, got %d", len(diffs))
	}
	if err := os.WriteFile(fwPath, diffs[0].UpdatedBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	// Verify gofmt clean and re-index round-trip + idempotency.
	formatted, err := format.Source(diffs[0].UpdatedBytes)
	if err != nil || !bytes.Equal(formatted, diffs[0].UpdatedBytes) {
		t.Fatalf("UpdatedBytes not gofmt-formatted: err=%v", err)
	}

	targets2, err := IndexServiceDir(tmpDir)
	if err != nil {
		t.Fatalf("re-index failed: %v", err)
	}
	rt2 := targets2[0]
	if !rt2.HasResDescription || rt2.CurrentResDesc != "Resource-level overview for `google_sample_fw`." {
		t.Fatalf("resource description mismatch: %+v", rt2)
	}
	for _, p := range wantFields {
		got := rt2.Fields[p][0].CurrentDescription
		want := "Doc for `" + p + "`."
		if got != want {
			t.Errorf("field %s description = %q, want %q", p, got, want)
		}
	}

	// Re-run with identical patches -> 0 diffs!
	var patches2 []PatchRequest
	for _, p := range wantFields {
		patches2 = append(patches2, PatchRequest{
			Target:         rt2.Fields[p][0],
			NewDescription: "Doc for `" + p + "`.",
		})
	}
	diffs2, err := ApplyPatches(patches2, []ResourceDescPatch{{
		Resource:       rt2,
		NewDescription: "Resource-level overview for `google_sample_fw`.",
	}})
	if err != nil || len(diffs2) != 0 {
		t.Fatalf("expected idempotent 0 diffs, got len=%d err=%v", len(diffs2), err)
	}
}

func TestApplyPatches_RealRepoSandboxSimulation(t *testing.T) {
	realServices := filepath.Join("..", "..", "..", "mmv1", "third_party", "terraform", "services")
	sandboxRoot := t.TempDir()

	// Copy complete real service packages with the trickiest .go and .go.tmpl schemas into sandboxRoot.
	servicesToTest := []string{"storage", "dataplex", "resourcemanager", "apigee"}
	for _, svc := range servicesToTest {
		srcDir := filepath.Join(realServices, svc)
		dstDir := filepath.Join(sandboxRoot, svc)
		if err := os.MkdirAll(dstDir, 0o755); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(srcDir)
		if err != nil {
			t.Fatal(err)
		}
		for _, ent := range entries {
			if ent.IsDir() || shouldSkipServiceFile(ent.Name()) {
				continue
			}
			b, err := os.ReadFile(filepath.Join(srcDir, ent.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dstDir, ent.Name()), b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	targets, err := IndexServicesRoot(sandboxRoot)
	if err != nil {
		t.Fatalf("IndexServicesRoot on sandbox failed: %v", err)
	}

	// Build deterministic patches for every literal/missing target (deduplicated by TargetID).
	seen := make(map[string]bool)
	var patches []PatchRequest
	var resPatches []ResourceDescPatch

	resNames := make([]string, 0, len(targets))
	for rn := range targets {
		resNames = append(resNames, rn)
	}
	sort.Strings(resNames)

	for _, rn := range resNames {
		rt := targets[rn]
		if rt.CanPatchResDesc {
			resPatches = append(resPatches, ResourceDescPatch{
				Resource:       rt,
				NewDescription: fmt.Sprintf("Backfilled overview for `%s` in projects/{{project}}.", rn),
			})
		}
		fpaths := make([]string, 0, len(rt.Fields))
		for fp := range rt.Fields {
			fpaths = append(fpaths, fp)
		}
		sort.Strings(fpaths)
		for _, fp := range fpaths {
			for _, ft := range rt.Fields[fp] {
				if !ft.IsLiteralOrMissing || seen[ft.TargetID] {
					continue
				}
				seen[ft.TargetID] = true
				patches = append(patches, PatchRequest{
					Target:         ft,
					NewDescription: fmt.Sprintf("Backfilled `%s` (%s) format projects/{{project}}.\nSecond line.", fp, rn),
				})
			}
		}
	}

	diffs, err := ApplyPatches(patches, resPatches)
	if err != nil {
		t.Fatalf("ApplyPatches on real sandbox failed: %v", err)
	}
	if len(diffs) == 0 {
		t.Fatal("expected non-zero FileDiffs on sandbox")
	}

	for _, d := range diffs {
		if err := os.WriteFile(d.FilePath, d.UpdatedBytes, 0o644); err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(d.FilePath, ".go.tmpl") {
			assertValidRenderedGoTemplate(t, d.FilePath, d.UpdatedBytes)
		} else {
			fset := token.NewFileSet()
			if _, err := parser.ParseFile(fset, d.FilePath, d.UpdatedBytes, parser.ParseComments); err != nil {
				t.Fatalf("patched .go file %s failed to parse: %v", d.FilePath, err)
			}
		}
	}

	// Re-index sandbox and verify 2nd ApplyPatches run produces 0 diffs (100% idempotency)!
	targetsAfter, err := IndexServicesRoot(sandboxRoot)
	if err != nil {
		t.Fatalf("re-indexing patched sandbox failed: %v", err)
	}

	seen2 := make(map[string]bool)
	var patches2 []PatchRequest
	var resPatches2 []ResourceDescPatch
	for _, rn := range resNames {
		rt := targetsAfter[rn]
		if rt.CanPatchResDesc {
			resPatches2 = append(resPatches2, ResourceDescPatch{
				Resource:       rt,
				NewDescription: fmt.Sprintf("Backfilled overview for `%s` in projects/{{project}}.", rn),
			})
		}
		fpaths := make([]string, 0, len(rt.Fields))
		for fp := range rt.Fields {
			fpaths = append(fpaths, fp)
		}
		sort.Strings(fpaths)
		for _, fp := range fpaths {
			for _, ft := range rt.Fields[fp] {
				if !ft.IsLiteralOrMissing || seen2[ft.TargetID] {
					continue
				}
				seen2[ft.TargetID] = true
				patches2 = append(patches2, PatchRequest{
					Target:         ft,
					NewDescription: fmt.Sprintf("Backfilled `%s` (%s) format projects/{{project}}.\nSecond line.", fp, rn),
				})
			}
		}
	}

	diffs2, err := ApplyPatches(patches2, resPatches2)
	if err != nil {
		t.Fatalf("second ApplyPatches on sandbox failed: %v", err)
	}
	if len(diffs2) != 0 {
		t.Fatalf("expected 0 diffs on idempotent sandbox re-run, got %d", len(diffs2))
	}
}

func assertValidRenderedGoTemplate(t *testing.T, path string, tmplBytes []byte) {
	t.Helper()
	tmpl, err := template.New(filepath.Base(path)).Parse(string(tmplBytes))
	if err != nil {
		t.Fatalf("text/template failed to parse patched %s: %v", path, err)
	}
	for _, ver := range []string{"ga", "beta"} {
		var rendered bytes.Buffer
		data := map[string]any{
			"TargetVersionName": ver,
			"Compiler":          "terraform",
		}
		if err := tmpl.Execute(&rendered, data); err != nil {
			t.Fatalf("text/template Execute(%s, %s) failed: %v", path, ver, err)
		}
		if strings.TrimSpace(rendered.String()) == "" {
			continue
		}
		fset := token.NewFileSet()
		if _, err := parser.ParseFile(fset, path, rendered.Bytes(), parser.ParseComments); err != nil {
			t.Fatalf("rendered %s (version=%s) is not valid Go: %v", path, ver, err)
		}
		if _, err := format.Source(rendered.Bytes()); err != nil {
			t.Fatalf("rendered %s (version=%s) failed format.Source: %v", path, ver, err)
		}
	}
}
