package acctest

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	fwResource "github.com/hashicorp/terraform-plugin-framework/resource"
	fwschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/hashicorp/terraform-provider-google/google/fwprovider"
	tpgprovider "github.com/hashicorp/terraform-provider-google/google/provider"
)

const (
	// RefreshStateRuleA flags an un-refreshed ImportStateVerify step targeting a resource
	// with at least one un-ignored SDKv2 TypeSet or Plugin Framework Set attribute/block.
	RefreshStateRuleA = "Rule A (un-ignored Set)"

	// RefreshStateRuleB flags >= 2 un-refreshed ImportStateVerify steps after the same
	// mutating Config step when a target resource has an un-ignored "etag" attribute.
	RefreshStateRuleB = "Rule B (multi-import etag)"

	// RefreshStateRuleStepProviders flags a RefreshState step omitting step-level provider
	// configuration inside a TestCase without top-level providers.
	RefreshStateRuleStepProviders = "RefreshState missing step providers"

	// RefreshStateRuleStepProvidersExpectNonEmptyPlan flags a RefreshState step with step-level
	// providers omitting ExpectNonEmptyPlan: true (testing_new.go overwrites the working directory
	// .tf config with a provider-only block when step-level providers are set, causing the
	// post-refresh plan to propose destroying all resources in state).
	RefreshStateRuleStepProvidersExpectNonEmptyPlan = "RefreshState with step providers missing ExpectNonEmptyPlan"
)

// RefreshStateSetField describes a Set attribute or block path within a resource schema.
// AncestorMaxItems records how each ancestor container serializes in shimmed flatmap state:
//   - 1: single-item list/set slice (flatmap key always uses ".0.")
//   - -1: single nested object/map (flatmap key has no index segment)
//   - 0 (or >1): multi-item slice/map (flatmap key index varies across elements)
type RefreshStateSetField struct {
	Path             string
	AncestorMaxItems []int
}

// RefreshStateSchemaInfo summarizes Set paths and top-level etag presence for a resource type.
type RefreshStateSchemaInfo struct {
	ResourceType string
	SetFields    []RefreshStateSetField
	HasEtag      bool
}

// RefreshStateStepInput is a normalized representation of a resource.TestStep shared by
// the runtime VcrTest check and the static AST check in allservices_test.
type RefreshStateStepInput struct {
	Index                   int
	Line                    int
	HasConfig               bool
	Destroy                 bool
	PlanOnly                bool
	HasExpectError          bool
	Query                   bool
	RefreshState            bool
	ExpectNonEmptyPlan      bool
	ImportState             bool
	ImportStateVerify       bool
	ResourceName            string
	ImportStateVerifyIgnore []string
	// HasStepProviders reports whether the step specifies step-level provider fields
	// (Providers, ProviderFactories, ProtoV5ProviderFactories, ProtoV6ProviderFactories, ExternalProviders).
	HasStepProviders bool
	// HasConfigDirOrFile reports whether ConfigDirectory or ConfigFile is set on the step.
	HasConfigDirOrFile bool
	// SkipImportVerifyCheck is set only by static AST analysis when ResourceName or
	// ImportStateVerifyIgnore uses a dynamic expression that cannot be resolved statically
	// (runtime VcrTest still validates the concrete evaluated step).
	SkipImportVerifyCheck bool
}

// RefreshStateViolation describes a missing `{ RefreshState: true }` transition.
type RefreshStateViolation struct {
	ConfigStepIndex   int
	ConfigStepLine    int
	ImportStepIndex   int
	ImportStepLine    int
	ResourceName      string
	ResourceType      string
	Rule              string
	UnignoredSetPaths []string
}

var (
	refreshStateSchemaOnce sync.Once
	refreshStateSchemaMap  map[string]RefreshStateSchemaInfo
)

func getRuntimeRefreshStateSchemas() map[string]RefreshStateSchemaInfo {
	refreshStateSchemaOnce.Do(func() {
		ctx := context.Background()
		sdkMap := tpgprovider.ResourceMap()
		fwFuncs := fwprovider.New(tpgprovider.Provider()).Resources(ctx)
		refreshStateSchemaMap = BuildRefreshStateSchemaMap(sdkMap, fwFuncs)
	})
	return refreshStateSchemaMap
}

