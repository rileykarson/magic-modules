package resource_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/GoogleCloudPlatform/magic-modules/mmv1/api/resource"
)

func TestSample_TestDependencies(t *testing.T) {
	cases := []struct {
		name                     string
		sample                   resource.Sample
		resourcePrefixServiceMap map[string]string
		want                     map[string]string
	}{
		{
			name: "empty",
			sample: resource.Sample{
				Steps: []*resource.Step{
					{
						BootstrapResources: map[string]string{},
						TestHCLText:        "",
					},
				},
			},
			resourcePrefixServiceMap: map[string]string{},
			want:                     map[string]string{},
		},
		{
			name: "bootstrap iam",
			sample: resource.Sample{
				BootstrapIam: []resource.IamMember{
					{
						Member: "whatever",
						Role:   "role",
					},
				},
			},
			resourcePrefixServiceMap: map[string]string{},
			want: map[string]string{
				"services/resourcemanager": "",
			},
		},
		{
			name: "no conflict",
			sample: resource.Sample{
				Steps: []*resource.Step{
					{
						BootstrapResources: map[string]string{
							"network": "compute.BootstrapSubnet",
						},
						TestHCLText: "",
					},
					{
						BootstrapResources: map[string]string{
							"network": "compute.BootstrapSubnet",
						},
						TestHCLText: "",
					},
				},
			},
			resourcePrefixServiceMap: map[string]string{},
			want: map[string]string{
				"services/compute": "",
			},
		},
		{
			name: "underscore vs empty string conflict",
			sample: resource.Sample{
				Steps: []*resource.Step{
					{
						BootstrapResources: map[string]string{
							"network": "compute.BootstrapSubnet",
						},
						TestHCLText: "",
					},
					{
						BootstrapResources: map[string]string{},
						TestHCLText:        `resource "google_compute_instance"`,
					},
				},
			},
			resourcePrefixServiceMap: map[string]string{
				"google_compute_": "services/compute",
			},
			want: map[string]string{
				"services/compute": "",
			},
		},
		{
			name: "merge boostrapped iam",
			sample: resource.Sample{
				BootstrapIam: []resource.IamMember{
					{
						Member: "whatever",
						Role:   "role",
					},
				},
				Steps: []*resource.Step{
					{
						BootstrapResources: map[string]string{},
						TestHCLText:        `resource "google_project"`,
					},
				},
			},
			resourcePrefixServiceMap: map[string]string{
				"google_project": "services/resourcemanager",
			},
			want: map[string]string{
				"services/resourcemanager": "",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.sample.TestDependencies(tc.resourcePrefixServiceMap)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("TestDependencies() mismatch (-want +got:\n%s", diff)
			}
		})
	}
}

func TestSample_Validate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		sample  resource.Sample
		wantErr bool
	}{
		{
			name: "valid oics_vars_overrides when exclude_test is false",
			sample: resource.Sample{
				Name:        "basic",
				ExcludeTest: false,
				Steps: []*resource.Step{
					{
						Name: "basic",
						OicsVarsOverrides: map[string]string{
							"deletion_protection": "false",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "oics_vars_overrides disallowed when exclude_test is true",
			sample: resource.Sample{
				Name:        "basic",
				ExcludeTest: true,
				Steps: []*resource.Step{
					{
						Name: "basic",
						OicsVarsOverrides: map[string]string{
							"deletion_protection": "false",
						},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			errs := tc.sample.Validate("TestResource")
			if tc.wantErr && len(errs) == 0 {
				t.Errorf("Validate() expected error, got nil")
			}
			if !tc.wantErr && len(errs) > 0 {
				t.Errorf("Validate() unexpected error: %v", errs)
			}
		})
	}
}
