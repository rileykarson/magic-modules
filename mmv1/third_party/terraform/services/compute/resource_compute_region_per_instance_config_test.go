package compute_test

import (
	"fmt"
	"github.com/hashicorp/terraform-provider-google/google/acctest"
	"github.com/hashicorp/terraform-provider-google/google/envvar"
	_ "github.com/hashicorp/terraform-provider-google/google/services/compute"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccComputeRegionPerInstanceConfig_statefulBasic(t *testing.T) {
	// Multiple fine-grained resources
	acctest.SkipIfVcr(t)
	t.Parallel()

	suffix := acctest.RandString(t, 10)
	rigmName := fmt.Sprintf("tf-test-rigm-%s", suffix)
	context := map[string]interface{}{
		"rigm_name":     rigmName,
		"random_suffix": suffix,
		"config_name":   fmt.Sprintf("tf-test-instance-%s", acctest.RandString(t, 10)),
		"config_name2":  fmt.Sprintf("tf-test-instance-%s", acctest.RandString(t, 10)),
		"config_name3":  fmt.Sprintf("tf-test-instance-%s", acctest.RandString(t, 10)),
		"config_name4":  fmt.Sprintf("tf-test-instance-%s", acctest.RandString(t, 10)),
	}
	rigmId := fmt.Sprintf("projects/%s/regions/%s/instanceGroupManagers/%s",
		envvar.GetTestProjectFromEnv(), envvar.GetTestRegionFromEnv(), rigmName)

	acctest.VcrTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.AccTestPreCheck(t) },
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		Steps: []resource.TestStep{
			{
				// Create one endpoint
				Config: testAccComputeRegionPerInstanceConfig_statefulBasic(acctest.MergeMap(context, map[string]interface{}{
					"region": nil,
					"name":   `"tf-test-rigm%{random_suffix}-1"`,
				})),
			},
			{
				ResourceName:            "google_compute_region_per_instance_config.default",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"remove_instance_state_on_destroy", "region"},
			},
			{
				// Force-recreate old config
				Config: testAccComputeRegionPerInstanceConfig_statefulBasic(acctest.MergeMap(context, map[string]interface{}{
					"region": `google_compute_region_instance_group_manager.rigm.region`,
					"name":   `"tf-test-rigm%{random_suffix}-2"`,
				})),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckComputeRegionPerInstanceConfigDestroyed(t, rigmId, context["config_name"].(string)),
				),
			},
			{
				ResourceName:            "google_compute_region_per_instance_config.default",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"remove_instance_state_on_destroy", "region"},
			},
			{
				// Add two new endpoints
				Config: testAccComputeRegionPerInstanceConfig_statefulAdditional(context),
			},
			{
				ResourceName:            "google_compute_region_per_instance_config.default",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"remove_instance_state_on_destroy", "region"},
			},
			{
				ResourceName:            "google_compute_region_per_instance_config.with_disks",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"most_disruptive_allowed_action", "minimal_action", "remove_instance_state_on_destroy"},
			},
			{
				ResourceName:            "google_compute_region_per_instance_config.add2",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"remove_instance_state_on_destroy", "region"},
			},
			{
				// delete all configs
				Config: testAccComputeRegionPerInstanceConfig_rigm(context),
				Check: resource.ComposeTestCheckFunc(
					// Config with remove_instance_state_on_destroy = false won't be destroyed (config4)
					testAccCheckComputeRegionPerInstanceConfigDestroyed(t, rigmId, context["config_name2"].(string)),
					testAccCheckComputeRegionPerInstanceConfigDestroyed(t, rigmId, context["config_name3"].(string)),
				),
			},
		},
	})
}

func TestAccComputeRegionPerInstanceConfig_update(t *testing.T) {
	t.Parallel()

	context := map[string]interface{}{
		"random_suffix": acctest.RandString(t, 10),
		"rigm_name":     fmt.Sprintf("tf-test-rigm-%s", acctest.RandString(t, 10)),
		"config_name":   fmt.Sprintf("tf-test-instance-%s", acctest.RandString(t, 10)),
	}

	acctest.VcrTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.AccTestPreCheck(t) },
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		Steps: []resource.TestStep{
			{
				// Create one config
				Config: testAccComputeRegionPerInstanceConfig_statefulBasic(acctest.MergeMap(context, map[string]interface{}{
					"region": nil,
					"name":   `"tf-test-rigm%{random_suffix}-1"`,
				})),
			},
			{
				ResourceName:            "google_compute_region_per_instance_config.default",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"remove_instance_state_on_destroy", "region"},
			},
			{
				// Update an existing config
				Config: testAccComputeRegionPerInstanceConfig_update(context),
			},
			{
				ResourceName:            "google_compute_region_per_instance_config.default",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"remove_instance_state_on_destroy", "region"},
			},
		},
	})
}