// BuildRefreshStateSchemaMap extracts Set field paths and etag presence across SDKv2 and Plugin Framework resources.
func BuildRefreshStateSchemaMap(sdkResources map[string]*schema.Resource, fwResources []func() fwResource.Resource) map[string]RefreshStateSchemaInfo {
	out := make(map[string]RefreshStateSchemaInfo, len(sdkResources)+len(fwResources))

	for resType, res := range sdkResources {
		if res == nil {
			continue
		}
		var fields []RefreshStateSetField
		visited := make(map[*schema.Resource]bool)
		walkSDKRefreshStateSchema(res.Schema, "", nil, visited, &fields)
		sort.Slice(fields, func(i, j int) bool { return fields[i].Path < fields[j].Path })
		_, hasEtag := res.Schema["etag"]
		out[resType] = RefreshStateSchemaInfo{
			ResourceType: resType,
			SetFields:    fields,
			HasEtag:      hasEtag,
		}
	}

	ctx := context.Background()
	for _, fn := range fwResources {
		if fn == nil {
			continue
		}
		r := fn()
		var metaResp fwResource.MetadataResponse
		r.Metadata(ctx, fwResource.MetadataRequest{ProviderTypeName: "google"}, &metaResp)
		var schemaResp fwResource.SchemaResponse
		r.Schema(ctx, fwResource.SchemaRequest{}, &schemaResp)

		var fields []RefreshStateSetField
		walkFWRefreshStateAttrs(schemaResp.Schema.Attributes, "", nil, &fields)
		walkFWRefreshStateBlocks(schemaResp.Schema.Blocks, "", nil, &fields)
		sort.Slice(fields, func(i, j int) bool { return fields[i].Path < fields[j].Path })
		_, hasEtag := schemaResp.Schema.Attributes["etag"]
		out[metaResp.TypeName] = RefreshStateSchemaInfo{
			ResourceType: metaResp.TypeName,
			SetFields:    fields,
			HasEtag:      hasEtag,
		}
	}

	return out
}

func walkSDKRefreshStateSchema(m map[string]*schema.Schema, prefix string, ancestorMax []int, visited map[*schema.Resource]bool, out *[]RefreshStateSetField) {
	for name, s := range m {
		if s == nil {
			continue
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		if s.Type == schema.TypeSet {
			ancCopy := append([]int(nil), ancestorMax...)
			*out = append(*out, RefreshStateSetField{
				Path:             path,
				AncestorMaxItems: ancCopy,
			})
		}
		if subRes, ok := s.Elem.(*schema.Resource); ok && subRes != nil {
			if !visited[subRes] && len(ancestorMax) < 15 {
				visited[subRes] = true
				ancMax := 0
				if s.MaxItems == 1 {
					ancMax = 1
				}
				nextAnc := append(append([]int(nil), ancestorMax...), ancMax)
				walkSDKRefreshStateSchema(subRes.Schema, path, nextAnc, visited, out)
				delete(visited, subRes)
			}
		}
	}
}

func walkFWRefreshStateAttrs(attrs map[string]fwschema.Attribute, prefix string, anc []int, out *[]RefreshStateSetField) {
	for name, attr := range attrs {
		p := name
		if prefix != "" {
			p = prefix + "." + name
		}
		ancCopy := append([]int(nil), anc...)
		switch a := attr.(type) {
		case fwschema.SetAttribute:
			*out = append(*out, RefreshStateSetField{
				Path:             p,
				AncestorMaxItems: ancCopy,
			})
		case fwschema.SetNestedAttribute:
			*out = append(*out, RefreshStateSetField{
				Path:             p,
				AncestorMaxItems: ancCopy,
			})
			walkFWRefreshStateAttrs(a.NestedObject.Attributes, p, append(ancCopy, 0), out)
		case fwschema.ListNestedAttribute:
			walkFWRefreshStateAttrs(a.NestedObject.Attributes, p, append(ancCopy, 0), out)
		case fwschema.MapNestedAttribute:
			walkFWRefreshStateAttrs(a.NestedObject.Attributes, p, append(ancCopy, 0), out)
		case fwschema.SingleNestedAttribute:
			walkFWRefreshStateAttrs(a.Attributes, p, append(ancCopy, -1), out)
		}
	}
}

func walkFWRefreshStateBlocks(blocks map[string]fwschema.Block, prefix string, anc []int, out *[]RefreshStateSetField) {
	for name, blk := range blocks {
		p := name
		if prefix != "" {
			p = prefix + "." + name
		}
		ancCopy := append([]int(nil), anc...)
		switch b := blk.(type) {
		case fwschema.SetNestedBlock:
			*out = append(*out, RefreshStateSetField{
				Path:             p,
				AncestorMaxItems: ancCopy,
			})
			walkFWRefreshStateAttrs(b.NestedObject.Attributes, p, append(ancCopy, 0), out)
			walkFWRefreshStateBlocks(b.NestedObject.Blocks, p, append(ancCopy, 0), out)
		case fwschema.ListNestedBlock:
			walkFWRefreshStateAttrs(b.NestedObject.Attributes, p, append(ancCopy, 0), out)
			walkFWRefreshStateBlocks(b.NestedObject.Blocks, p, append(ancCopy, 0), out)
		case fwschema.SingleNestedBlock:
			walkFWRefreshStateAttrs(b.Attributes, p, append(ancCopy, -1), out)
			walkFWRefreshStateBlocks(b.Blocks, p, append(ancCopy, -1), out)
		}
	}
}

// ExtractResourceTypeFromAddr extracts the Terraform resource type (e.g. "google_compute_instance")
// from a resource address such as "google_compute_instance.foobar", "google_compute_instance.foobar[0]",
// or "module.m.google_compute_instance.foobar". Returns "" for data sources or invalid addresses.
func ExtractResourceTypeFromAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	parts := strings.Split(addr, ".")
	for len(parts) >= 3 && parts[0] == "module" {
		parts = parts[2:]
	}
	if len(parts) < 2 || parts[0] == "data" {
		return ""
	}
	resType := parts[0]
	if bracket := strings.IndexByte(resType, '['); bracket >= 0 {
		resType = resType[:bracket]
	}
	return resType
}

