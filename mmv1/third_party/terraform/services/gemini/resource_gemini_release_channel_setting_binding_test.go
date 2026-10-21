package gemini_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/hashicorp/terraform-provider-google/google/acctest"
	_ "github.com/hashicorp/terraform-provider-google/google/services/gemini"
	_ "github.com/hashicorp/terraform-provider-google/google/services/resourcemanager"
)

func TestAccGeminiReleaseChannelSettingBinding_update(t *testing.T) {
	t.Parallel()

	releaseChannelSettingId := fmt.Sprintf("tf-test-ls-%s", acctest.RandString(t, 10))
	settingBindingId := fmt.Sprintf("tf-test-lsb-%s", acctest.RandString(t, 10))

	acctest.VcrTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.AccTestPreCheck(t) },
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		Steps: []resource.TestStep{
			{
				Config: testAccGeminiReleaseChannelSettingBinding_basic(map[string]interface{}{
					"release_channel_setting_id": releaseChannelSettingId,
					"setting_binding_id":         settingBindingId,
					"labels":                     nil,
					"product":                    nil,
				}),
			},
			{
				ResourceName:            "google_gemini_release_channel_setting_binding.basic_binding",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"labels", "location", "release_channel_setting_id", "terraform_labels"},
			},
			{
				Config: testAccGeminiReleaseChannelSettingBinding_basic(map[string]interface{}{
					"release_channel_setting_id": releaseChannelSettingId,
					"setting_binding_id":         settingBindingId,
					"labels":                     `{"my_key" = "my_value"}`,
					"product":                    "GEMINI_CODE_ASSIST",
				}),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("google_gemini_release_channel_setting_binding.basic_binding", plancheck.ResourceActionUpdate),
					},
				},
			},
			{
				ResourceName:            "google_gemini_release_channel_setting_binding.basic_binding",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"labels", "location", "release_channel_setting_id", "terraform_labels"},
			},
		},
	})
}

func testAccGeminiReleaseChannelSettingBinding_basic(context map[string]interface{}) string {
	return acctest.Nprintf(`
data "google_project" "project" {
}

resource "google_gemini_release_channel_setting" "basic" {
    release_channel_setting_id = "%{release_channel_setting_id}"
    location = "global"
    labels = {"my_key" = "my_value"}
    release_channel = "EXPERIMENTAL"
}

resource "google_gemini_release_channel_setting_binding" "basic_binding" {
    release_channel_setting_id = google_gemini_release_channel_setting.basic.release_channel_setting_id
    setting_binding_id = "%{setting_binding_id}"
	location = "global"
	target = "projects/${data.google_project.project.number}"
    labels = %{labels}
    product = "%{product}"
}
`, context)
}
