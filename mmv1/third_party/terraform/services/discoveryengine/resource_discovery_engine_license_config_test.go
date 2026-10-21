package discoveryengine_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-provider-google/google/acctest"
	_ "github.com/hashicorp/terraform-provider-google/google/services/discoveryengine"
)

func TestAccDiscoveryEngineLicenseConfig_discoveryengineLicenseconfigBasicExample_update(t *testing.T) {
	t.Skip("License Config creation has been deprecated at the project level for Gemini Enterprise")
	t.Parallel()

	context := map[string]interface{}{
		"random_suffix": acctest.RandString(t, 10),
	}

	acctest.VcrTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.AccTestPreCheck(t) },
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		Steps: []resource.TestStep{
			{
				Config: testAccDiscoveryEngineLicenseConfig_discoveryengineLicenseconfigBasicExample_basic(acctest.MergeMap(context, map[string]interface{}{
					"license_count":         `100`,
					"last_user_update_time": nil,
				})),
			},
			{
				ResourceName:            "google_discovery_engine_license_config.basic",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"location"},
			},
			{
				Config: testAccDiscoveryEngineLicenseConfig_discoveryengineLicenseconfigBasicExample_basic(acctest.MergeMap(context, map[string]interface{}{
					"license_count":         `200`,
					"last_user_update_time": `"2026-08-19T20:00:00Z"`,
				})),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("google_discovery_engine_license_config.basic", "last_user_update_time", "2026-08-19T20:00:00Z"),
				),
			},
			{
				ResourceName:            "google_discovery_engine_license_config.basic",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"location"},
			},
		},
	})
}

func testAccDiscoveryEngineLicenseConfig_discoveryengineLicenseconfigBasicExample_basic(context map[string]interface{}) string {
	return acctest.Nprintf(`
resource "google_discovery_engine_license_config" "license" {
  location              = "global"
  license_count         = %{license_count}
  last_user_update_time = %{last_user_update_time}
}
`, context)
}
