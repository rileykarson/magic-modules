package dataplex_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-provider-google/google/acctest"
	"github.com/hashicorp/terraform-provider-google/google/envvar"
	_ "github.com/hashicorp/terraform-provider-google/google/services/bigquery"
	dataplex "github.com/hashicorp/terraform-provider-google/google/services/dataplex"
)

func TestFilterEntryLinkAspects(t *testing.T) {
	testCases := []struct {
		name            string
		aspectKeySet    map[string]struct{}
		resInput        map[string]interface{}
		expectedAspects map[string]interface{}
		expectError     bool
		errorMsg        string
	}{
		{"aspects key is absent", map[string]struct{}{"keep": {}}, map[string]interface{}{"otherKey": "value"}, nil, false, ""},
		{"aspects value is nil", map[string]struct{}{"keep": {}}, map[string]interface{}{"aspects": nil}, nil, false, ""},
		{"empty aspectKeySet", map[string]struct{}{}, map[string]interface{}{"aspects": map[string]interface{}{"one": map[string]interface{}{"data": 1}, "two": map[string]interface{}{"data": 2}}}, map[string]interface{}{}, false, ""},
		{"keep all aspects", map[string]struct{}{"one": {}, "two": {}}, map[string]interface{}{"aspects": map[string]interface{}{"one": map[string]interface{}{"data": 1}, "two": map[string]interface{}{"data": 2}}}, map[string]interface{}{"one": map[string]interface{}{"data": 1}, "two": map[string]interface{}{"data": 2}}, false, ""},
		{"keep some aspects", map[string]struct{}{"two": {}, "three_not_present": {}}, map[string]interface{}{"aspects": map[string]interface{}{"one": map[string]interface{}{"data": 1}, "two": map[string]interface{}{"data": 2}}}, map[string]interface{}{"two": map[string]interface{}{"data": 2}}, false, ""},
		{"input aspects map is empty", map[string]struct{}{"keep": {}}, map[string]interface{}{"aspects": map[string]interface{}{}}, map[string]interface{}{}, false, ""},
		{"aspects is wrong type", map[string]struct{}{"keep": {}}, map[string]interface{}{"aspects": "not a map"}, nil, true, "FilterEntryLinkAspects: 'aspects' field is not a map[string]interface{}, got string"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resCopy := deepCopyMap(tc.resInput)
			originalAspectsBeforeCall := deepCopyValue(resCopy["aspects"])

			err := dataplex.FilterEntryLinkAspects(tc.aspectKeySet, resCopy)

			if tc.expectError {
				if err == nil {
					t.Fatalf("%s: Expected an error, but got nil", tc.name)
				}
				if tc.errorMsg != "" && !strings.Contains(err.Error(), tc.errorMsg) {
					t.Errorf("%s: Expected error message containing %q, got %q", tc.name, tc.errorMsg, err.Error())
				}
				if !reflect.DeepEqual(resCopy["aspects"], originalAspectsBeforeCall) {
					t.Errorf("%s: resCopy['aspects'] was modified during error case.\nBefore: %#v\nAfter: %#v", tc.name, originalAspectsBeforeCall, resCopy["aspects"])
				}
				return
			}

			if err != nil {
				t.Fatalf("%s: Did not expect an error, but got: %v", tc.name, err)
			}

			actualAspectsRaw, aspectsKeyExists := resCopy["aspects"]

			if tc.expectedAspects == nil {
				if aspectsKeyExists && actualAspectsRaw != nil {
					if tc.name == "aspects key is absent" {
						if aspectsKeyExists {
							t.Errorf("%s: Expected 'aspects' key to be absent, but it exists with value: %v", tc.name, actualAspectsRaw)
						}
					} else {
						t.Errorf("%s: Expected 'aspects' value to be nil, but got: %v", tc.name, actualAspectsRaw)
					}
				}
				return
			}

			if !aspectsKeyExists {
				t.Fatalf("%s: Expected 'aspects' key to exist, but it was absent. Expected value: %#v", tc.name, tc.expectedAspects)
			}

			actualAspects, ok := actualAspectsRaw.(map[string]interface{})
			if !ok {
				t.Fatalf("%s: Expected 'aspects' to be a map[string]interface{}, but got %T. Value: %#v", tc.name, actualAspectsRaw, actualAspectsRaw)
			}

			if !reflect.DeepEqual(actualAspects, tc.expectedAspects) {
				t.Errorf("%s: FilterEntryLinkAspects() result mismatch:\ngot:  %#v\nwant: %#v", tc.name, actualAspects, tc.expectedAspects)
			}
		})
	}
}