// IsSetFieldIgnored reports whether every possible flatmap key produced by sf in terraform-plugin-testing's
// shimmedFlatmap is stripped by strings.HasPrefix(k, ig) for at least one ignore entry in ignores.
func IsSetFieldIgnored(sf RefreshStateSetField, ignores []string) bool {
	segs := strings.Split(sf.Path, ".")
	for _, ig := range ignores {
		if ig == "" {
			return true
		}
		if igMatchesAllSetKeys(segs, sf.AncestorMaxItems, ig) {
			return true
		}
	}
	return false
}

func igMatchesAllSetKeys(segs []string, ancestorMaxItems []int, ig string) bool {
	prefix := ""
	for d, seg := range segs {
		segPrefix := seg
		if prefix != "" {
			segPrefix = prefix + "." + seg
		}
		if strings.HasPrefix(segPrefix+".", ig) {
			return true
		}
		if d < len(segs)-1 {
			ancKind := 0
			if d < len(ancestorMaxItems) {
				ancKind = ancestorMaxItems[d]
			}
			switch ancKind {
			case 1:
				prefix = segPrefix + ".0"
				if strings.HasPrefix(prefix+".", ig) {
					return true
				}
			case -1:
				prefix = segPrefix
			default:
				return false
			}
		}
	}
	return false
}

func isTopLevelAttrIgnored(attr string, ignores []string) bool {
	for _, ig := range ignores {
		if ig != "" && strings.HasPrefix(attr, ig) {
			return true
		}
	}
	return false
}

