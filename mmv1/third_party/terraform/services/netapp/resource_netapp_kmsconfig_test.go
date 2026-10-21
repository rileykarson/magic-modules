package netapp_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/hashicorp/terraform-provider-google/google/acctest"
	"github.com/hashicorp/terraform-provider-google/google/services/kms"
	_ "github.com/hashicorp/terraform-provider-google/google/services/netapp"
)

func TestAccNetappkmsconfig_kmsConfigCreateExample_Update(t *testing.T) {
	// t.Parallel()

	randomSuffix := acctest.RandString(t, 10)
	kmsKeyName := kms.BootstrapKMSKeyWithPurposeInLocationAndName(t, "ENCRYPT_DECRYPT", "us-east4", "tf-bootstrap-netapp-kmsconfig-key2").CryptoKey.Name

	acctest.VcrTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.AccTestPreCheck(t) },
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		CheckDestroy:             testAccCheckNetappkmsconfigDestroyProducer(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNetappkmsconfig_kmsConfigCreateExample_Full(map[string]interface{}{
					"random_suffix": randomSuffix,
					"kms_key_name":  kmsKeyName,
					"description":   "this is a test description",
					"labels":        nil,
				}),
			},
			{
				ResourceName:            "google_netapp_kmsconfig.kmsConfig",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"location", "name", "labels", "terraform_labels"},
			},
			{
				Config: testAccNetappkmsconfig_kmsConfigCreateExample_Full(map[string]interface{}{
					"random_suffix": randomSuffix,
					"kms_key_name":  kmsKeyName,
					"description":   "kmsconfig update",
					"labels":        `{"foo" = "bar"}`,
				}),
			},
			{
				ResourceName:            "google_netapp_kmsconfig.kmsConfig",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"location", "name", "labels", "terraform_labels"},
			},
		},
	})
}

func testAccNetappkmsconfig_kmsConfigCreateExample_Full(context map[string]interface{}) string {
	return acctest.Nprintf(` 
	resource "google_netapp_kmsconfig" "kmsConfig" {
		name = "tf-test-kms-test%{random_suffix}"
		description="%{description}"
		crypto_key_name="%{kms_key_name}"
		location="us-east4"
		labels = %{labels}
	}
`, context)
}
