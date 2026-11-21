package tpgresource

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// NullableFieldPresenceAttr is the top-level Computed schema attribute used on
// SDKv2 resources to track field presence for proto3-optional primitives
// (OptionalBoolean, OptionalDouble, OptionalInteger, OptionalString).
const NullableFieldPresenceAttr = "nullable_field_presence"

// NullableSegmentKind identifies the kind of container or terminal attribute
// along the path to a proto3-optional primitive field.
type NullableSegmentKind int

const (
	NullableSegmentAttr NullableSegmentKind = iota
	NullableSegmentNestedObject
	NullableSegmentFlattenObject
	NullableSegmentArray
	NullableSegmentMap
)

// NullablePathSegment describes a single step along the Terraform and API path
// to a proto3-optional primitive field.
type NullablePathSegment struct {
	TFName           string
	APIName          string
	Kind             NullableSegmentKind
	KeyName          string
	AllowEmptyObject bool
}

// NullableFieldSpec describes a proto3-optional primitive field on a resource.
type NullableFieldSpec struct {
	Segments []NullablePathSegment
	ForceNew bool
}

// FormatNullableMapSegment formats a Map entry step for nullable_field_presence keys.
func FormatNullableMapSegment(tfName, keyVal string) string {
	return fmt.Sprintf("%s[%q]", tfName, keyVal)
}

// JoinNullablePrefix joins an optional parent path prefix and child segment name.
func JoinNullablePrefix(prefix, next string) string {
	if prefix == "" {
		return next
	}
	if next == "" {
		return prefix
	}
	return prefix + "." + next
}

func joinNullablePrefix(prefix, next string) string {
	return JoinNullablePrefix(prefix, next)
}

// EvaluateNullableConfigPresence walks rawConfig using specs and returns a map
// of present field paths (value true), whether any evaluated branch contained an
// unknown value, and whether any unknown branch belonged to a ForceNew field.
func EvaluateNullableConfigPresence(rawConfig cty.Value, specs []NullableFieldSpec) (presence map[string]interface{}, hasUnknown bool, hasUnknownForceNew bool) {
	presence = make(map[string]interface{})
	if rawConfig == cty.NilVal || rawConfig.IsNull() {
		return presence, false, false
	}
	if !rawConfig.IsKnown() {
		for _, s := range specs {
			if s.ForceNew {
				return nil, true, true
			}
		}
		return nil, true, false
	}

	var walk func(curr cty.Value, prefix string, segs []NullablePathSegment, forceNew bool)
	walk = func(curr cty.Value, prefix string, segs []NullablePathSegment, forceNew bool) {
		if len(segs) == 0 || curr == cty.NilVal || curr.IsNull() {
			return
		}
		if !curr.IsKnown() {
			hasUnknown = true
			if forceNew {
				hasUnknownForceNew = true
			}
			return
		}

		seg := segs[0]
		switch seg.Kind {
		case NullableSegmentFlattenObject:
			walk(curr, prefix, segs[1:], forceNew)

		case NullableSegmentNestedObject, NullableSegmentArray:
			if !curr.Type().IsObjectType() || !curr.Type().HasAttribute(seg.TFName) {
				return
			}
			listVal := curr.GetAttr(seg.TFName)
			if !listVal.IsKnown() {
				hasUnknown = true
				if forceNew {
					hasUnknownForceNew = true
				}
				return
			}
			if listVal.IsNull() || !listVal.CanIterateElements() || listVal.LengthInt() == 0 {
				return
			}
			i := 0
			for it := listVal.ElementIterator(); it.Next(); i++ {
				_, elem := it.Element()
				if !elem.IsKnown() {
					hasUnknown = true
					if forceNew {
						hasUnknownForceNew = true
					}
					return
				}
				if elem.IsNull() {
					continue
				}
				nextPrefix := joinNullablePrefix(prefix, fmt.Sprintf("%s.%d", seg.TFName, i))
				walk(elem, nextPrefix, segs[1:], forceNew)
			}

		case NullableSegmentMap:
			if !curr.Type().IsObjectType() || !curr.Type().HasAttribute(seg.TFName) {
				return
			}
			setVal := curr.GetAttr(seg.TFName)
			if !setVal.IsKnown() {
				hasUnknown = true
				if forceNew {
					hasUnknownForceNew = true
				}
				return
			}
			if setVal.IsNull() || !setVal.CanIterateElements() || setVal.LengthInt() == 0 {
				return
			}
			for it := setVal.ElementIterator(); it.Next(); {
				_, elem := it.Element()
				if !elem.IsKnown() {
					hasUnknown = true
					if forceNew {
						hasUnknownForceNew = true
					}
					return
				}
				if elem.IsNull() || !elem.Type().IsObjectType() || !elem.Type().HasAttribute(seg.KeyName) {
					continue
				}
				kv := elem.GetAttr(seg.KeyName)
				if !kv.IsKnown() {
					hasUnknown = true
					if forceNew {
						hasUnknownForceNew = true
					}
					return
				}
				if kv.IsNull() {
					continue
				}
				nextPrefix := joinNullablePrefix(prefix, FormatNullableMapSegment(seg.TFName, kv.AsString()))
				walk(elem, nextPrefix, segs[1:], forceNew)
			}

		case NullableSegmentAttr:
			if !curr.Type().IsObjectType() || !curr.Type().HasAttribute(seg.TFName) {
				return
			}
			attrVal := curr.GetAttr(seg.TFName)
			if !attrVal.IsKnown() {
				hasUnknown = true
				if forceNew {
					hasUnknownForceNew = true
				}
				return
			}
			if !attrVal.IsNull() {
				presence[joinNullablePrefix(prefix, seg.TFName)] = true
			}
		}
	}

	for _, spec := range specs {
		walk(rawConfig, "", spec.Segments, spec.ForceNew)
	}
	if hasUnknown {
		return nil, true, hasUnknownForceNew
	}
	return presence, false, false
}

