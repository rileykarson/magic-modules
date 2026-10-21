package tpgdclresource

import (
	"strconv"

	"github.com/kylelemons/godebug/pretty"
)

// LifecycleParam is used to specify what operations are acceptable.
type LifecycleParam int

const (
	// BlockDestruction blocks deleting the resource.  This will
	// cause some Apply operations to be impossible - some fields
	// cannot be modified and require a destroy/recreate.
	BlockDestruction LifecycleParam = iota
	// BlockAcquire will error if the resource already exists.
	BlockAcquire
	// BlockCreation will error if the resource does not exist.
	BlockCreation
	// BlockModification will error if the resource is not in the desired state.
	BlockModification
)

// HasLifecycleParam returns whether the given slice has the requested param.
func HasLifecycleParam(lps []LifecycleParam, p LifecycleParam) bool {
	for _, lp := range lps {
		if lp == p {
			return true
		}
	}
	return false
}

// SprintResourceCompact prints a struct into a compact single line string.
func SprintResourceCompact(v any) string {
	prettyConfig := &pretty.Config{
		Compact:           true,
		IncludeUnexported: true,
	}
	return prettyConfig.Sprint(v)
}

// SprintResource prints a struct into a multiline string to display to readers.
func SprintResource(v any) string {
	prettyConfig := &pretty.Config{
		Diffable:          true, // add line between braces and first/last val
		IncludeUnexported: true,
	}
	return prettyConfig.Sprint(v)
}

// EmptyValue returns an empty value to exclude PARAMETER-type values from
// being expanded
func EmptyValue() (map[string]any, error) {
	return nil, nil
}

/*
 *
 * Default Flatten functions
 *
 * Flatten functions are expected to return the value stored in the interface{},
 * returning a zero value otherwise. For primitive types and arrays that's a nil,
 * but for types like time.Time it may be an empty object.
 *
 * Flattens need to consider the behaviour of https://developers.google.com/discovery/v1/type-format
 * and of https://golang.org/pkg/encoding/json/#Unmarshal. As well, values may
 * get inserted into the JSON map with their correct types already by decoders.
 */

// FlattenInteger turns an interface pointing to an arbitary type into *in64,
// taking into account that it may have been represented as various types per
// https://developers.google.com/discovery/v1/type-format.
// string, float64, int64, and int64 values will return a *int64.
// nil and unrecognised types will return 0.
func FlattenInteger(v any) *int64 {
	if v == nil {
		return nil
	}
	// int64 values are represented as strings
	if strVal, ok := v.(string); ok {
		if intVal, err := strconv.ParseInt(strVal, 10, 64); err == nil {
			return &intVal
		}
	}

	// number values are represented as float64
	if floatVal, ok := v.(float64); ok {
		intVal := int64(floatVal)
		return &intVal
	}

	// ints won't appear per https://golang.org/pkg/encoding/json/#Unmarshal
	// but may get inserted by decoders in unmarshalResource
	if int32Val, ok := v.(int32); ok {
		intVal := int64(int32Val)
		return &intVal
	}

	if intVal, ok := v.(int64); ok {
		return &intVal
	}

	if machineIntVal, ok := v.(int); ok {
		intVal := int64(machineIntVal)
		return &intVal
	}

	return Int64(0)
}

// FlattenDouble asserts that an interface is a float64 and returns a pointer to it,
// or to 0.0 if the value is invalid.
func FlattenDouble(v any) *float64 {
	if v == nil {
		return nil
	}
	p, ok := v.(float64)
	if !ok {
		return Float64(0.0)
	}

	return &p
}

// FlattenKeyValuePairs asserts that an interface is a map[string]string and
// returns it, or an empty map if the value is invalid.
func FlattenKeyValuePairs(v any) map[string]string {
	if v == nil {
		return nil
	}
	if ss, ok := v.(map[string]string); ok {
		return ss
	}
	p, ok := v.(map[string]any)
	if !ok {
		return map[string]string{}
	}

	return assertStringMap(p)
}

// Returns a map[string]string from a map[string]interface{}
// Non-string values are skipped.
func assertStringMap(mi map[string]any) map[string]string {
	ms := make(map[string]string)
	for k, v := range mi {
		if v == nil {
			continue
		}

		if s, ok := v.(string); ok {
			ms[k] = s
		}
	}
	return ms
}

// FlattenIntSlice asserts that an interface is a []int and returns
// it.
func FlattenIntSlice(v any) []int64 {
	if v == nil {
		return nil
	}
	p, ok := v.([]any)
	if !ok {
		return []int64{}
	}

	return assertIntSlice(p)
}

// Returns a []int64 from an []interface
// Non-int values are skipped.
func assertIntSlice(id []any) []int64 {
	dd := []int64{}
	for _, v := range id {
		if v == nil {
			continue
		}
		if f, ok := v.(float64); ok {
			dd = append(dd, int64(f))
			continue
		}
		if d, ok := v.(int64); ok {
			dd = append(dd, d)
		}
	}

	return dd
}

// FlattenStringSlice asserts that an interface is a []string and returns
// it.
func FlattenStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	p, ok := v.([]any)
	if !ok {
		return []string{}
	}

	return assertStringSlice(p)
}

// Returns a []string from an []interface
// Non-string values are skipped.
func assertStringSlice(is []any) []string {
	ss := []string{}
	for _, v := range is {
		if v == nil {
			continue
		}

		if s, ok := v.(string); ok {
			ss = append(ss, s)
		}
	}

	return ss
}

// FlattenString asserts that an interface is a string and returns a pointer to
// it, or to the empty string if the value is invalid.
func FlattenString(v any) *string {
	if v == nil {
		return nil
	}
	p, ok := v.(string)
	if !ok {
		return String("")
	}

	return &p
}

// FlattenBool asserts that an interface is a bool and returns a pointer to it, or
// a pointer to false if the value is invalid.
func FlattenBool(v any) *bool {
	if v == nil {
		return nil
	}
	p, ok := v.(bool)
	if !ok {
		return Bool(false)
	}

	return &p
}

// FlattenSecretValue behaves the same way as FlattenString, except that it
// returns nil if the value is not present.
func FlattenSecretValue(v any) *string {
	p, ok := v.(string)
	if !ok {
		return nil
	}

	return &p

}
