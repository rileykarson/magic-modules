package acctest

import (
	"context"
	"regexp"
	"strings"
	"testing"

	fwResource "github.com/hashicorp/terraform-plugin-framework/resource"
	fwschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	fwtypes "github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

type fakeFWSetResource struct {
	typeName string
	sch      fwschema.Schema
}

func (f *fakeFWSetResource) Metadata(_ context.Context, _ fwResource.MetadataRequest, resp *fwResource.MetadataResponse) {
	resp.TypeName = f.typeName
}

func (f *fakeFWSetResource) Schema(_ context.Context, _ fwResource.SchemaRequest, resp *fwResource.SchemaResponse) {
	resp.Schema = f.sch
}

func (f *fakeFWSetResource) Create(context.Context, fwResource.CreateRequest, *fwResource.CreateResponse) {
}
func (f *fakeFWSetResource) Read(context.Context, fwResource.ReadRequest, *fwResource.ReadResponse) {
}
func (f *fakeFWSetResource) Update(context.Context, fwResource.UpdateRequest, *fwResource.UpdateResponse) {
}
func (f *fakeFWSetResource) Delete(context.Context, fwResource.DeleteRequest, *fwResource.DeleteResponse) {
}

func buildTestRefreshSchemas() map[string]RefreshStateSchemaInfo {
	sdkMap := map[string]*schema.Resource{
		"google_plain_resource": {
			Schema: map[string]*schema.Schema{
				"name": {Type: schema.TypeString, Required: true},
			},
		},
		"google_etag_resource": {
			Schema: map[string]*schema.Schema{
				"name": {Type: schema.TypeString, Required: true},
				"etag": {Type: schema.TypeString, Computed: true},
			},
		},
		"google_top_set_resource": {
			Schema: map[string]*schema.Schema{
				"name": {Type: schema.TypeString, Required: true},
				"tags": {
					Type:     schema.TypeSet,
					Optional: true,
					Elem:     &schema.Schema{Type: schema.TypeString},
				},
			},
		},
		"google_nested_set_resource": {
			Schema: map[string]*schema.Schema{
				"single_block": {
					Type:     schema.TypeList,
					Optional: true,
					MaxItems: 1,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"nested_set": {
								Type:     schema.TypeSet,
								Optional: true,
								Elem:     &schema.Schema{Type: schema.TypeString},
							},
						},
					},
				},
				"multi_block": {
					Type:     schema.TypeList,
					Optional: true,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"nested_set": {
								Type:     schema.TypeSet,
								Optional: true,
								Elem:     &schema.Schema{Type: schema.TypeString},
							},
						},
					},
				},
			},
		},
	}

	fwSlice := []func() fwResource.Resource{
		func() fwResource.Resource {
			return &fakeFWSetResource{
				typeName: "google_fw_set_resource",
				sch: fwschema.Schema{
					Attributes: map[string]fwschema.Attribute{
						"event_types": fwschema.SetAttribute{
							ElementType: fwtypes.StringType,
							Optional:    true,
						},
						"single_nested": fwschema.SingleNestedAttribute{
							Optional: true,
							Attributes: map[string]fwschema.Attribute{
								"items": fwschema.SetAttribute{
									ElementType: fwtypes.StringType,
									Optional:    true,
								},
							},
						},
					},
				},
			}
		},
	}

	return BuildRefreshStateSchemaMap(sdkMap, fwSlice)
}

