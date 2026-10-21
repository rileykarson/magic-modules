package container_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-provider-google/google/acctest"
	tpgcompute "github.com/hashicorp/terraform-provider-google/google/services/compute"
)

func TestAccContainerNodeConfig_effectiveCgroupMode(t *testing.T) {
	t.Parallel()

	clusterName := fmt.Sprintf("tf-test-cluster-%s", acctest.RandString(t, 10))
	networkName := tpgcompute.BootstrapSharedTestNetwork(t, "gke-cluster")
	subnetworkName := tpgcompute.BootstrapSubnetInRegion(t, "gke-cluster", networkName, "us-central1", "10.79.0.0/20")

	context := map[string]interface{}{
		"cluster_name":   clusterName,
		"network_name":   networkName,
		"subnetwork_name": subnetworkName,
	}

	acctest.VcrTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.AccTestPreCheck(t) },
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		CheckDestroy:             testAccCheckContainerClusterDestroyProducer(t),
		Steps: []resource.TestStep{
			{
				Config: testAccContainerNodeConfig_effectiveCgroupMode(context),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("google_container_cluster.primary", "node_config.0.effective_cgroup_mode"),
				),
			},
		},
	})
}

func testAccContainerNodeConfig_effectiveCgroupMode(context map[string]interface{}) string {
	return acctest.Nprintf(`
resource "google_container_cluster" "primary" {
  name               = "%{cluster_name}"
  location           = "us-central1-c"
  initial_node_count = 1
  deletion_policy    = "DELETE"
  network            = "%{network_name}"
  subnetwork         = "%{subnetwork_name}"

  node_config {
    machine_type = "e2-medium"
  }
}
`, context)
}