// MatchNullableFieldKey reports whether a flattened presence key matches spec.
func MatchNullableFieldKey(key string, spec NullableFieldSpec) bool {
	return matchNullableSegments(key, spec.Segments)
}

func matchNullableSegments(rem string, segs []NullablePathSegment) bool {
	if len(segs) == 0 {
		return false
	}
	seg := segs[0]
	switch seg.Kind {
	case NullableSegmentFlattenObject:
		return matchNullableSegments(rem, segs[1:])

	case NullableSegmentNestedObject, NullableSegmentArray:
		p := seg.TFName + "."
		if !strings.HasPrefix(rem, p) {
			return false
		}
		after := rem[len(p):]
		dot := strings.IndexByte(after, '.')
		if dot <= 0 {
			return false
		}
		for i := 0; i < dot; i++ {
			if after[i] < '0' || after[i] > '9' {
				return false
			}
		}
		return matchNullableSegments(after[dot+1:], segs[1:])

	case NullableSegmentMap:
		p := seg.TFName + `["`
		if !strings.HasPrefix(rem, p) {
			return false
		}
		// Scan past Go-quoted key string `"..."` followed by `].`
		inStr := rem[len(seg.TFName)+1:] // starts at `"`
		escaped := false
		endQuote := -1
		for i := 1; i < len(inStr); i++ {
			if escaped {
				escaped = false
				continue
			}
			if inStr[i] == '\\' {
				escaped = true
				continue
			}
			if inStr[i] == '"' {
				endQuote = i
				break
			}
		}
		if endQuote == -1 || len(inStr) <= endQuote+2 || inStr[endQuote+1] != ']' || inStr[endQuote+2] != '.' {
			return false
		}
		if _, err := strconv.Unquote(inStr[:endQuote+1]); err != nil {
			return false
		}
		return matchNullableSegments(inStr[endQuote+3:], segs[1:])

	case NullableSegmentAttr:
		return len(segs) == 1 && rem == seg.TFName
	}
	return false
}

func toNullablePresenceBoolMap(raw interface{}) map[string]bool {
	out := make(map[string]bool)
	switch m := raw.(type) {
	case map[string]interface{}:
		for k, v := range m {
			switch bv := v.(type) {
			case bool:
				if bv {
					out[k] = true
				}
			case string:
				if b, err := strconv.ParseBool(bv); err == nil && b {
					out[k] = true
				}
			}
		}
	case map[string]bool:
		for k, v := range m {
			if v {
				out[k] = true
			}
		}
	}
	return out
}

