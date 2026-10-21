package tpgdclresource

import (
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"time"
)

var selfLinkIgnorableComponents = map[string]struct{}{
	"projects":      {},
	"regions":       {},
	"locations":     {},
	"zones":         {},
	"organizations": {},
	"compute":       {},
	"v1":            {},
	"v1beta1":       {},
	"beta":          {},
}

func hasKey(m map[string]struct{}, k string) bool {
	_, ok := m[k]
	return ok
}

// SelfLinkToSelfLink returns true if left and right are equivalent for selflinks.
// That means that they are piecewise equal, comparing components, allowing for
// certain elements to be dropped ("projects", "regions", etc.).  It also allows
// any value to be present in the second-to-last field (where "instances" or
// "databases", etc, would be.)
func SelfLinkToSelfLink(l, r *string) bool {
	if l == nil && r == nil {
		return true
	}
	if l == nil || r == nil {
		return false
	}
	left := *l
	right := *r

	if lurl, err := url.Parse(left); err == nil {
		left = lurl.EscapedPath()
	}
	if rurl, err := url.Parse(right); err == nil {
		right = rurl.EscapedPath()
	}
	if strings.HasPrefix(left, "/") {
		left = left[1:len(left)]
	}
	if strings.HasPrefix(right, "/") {
		right = right[1:len(right)]
	}
	if strings.HasSuffix(left, right) || strings.HasSuffix(right, left) {
		return true
	}
	lcomp := strings.Split(left, "/")
	rcomp := strings.Split(right, "/")
	li := 0
	ri := 0
	for li < len(lcomp) && ri < len(rcomp) {
		switch {
		case lcomp[li] == rcomp[ri]:
			li++
			ri++
		case hasKey(selfLinkIgnorableComponents, lcomp[li]):
			li++
		case hasKey(selfLinkIgnorableComponents, rcomp[ri]):
			ri++
		// The second-to-last element in a long-form self-link contains the
		// name of the resource.  The name of the resource might be anything,
		// rather than keep a list of all resources, we will just ignore
		// the second-to-last field if one argument is exactly one remaining
		// field longer than the other.
		case len(lcomp) == li+2 && len(rcomp) == ri+1:
			li++
		case len(rcomp) == ri+2 && len(lcomp) == li+1:
			ri++
		default:
			return false
		}
	}
	return true
}

// StringCanonicalize checks canonicalization for strings. It matches self-links using NameToSelfLink.
func StringCanonicalize(l, r *string) bool {
	if l == nil && r == nil {
		return true
	}
	if l == nil || r == nil {
		return false
	}
	left := *l
	right := *r

	if left == right {
		return true
	}

	if IsPartialSelfLink(left) || IsPartialSelfLink(right) || IsSelfLink(left) || IsSelfLink(right) {
		return NameToSelfLink(l, r)
	}

	return false
}

// StringArrayCanonicalize checks canonicalization for arrays of strings. It matches self-links using NameToSelfLink.
func StringArrayCanonicalize(l, r []string) bool {
	if len(l) != len(r) {
		return false
	}
	for i := range l {
		if !StringCanonicalize(&l[i], &r[i]) {
			return false
		}
	}
	return true
}

// BoolCanonicalize checks canonicalization for booleans.
func BoolCanonicalize(l, r *bool) bool {
	if l == nil && r == nil {
		return true
	}
	if l != nil && r == nil {
		left := *l
		return left == false
	}

	if r != nil && l == nil {
		right := *r
		return right == false
	}

	left := *l
	right := *r

	return left == right
}

// NameToSelfLink returns true if left and right are equivalent for Names / SelfLinks.
// It allows all the deviations that SelfLinkToSelfLink allows, plus it allows one
// of the values to simply be the last element of the other value.
func NameToSelfLink(l, r *string) bool {
	if l == nil && r == nil {
		return true
	}
	if l == nil || r == nil {
		return false
	}
	left := *l
	right := *r

	if left == right {
		return true
	}
	lcomp := strings.Split(left, "/")
	rcomp := strings.Split(right, "/")
	if len(lcomp) > 1 && len(rcomp) > 1 {
		return SelfLinkToSelfLink(&left, &right)
	}
	if len(lcomp) > 1 && lcomp[len(lcomp)-1] == right {
		return true
	}
	if len(rcomp) > 1 && rcomp[len(rcomp)-1] == left {
		return true
	}
	return false
}

