package compute_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-provider-google/google/acctest"
)

var interconnectLoc = "z2z-us-west8-zone2-ncphxk-z"

func testAccDataSourceCheckInterconnectLocation() func(s *terraform.State) error {
	return func(s *terraform.State) error {
		data_source_name := "data.google_compute_interconnect_location.my_location"
		ds, ok := s.RootModule().Resources[data_source_name]
		if !ok {
			return fmt.Errorf("root module has no resource called %s", data_source_name)
		}
		ds_attr := ds.Primary.Attributes
		expected := map[string]string{
			"name":              interconnectLoc,
			"description":       "Google Z2Z us-west8 (ncphxk-z)",
			"facility_provider": "Google",
		}
		for attr, expect_value := range expected {
			if ds_attr[attr] != expect_value {
				return fmt.Errorf("%s is %s; want %s", attr, ds_attr[attr], expect_value)
			}
		}
		if ds_attr["self_link"] == "" {
			return fmt.Errorf("self_link is not set")
		}
		return nil
	}
}
func TestAccDataSourceGoogleComputeInterconnectLocation_basic(t *testing.T) {
	acctest.VcrTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.AccTestPreCheck(t) },
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceGoogleComputeInterconnectLocationConfig(interconnectLoc),
				Check: resource.ComposeTestCheckFunc(
					testAccDataSourceCheckInterconnectLocation(),
				),
			},
		},
	})
}
func testAccDataSourceGoogleComputeInterconnectLocationConfig(locationName string) string {
	return fmt.Sprintf(`
data "google_compute_interconnect_location" "my_location" {
 name = "%s"
}
`, locationName)
}