func forceNewPresenceChanged(oldPresence, plannedPresence map[string]bool, specs []NullableFieldSpec) bool {
	var forceNewSpecs []NullableFieldSpec
	for _, s := range specs {
		if s.ForceNew {
			forceNewSpecs = append(forceNewSpecs, s)
		}
	}
	if len(forceNewSpecs) == 0 {
		return false
	}
	matchesAnyForceNew := func(k string) bool {
		for _, s := range forceNewSpecs {
			if MatchNullableFieldKey(k, s) {
				return true
			}
		}
		return false
	}
	for k, v := range oldPresence {
		if v && !plannedPresence[k] && matchesAnyForceNew(k) {
			return true
		}
	}
	for k, v := range plannedPresence {
		if v && !oldPresence[k] && matchesAnyForceNew(k) {
			return true
		}
	}
	return false
}

// SetNullableFieldPresenceDiff returns a CustomizeDiffFunc that populates
// nullable_field_presence from RawConfig so SDKv2 plans standalone null <-> zero
// transitions and triggers ForceNew when immutable Optional* fields change presence.
func SetNullableFieldPresenceDiff(specs []NullableFieldSpec) schema.CustomizeDiffFunc {
	return func(_ context.Context, diff *schema.ResourceDiff, _ interface{}) error {
		oldRaw, _ := diff.GetChange(NullableFieldPresenceAttr)
		oldPresence := toNullablePresenceBoolMap(oldRaw)

		planned, hasUnknown, hasUnknownForceNew := EvaluateNullableConfigPresence(diff.GetRawConfig(), specs)
		if hasUnknown {
			if err := diff.SetNewComputed(NullableFieldPresenceAttr); err != nil {
				return err
			}
			if hasUnknownForceNew && diff.Id() != "" {
				return diff.ForceNew(NullableFieldPresenceAttr)
			}
			return nil
		}

		if err := diff.SetNew(NullableFieldPresenceAttr, planned); err != nil {
			return err
		}

		if diff.Id() != "" && diff.HasChange(NullableFieldPresenceAttr) {
			plannedBool := toNullablePresenceBoolMap(planned)
			if forceNewPresenceChanged(oldPresence, plannedBool, specs) {
				return diff.ForceNew(NullableFieldPresenceAttr)
			}
		}
		return nil
	}
}

// CurrentNullableFieldPresence returns the active presence map for Create,
// Update, or tfplan2cai. It evaluates d.GetRawConfig() directly when available
// (resolving values that were unknown at plan time), falling back to the
// planned nullable_field_presence attribute when RawConfig is absent (e.g. TGC).
func CurrentNullableFieldPresence(d TerraformResourceData, specs []NullableFieldSpec) map[string]bool {
	if d == nil {
		return nil
	}
	rawCfg := d.GetRawConfig()
	if rawCfg != cty.NilVal && !rawCfg.IsNull() && rawCfg.IsKnown() && rawCfg.Type().IsObjectType() {
		planned, hasUnknown, _ := EvaluateNullableConfigPresence(rawCfg, specs)
		if !hasUnknown {
			return toNullablePresenceBoolMap(planned)
		}
	}
	return toNullablePresenceBoolMap(d.Get(NullableFieldPresenceAttr))
}

// IsNullableFieldPresent reports whether the Optional* field at path is
// explicitly set in the current configuration / plan.
func IsNullableFieldPresent(d TerraformResourceData, path string, specs []NullableFieldSpec) bool {
	return CurrentNullableFieldPresence(d, specs)[path]
}

// PriorNullableFieldPresence returns the prior state's nullable_field_presence
// map during Update, or nil on Create.
func PriorNullableFieldPresence(d TerraformResourceData) map[string]bool {
	if d == nil {
		return nil
	}
	if rd, ok := d.(*schema.ResourceData); ok && rd.IsNewResource() {
		return nil
	}
	if dc, ok := d.(TerraformResourceDataChange); ok {
		oldRaw, _ := dc.GetChange(NullableFieldPresenceAttr)
		return toNullablePresenceBoolMap(oldRaw)
	}
	return nil
}

