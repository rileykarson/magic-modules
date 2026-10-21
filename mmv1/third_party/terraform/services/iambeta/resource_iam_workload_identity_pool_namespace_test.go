package iambeta_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/hashicorp/terraform-provider-google/google/acctest"
	_ "github.com/hashicorp/terraform-provider-google/google/services/iambeta"
)

func TestAccIAMBetaWorkloadIdentityPoolNamespace_minimal(t *testing.T) {
	t.Parallel()

	randomSuffix := acctest.RandString(t, 10)

	acctest.VcrTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.AccTestPreCheck(t) },
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		CheckDestroy:             testAccCheckIAMBetaWorkloadIdentityPoolNamespaceDestroyProducer(t),
		Steps: []resource.TestStep{
			{
				Config: testAccIAMBetaWorkloadIdentityPoolNamespace_minimal(map[string]interface{}{
					"random_suffix": randomSuffix,
					"description":   nil,
					"disabled":      nil,
				}),
			},
			{
				ResourceName:            "google_iam_workload_identity_pool_namespace.example",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"workload_identity_pool_id", "workload_identity_pool_namespace_id"},
			},
			{
				Config: testAccIAMBetaWorkloadIdentityPoolNamespace_minimal(map[string]interface{}{
					"random_suffix": randomSuffix,
					"description":   "Updated Namespace in a Workload Identity Pool",
					"disabled":      false,
				}),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("google_iam_workload_identity_pool_namespace.example", plancheck.ResourceActionUpdate),
					},
				},
			},
			{
				ResourceName:            "google_iam_workload_identity_pool_namespace.example",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"workload_identity_pool_id", "workload_identity_pool_namespace_id"},
			},
		},
	})
}

func TestAccIAMBetaWorkloadIdentityPoolNamespace_full(t *testing.T) {
	t.Parallel()

	randomSuffix := acctest.RandString(t, 10)

	acctest.VcrTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.AccTestPreCheck(t) },
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		CheckDestroy:             testAccCheckIAMBetaWorkloadIdentityPoolNamespaceDestroyProducer(t),
		Steps: []resource.TestStep{
			{
				Config: testAccIAMBetaWorkloadIdentityPoolNamespace_minimal(map[string]interface{}{
					"random_suffix": randomSuffix,
					"description":   "Example Namespace in a Workload Identity Pool",
					"disabled":      true,
				}),
			},
			{
				ResourceName:            "google_iam_workload_identity_pool_namespace.example",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"workload_identity_pool_id", "workload_identity_pool_namespace_id"},
			},
			{
				Config: testAccIAMBetaWorkloadIdentityPoolNamespace_minimal(map[string]interface{}{
					"random_suffix": randomSuffix,
					"description":   "Updated Namespace in a Workload Identity Pool",
					"disabled":      false,
				}),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("google_iam_workload_identity_pool_namespace.example", plancheck.ResourceActionUpdate),
					},
				},
			},
			{
				ResourceName:            "google_iam_workload_identity_pool_namespace.example",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"workload_identity_pool_id", "workload_identity_pool_namespace_id"},
			},
		},
	})
}

func testAccIAMBetaWorkloadIdentityPoolNamespace_minimal(context map[string]interface{}) string {
	return acctest.Nprintf(`
resource "google_iam_workload_identity_pool" "pool" {
  workload_identity_pool_id = "tf-test-example-pool%{random_suffix}"
  mode                      = "TRUST_DOMAIN"
}

resource "google_iam_workload_identity_pool_namespace" "example" {
  workload_identity_pool_id           = google_iam_workload_identity_pool.pool.workload_identity_pool_id
  workload_identity_pool_namespace_id = "tf-test-example-namespace%{random_suffix}"
  description                         = "%{description}"
  disabled                            = %{disabled}
}
`, context)
}
