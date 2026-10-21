package healthcare_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/hashicorp/terraform-provider-google/google/acctest"
	_ "github.com/hashicorp/terraform-provider-google/google/services/healthcare"
)

func TestAccHealthcareWorkspace_update(t *testing.T) {
	t.Parallel()

	randomSuffix := acctest.RandString(t, 10)

	acctest.VcrTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.AccTestPreCheck(t) },
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		CheckDestroy:             testAccCheckHealthcareWorkspaceDestroyProducer(t),
		Steps: []resource.TestStep{
			{
				Config: testAccHealthcareWorkspace_basic(map[string]interface{}{
					"random_suffix": randomSuffix,
					"labels":        `{ label1 = "labelvalue1" }`,
				}),
			},
			{
				ResourceName:            "google_healthcare_workspace.default",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"dataset", "labels", "terraform_labels"},
			},
			{
				Config: testAccHealthcareWorkspace_basic(map[string]interface{}{
					"random_suffix": randomSuffix,
					"labels":        nil,
				}),
			},
			{
				ResourceName:            "google_healthcare_workspace.default",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"dataset", "labels", "terraform_labels"},
			},
		},
	})
}

func testAccHealthcareWorkspace_basic(context map[string]interface{}) string {
	return acctest.Nprintf(`
resource "google_healthcare_workspace" "default" {
  name    = "tf-test-example-dm-workspace%{random_suffix}"
  dataset = google_healthcare_dataset.dataset.id

  settings {
    data_project_ids = ["tf-test-example-data-source-project-id%{random_suffix}"]
  }
  
  labels = %{labels}
}


resource "google_healthcare_dataset" "dataset" {
  name     = "tf-test-example-dataset%{random_suffix}"
  location = "us-central1"
}
`, context)
}