// CheckRefreshStateSteps evaluates Rules A and B across a sequence of normalized TestSteps.
func CheckRefreshStateSteps(steps []RefreshStateStepInput, schemas map[string]RefreshStateSchemaInfo) []RefreshStateViolation {
	var violations []RefreshStateViolation

	stateNeedsRefresh := false
	configStepIdx := -1
	configStepLine := 0
	importVerifyCount := 0
	var firstUnignoredEtagStep *RefreshStateStepInput
	var windowRuleAViolation *RefreshStateViolation

	flushWindow := func() {
		if !stateNeedsRefresh {
			return
		}
		if windowRuleAViolation != nil {
			violations = append(violations, *windowRuleAViolation)
		} else if importVerifyCount >= 2 && firstUnignoredEtagStep != nil {
			violations = append(violations, RefreshStateViolation{
				ConfigStepIndex: configStepIdx,
				ConfigStepLine:  configStepLine,
				ImportStepIndex: firstUnignoredEtagStep.Index,
				ImportStepLine:  firstUnignoredEtagStep.Line,
				ResourceName:    firstUnignoredEtagStep.ResourceName,
				ResourceType:    ExtractResourceTypeFromAddr(firstUnignoredEtagStep.ResourceName),
				Rule:            RefreshStateRuleB,
			})
		}
		stateNeedsRefresh = false
		configStepIdx = -1
		configStepLine = 0
		importVerifyCount = 0
		firstUnignoredEtagStep = nil
		windowRuleAViolation = nil
	}

	for _, st := range steps {
		if st.RefreshState {
			flushWindow()
			continue
		}

		isMutatingConfig := st.HasConfig && !st.Destroy && !st.PlanOnly && !st.HasExpectError && !st.Query && !st.ImportState && !st.RefreshState
		if isMutatingConfig {
			flushWindow()
			stateNeedsRefresh = true
			configStepIdx = st.Index
			configStepLine = st.Line
			continue
		}

		isSuccessfulDestroy := st.HasConfig && st.Destroy && !st.PlanOnly && !st.HasExpectError && !st.ImportState
		if isSuccessfulDestroy {
			flushWindow()
			continue
		}

		if !stateNeedsRefresh || !st.ImportState || !st.ImportStateVerify {
			continue
		}

		importVerifyCount++
		if st.SkipImportVerifyCheck {
			continue
		}

		resType := ExtractResourceTypeFromAddr(st.ResourceName)
		if resType == "" {
			continue
		}
		resInfo, ok := schemas[resType]
		if !ok {
			continue
		}

		effectiveIgnores := make([]string, 0, len(st.ImportStateVerifyIgnore)+3)
		effectiveIgnores = append(effectiveIgnores, st.ImportStateVerifyIgnore...)
		effectiveIgnores = append(effectiveIgnores, "terraform_labels", "deletion_policy", "timeouts")

		if resInfo.HasEtag && firstUnignoredEtagStep == nil && !isTopLevelAttrIgnored("etag", effectiveIgnores) {
			stCopy := st
			firstUnignoredEtagStep = &stCopy
		}

		if windowRuleAViolation == nil && len(resInfo.SetFields) > 0 {
			var unignoredSets []string
			for _, sf := range resInfo.SetFields {
				if !IsSetFieldIgnored(sf, effectiveIgnores) {
					unignoredSets = append(unignoredSets, sf.Path)
				}
			}
			if len(unignoredSets) > 0 {
				windowRuleAViolation = &RefreshStateViolation{
					ConfigStepIndex:   configStepIdx,
					ConfigStepLine:    configStepLine,
					ImportStepIndex:   st.Index,
					ImportStepLine:    st.Line,
					ResourceName:      st.ResourceName,
					ResourceType:      resType,
					Rule:              RefreshStateRuleA,
					UnignoredSetPaths: unignoredSets,
				}
			}
		}
	}

	flushWindow()
	return violations
}

// CheckRefreshStateProviderSteps validates per-step provider configuration and ExpectNonEmptyPlan
// on RefreshState steps:
//  1. When a TestCase lacks top-level provider configuration, every RefreshState step must specify
//     step-level providers (or ConfigDirectory/ConfigFile).
//  2. Whenever a RefreshState step specifies step-level providers, terraform-plugin-testing's
//     testing_new.go overwrites the working directory .tf configuration with a provider-only block
//     (zero resource blocks), so RefreshState's post-refresh plan is non-empty and requires
//     ExpectNonEmptyPlan: true.
func CheckRefreshStateProviderSteps(testCaseHasProviders bool, steps []RefreshStateStepInput) []RefreshStateViolation {
	var violations []RefreshStateViolation
	for _, st := range steps {
		if !st.RefreshState {
			continue
		}
		if !testCaseHasProviders && !st.HasStepProviders && !st.HasConfigDirOrFile {
			violations = append(violations, RefreshStateViolation{
				ImportStepIndex: st.Index,
				ImportStepLine:  st.Line,
				Rule:            RefreshStateRuleStepProviders,
			})
		} else if st.HasStepProviders && !st.HasConfigDirOrFile && !st.ExpectNonEmptyPlan {
			violations = append(violations, RefreshStateViolation{
				ImportStepIndex: st.Index,
				ImportStepLine:  st.Line,
				Rule:            RefreshStateRuleStepProvidersExpectNonEmptyPlan,
			})
		}
	}
	return violations
}

