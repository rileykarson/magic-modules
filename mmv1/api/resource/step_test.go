package resource_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/GoogleCloudPlatform/magic-modules/mmv1/api/resource"
)

func TestStep_TestDependencies(t *testing.T) {
	cases := []struct {
		name                     string
		step                     resource.Step
		resourcePrefixServiceMap map[string]string
		want                     map[string]string
	}{
		{
			name: "empty",
			step: resource.Step{
				BootstrapResources: map[string]string{},
				TestHCLText:        "",
			},
			resourcePrefixServiceMap: map[string]string{},
			want:                     map[string]string{},
		},
		{
			name: "compute bootstrapped",
			step: resource.Step{
				BootstrapResources: map[string]string{
					"network": "compute.BootstrapSubnet",
				},
				TestHCLText: "",
			},
			resourcePrefixServiceMap: map[string]string{},
			want: map[string]string{
				"services/compute": "",
			},
		},
		{
			name: "container bootstrapped",
			step: resource.Step{
				BootstrapResources: map[string]string{
					"cluster": `container.BootstrapSharedGKECluster(t, "gkebackup")`,
				},
				TestHCLText: "",
			},
			resourcePrefixServiceMap: map[string]string{},
			want: map[string]string{
				"services/container": "",
			},
		},
		{
			name: "kms bootstrapped",
			step: resource.Step{
				BootstrapResources: map[string]string{
					"kms_key": "kms.BootstrapKMSKey",
				},
				TestHCLText: "",
			},
			resourcePrefixServiceMap: map[string]string{},
			want: map[string]string{
				"services/kms": "",
			},
		},
		{
			name: "servicenetworking bootstrapped",
			step: resource.Step{
				BootstrapResources: map[string]string{
					"network_name": "servicenetworking.BootstrapSharedServiceNetworkingConnection",
				},
				TestHCLText: "",
			},
			resourcePrefixServiceMap: map[string]string{},
			want: map[string]string{
				"services/servicenetworking": "",
			},
		},
		{
			name: "hcl without prefixes",
			step: resource.Step{
				BootstrapResources: map[string]string{},
				TestHCLText: `
resource "google_compute_instance" "foobar" {
}
resource "google_kms_crypto_key" "foobar" {
}`,
			},
			resourcePrefixServiceMap: map[string]string{},
			want:                     map[string]string{},
		},
		{
			name: "prefixes without hcl",
			step: resource.Step{
				BootstrapResources: map[string]string{},
				TestHCLText:        "",
			},
			resourcePrefixServiceMap: map[string]string{
				"google_compute_": "services/compute",
				"google_kms_":     "services/kms",
			},
			want: map[string]string{},
		},
		{
			name: "hcl with prefixes",
			step: resource.Step{
				BootstrapResources: map[string]string{},
				TestHCLText: `
resource "google_compute_instance" "foobar" {
}
resource "google_kms_crypto_key" "foobar" {
}`,
			},
			resourcePrefixServiceMap: map[string]string{
				"google_compute_": "services/compute",
				"google_kms_":     "services/kms",
			},
			want: map[string]string{
				"services/compute": "_",
				"services/kms":     "_",
			},
		},
		{
			name: "hcl with prefixes plus bootstrapped",
			step: resource.Step{
				BootstrapResources: map[string]string{
					"network": "compute.BootstrapSubnet",
					"kms_key": "kms.BootstrapKMSKey",
				},
				TestHCLText: `
resource "google_compute_instance" "foobar" {
}
resource "google_kms_crypto_key" "foobar" {
}`,
			},
			resourcePrefixServiceMap: map[string]string{
				"google_compute_": "services/compute",
				"google_kms_":     "services/kms",
			},
			want: map[string]string{
				"services/compute": "",
				"services/kms":     "",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.step.TestDependencies(tc.resourcePrefixServiceMap)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("TestDependencies() mismatch (-want +got:\n%s", diff)
			}
		})
	}
}

func TestStep_Validate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		step    resource.Step
		wantErr bool
	}{
		{
			name: "valid with bootstrap_resources and test_vars_overrides",
			step: resource.Step{
				Name: "basic",
				BootstrapResources: map[string]string{
					"network": `compute.BootstrapSharedTestNetwork(t, "test")`,
				},
				TestVarsOverrides: map[string]string{
					"deletion_protection": "false",
				},
			},
			wantErr: false,
		},
		{
			name: "duplicate key in bootstrap_resources and test_vars_overrides",
			step: resource.Step{
				Name: "basic",
				BootstrapResources: map[string]string{
					"network": `compute.BootstrapSharedTestNetwork(t, "test")`,
				},
				TestVarsOverrides: map[string]string{
					"network": `"other"`,
				},
			},
			wantErr: true,
		},
		{
			name: "bootstrap function in test_vars_overrides",
			step: resource.Step{
				Name: "basic",
				TestVarsOverrides: map[string]string{
					"network": `compute.BootstrapSharedTestNetwork(t, "test")`,
				},
			},
			wantErr: true,
		},
		{
			name: "non-bootstrap value in bootstrap_resources",
			step: resource.Step{
				Name: "basic",
				BootstrapResources: map[string]string{
					"deletion_protection": "false",
				},
			},
			wantErr: true,
		},
		{
			name: "oics_vars_overrides with test_env_vars",
			step: resource.Step{
				Name: "basic",
				TestEnvVars: map[string]string{
					"project": "PROJECT_NAME",
				},
				OicsVarsOverrides: map[string]string{
					"deletion_protection": "false",
				},
			},
			wantErr: true,
		},
		{
			name: "oics_vars_overrides with bootstrap_resources",
			step: resource.Step{
				Name: "basic",
				BootstrapResources: map[string]string{
					"network": `compute.BootstrapSharedTestNetwork(t, "test")`,
				},
				OicsVarsOverrides: map[string]string{
					"deletion_protection": "false",
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			errs := tc.step.Validate("TestResource", "test_sample")
			if tc.wantErr && len(errs) == 0 {
				t.Errorf("Validate() expected error, got nil")
			}
			if !tc.wantErr && len(errs) > 0 {
				t.Errorf("Validate() unexpected error: %v", errs)
			}
		})
	}
}
