package tpgdclresource

import (
	"strings"
)

// SelfLinkToName returns the element of a string after the last slash.
func SelfLinkToName(v *string) *string {
	if v == nil {
		return nil
	}
	val := *v
	comp := strings.Split(val, "/")
	ret := comp[len(comp)-1]
	return &ret
}
