package tpgdclresource

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
)

func ExpandStringArray(v interface{}) []string {
	arr, ok := v.([]string)

	if ok {
		return arr
	}

	if arr, ok := v.(*schema.Set); ok {
		return tpgresource.ConvertStringSet(arr)
	}

	arr = tpgresource.ConvertStringArr(v.([]interface{}))
	if arr == nil {
		// Send empty array specifically instead of nil
		return make([]string, 0)
	}
	return arr
}

func ExpandIntegerArray(v interface{}) []int64 {
	arr, ok := v.([]int64)

	if ok {
		return arr
	}

	if arr, ok := v.(*schema.Set); ok {
		return convertIntegerSet(arr)
	}

	return convertIntegerArr(v.([]interface{}))
}

func convertIntegerSet(v *schema.Set) []int64 {
	return convertIntegerArr(v.List())
}

func convertIntegerArr(v []interface{}) []int64 {
	var vi []int64
	for _, vs := range v {
		vi = append(vi, int64(vs.(int)))
	}
	return vi
}