func TestAddEntryLinkAspectsToSet(t *testing.T) {
	testCases := []struct {
		name         string
		initialSet   map[string]struct{}
		aspectsInput interface{}
		expectedSet  map[string]struct{}
		expectError  bool
		errorMsg     string
	}{
		{"add to empty set", map[string]struct{}{}, []interface{}{map[string]interface{}{"aspect_key": "key1"}, map[string]interface{}{"aspect_key": "key2"}}, map[string]struct{}{"key1": {}, "key2": {}}, false, ""},
		{"add to existing set", map[string]struct{}{"existing": {}}, []interface{}{map[string]interface{}{"aspect_key": "key1"}}, map[string]struct{}{"existing": {}, "key1": {}}, false, ""},
		{"add duplicate keys", map[string]struct{}{}, []interface{}{map[string]interface{}{"aspect_key": "key1"}, map[string]interface{}{"aspect_key": "key1"}, map[string]interface{}{"aspect_key": "key2"}}, map[string]struct{}{"key1": {}, "key2": {}}, false, ""},
		{"input aspects is empty slice", map[string]struct{}{"existing": {}}, []interface{}{}, map[string]struct{}{"existing": {}}, false, ""},
		{"input aspects is nil", map[string]struct{}{"original": {}}, nil, map[string]struct{}{"original": {}}, false, ""},
		{"input aspects is wrong type", map[string]struct{}{}, "not a slice", map[string]struct{}{}, true, "AddEntryLinkAspectsToSet: input 'aspects' is not a []interface{}, got string"},
		{"item in slice is not a map", map[string]struct{}{}, []interface{}{"not a map"}, map[string]struct{}{}, true, "AddEntryLinkAspectsToSet: item at index 0 is not a map[string]interface{}, got string"},
		{"item map missing aspect_key", map[string]struct{}{}, []interface{}{map[string]interface{}{"wrong_key": "key1"}}, map[string]struct{}{}, true, "AddEntryLinkAspectsToSet: 'aspect_key' not found in aspect item at index 0"},
		{"aspect_key is not a string", map[string]struct{}{}, []interface{}{map[string]interface{}{"aspect_key": 123}}, map[string]struct{}{}, true, "AddEntryLinkAspectsToSet: 'aspect_key' in item at index 0 is not a string, got int"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentSet := make(map[string]struct{})
			for k, v := range tc.initialSet {
				currentSet[k] = v
			}

			err := dataplex.AddEntryLinkAspectsToSet(currentSet, tc.aspectsInput)

			if tc.expectError {
				if err == nil {
					t.Fatalf("%s: Expected an error, but got nil", tc.name)
				}
				if tc.errorMsg != "" && !strings.Contains(err.Error(), tc.errorMsg) {
					t.Errorf("%s: Expected error message containing %q, got %q", tc.name, tc.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("%s: Did not expect an error, but got: %v", tc.name, err)
				}
				if !reflect.DeepEqual(currentSet, tc.expectedSet) {
					t.Errorf("%s: AddEntryLinkAspectsToSet() result mismatch:\ngot:  %v\nwant: %v", tc.name, currentSet, tc.expectedSet)
				}
			}
		})
	}
}