func stepsToRefreshStateInputs(steps []resource.TestStep) []RefreshStateStepInput {
	inputs := make([]RefreshStateStepInput, len(steps))
	for i, s := range steps {
		hasStepProviders := len(s.ExternalProviders) > 0 ||
			len(s.ProtoV5ProviderFactories) > 0 ||
			len(s.ProtoV6ProviderFactories) > 0 ||
			len(s.ProviderFactories) > 0
		hasConfigDirOrFile := s.ConfigDirectory != nil || s.ConfigFile != nil
		inputs[i] = RefreshStateStepInput{
			Index:                   i,
			HasConfig:               s.Config != "" || hasConfigDirOrFile,
			Destroy:                 s.Destroy,
			PlanOnly:                s.PlanOnly,
			HasExpectError:          s.ExpectError != nil,
			Query:                   s.Query,
			RefreshState:            s.RefreshState,
			ExpectNonEmptyPlan:      s.ExpectNonEmptyPlan,
			ImportState:             s.ImportState,
			ImportStateVerify:       s.ImportStateVerify,
			ResourceName:            s.ResourceName,
			ImportStateVerifyIgnore: s.ImportStateVerifyIgnore,
			HasStepProviders:        hasStepProviders,
			HasConfigDirOrFile:      hasConfigDirOrFile,
		}
	}
	return inputs
}

func formatRefreshStateViolations(testName string, violations []RefreshStateViolation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "RefreshState validation failed in %s:\n", testName)
	hasMissingRefresh := false
	for _, v := range violations {
		switch v.Rule {
		case RefreshStateRuleStepProviders:
			fmt.Fprintf(&b, "  - RefreshState step %d (index %d) omits step-level provider configuration (`ProtoV5ProviderFactories`, `ExternalProviders`, etc.) and `ExpectNonEmptyPlan: true` in a `TestCase` without top-level providers [%s]\n",
				v.ImportStepIndex+1, v.ImportStepIndex, v.Rule)
		case RefreshStateRuleStepProvidersExpectNonEmptyPlan:
			fmt.Fprintf(&b, "  - RefreshState step %d (index %d) specifies step-level providers without `ExpectNonEmptyPlan: true` (terraform-plugin-testing overwrites .tf config with a provider-only block when step-level providers are set, making the post-refresh plan non-empty) [%s]\n",
				v.ImportStepIndex+1, v.ImportStepIndex, v.Rule)
		default:
			hasMissingRefresh = true
			detail := v.Rule
			if len(v.UnignoredSetPaths) > 0 {
				detail = fmt.Sprintf("%s: %s", v.Rule, strings.Join(v.UnignoredSetPaths, ", "))
			}
			fmt.Fprintf(&b, "  - missing `{RefreshState: true},` after Config step %d (index %d), before ImportStateVerify step %d (index %d) for %q [%s]\n",
				v.ConfigStepIndex+1, v.ConfigStepIndex, v.ImportStepIndex+1, v.ImportStepIndex, v.ResourceName, detail)
		}
	}
	if hasMissingRefresh {
		b.WriteString("Insert `{RefreshState: true},` immediately after the mutating Config step (before ImportStateVerify) so post-apply state is refreshed before ImportStateVerify.")
	}
	return b.String()
}

func testCaseHasTopLevelProviders(c resource.TestCase) bool {
	return len(c.ExternalProviders) > 0 ||
		len(c.ProtoV5ProviderFactories) > 0 ||
		len(c.ProtoV6ProviderFactories) > 0 ||
		len(c.ProviderFactories) > 0 ||
		len(c.Providers) > 0
}

func validateRefreshStateBeforeImportVerify(t *testing.T, c resource.TestCase) {
	t.Helper()
	inputs := stepsToRefreshStateInputs(c.Steps)
	violations := CheckRefreshStateProviderSteps(testCaseHasTopLevelProviders(c), inputs)
	violations = append(violations, CheckRefreshStateSteps(inputs, getRuntimeRefreshStateSchemas())...)
	if len(violations) > 0 {
		t.Fatalf("acctest.VcrTest: %s", formatRefreshStateViolations(t.Name(), violations))
	}
}