func TestAccComputeRegionPerInstanceConfig_statefulIps(t *testing.T) {
	t.Parallel()

	context := map[string]interface{}{
		"random_suffix": acctest.RandString(t, 10),
		"rigm_name":     fmt.Sprintf("tf-test-rigm-%s", acctest.RandString(t, 10)),
		"config_name":   fmt.Sprintf("tf-test-instance-%s", acctest.RandString(t, 10)),
		"network":       fmt.Sprintf("tf-test-rigm-%s", acctest.RandString(t, 10)),
		"subnetwork":    fmt.Sprintf("tf-test-rigm-%s", acctest.RandString(t, 10)),
		"address1":      fmt.Sprintf("tf-test-rigm-address%s", acctest.RandString(t, 10)),
		"address2":      fmt.Sprintf("tf-test-rigm-address%s", acctest.RandString(t, 10)),
	}

	acctest.VcrTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.AccTestPreCheck(t) },
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		Steps: []resource.TestStep{
			{
				// Create one config
				Config: testAccComputeRegionPerInstanceConfig_statefulIpsBasic(context),
			},
			{
				ResourceName:            "google_compute_region_per_instance_config.default",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"remove_instance_state_on_destroy", "region"},
			},
			{
				// Update an existing config
				Config: testAccComputeRegionPerInstanceConfig_statefulIpsUpdate(context),
			},
			{
				ResourceName:            "google_compute_region_per_instance_config.default",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"remove_instance_state_on_destroy", "region"},
			},
		},
	})
}

func TestAccComputeRegionPerInstanceConfig_removeInstanceOnDestroy(t *testing.T) {
	t.Parallel()

	rigmName := fmt.Sprintf("tf-test-rigm-%s", acctest.RandString(t, 10))
	context := map[string]interface{}{
		"random_suffix": acctest.RandString(t, 10),
		"rigm_name":     rigmName,
		"config_name":   fmt.Sprintf("tf-test-instance-%s", acctest.RandString(t, 10)),
		"config_name2":  fmt.Sprintf("tf-test-instance-%s", acctest.RandString(t, 10)),
		"network":       fmt.Sprintf("tf-test-rigm-%s", acctest.RandString(t, 10)),
		"subnetwork":    fmt.Sprintf("tf-test-rigm-%s", acctest.RandString(t, 10)),
		"address1":      fmt.Sprintf("tf-test-rigm-address%s", acctest.RandString(t, 10)),
		"address2":      fmt.Sprintf("tf-test-rigm-address%s", acctest.RandString(t, 10)),
	}
	rigmId := fmt.Sprintf("projects/%s/regions/%s/instanceGroupManagers/%s",
		envvar.GetTestProjectFromEnv(), envvar.GetTestRegionFromEnv(), rigmName)

	acctest.VcrTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.AccTestPreCheck(t) },
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		Steps: []resource.TestStep{
			{
				Config: testAccComputeRegionPerInstanceConfig_removeInstanceOnDestroyBefore(context),
			},
			{
				ResourceName:            "google_compute_region_per_instance_config.config_one",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"remove_instance_on_destroy", "zone"},
			},
			{
				ResourceName:            "google_compute_region_per_instance_config.config_two",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"remove_instance_on_destroy", "zone"},
			},
			{
				Config: testAccComputeRegionPerInstanceConfig_removeInstanceOnDestroyAfter(context),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckComputeRegionPerInstanceConfigDestroyed(t, rigmId, context["config_name"].(string)),
					testAccCheckComputeRegionPerInstanceConfigInstanceDestroyed(t, rigmId, context["config_name"].(string)),
				),
			},
			{
				ResourceName:            "google_compute_region_per_instance_config.config_two",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"remove_instance_on_destroy", "zone"},
			},
			{
				// delete all configs
				Config: testAccComputeRegionPerInstanceConfig_rigm(context),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckComputeRegionPerInstanceConfigDestroyed(t, rigmId, context["config_name2"].(string)),
					testAccCheckComputeRegionPerInstanceConfigInstanceDestroyed(t, rigmId, context["config_name2"].(string)),
				),
			},
		},
	})
}