func TestInverseTransformEntryLinkAspects(t *testing.T) {
	testCases := []struct {
		name             string
		resInput         map[string]interface{}
		expectedAspects  []interface{}
		expectNilAspects bool
		expectError      bool
		errorMsg         string
	}{
		{"aspects key is absent", map[string]interface{}{"otherKey": "value"}, nil, true, false, ""},
		{"aspects value is nil", map[string]interface{}{"aspects": nil}, nil, true, false, ""},
		{"aspects is empty map", map[string]interface{}{"aspects": map[string]interface{}{}}, []interface{}{}, false, false, ""},
		{"aspects with one entry", map[string]interface{}{"aspects": map[string]interface{}{"key1": map[string]interface{}{"data": "value1"}}}, []interface{}{map[string]interface{}{"aspectKey": "key1", "aspect": map[string]interface{}{"data": "value1"}}}, false, false, ""},
		{"aspects with multiple entries", map[string]interface{}{"aspects": map[string]interface{}{"key2": map[string]interface{}{"data": "value2"}, "key1": map[string]interface{}{"data": "value1"}}}, []interface{}{map[string]interface{}{"aspectKey": "key1", "aspect": map[string]interface{}{"data": "value1"}}, map[string]interface{}{"aspectKey": "key2", "aspect": map[string]interface{}{"data": "value2"}}}, false, false, ""},
		{"aspects is wrong type (not map)", map[string]interface{}{"aspects": "not a map"}, nil, false, true, "InverseTransformEntryLinkAspects: 'aspects' field is not a map[string]interface{}, got string"},
		{"aspect value is not a map", map[string]interface{}{"aspects": map[string]interface{}{"key1": "not a map value"}}, nil, false, true, "InverseTransformEntryLinkAspects: value for key 'key1' is not a map[string]interface{}, got string"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resCopy := deepCopyMap(tc.resInput)
			originalAspectsBeforeCall := deepCopyValue(resCopy["aspects"])

			err := dataplex.InverseTransformEntryLinkAspects(resCopy)

			if tc.expectError {
				if err == nil {
					t.Fatalf("%s: Expected an error, but got nil", tc.name)
				}
				if tc.errorMsg != "" && !strings.Contains(err.Error(), tc.errorMsg) {
					t.Errorf("%s: Expected error message containing %q, got %q", tc.name, tc.errorMsg, err.Error())
				}
				if !reflect.DeepEqual(resCopy["aspects"], originalAspectsBeforeCall) {
					t.Errorf("%s: resCopy['aspects'] was modified during error case.\nBefore: %#v\nAfter: %#v", tc.name, originalAspectsBeforeCall, resCopy["aspects"])
				}
				return
			}

			if err != nil {
				t.Fatalf("%s: Did not expect an error, but got: %v", tc.name, err)
			}

			actualAspectsRaw, aspectsKeyExists := resCopy["aspects"]

			if tc.expectNilAspects {
				if aspectsKeyExists && actualAspectsRaw != nil {
					t.Errorf("%s: Expected 'aspects' to be nil or absent, but got: %#v", tc.name, actualAspectsRaw)
				}
				return
			}

			if !aspectsKeyExists {
				t.Fatalf("%s: Expected 'aspects' key in result map, but it was missing. Expected value: %#v", tc.name, tc.expectedAspects)
			}
			if actualAspectsRaw == nil && tc.expectedAspects != nil {
				t.Fatalf("%s: Expected 'aspects' to be non-nil, but got nil. Expected value: %#v", tc.name, tc.expectedAspects)
			}

			actualAspectsSlice, ok := actualAspectsRaw.([]interface{})
			if !ok {
				if tc.expectedAspects != nil || actualAspectsRaw != nil {
					t.Fatalf("%s: Expected 'aspects' to be []interface{}, but got %T. Value: %#v", tc.name, actualAspectsRaw, actualAspectsRaw)
				}
			}

			if actualAspectsSlice != nil {
				sortAspectSlice(actualAspectsSlice)
			}
			if tc.expectedAspects != nil {
				sortAspectSlice(tc.expectedAspects)
			}

			if !reflect.DeepEqual(actualAspectsSlice, tc.expectedAspects) {
				t.Errorf("%s: InverseTransformEntryLinkAspects() result mismatch:\ngot:  %#v\nwant: %#v", tc.name, actualAspectsSlice, tc.expectedAspects)
			}
		})
	}
}

