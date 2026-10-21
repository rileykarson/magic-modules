package tpgdclresource

// Bool converts a bool to a *bool
func Bool(b bool) *bool {
	return &b
}

// Float64 converts a float64 to *float64
func Float64(f float64) *float64 {
	return &f
}

// Int64 converts an int64 to *int64
func Int64(i int64) *int64 {
	return &i
}

// Int64OrNil converts an int64 to *int64, returning nil if it's empty (0).
func Int64OrNil(i int64) *int64 {
	if i == 0 {
		return nil
	}
	return &i
}

// String converts a string to a *string
func String(s string) *string {
	return &s
}

// StringOrNil converts a string to a *string, returning nil if it's empty ("").
func StringOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
