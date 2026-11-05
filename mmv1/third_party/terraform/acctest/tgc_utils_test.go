package acctest

import (
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestGetServiceDomain(t *testing.T) {
	tests := []struct {
		name string
		cai  string
		want string
	}{
		{
			name: "valid cai",
			cai:  "//container.googleapis.com/projects/foo",
			want: "container.googleapis.com",
		},
		{
			name: "valid cai with deeper path",
			cai:  "//storage.googleapis.com/projects/foo/buckets/bar",
			want: "storage.googleapis.com",
		},
		{
			name: "invalid cai - too short",
			cai:  "foo",
			want: "",
		},
		{
			name: "empty",
			cai:  "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getServiceDomain(tt.cai); got != tt.want {
				t.Errorf("getServiceDomain() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveContainerCaiAssetName(t *testing.T) {
	formats := []string{
		"//container.googleapis.com/projects/{{project}}/zones/{{location}}/clusters/{{cluster}}",
	}

	tests := []struct {
		name     string
		rName    string
		location string
		want     []string
	}{
		{
			name:     "zonal location replacement",
			rName:    "projects/p/locations/us-central1-a/clusters/c",
			location: "us-central1-a",
			want: []string{
				"//container.googleapis.com/projects/p/zones/us-central1-a/clusters/c",
			},
		},
		{
			name:     "regional location no change",
			rName:    "projects/p/locations/us-central1/clusters/c",
			location: "us-central1",
			want: []string{
				"//container.googleapis.com/projects/p/locations/us-central1/clusters/c",
			},
		},
		{
			name:     "unknown location type defaults to no change",
			rName:    "projects/p/locations/global/clusters/c",
			location: "global",
			want: []string{
				"//container.googleapis.com/projects/p/locations/global/clusters/c",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveContainerCaiAssetName(formats, tt.rName, tt.location)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("resolveContainerCaiAssetName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetermineImportMetadata(t *testing.T) {
	idFunc := func(*terraform.State) (string, error) { return "dyn", nil }

	tests := []struct {
		name             string
		steps            []resource.TestStep
		currentStepIndex int
		resourceName     string
		want             ImportMetadata
	}{
		{
			name: "adjacent import step",
			steps: []resource.TestStep{
				{Config: "resource \"google_foo\" \"bar\" {}"},
				{
					ResourceName:            "google_foo.bar",
					ImportState:             true,
					ImportStateVerify:       true,
					ImportStateVerifyIgnore: []string{"secret"},
				},
			},
			currentStepIndex: 0,
			resourceName:     "google_foo.bar",
			want: ImportMetadata{
				Id:            "<AUTO_ID>",
				IgnoredFields: []string{"secret"},
			},
		},
		{
			name: "intervening RefreshState step before import",
			steps: []resource.TestStep{
				{Config: "resource \"google_foo\" \"bar\" {}"},
				{RefreshState: true},
				{
					ResourceName:            "google_foo.bar",
					ImportState:             true,
					ImportStateVerify:       true,
					ImportStateId:           "projects/p/foos/bar",
					ImportStateVerifyIgnore: []string{"etag"},
				},
			},
			currentStepIndex: 0,
			resourceName:     "google_foo.bar",
			want: ImportMetadata{
				Id:            "projects/p/foos/bar",
				IgnoredFields: []string{"etag"},
			},
		},
		{
			name: "multiple consecutive imports after RefreshState",
			steps: []resource.TestStep{
				{Config: "resource \"google_foo\" \"bar\" {}\nresource \"google_foo\" \"baz\" {}"},
				{RefreshState: true},
				{
					ResourceName:      "google_foo.bar",
					ImportState:       true,
					ImportStateVerify: true,
				},
				{
					ResourceName:      "google_foo.baz",
					ImportState:       true,
					ImportStateVerify: true,
					ImportStateIdFunc: idFunc,
				},
			},
			currentStepIndex: 0,
			resourceName:     "google_foo.baz",
			want: ImportMetadata{
				Id: "<DYNAMIC_IMPORT_ID>",
			},
		},
		{
			name: "stops at next Config step without crossing step boundary",
			steps: []resource.TestStep{
				{Config: "resource \"google_foo\" \"bar\" {}"},
				{RefreshState: true},
				{Config: "resource \"google_foo\" \"bar\" {}"},
				{
					ResourceName:      "google_foo.bar",
					ImportState:       true,
					ImportStateVerify: true,
				},
			},
			currentStepIndex: 0,
			resourceName:     "google_foo.bar",
			want:             ImportMetadata{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := determineImportMetadata(tt.steps, tt.currentStepIndex, tt.resourceName)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("determineImportMetadata() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