func TestTransformEntryLinkAspects(t *testing.T) {
	testCases := []struct {
		name             string
		objInput         map[string]interface{}
		expectedAspects  map[string]interface{}
		expectNilAspects bool
		expectError      bool
		errorMsg         string
	}{
		{"aspects key is absent", map[string]interface{}{"otherKey": "value"}, nil, true, false, ""},
		{"aspects value is nil", map[string]interface{}{"aspects": nil}, nil, true, false, ""},
		{"aspects is empty slice", map[string]interface{}{"aspects": []interface{}{}}, map[string]interface{}{}, false, false, ""},
		{"aspects with one item", map[string]interface{}{"aspects": []interface{}{map[string]interface{}{"aspectKey": "key1", "aspect": map[string]interface{}{"data": "value1"}}}}, map[string]interface{}{"key1": map[string]interface{}{"data": "value1"}}, false, false, ""},
		{"aspects with one item that has no aspect", map[string]interface{}{"aspects": []interface{}{map[string]interface{}{"aspectKey": "key1"}}}, map[string]interface{}{"key1": map[string]interface{}{"data": map[string]interface{}{}}}, false, false, ""},
		{"aspects with multiple items", map[string]interface{}{"aspects": []interface{}{map[string]interface{}{"aspectKey": "key1", "aspect": map[string]interface{}{"data": "value1"}}, map[string]interface{}{"aspectKey": "key2", "aspect": map[string]interface{}{"data": "value2"}}}}, map[string]interface{}{"key1": map[string]interface{}{"data": "value1"}, "key2": map[string]interface{}{"data": "value2"}}, false, false, ""},
		{"aspects with duplicate aspectKey", map[string]interface{}{"aspects": []interface{}{map[string]interface{}{"aspectKey": "key1", "aspect": map[string]interface{}{"data": "value_first"}}, map[string]interface{}{"aspectKey": "key2", "aspect": map[string]interface{}{"data": "value2"}}, map[string]interface{}{"aspectKey": "key1", "aspect": map[string]interface{}{"data": "value_last"}}}}, map[string]interface{}{"key1": map[string]interface{}{"data": "value_last"}, "key2": map[string]interface{}{"data": "value2"}}, false, false, ""},
		{"aspects is wrong type (not slice)", map[string]interface{}{"aspects": "not a slice"}, nil, false, true, "TransformEntryLinkAspects: 'aspects' field is not a []interface{}, got string"},
		{"item in slice is not a map", map[string]interface{}{"aspects": []interface{}{"not a map"}}, nil, false, true, "TransformEntryLinkAspects: item in 'aspects' slice at index 0 is not a map[string]interface{}, got string"},
		{"item map missing aspectKey", map[string]interface{}{"aspects": []interface{}{map[string]interface{}{"wrongKey": "k1", "aspect": map[string]interface{}{}}}}, nil, false, true, "TransformEntryLinkAspects: 'aspectKey' not found in aspect item at index 0"},
		{"aspectKey is not a string", map[string]interface{}{"aspects": []interface{}{map[string]interface{}{"aspectKey": 123, "aspect": map[string]interface{}{}}}}, nil, false, true, "TransformEntryLinkAspects: 'aspectKey' in item at index 0 is not a string, got int"},
		{"aspect is present but wrong type", map[string]interface{}{"aspects": []interface{}{map[string]interface{}{"aspectKey": "key1", "aspect": "not a map"}}}, map[string]interface{}{"key1": map[string]interface{}{"data": map[string]interface{}{}}}, false, false, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			objCopy := deepCopyMap(tc.objInput)
			originalAspectsBeforeCall := deepCopyValue(objCopy["aspects"])

			err := dataplex.TransformEntryLinkAspects(objCopy)

			if tc.expectError {
				if err == nil {
					t.Fatalf("%s: Expected an error, but got nil", tc.name)
				}
				if tc.errorMsg != "" && !strings.Contains(err.Error(), tc.errorMsg) {
					t.Errorf("%s: Expected error message containing %q, got %q", tc.name, tc.errorMsg, err.Error())
				}
				if !reflect.DeepEqual(objCopy["aspects"], originalAspectsBeforeCall) {
					t.Errorf("%s: objCopy['aspects'] was modified during error case.\nBefore: %#v\nAfter: %#v", tc.name, originalAspectsBeforeCall, objCopy["aspects"])
				}
				return
			}

			if err != nil {
				t.Fatalf("%s: Did not expect an error, but got: %v", tc.name, err)
			}

			actualAspectsRaw, aspectsKeyExists := objCopy["aspects"]

			if tc.expectNilAspects {
				if aspectsKeyExists && actualAspectsRaw != nil {
					t.Errorf("%s: Expected 'aspects' to be nil or absent, but got: %#v", tc.name, actualAspectsRaw)
				}
				return
			}

			if !aspectsKeyExists {
				t.Fatalf("%s: Expected 'aspects' key in result map, but it was missing. Expected value: %#v", tc.name, tc.expectedAspects)
			}
			if actualAspectsRaw == nil && tc.expectedAspects != nil {
				t.Fatalf("%s: Expected 'aspects' to be non-nil, but got nil. Expected value: %#v", tc.name, tc.expectedAspects)
			}

			actualAspectsMap, ok := actualAspectsRaw.(map[string]interface{})
			if !ok {
				if tc.expectedAspects != nil || actualAspectsRaw != nil {
					t.Fatalf("%s: Expected 'aspects' to be map[string]interface{}, but got %T. Value: %#v", tc.name, actualAspectsRaw, actualAspectsRaw)
				}
			}

			if !reflect.DeepEqual(actualAspectsMap, tc.expectedAspects) {
				t.Errorf("%s: TransformEntryLinkAspects() result mismatch:\ngot:  %#v\nwant: %#v", tc.name, actualAspectsMap, tc.expectedAspects)
			}
		})
	}
}

