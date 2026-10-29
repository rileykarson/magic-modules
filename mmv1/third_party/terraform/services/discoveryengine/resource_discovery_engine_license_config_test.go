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

	randomSuffix := acctest.RandString(t, 10)
	context := map[string]interface{}{
		"random_suffix":         randomSuffix,
		"license_count":         50,
		"last_user_update_time": nil,
	}
	contextUpdate := map[string]interface{}{
		"random_suffix":         randomSuffix,
		"license_count":         51,
		"last_user_update_time": "2026-08-19T20:00:00Z",
	}

	acctest.VcrTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.AccTestPreCheck(t) },
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		Steps: []resource.TestStep{
			{
				Config: testAccDiscoveryEngineLicenseConfig_discoveryengineLicenseconfigBasicExample_basic(context),
			},
			{
				ResourceName:            "google_discovery_engine_license_config.basic",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"location"},
			},
			{
				Config: testAccDiscoveryEngineLicenseConfig_discoveryengineLicenseconfigBasicExample_basic(contextUpdate),
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
resource "google_discovery_engine_license_config" "basic" {
  location = "global"
  license_config_id = "tf-test-license-config-id%{random_suffix}"
  license_count = %{license_count}
  subscription_tier = "SUBSCRIPTION_TIER_SEARCH_AND_ASSISTANT"
  start_date {
    year = 2099
    month = 1
    day = 1
  }
  end_date {
    year = 2100
    month = 1
    day = 1
  }
  subscription_term = "SUBSCRIPTION_TERM_ONE_YEAR"
  last_user_update_time = "%{last_user_update_time}"
}
`, context)
}