func TestExtractResourceTypeFromAddr(t *testing.T) {
	cases := []struct {
		addr string
		want string
	}{
		{"google_compute_instance.foobar", "google_compute_instance"},
		{"google_compute_instance.foobar[0]", "google_compute_instance"},
		{"module.net.google_compute_network.vpc", "google_compute_network"},
		{"module.a.module.b.google_storage_bucket.bkt", "google_storage_bucket"},
		{"data.google_project.project", ""},
		{"module.a.data.google_project.project", ""},
		{"invalid_no_dot", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := ExtractResourceTypeFromAddr(tc.addr); got != tc.want {
			t.Errorf("ExtractResourceTypeFromAddr(%q) = %q, want %q", tc.addr, got, tc.want)
		}
	}
}

func TestCheckRefreshStateSteps(t *testing.T) {
	schemas := buildTestRefreshSchemas()

	tests := []struct {
		name           string
		steps          []resource.TestStep
		wantViolations int
		wantRule       string
	}{
		{
			name: "plain resource without Set passes without RefreshState",
			steps: []resource.TestStep{
				{Config: `resource "google_plain_resource" "test" {}`},
				{ResourceName: "google_plain_resource.test", ImportState: true, ImportStateVerify: true},
			},
			wantViolations: 0,
		},
		{
			name: "SDKv2 TypeSet resource fails without RefreshState",
			steps: []resource.TestStep{
				{Config: `resource "google_top_set_resource" "test" {}`},
				{ResourceName: "google_top_set_resource.test", ImportState: true, ImportStateVerify: true},
			},
			wantViolations: 1,
			wantRule:       RefreshStateRuleA,
		},
		{
			name: "SDKv2 TypeSet resource passes with preceding RefreshState",
			steps: []resource.TestStep{
				{Config: `resource "google_top_set_resource" "test" {}`},
				{RefreshState: true},
				{ResourceName: "google_top_set_resource.test", ImportState: true, ImportStateVerify: true},
			},
			wantViolations: 0,
		},
		{
			name: "RefreshState with ExpectNonEmptyPlan satisfies check",
			steps: []resource.TestStep{
				{Config: `resource "google_top_set_resource" "test" {}`, ExpectNonEmptyPlan: true},
				{RefreshState: true, ExpectNonEmptyPlan: true},
				{ResourceName: "google_top_set_resource.test", ImportState: true, ImportStateVerify: true},
			},
			wantViolations: 0,
		},
		{
			name: "misplaced RefreshState after ImportStateVerify still fails",
			steps: []resource.TestStep{
				{Config: `resource "google_top_set_resource" "test" {}`},
				{ResourceName: "google_top_set_resource.test", ImportState: true, ImportStateVerify: true},
				{RefreshState: true},
			},
			wantViolations: 1,
			wantRule:       RefreshStateRuleA,
		},
		{
			name: "single RefreshState covers multiple consecutive ImportStateVerify steps",
			steps: []resource.TestStep{
				{Config: `resource "google_top_set_resource" "a" {}`},
				{RefreshState: true},
				{ResourceName: "google_top_set_resource.a", ImportState: true, ImportStateVerify: true},
				{ResourceName: "google_top_set_resource.b", ImportState: true, ImportStateVerify: true},
				{ResourceName: "google_top_set_resource.c", ImportState: true, ImportStateVerify: true},
			},
			wantViolations: 0,
		},
		{
			name: "ExpectError Config step after RefreshState keeps state refreshed",
			steps: []resource.TestStep{
				{Config: `resource "google_top_set_resource" "test" {}`},
				{RefreshState: true},
				{ResourceName: "google_top_set_resource.test", ImportState: true, ImportStateVerify: true},
				{Config: `resource "google_top_set_resource" "test" { bad = true }`, ExpectError: regexp.MustCompile("err")},
				{ResourceName: "google_top_set_resource.test", ImportState: true, ImportStateVerify: true},
			},
			wantViolations: 0,
		},
		{
			name: "top-level TypeSet ignored in ImportStateVerifyIgnore passes",
			steps: []resource.TestStep{
				{Config: `resource "google_top_set_resource" "test" {}`},
				{
					ResourceName:            "google_top_set_resource.test",
					ImportState:             true,
					ImportStateVerify:       true,
					ImportStateVerifyIgnore: []string{"tags"},
				},
			},
			wantViolations: 0,
		},
		{
			name: "nested MaxItems=1 set ignored via .0. path and multi-item set ignored via parent block passes",
			steps: []resource.TestStep{
				{Config: `resource "google_nested_set_resource" "test" {}`},
				{
					ResourceName:            "google_nested_set_resource.test",
					ImportState:             true,
					ImportStateVerify:       true,
					ImportStateVerifyIgnore: []string{"single_block.0.nested_set", "multi_block"},
				},
			},
			wantViolations: 0,
		},
		{
			name: "nested MaxItems=1 set with missing .0. in ignore path fails",
			steps: []resource.TestStep{
				{Config: `resource "google_nested_set_resource" "test" {}`},
				{
					ResourceName:            "google_nested_set_resource.test",
					ImportState:             true,
					ImportStateVerify:       true,
					ImportStateVerifyIgnore: []string{"single_block.nested_set", "multi_block"},
				},
			},
			wantViolations: 1,
			wantRule:       RefreshStateRuleA,
		},
		{
			name: "nested multi-item set ignored only at .0. index fails because index 1+ is not ignored",
			steps: []resource.TestStep{
				{Config: `resource "google_nested_set_resource" "test" {}`},
				{
					ResourceName:            "google_nested_set_resource.test",
					ImportState:             true,
					ImportStateVerify:       true,
					ImportStateVerifyIgnore: []string{"single_block", "multi_block.0.nested_set"},
				},
			},
			wantViolations: 1,
			wantRule:       RefreshStateRuleA,
		},
		{
			name: "Plugin Framework SetAttribute and SingleNestedAttribute Set handling",
			steps: []resource.TestStep{
				{Config: `resource "google_fw_set_resource" "test" {}`},
				{
					ResourceName:            "google_fw_set_resource.test",
					ImportState:             true,
					ImportStateVerify:       true,
					ImportStateVerifyIgnore: []string{"event_types"},
				},
			},
			wantViolations: 1,
			wantRule:       RefreshStateRuleA,
		},
		{
			name: "Plugin Framework SingleNestedAttribute Set ignored without .0. passes",
			steps: []resource.TestStep{
				{Config: `resource "google_fw_set_resource" "test" {}`},
				{
					ResourceName:            "google_fw_set_resource.test",
					ImportState:             true,
					ImportStateVerify:       true,
					ImportStateVerifyIgnore: []string{"event_types", "single_nested.items"},
				},
			},
			wantViolations: 0,
		},
		{
			name: "single ImportStateVerify on etag-only resource passes without RefreshState",
			steps: []resource.TestStep{
				{Config: `resource "google_etag_resource" "a" {}`},
				{ResourceName: "google_etag_resource.a", ImportState: true, ImportStateVerify: true},
			},
			wantViolations: 0,
		},
		{
			name: "multiple consecutive ImportStateVerify steps on un-ignored etag resource fails Rule B",
			steps: []resource.TestStep{
				{Config: `resource "google_etag_resource" "a" {}`},
				{ResourceName: "google_etag_resource.a", ImportState: true, ImportStateVerify: true},
				{ResourceName: "google_etag_resource.b", ImportState: true, ImportStateVerify: true},
			},
			wantViolations: 1,
			wantRule:       RefreshStateRuleB,
		},
		{
			name: "multiple consecutive ImportStateVerify steps on ignored etag resource passes",
			steps: []resource.TestStep{
				{Config: `resource "google_etag_resource" "a" {}`},
				{ResourceName: "google_etag_resource.a", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"etag"}},
				{ResourceName: "google_etag_resource.b", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"etag"}},
			},
			wantViolations: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckRefreshStateSteps(stepsToRefreshStateInputs(tt.steps), schemas)
			if len(got) != tt.wantViolations {
				t.Fatalf("CheckRefreshStateSteps() returned %d violations (%+v), want %d", len(got), got, tt.wantViolations)
			}
			if tt.wantViolations > 0 && got[0].Rule != tt.wantRule {
				t.Errorf("violation rule = %q, want %q", got[0].Rule, tt.wantRule)
			}
			if tt.wantViolations > 0 {
				msg := formatRefreshStateViolations("TestAccSynthetic", got)
				if !strings.Contains(msg, "RefreshState: true") {
					t.Errorf("expected actionable message mentioning RefreshState: true, got: %s", msg)
				}
			}
		})
	}
}