func TestAccDataplexEntryLink_update(t *testing.T) {
	t.Parallel()

	context := map[string]interface{}{
		"project_number": envvar.GetTestProjectNumberFromEnv(),
		"project_id":     envvar.GetTestProjectFromEnv(),
		"random_suffix":  acctest.RandString(t, 10),
	}

	acctest.VcrTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.AccTestPreCheck(t) },
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		CheckDestroy:             testAccCheckDataplexEntryLinkDestroyProducer(t),
		Steps: []resource.TestStep{
			{
				Config: testAccDataplexEntryLink_updatePrepare(context),
			},
			{
				ResourceName:            "google_dataplex_entry_link.full_entry_link_with_aspect",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"aspects", "dataset_id", "table_id", "entry_link_id", "location"},
			},

			{
				Config: testAccDataplexEntryLink_update(context),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("google_dataplex_entry_link.full_entry_link_with_aspect", plancheck.ResourceActionUpdate),
					},
				},
			},
			{
				ResourceName:            "google_dataplex_entry_link.full_entry_link_with_aspect",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"aspects", "dataset_id", "table_id", "entry_link_id", "location"},
			},
		},
	})
}

func testAccDataplexEntryLink_updatePrepare(context map[string]interface{}) string {
	return acctest.Nprintf(`

resource "google_bigquery_dataset" "bq_dataset" {
  dataset_id = "tf_test_dataset_%{random_suffix}"
  project    = "%{project_number}"
  location   = "us-central1"
}

resource "google_bigquery_table" "tf_test_table1" {
  deletion_protection = false
  dataset_id = google_bigquery_dataset.bq_dataset.dataset_id
  table_id   = "tf_test_table1_%{random_suffix}"
  project    = "%{project_number}"
  schema     = <<EOF
[
  {
    "name": "col1",
    "type": "STRING",
    "mode": "NULLABLE",
    "description": "Column 1"
  }
]
EOF
}

resource "google_bigquery_table" "tf_test_table2" {
  deletion_protection = false
  dataset_id = google_bigquery_dataset.bq_dataset.dataset_id
  table_id   = "tf_test_table2_%{random_suffix}"
  project    = "%{project_number}"
  schema     = <<EOF
[
  {
    "name": "colA",
    "type": "STRING",
    "mode": "NULLABLE",
    "description": "Column A"
  }
]
EOF
}

resource "google_dataplex_entry_link" "full_entry_link_with_aspect" {
  project = "%{project_number}"
  location = "us-central1"
  entry_group_id = "@bigquery"
  entry_link_id = "tf-test-full-entry-link%{random_suffix}"
  entry_link_type = "projects/655216118709/locations/global/entryLinkTypes/schema-join"
  entry_references {
    name = "projects/%{project_number}/locations/us-central1/entryGroups/@bigquery/entries/bigquery.googleapis.com/projects/%{project_id}/datasets/${google_bigquery_dataset.bq_dataset.dataset_id}/tables/${google_bigquery_table.tf_test_table1.table_id}"
    type = ""
  }
  entry_references {
    name = "projects/%{project_number}/locations/us-central1/entryGroups/@bigquery/entries/bigquery.googleapis.com/projects/%{project_id}/datasets/${google_bigquery_dataset.bq_dataset.dataset_id}/tables/${google_bigquery_table.tf_test_table2.table_id}"
    type = ""
  }
  aspects {
    aspect_key = "655216118709.global.schema-join"
	aspect {
		data = jsonencode({
			joins       = []
			userManaged = true
		})
	}
  }
}
`, context)
}