// PartialSelfLinkToSelfLink returns true if left and right are equivalent for SelfLinks and partial
// SelfLinks.  It allows all the deviations that SelfLink allows, except that it works
// backwards, and returns true when one or the other is empty - in that sense, it allows whatever
// specification, starting from the most-specific
func PartialSelfLinkToSelfLink(l, r *string) bool {
	if l == nil && r == nil {
		return true
	}
	if l == nil || r == nil {
		return false
	}
	left := *l
	right := *r

	if left == "" && right == "" {
		return true
	}
	if left == "" || right == "" {
		return false
	}
	if NameToSelfLink(&left, &right) {
		return true
	}
	lcomp := strings.Split(left, "/")
	rcomp := strings.Split(right, "/")
	li := len(lcomp) - 1
	ri := len(rcomp) - 1
	for li >= 0 && ri >= 0 {
		switch {
		case lcomp[li] == rcomp[ri]:
			li--
			ri--
		case hasKey(selfLinkIgnorableComponents, lcomp[li]):
			li--
		case hasKey(selfLinkIgnorableComponents, rcomp[ri]):
			ri--
		// As in SelfLinkToSelfLink, we permit any value in the second-to-last field
		// for the value which is longer.
		case len(lcomp) == li+2 && len(rcomp) == ri+2 && li > ri:
			li--
		case len(lcomp) == li+2 && len(rcomp) == ri+2 && ri > li:
			ri--
		default:
			return false
		}

	}
	return true
}

// IsZeroValue returns true if the argument is considered empty/unset.
func IsZeroValue(v any) bool {
	if t, ok := v.(time.Time); ok {
		return t.IsZero()
	}
	val := reflect.ValueOf(v)
	return !val.IsValid() || !reflect.Indirect(val).IsValid() || ((val.Kind() == reflect.Interface ||
		val.Kind() == reflect.Chan ||
		val.Kind() == reflect.Func ||
		val.Kind() == reflect.Ptr ||
		val.Kind() == reflect.Map ||
		val.Kind() == reflect.Slice) && val.IsNil())
}

// isIgnored returns true if this prefix should be ignored.
func isIgnored(v string, ignoredPrefixes []string) bool {
	for _, p := range ignoredPrefixes {
		if strings.Contains(v, p) {
			return true
		}
	}
	return false
}

// DeriveField deals with the outgoing portion of derived fields.  The derived fields'
// inputs might be in any form - for instance, a derived name field might be set to
// project/region/name, projects/project/regions/region/objects/name, or just name.
// This function returns the best reasonable guess at the user's intent.  If the current
// value (cVal) matches any of those, it will return the current value.  If it doesn't,
// it will be ignored (even if nil).
func DeriveField(pattern string, cVal *string, fs ...*string) (*string, error) {
	var currentValue string
	// interface{} for fmt.Sprintf.
	fields := make([]any, len(fs))
	if cVal == nil {
		// might still be doable from "fields"!
		currentValue = ""
	} else {
		currentValue = *cVal
	}
	for i, f := range fs {
		if IsEmptyValueIndirect(f) {
			if currentValue == "" {
				// This field may not be required, so we shouldn't error out.
				// Erroring out would cause the DCL to stop if this field isn't set (which it might not be!)
				return nil, nil
			}
			// might still be doable from currentValue
			fields[i] = ""
		} else {
			fields[i] = *f
		}
	}

	patternParts := strings.Split(pattern, "/")
	valueParts := strings.Split(currentValue, "/")

	// currentValue may be a full self-link, so we need to filter out unnecessary beginning parts.
	if len(valueParts) > len(patternParts) {
		for index, valuePart := range valueParts {
			if valuePart == patternParts[0] {
				valueParts = valueParts[index:len(valueParts)]
				break
			}
		}
	}

	if len(patternParts) == len(valueParts) {
		// check if the current value fits the pattern.
		match := true
		for i := range patternParts {
			if patternParts[i] != "%s" && valueParts[i] != patternParts[i] {
				match = false
				break
			}
		}
		if match {
			return &currentValue, nil
		}
	}
	if len(valueParts) == strings.Count(pattern, "%s") {
		iParts := make([]any, len(valueParts))
		for i, s := range valueParts {
			iParts[i] = s
		}
		value := fmt.Sprintf(pattern, iParts...)
		return &value, nil
	}
	value := fmt.Sprintf(pattern, fields...)
	return &value, nil
}

// IsEmptyValueIndirect returns true if the value provided is "empty", according
// to the golang rules.  This corresponds to whether the value should be sent by the
// client if the existing value is nil - it is useful for diffing a response against a provided
// value.  The "Indirect" refers to the fact that this method returns correct
// results even if the provided value is a pointer.
func IsEmptyValueIndirect(i any) bool {
	if i == nil {
		return true
	}

	rt := reflect.TypeOf(i)
	switch rt.Kind() {
	case reflect.Slice:
		return reflect.ValueOf(i).Len() == 0
	case reflect.Array:
		return rt.Len() == 0
	case reflect.Map:
		return len(reflect.ValueOf(i).MapKeys()) == 0
	}

	iv := reflect.Indirect(reflect.ValueOf(i))

	// All non-nil bool values are not empty.
	if iv.Kind() == reflect.Bool {
		return false
	}

	if !iv.IsValid() {
		return true
	}
	if iv.Kind() == reflect.Struct && iv.NumField() == 1 && iv.FieldByName("empty").IsValid() {
		return iv.FieldByName("empty").Bool()
	}
	if iv.IsZero() {
		return true
	}
	if hasEmptyStructField(i) {
		return true
	}
	return false
}

