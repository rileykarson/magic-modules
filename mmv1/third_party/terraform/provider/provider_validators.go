package provider

import (
	providerschema "github.com/hashicorp/terraform-provider-google/google/provider/schema"
)

func ValidateCredentials(v interface{}, k string) (warnings []string, errors []error) {
	return providerschema.ValidateCredentials(v, k)
}

func ValidateCustomEndpoint(v interface{}, k string) (ws []string, errors []error) {
	return providerschema.ValidateCustomEndpoint(v, k)
}

func ValidateEmptyStrings(v interface{}, k string) (warnings []string, errors []error) {
	return providerschema.ValidateEmptyStrings(v, k)
}

func ValidateJWT(v interface{}, k string) (warnings []string, errors []error) {
	return providerschema.ValidateJWT(v, k)
}

func ValidateServiceAccountEmail(v interface{}, k string) (warnings []string, errors []error) {
	return providerschema.ValidateServiceAccountEmail(v, k)
}