func testAccDataplexEntryLink_update(context map[string]interface{}) string {
	return acctest.Nprintf(`

resource "google_bigquery_dataset" "bq_dataset" {
  dataset_id = "tf_test_dataset_%{random_suffix}"
  project    = "%{project_number}"
  location   = "us-central1"
}

resource "google_bigquery_table" "tf_test_table1" {
  deletion_protection = false
  dataset_id = google_bigquery_dataset.bq_dataset.dataset_id
  table_id   = "tf_test_table1_%{random_suffix}"
  project    = "%{project_number}"
  schema     = <<EOF
[
  {
    "name": "col1",
    "type": "STRING",
    "mode": "NULLABLE",
    "description": "Column 1"
  }
]
EOF
}

resource "google_bigquery_table" "tf_test_table2" {
  deletion_protection = false
  dataset_id = google_bigquery_dataset.bq_dataset.dataset_id
  table_id   = "tf_test_table2_%{random_suffix}"
  project    = "%{project_number}"
  schema     = <<EOF
[
  {
    "name": "colA",
    "type": "STRING",
    "mode": "NULLABLE",
    "description": "Column A"
  }
]
EOF
}

resource "google_dataplex_entry_link" "full_entry_link_with_aspect" {
  project = "%{project_number}"
  location = "us-central1"
  entry_group_id = "@bigquery"
  entry_link_id = "tf-test-full-entry-link%{random_suffix}"
  entry_link_type = "projects/655216118709/locations/global/entryLinkTypes/schema-join"
  entry_references {
    name = "projects/%{project_number}/locations/us-central1/entryGroups/@bigquery/entries/bigquery.googleapis.com/projects/%{project_id}/datasets/${google_bigquery_dataset.bq_dataset.dataset_id}/tables/${google_bigquery_table.tf_test_table1.table_id}"
    type = ""
  }
  entry_references {
    name = "projects/%{project_number}/locations/us-central1/entryGroups/@bigquery/entries/bigquery.googleapis.com/projects/%{project_id}/datasets/${google_bigquery_dataset.bq_dataset.dataset_id}/tables/${google_bigquery_table.tf_test_table2.table_id}"
    type = ""
  }
  aspects {
    aspect_key = "655216118709.global.schema-join"
	aspect {
		data = jsonencode({
			joins       = []
			userManaged = false
		})
	}
  }
}
`, context)
}