// WasNullableFieldPresent reports whether path was present in prior state.
func WasNullableFieldPresent(d TerraformResourceData, path string) bool {
	return PriorNullableFieldPresence(d)[path]
}

func matchesNullableKeyOrPrefix(key, keyOrPrefix string) bool {
	return key == keyOrPrefix ||
		strings.HasPrefix(key, keyOrPrefix+".") ||
		strings.HasPrefix(key, keyOrPrefix+"[")
}

// WasNullableFieldPresentUnderPrefix reports whether keyOrPrefix (or any nested
// field beneath it) was present in prior state.
func WasNullableFieldPresentUnderPrefix(d TerraformResourceData, keyOrPrefix string) bool {
	for k, v := range PriorNullableFieldPresence(d) {
		if v && matchesNullableKeyOrPrefix(k, keyOrPrefix) {
			return true
		}
	}
	return false
}

// HasNullableFieldPresenceChange reports whether presence changed between prior
// state and current config for keyOrPrefix (or any nested field beneath it).
func HasNullableFieldPresenceChange(d TerraformResourceData, keyOrPrefix string, specs []NullableFieldSpec) bool {
	oldMap := PriorNullableFieldPresence(d)
	newMap := CurrentNullableFieldPresence(d, specs)
	for k, v := range oldMap {
		if v && !newMap[k] && matchesNullableKeyOrPrefix(k, keyOrPrefix) {
			return true
		}
	}
	for k, v := range newMap {
		if v && !oldMap[k] && matchesNullableKeyOrPrefix(k, keyOrPrefix) {
			return true
		}
	}
	return false
}

// ExtractNullableFieldPresenceFromAPIResponse walks a decoded API JSON response
// map and returns the nullable_field_presence map reflecting which Optional*
// fields were returned with non-nil values by the server.
func ExtractNullableFieldPresenceFromAPIResponse(res map[string]interface{}, specs []NullableFieldSpec) map[string]interface{} {
	presence := make(map[string]interface{})
	if len(res) == 0 {
		return presence
	}

	var walk func(curr map[string]interface{}, prefix string, segs []NullablePathSegment)
	walk = func(curr map[string]interface{}, prefix string, segs []NullablePathSegment) {
		if len(segs) == 0 || curr == nil {
			return
		}
		seg := segs[0]
		switch seg.Kind {
		case NullableSegmentFlattenObject:
			sub, ok := curr[seg.APIName].(map[string]interface{})
			if !ok || len(sub) == 0 {
				return
			}
			walk(sub, prefix, segs[1:])

		case NullableSegmentNestedObject:
			sub, ok := curr[seg.APIName].(map[string]interface{})
			if !ok || (!seg.AllowEmptyObject && len(sub) == 0) {
				return
			}
			walk(sub, joinNullablePrefix(prefix, seg.TFName+".0"), segs[1:])

		case NullableSegmentArray:
			slice, ok := curr[seg.APIName].([]interface{})
			if !ok {
				return
			}
			outIdx := 0
			for _, raw := range slice {
				elem, ok := raw.(map[string]interface{})
				if !ok || len(elem) < 1 {
					// Matches flattenPropertyMethod skipping empty JSON objects in arrays.
					continue
				}
				walk(elem, joinNullablePrefix(prefix, fmt.Sprintf("%s.%d", seg.TFName, outIdx)), segs[1:])
				outIdx++
			}

		case NullableSegmentMap:
			m, ok := curr[seg.APIName].(map[string]interface{})
			if !ok {
				return
			}
			for k, raw := range m {
				elem, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				walk(elem, joinNullablePrefix(prefix, FormatNullableMapSegment(seg.TFName, k)), segs[1:])
			}

		case NullableSegmentAttr:
			if val, ok := curr[seg.APIName]; ok && val != nil {
				presence[joinNullablePrefix(prefix, seg.TFName)] = true
			}
		}
	}

	for _, spec := range specs {
		walk(res, "", spec.Segments)
	}
	return presence
}