func TestCheckRefreshStateProviderSteps(t *testing.T) {
	extProviders := map[string]resource.ExternalProvider{
		"google": {Source: "hashicorp/google", VersionConstraint: "5.30.0"},
	}

	tests := []struct {
		name           string
		tcHasProviders bool
		steps          []resource.TestStep
		wantViolations int
		wantRule       string
	}{
		{
			name:           "top-level TestCase providers with plain RefreshState passes",
			tcHasProviders: true,
			steps: []resource.TestStep{
				{Config: `resource "google_plain_resource" "a" {}`},
				{RefreshState: true},
			},
			wantViolations: 0,
		},
		{
			name:           "per-step TestCase with RefreshState omitting step providers fails",
			tcHasProviders: false,
			steps: []resource.TestStep{
				{Config: `resource "google_plain_resource" "a" {}`, ExternalProviders: extProviders},
				{RefreshState: true},
			},
			wantViolations: 1,
			wantRule:       RefreshStateRuleStepProviders,
		},
		{
			name:           "per-step TestCase with RefreshState having step providers but omitting ExpectNonEmptyPlan fails",
			tcHasProviders: false,
			steps: []resource.TestStep{
				{Config: `resource "google_plain_resource" "a" {}`, ExternalProviders: extProviders},
				{RefreshState: true, ExternalProviders: extProviders},
			},
			wantViolations: 1,
			wantRule:       RefreshStateRuleStepProvidersExpectNonEmptyPlan,
		},
		{
			name:           "per-step TestCase with RefreshState having step providers and ExpectNonEmptyPlan passes",
			tcHasProviders: false,
			steps: []resource.TestStep{
				{Config: `resource "google_plain_resource" "a" {}`, ExternalProviders: extProviders},
				{RefreshState: true, ExpectNonEmptyPlan: true, ExternalProviders: extProviders},
			},
			wantViolations: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckRefreshStateProviderSteps(tt.tcHasProviders, stepsToRefreshStateInputs(tt.steps))
			if len(got) != tt.wantViolations {
				t.Fatalf("CheckRefreshStateProviderSteps() returned %d violations (%+v), want %d", len(got), got, tt.wantViolations)
			}
			if tt.wantViolations > 0 && got[0].Rule != tt.wantRule {
				t.Errorf("violation rule = %q, want %q", got[0].Rule, tt.wantRule)
			}
		})
	}
}