// hasEmptyStructField returns true if the provided value is a struct
// with an unexported field called 'empty', and that value is a boolean,
// and that boolean is true.  This is useful when a user needs to explicitly
// set their intention that a value be empty.
func hasEmptyStructField(i any) bool {
	iv := reflect.Indirect(reflect.ValueOf(i))
	if !iv.IsValid() {
		return false
	}
	if iv.Kind() == reflect.Struct {
		if iv.FieldByName("empty").IsValid() && iv.FieldByName("empty").Bool() {
			return true
		}
	}
	return false
}

// DeriveFromPattern attempts to achieve the same end goal as DeriveField
// but by using regular expressions rather than assumptions about the
// format of the inputs based on the number of `/`. This is important for fields that allow `/`
// characters in their names.
func DeriveFromPattern(pattern string, cVal *string, fs ...*string) (*string, error) {
	var currentValue string
	if cVal == nil {
		// might still be doable from "fields"!
		currentValue = ""
	} else {
		currentValue = *cVal
	}

	if !strings.HasSuffix(pattern, "%s") {
		// If the pattern does not end with %s we cannot assume anything past the last expected
		// `/` character is part of a name
		return nil, fmt.Errorf("pattern did not end with %%s, it does not work with the current implementation %v", pattern)
	}
	// Build regexp from pattern
	regex, err := regexFromPattern(pattern)
	if err != nil {
		return nil, err
	}

	if matches := regex.FindStringSubmatch(currentValue); len(matches) > 0 {
		// Found a match to the pattern, use the capture groups to populate the pattern
		s := make([]any, len(matches))
		for i, v := range matches {
			s[i] = v
		}
		value := fmt.Sprintf(pattern, s[1:]...)
		return &value, nil
	}

	// Did not find a match to the pattern, use the fields to populate the pattern
	fields := make([]any, len(fs))

	for i, f := range fs {
		if f == nil {
			// This field may not be required, so we shouldn't error out.
			// Erroring out would cause the DCL to stop if this field isn't set (which it might not be!)
			return nil, nil
		}
		fields[i] = *f
	}
	value := fmt.Sprintf(pattern, fields...)
	return &value, nil
}

func regexFromPattern(pattern string) (*regexp.Regexp, error) {
	// Replace string formatting with capture groups except for the last one
	// the last one will capture all trailing values
	re := strings.Replace(pattern, "%s", "([^/]+)", strings.Count(pattern, "%s")-1)
	// Wildcard capture at the end, allows for the last value to include `/` characters
	re = strings.ReplaceAll(re, "%s", "(.+)")
	return regexp.Compile(re)
}

// StringEqualsWithSelfLink returns true if these two strings are equal.
// If these functions are self links, they'll do self-link comparisons.
func StringEqualsWithSelfLink(l, r *string) bool {
	if l == nil && r == nil {
		return true
	}

	if l == nil || r == nil {
		return false
	}

	left := *l
	right := *r

	if IsSelfLink(left) || IsSelfLink(right) || IsPartialSelfLink(left) || IsPartialSelfLink(right) {
		lp := strings.Split(left, "/")
		rp := strings.Split(right, "/")
		return lp[len(lp)-1] == rp[len(rp)-1]
	} else {
		return left == right
	}
}

// IsPartialSelfLink returns true if this string represents a partial self link.
func IsPartialSelfLink(s string) bool {
	return strings.HasPrefix(s, "projects/") || strings.HasPrefix(s, "organizations/") || strings.HasPrefix(s, "folders/") || strings.HasPrefix(s, "billingAccounts/") || strings.HasPrefix(s, "tagKeys/") || strings.HasPrefix(s, "tagValues/") || strings.HasPrefix(s, "groups/")
}

// IsSelfLink returns true if this string represents a full self link.
func IsSelfLink(s string) bool {
	r := regexp.MustCompile(`(https:\/\/)?(www\.)?([a-z]*)?googleapis\.com\/`)
	return r.MatchString(s)
}

// ValueShouldBeSent returns if a value should be sent as part of the JSON request.
func ValueShouldBeSent(v any) bool {
	if v == nil {
		return false
	}

	iv := reflect.Indirect(reflect.ValueOf(v))

	// All booleans should be sent.
	if iv.Kind() == reflect.Bool {
		return true
	}

	if !iv.IsValid() || iv.IsZero() {
		return false
	}

	return !IsEmptyValueIndirect(v)
}
