package gkebackup_test

import (
	"testing"

	"github.com/hashicorp/terraform-provider-google/google/acctest"
	tpgcontainer "github.com/hashicorp/terraform-provider-google/google/services/container"
	_ "github.com/hashicorp/terraform-provider-google/google/services/gkebackup"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccGKEBackupBackupPlan_update(t *testing.T) {
	t.Parallel()

	cluster := tpgcontainer.BootstrapSharedGKECluster(t, "gkebackup")
	randomSuffix := acctest.RandString(t, 10)
	context := map[string]interface{}{
		"cluster":       cluster,
		"random_suffix": randomSuffix,
	}

	acctest.VcrTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.AccTestPreCheck(t) },
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		CheckDestroy:             testAccCheckGKEBackupBackupPlanDestroyProducer(t),
		Steps: []resource.TestStep{
			{
				Config: testAccGKEBackupBackupPlan_basic(map[string]interface{}{
					"cluster":         cluster,
					"random_suffix":   randomSuffix,
					"permissive_mode": nil,
				}),
			},
			{
				ResourceName:            "google_gke_backup_backup_plan.backupplan",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"labels", "terraform_labels"},
			},
			{
				Config: testAccGKEBackupBackupPlan_basic(map[string]interface{}{
					"cluster":         cluster,
					"random_suffix":   randomSuffix,
					"permissive_mode": true,
				}),
			},
			{
				ResourceName:            "google_gke_backup_backup_plan.backupplan",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"labels", "terraform_labels"},
			},
			{
				Config: testAccGKEBackupBackupPlan_full(context),
			},
			{
				ResourceName:            "google_gke_backup_backup_plan.backupplan",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"labels", "terraform_labels"},
			},
			{
				Config: testAccGKEBackupBackupPlan_rpo_daily_window(context),
			},
			{
				ResourceName:            "google_gke_backup_backup_plan.backupplan",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"labels", "terraform_labels"},
			},
			{
				Config: testAccGKEBackupBackupPlan_rpo_weekly_window(context),
			},
			{
				ResourceName:            "google_gke_backup_backup_plan.backupplan",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"labels", "terraform_labels"},
			},
			{
				Config: testAccGKEBackupBackupPlan_full(context),
			},
			{
				ResourceName:            "google_gke_backup_backup_plan.backupplan",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"labels", "terraform_labels"},
			},
		},
	})
}

func testAccGKEBackupBackupPlan_basic(context map[string]interface{}) string {
	return acctest.Nprintf(`
resource "google_gke_backup_backup_plan" "backupplan" {
  name = "tf-test-testplan%{random_suffix}"
  cluster = "%{cluster}"
  location = "us-east1"
  backup_config {
    include_volume_data = false
    include_secrets = false
    all_namespaces = true
    permissive_mode = %{permissive_mode}
  }
  labels = {
    "some-key-1": "some-value-1"
  }
}
`, context)
}

func testAccGKEBackupBackupPlan_full(context map[string]interface{}) string {
	return acctest.Nprintf(`
resource "google_gke_backup_backup_plan" "backupplan" {
  name = "tf-test-testplan%{random_suffix}"
  cluster = "%{cluster}"
  location = "us-east1"
  retention_policy {
    backup_delete_lock_days = 30
    backup_retain_days = 180
  }
  backup_schedule {
    cron_schedule = "0 9 * * 1"
  }
  backup_config {
    include_volume_data = true
    include_secrets = true
    selected_applications {
      namespaced_names {
        name = "app1"
        namespace = "ns1"
      }
      namespaced_names {
        name = "app2"
        namespace = "ns2"
      }
    }
  }
  labels = {
    "some-key-2": "some-value-2"
  }
}
`, context)
}

func testAccGKEBackupBackupPlan_rpo_daily_window(context map[string]interface{}) string {
	return acctest.Nprintf(`
resource "google_gke_backup_backup_plan" "backupplan" {
  name = "tf-test-testplan%{random_suffix}"
  cluster = "%{cluster}"
  location = "us-east1"
  retention_policy {
    backup_delete_lock_days = 30
    backup_retain_days = 180
  }
  backup_schedule {
    paused = true
    rpo_config {
      target_rpo_minutes=1440
      exclusion_windows {
        start_time  {
          hours = 12
        }
        duration = "7200s"
        daily = true
      }
      exclusion_windows {
        start_time  {
          hours = 8
          minutes = 40
          seconds = 1
        }
        duration = "3600s"
        single_occurrence_date {
          year = 2024
          month = 3
          day = 16
        }
      }
    }
  }
  backup_config {
    include_volume_data = true
    include_secrets = true
    selected_applications {
      namespaced_names {
        name = "app1"
        namespace = "ns1"
      }
      namespaced_names {
        name = "app2"
        namespace = "ns2"
      }
    }
  }
  labels = {
    "some-key-2": "some-value-2"
  }
}
`, context)
}

func testAccGKEBackupBackupPlan_rpo_weekly_window(context map[string]interface{}) string {
	return acctest.Nprintf(`
resource "google_gke_backup_backup_plan" "backupplan" {
  name = "tf-test-testplan%{random_suffix}"
  cluster = "%{cluster}"
  location = "us-east1"
  retention_policy {
    backup_delete_lock_days = 30
    backup_retain_days = 180
  }
  backup_schedule {
    paused = true
    rpo_config {
      target_rpo_minutes=1400
      exclusion_windows {
        start_time  {
          hours = 1
          minutes = 23
        }
        duration = "1800s"
        days_of_week {
          days_of_week = ["MONDAY", "THURSDAY"]
        }
      }
      exclusion_windows {
        start_time  {
          hours = 12
        }
        duration = "3600s"
        single_occurrence_date {
          year = 2024
          month = 3
          day = 17
        }
      }
      exclusion_windows {
        start_time  {
          hours = 8
          minutes = 40
        }
        duration = "600s"
        single_occurrence_date {
          year = 2024
          month = 3
          day = 18
        }
      }
    }
  }
  backup_config {
    include_volume_data = true
    include_secrets = true
    selected_applications {
      namespaced_names {
        name = "app1"
        namespace = "ns1"
      }
      namespaced_names {
        name = "app2"
        namespace = "ns2"
      }
    }
  }
  labels = {
    "some-key-2": "some-value-2"
  }
}
`, context)
}
