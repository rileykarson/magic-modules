package tpgdclresource

import (
	"encoding/json"
	"fmt"
	"reflect"
	re "regexp"
	"strconv"
	"strings"
)

// GetMapEntry returns the value at `path` from `m`, following the same rules as
// `MoveMapEntry` except that a missing map or value is an error.
func GetMapEntry(m map[string]any, path []string) (any, error) {
	if len(path) == 0 {
		return m, nil
	}
	fetch := m
	// All elements before the last must point to a map[string]interface{} -
	// this ranges over all those elements, so at the end of this loop, we have
	// the map which contains the element to fetch.
	for _, idx := range path[:len(path)-1] {
		f, err := mapEntry(fetch, idx)
		if err != nil {
			return nil, err
		}
		var ok bool
		fetch, ok = f.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("could not fetch %q from %v", idx, fetch)
		}
	}

	value, err := mapEntry(fetch, path[len(path)-1])
	if err != nil {
		return nil, err
	}
	return value, nil
}

// mapEntry grabs item from fetch, and indexes into the array if the [num] notation is present.
func mapEntry(fetch map[string]any, item string) (any, error) {
	// Check if we're fetching from an array.
	arrayRegexp := re.MustCompile(`\[([0-9]*)\]`)
	if arrayRegexp.MatchString(item) {
		field := strings.Split(item, "[")[0]
		items := arrayRegexp.FindAllStringSubmatch(item, 1)
		index, err := strconv.Atoi(items[0][1])
		if err != nil {
			return nil, err
		}

		f, ok := fetch[field]
		if !ok {
			return nil, fmt.Errorf("could not find %q in %v", item, fetch)
		}

		if f == nil {
			return nil, &AttemptToIndexNilArray{FieldName: field}
		}

		fetch, ok := f.([]any)
		if !ok {
			return nil, fmt.Errorf("field %s is a %T, not an array", field, f)
		}

		if len(fetch) < index {
			return nil, fmt.Errorf("field %s only has %v elements, needs %v", field, len(fetch), index)
		}

		return fetch[index], nil
	}

	f, ok := fetch[item]
	if !ok {
		return nil, fmt.Errorf("could not find %q in %v", item, fetch)
	}
	return f, nil
}

// PutMapEntry inserts `item` at `path` into `m` - the inverse of GetMapEntry.
func PutMapEntry(m map[string]any, path []string, item any) error {
	if len(path) == 0 {
		return fmt.Errorf("cannot insert value at empty path")
	}
	put := m
	// All elements before the last must point to a map[string]interface{} -
	// this ranges over all those elements, so at the end of this loop, we have
	// the map which contains the element to fetch.
	for _, idx := range path[:len(path)-1] {
		f, ok := put[idx]
		if !ok {
			f = make(map[string]any)
			put[idx] = f
		}
		put, ok = f.(map[string]any)
		if !ok {
			return fmt.Errorf("could not cast %q from %v as map[string]interface{}", idx, put)
		}
	}
	put[path[len(path)-1]] = item
	return nil
}

// ConvertToMap converts the specified object into the map[string]interface{} which can
// be serialized into the same json object as the input object.
func ConvertToMap(obj any) (map[string]any, error) {
	var m map[string]any
	b, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// ValueOrEmptyString takes a scalar or pointer to a scalar and returns either the empty string or its value.
func ValueOrEmptyString(i any) string {
	if i == nil {
		return ""
	}
	v := reflect.ValueOf(i)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.IsValid() {
		switch v.Kind() {
		case reflect.Bool, reflect.Int, reflect.Int64, reflect.Float64, reflect.String:
			return fmt.Sprintf("%v", v.Interface())
		}
	}
	return ""
}
