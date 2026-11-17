package acctest

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
)

var _ statecheck.StateCheck = expectIamIdentityMatchesState{}

var iamNonParentStateKeys = map[string]bool{
	"id":          true,
	"etag":        true,
	"role":        true,
	"member":      true,
	"members":     true,
	"condition":   true,
	"policy_data": true,
}

type expectIamIdentityMatchesState struct {
	resourceAddress string
}

// ExpectIamIdentityMatchesState returns a StateCheck that verifies an IAM resource's
// ResourceIdentity in state (*_iam_binding, *_iam_member, or *_iam_policy) matches
// its Terraform state attributes:
//   - IdentitySchemaVersion == 1
//   - Every parent identifier attribute in state is present in IdentityValues with the exact same value
//   - For *_iam_binding and *_iam_member: "role" matches state, and "condition_title" is nil when
//     no condition block is set or matches condition[0].title when a condition block is present
//   - For *_iam_member: "member" matches the normalized principal of state["member"]
//   - No extra or missing identity attributes exist
func ExpectIamIdentityMatchesState(resourceAddress string) statecheck.StateCheck {
	return expectIamIdentityMatchesState{
		resourceAddress: resourceAddress,
	}
}

func (e expectIamIdentityMatchesState) CheckState(_ context.Context, req statecheck.CheckStateRequest, resp *statecheck.CheckStateResponse) {
	if req.State == nil {
		resp.Error = fmt.Errorf("state is nil")
		return
	}
	if req.State.Values == nil || req.State.Values.RootModule == nil {
		resp.Error = fmt.Errorf("state does not contain root module values")
		return
	}

	var res *tfjson.StateResource
	for _, r := range req.State.Values.RootModule.Resources {
		if r.Address == e.resourceAddress {
			res = r
			break
		}
	}
	if res == nil {
		resp.Error = fmt.Errorf("%s - resource not found in state", e.resourceAddress)
		return
	}

	if res.IdentitySchemaVersion == nil {
		resp.Error = fmt.Errorf("%s - IdentitySchemaVersion is nil in state", e.resourceAddress)
		return
	}
	if *res.IdentitySchemaVersion != 1 {
		resp.Error = fmt.Errorf("%s - expected IdentitySchemaVersion 1, got %d", e.resourceAddress, *res.IdentitySchemaVersion)
		return
	}
	if len(res.IdentityValues) == 0 {
		resp.Error = fmt.Errorf("%s - IdentityValues is empty in state", e.resourceAddress)
		return
	}

	var isBinding, isMember, isPolicy bool
	switch {
	case strings.HasSuffix(res.Type, "_iam_binding"):
		isBinding = true
	case strings.HasSuffix(res.Type, "_iam_member"):
		isMember = true
	case strings.HasSuffix(res.Type, "_iam_policy"):
		isPolicy = true
	default:
		resp.Error = fmt.Errorf("%s - unsupported IAM resource type %q", e.resourceAddress, res.Type)
		return
	}
	_ = isPolicy

	expectedKeys := make(map[string]bool)
	var parentKeys []string
	for k := range res.AttributeValues {
		if !iamNonParentStateKeys[k] {
			parentKeys = append(parentKeys, k)
			expectedKeys[k] = true
		}
	}
	slices.Sort(parentKeys)
	if len(parentKeys) == 0 {
		resp.Error = fmt.Errorf("%s - no parent identifier attributes found in resource state", e.resourceAddress)
		return
	}

	if isBinding || isMember {
		expectedKeys["role"] = true
		expectedKeys["condition_title"] = true
	}
	if isMember {
		expectedKeys["member"] = true
	}

	// Verify exact identity attribute key set (no missing or extra keys).
	var missingKeys, extraKeys []string
	for _, k := range slices.Sorted(maps.Keys(expectedKeys)) {
		if _, ok := res.IdentityValues[k]; !ok {
			missingKeys = append(missingKeys, k)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(res.IdentityValues)) {
		if !expectedKeys[k] {
			extraKeys = append(extraKeys, k)
		}
	}
	if len(missingKeys) > 0 || len(extraKeys) > 0 {
		resp.Error = fmt.Errorf("%s - identity key mismatch (missing=%v, extra=%v)", e.resourceAddress, missingKeys, extraKeys)
		return
	}

	// Verify all parent attributes match state and at least one is non-empty.
	hasNonEmptyParent := false
	for _, k := range parentKeys {
		stateStr, ok := optionalStringValue(res.AttributeValues[k])
		if !ok {
			resp.Error = fmt.Errorf("%s - parent state attribute %q has unexpected non-string type %T", e.resourceAddress, k, res.AttributeValues[k])
			return
		}
		identVal := res.IdentityValues[k]
		identStr, ok := optionalStringValue(identVal)
		if !ok {
			resp.Error = fmt.Errorf("%s - parent identity attribute %q has unexpected non-string type %T", e.resourceAddress, k, identVal)
			return
		}
		if stateStr != "" {
			if identVal == nil || identStr != stateStr {
				resp.Error = fmt.Errorf("%s - parent attribute %q mismatch: identity=%v, state=%q", e.resourceAddress, k, identVal, stateStr)
				return
			}
			hasNonEmptyParent = true
		} else if identStr != "" {
			resp.Error = fmt.Errorf("%s - unset parent attribute %q should be empty in identity, got %q", e.resourceAddress, k, identStr)
			return
		}
	}
	if !hasNonEmptyParent {
		resp.Error = fmt.Errorf("%s - all parent identity attributes (%v) are empty", e.resourceAddress, parentKeys)
		return
	}

	if isBinding || isMember {
		stateRole, ok := nonEmptyStringValue(res.AttributeValues["role"])
		if !ok {
			resp.Error = fmt.Errorf("%s - state attribute %q must be a non-empty string, got %v", e.resourceAddress, "role", res.AttributeValues["role"])
			return
		}
		identRole, ok := nonEmptyStringValue(res.IdentityValues["role"])
		if !ok || identRole != stateRole {
			resp.Error = fmt.Errorf("%s - identity attribute %q mismatch: identity=%v, state=%q", e.resourceAddress, "role", res.IdentityValues["role"], stateRole)
			return
		}

		expectedCondTitle, err := extractConditionTitleFromState(res.AttributeValues["condition"])
		if err != nil {
			resp.Error = fmt.Errorf("%s - %w", e.resourceAddress, err)
			return
		}
		identCondVal := res.IdentityValues["condition_title"]
		if expectedCondTitle == "" {
			if identCondVal != nil {
				resp.Error = fmt.Errorf("%s - expected nil identity %q when no condition is set, got %v", e.resourceAddress, "condition_title", identCondVal)
				return
			}
		} else {
			identCondStr, ok := nonEmptyStringValue(identCondVal)
			if !ok || identCondStr != expectedCondTitle {
				resp.Error = fmt.Errorf("%s - identity attribute %q mismatch: identity=%v, state condition title=%q", e.resourceAddress, "condition_title", identCondVal, expectedCondTitle)
				return
			}
		}
	}

	if isMember {
		stateMember, ok := nonEmptyStringValue(res.AttributeValues["member"])
		if !ok {
			resp.Error = fmt.Errorf("%s - state attribute %q must be a non-empty string, got %v", e.resourceAddress, "member", res.AttributeValues["member"])
			return
		}
		wantMember := tpgresource.NormalizeIamPrincipalCasing(stateMember)
		identMember, ok := nonEmptyStringValue(res.IdentityValues["member"])
		if !ok || identMember != wantMember {
			resp.Error = fmt.Errorf("%s - identity attribute %q mismatch: identity=%v, expected=%q (state=%q)", e.resourceAddress, "member", res.IdentityValues["member"], wantMember, stateMember)
			return
		}
	}
}

func optionalStringValue(v any) (string, bool) {
	if v == nil {
		return "", true
	}
	s, ok := v.(string)
	return s, ok
}

func nonEmptyStringValue(v any) (string, bool) {
	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

func extractConditionTitleFromState(condVal any) (string, error) {
	if condVal == nil {
		return "", nil
	}
	condList, ok := condVal.([]any)
	if !ok {
		return "", fmt.Errorf("unexpected type %T for state condition attribute", condVal)
	}
	if len(condList) == 0 {
		return "", nil
	}
	condObj, ok := condList[0].(map[string]any)
	if !ok {
		return "", fmt.Errorf("unexpected type %T for state condition[0]", condList[0])
	}
	title, ok := nonEmptyStringValue(condObj["title"])
	if !ok {
		return "", fmt.Errorf("expected non-empty string for state condition[0].title, got %v", condObj["title"])
	}
	return title, nil
}