func testAccComputeRegionPerInstanceConfig_statefulBasic(context map[string]interface{}) string {
	return acctest.Nprintf(`
data "google_compute_image" "my_image" {
  family  = "debian-11"
  project = "debian-cloud"
}

resource "google_compute_instance_template" "rigm-basic" {
  name           = "tf-test-rigm%{random_suffix}"
  machine_type   = "e2-medium"
  can_ip_forward = false
  tags           = ["foo", "bar"]

  disk {
    source_image = data.google_compute_image.my_image.self_link
    auto_delete  = true
    boot         = true
  }

  network_interface {
    network = "default"
  }

  service_account {
    scopes = ["userinfo-email", "compute-ro", "storage-ro"]
  }
}

resource "google_compute_region_instance_group_manager" "rigm" {
  description = "Terraform test instance group manager"
  name        = "tf-test-rigm%{random_suffix}"
  version {
    instance_template = google_compute_instance_template.rigm-basic.self_link
    name              = "primary"
  }
  base_instance_name = "tf-test-rigm%{random_suffix}"
  region             = "us-central1"
  target_size        = 2
}

resource "google_compute_region_per_instance_config" "default" {
  region = %{region}
  region_instance_group_manager = google_compute_region_instance_group_manager.rigm.name
  name = %{name}
  preserved_state {
    metadata = {
      foo = "bar"
      instance_template = google_compute_instance_template.rigm-basic.self_link
    }
  }
}
`, context)
}

func testAccComputeRegionPerInstanceConfig_removeInstanceOnDestroyBefore(context map[string]interface{}) string {
	return acctest.Nprintf(`
resource "google_compute_network" "default" {
  name = "%{network}"
}

resource "google_compute_subnetwork" "default" {
  name          = "%{subnetwork}"
  ip_cidr_range = "10.0.0.0/16"
  region        = "us-central1"
  network       = google_compute_network.default.id
}

resource "google_compute_address" "static_internal_ip" {
  name         = "%{address1}"
  address_type = "INTERNAL"
}

resource "google_compute_address" "static_external_ip" {
  name         = "%{address2}"
  address_type = "EXTERNAL"
}

resource "google_compute_region_per_instance_config" "config_one" {
  region = google_compute_region_instance_group_manager.rigm.region
  region_instance_group_manager = google_compute_region_instance_group_manager.rigm.name
  name = "%{config_name}"
  remove_instance_on_destroy = true
  preserved_state {
    metadata = {
      asdf = "config-one"
    }
    disk {
      device_name = "my-stateful-disk1"
      source      = google_compute_disk.disk.id
    }

    disk {
      device_name = "my-stateful-disk2"
      source      = google_compute_disk.disk1.id
    }
    internal_ip {
      ip_address {
				address = google_compute_address.static_internal_ip.self_link
      }
      auto_delete    = "NEVER"
      interface_name = "nic0"
    }
    external_ip {
      ip_address {
        address = google_compute_address.static_external_ip.self_link
      }
      auto_delete    = "NEVER"
      interface_name = "nic0"
    }
  }
}

resource "google_compute_disk" "disk" {
  name  = "tf-test-test-disk-%{random_suffix}"
  type  = "pd-ssd"
  zone  = "us-central1-c"
  image = "debian-8-jessie-v20170523"
  physical_block_size_bytes = 4096
}

resource "google_compute_disk" "disk1" {
  name  = "tf-test-test-disk2-%{random_suffix}"
  type  = "pd-ssd"
  zone  = "us-central1-c"
  image = "debian-cloud/debian-13"
  physical_block_size_bytes = 4096
}

resource "google_compute_region_per_instance_config" "config_two" {
	region = google_compute_region_instance_group_manager.rigm.region
	region_instance_group_manager = google_compute_region_instance_group_manager.rigm.name
	name = "%{config_name2}"
	remove_instance_on_destroy = true
	preserved_state {
		metadata = {
			asdf = "config-two"
		}
	}
}
`, context) + testAccComputeRegionPerInstanceConfig_rigm(context)
}

func testAccComputeRegionPerInstanceConfig_removeInstanceOnDestroyAfter(context map[string]interface{}) string {
	return acctest.Nprintf(`
resource "google_compute_region_per_instance_config" "config_two" {
	region = google_compute_region_instance_group_manager.rigm.region
	region_instance_group_manager = google_compute_region_instance_group_manager.rigm.name
	name = "%{config_name2}"
	remove_instance_on_destroy = true
	preserved_state {
		metadata = {
			asdf = "config-two"
		}
	}
}
`, context) + testAccComputeRegionPerInstanceConfig_rigm(context)
}

