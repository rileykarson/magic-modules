package iap_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/hashicorp/terraform-provider-google/google/acctest"
	_ "github.com/hashicorp/terraform-provider-google/google/services/iap"
)

func TestAccIapTunnelDestGroup_updates(t *testing.T) {
	t.Parallel()

	randomSuffix := acctest.RandString(t, 10)

	acctest.VcrTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.AccTestPreCheck(t) },
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		ExternalProviders: map[string]resource.ExternalProvider{
			"random": {},
			"time":   {},
		},
		CheckDestroy: testAccCheckIapTunnelDestGroupDestroyProducer(t),
		Steps: []resource.TestStep{
			{
				Config: testAccIapTunnelDestGroup_full(map[string]interface{}{
					"random_suffix": randomSuffix,
					"cidrs":         `["10.1.0.0/16", "192.168.10.0/24"]`,
					"fqdns":         nil,
				}),
			},
			{
				ResourceName:            "google_iap_tunnel_dest_group.dest_group",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"region", "group_name"},
			},
			{
				Config: testAccIapTunnelDestGroup_full(map[string]interface{}{
					"random_suffix": randomSuffix,
					"cidrs":         `["10.1.0.0/16"]`,
					"fqdns":         nil,
				}),
			},
			{
				ResourceName:            "google_iap_tunnel_dest_group.dest_group",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"region", "group_name"},
			},
			{
				Config: testAccIapTunnelDestGroup_full(map[string]interface{}{
					"random_suffix": randomSuffix,
					"cidrs":         `["10.1.0.0/16"]`,
					"fqdns":         `["proxied.lan"]`,
				}),
			},
			{
				ResourceName:            "google_iap_tunnel_dest_group.dest_group",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"region", "group_name"},
			},
		},
	})
}

func testAccIapTunnelDestGroup_full(context map[string]interface{}) string {
	return acctest.Nprintf(`
resource "google_iap_tunnel_dest_group" "dest_group" {
  region = "us-central1"
  group_name = "tf-test-testgroup%{random_suffix}"
  cidrs = %{cidrs}
  fqdns = %{fqdns}
}
`, context)
}