func testAccComputeRegionPerInstanceConfig_statefulIpsBasic(context map[string]interface{}) string {
	return acctest.Nprintf(`
resource "google_compute_network" "default" {
  name = "%{network}"
}

resource "google_compute_subnetwork" "default" {
  name          = "%{subnetwork}"
  ip_cidr_range = "10.0.0.0/16"
  region        = "us-central1"
  network       = google_compute_network.default.id
}

resource "google_compute_address" "static_internal_ip" {
  name         = "%{address1}"
  address_type = "INTERNAL"
}

resource "google_compute_address" "static_external_ip" {
  name         = "%{address2}"
  address_type = "EXTERNAL"
}

resource "google_compute_region_per_instance_config" "default" {
  region = google_compute_region_instance_group_manager.rigm.region
  region_instance_group_manager = google_compute_region_instance_group_manager.rigm.name
  name = "%{config_name}"
  remove_instance_state_on_destroy = true
  preserved_state {
    metadata = {
      asdf = "asdf"
    }
    disk {
      device_name = "my-stateful-disk1"
      source      = google_compute_disk.disk.id
    }

    disk {
      device_name = "my-stateful-disk2"
      source      = google_compute_disk.disk1.id
    }
    internal_ip {
      ip_address {
	    address = google_compute_address.static_internal_ip.self_link
      }
      auto_delete    = "NEVER"
      interface_name = "nic0"
    }
    external_ip {
      ip_address {
        address = google_compute_address.static_external_ip.self_link
      }
      auto_delete    = "NEVER"
      interface_name = "nic0"
    }
  }
}

resource "google_compute_disk" "disk" {
  name  = "tf-test-test-disk-%{random_suffix}"
  type  = "pd-ssd"
  zone  = "us-central1-c"
  image = "debian-8-jessie-v20170523"
  physical_block_size_bytes = 4096
}

resource "google_compute_disk" "disk1" {
  name  = "tf-test-test-disk2-%{random_suffix}"
  type  = "pd-ssd"
  zone  = "us-central1-c"
  image = "debian-cloud/debian-13"
  physical_block_size_bytes = 4096
}
`, context) + testAccComputeRegionPerInstanceConfig_rigm(context)
}

func testAccComputeRegionPerInstanceConfig_statefulIpsUpdate(context map[string]interface{}) string {
	return acctest.Nprintf(`
resource "google_compute_network" "default" {
  name = "%{network}"
}

resource "google_compute_subnetwork" "default" {
  name          = "%{subnetwork}"
  ip_cidr_range = "10.0.0.0/16"
  region        = "us-central1"
  network       = google_compute_network.default.id
}

resource "google_compute_address" "static_internal_ip" {
  name         = "%{address1}"
  address_type = "INTERNAL"
}

resource "google_compute_address" "static_external_ip" {
  name         = "%{address2}"
  address_type = "EXTERNAL"
}

resource "google_compute_region_per_instance_config" "default" {
  region = google_compute_region_instance_group_manager.rigm.region
  region_instance_group_manager = google_compute_region_instance_group_manager.rigm.name
  name = "%{config_name}"
  remove_instance_state_on_destroy = true
  preserved_state {
    metadata = {
      asdf = "asdf"
    }
    disk {
      device_name = "my-stateful-disk1"
      source      = google_compute_disk.disk.id
    }

    disk {
      device_name = "my-stateful-disk2"
      source      = google_compute_disk.disk1.id
    }
    internal_ip {
      ip_address {
	    address = google_compute_address.static_internal_ip.self_link
      }
      auto_delete    = "ON_PERMANENT_INSTANCE_DELETION"
      interface_name = "nic0"
    }
    external_ip {
      ip_address {
        address = google_compute_address.static_external_ip.self_link
      }
      auto_delete    = "ON_PERMANENT_INSTANCE_DELETION"
      interface_name = "nic0"
    }
  }
}

resource "google_compute_disk" "disk" {
  name  = "tf-test-test-disk-%{random_suffix}"
  type  = "pd-ssd"
  zone  = "us-central1-c"
  image = "debian-8-jessie-v20170523"
  physical_block_size_bytes = 4096
}

resource "google_compute_disk" "disk1" {
  name  = "tf-test-test-disk2-%{random_suffix}"
  type  = "pd-ssd"
  zone  = "us-central1-c"
  image = "debian-cloud/debian-13"
  physical_block_size_bytes = 4096
}
`, context) + testAccComputeRegionPerInstanceConfig_rigm(context)
}

// Checks that the per instance config with the given name was destroyed
func testAccCheckComputeRegionPerInstanceConfigDestroyed(t *testing.T, rigmId, configName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		foundNames, err := testAccComputePerInstanceConfigListNames(t, rigmId)
		if err != nil {
			return fmt.Errorf("unable to confirm config with name %s was destroyed: %v", configName, err)
		}
		if _, ok := foundNames[configName]; ok {
			return fmt.Errorf("config with name %s still exists", configName)
		}

		return nil
	}
}

// Checks that the instance with the given name was destroyed.
func testAccCheckComputeRegionPerInstanceConfigInstanceDestroyed(t *testing.T, rigmId, configName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		foundNames, err := testAccComputePerInstanceConfigListInstances(t, rigmId)
		if err != nil {
			return fmt.Errorf("unable to confirm instance with name %s was destroyed: %v", configName, err)
		}
		if _, ok := foundNames[configName]; ok {
			return fmt.Errorf("instance with name %s still exists", configName)
		}

		return nil
	}
}
