package identityplatform_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/hashicorp/terraform-provider-google/google/acctest"
	"github.com/hashicorp/terraform-provider-google/google/envvar"
	_ "github.com/hashicorp/terraform-provider-google/google/services/identityplatform"
	_ "github.com/hashicorp/terraform-provider-google/google/services/resourcemanager"
)

func TestAccIdentityPlatformConfig_update(t *testing.T) {
	t.Parallel()

	orgId := envvar.GetTestOrgFromEnv(t)
	billingAcct := envvar.GetTestBillingAccountFromEnv(t)
	quotaStartTime := time.Now().AddDate(0, 0, 1).Format(time.RFC3339)
	randomSuffix := acctest.RandString(t, 10)
	context := map[string]interface{}{
		"org_id":           orgId,
		"billing_acct":     billingAcct,
		"quota_start_time": quotaStartTime,
		"random_suffix":    randomSuffix,
	}

	acctest.VcrTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.AccTestPreCheck(t) },
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		Steps: []resource.TestStep{
			{
				Config: testAccIdentityPlatformConfig_basic(map[string]interface{}{
					"org_id":                  orgId,
					"billing_acct":            billingAcct,
					"quota_start_time":        quotaStartTime,
					"random_suffix":           randomSuffix,
					"allow_tenants":           true,
					"default_tenant_location": fmt.Sprintf("organizations/%s", orgId),
				}),
			},
			{
				ResourceName:            "google_identity_platform_config.basic",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"client.0.api_key", "client.0.firebase_subdomain"},
			},
			{
				Config: testAccIdentityPlatformConfig_basic(map[string]interface{}{
					"org_id":                  orgId,
					"billing_acct":            billingAcct,
					"quota_start_time":        quotaStartTime,
					"random_suffix":           randomSuffix,
					"allow_tenants":           false,
					"default_tenant_location": nil,
				}),
			},
			{
				ResourceName:            "google_identity_platform_config.basic",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"client.0.api_key", "client.0.firebase_subdomain"},
			},
			{
				Config: testAccIdentityPlatformConfig_update(context),
			},
			{
				ResourceName:            "google_identity_platform_config.basic",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"client.0.api_key", "client.0.firebase_subdomain"},
			},
		},
	})
}

func testAccIdentityPlatformConfig_basic(context map[string]interface{}) string {
	return acctest.Nprintf(`
resource "google_project" "basic" {
  project_id = "tf-test-my-project%{random_suffix}"
  name       = "tf-test-my-project%{random_suffix}"
  org_id     = "%{org_id}"
  billing_account =  "%{billing_acct}"
  deletion_policy = "DELETE"
  labels = {
    firebase = "enabled"
  }
}

resource "google_project_service" "identitytoolkit" {
  project = google_project.basic.project_id
  service = "identitytoolkit.googleapis.com"
}

resource "google_identity_platform_config" "basic" {
  project = google_project.basic.project_id
  autodelete_anonymous_users = true
  sign_in {
    allow_duplicate_emails = true

    anonymous {
        enabled = true
    }
    email {
        enabled = true
        password_required = false
    }
    phone_number {
        enabled = true
        test_phone_numbers = {
            "+11231231234" = "000000"
        }
    }
  }
  sms_region_config {
    allow_by_default {
      disallowed_regions = [
        "CA",
        "US",
      ]
    }
  }

  client {
    permissions {
      disabled_user_deletion = true
      disabled_user_signup   = true
    }
  }

  mfa {
    enabled_providers = ["PHONE_SMS"]
    provider_configs {
      state = "ENABLED"
      totp_provider_config {
        adjacent_intervals = 3
      }
    }
    state = "ENABLED"
  }

  monitoring {
    request_logging {
      enabled = true
    }
  }

  multi_tenant {
    allow_tenants           = %{allow_tenants}
    default_tenant_location = "%{default_tenant_location}"
  }

  depends_on = [google_project_service.identitytoolkit]
}
`, context)
}

func testAccIdentityPlatformConfig_update(context map[string]interface{}) string {
	return acctest.Nprintf(`
resource "google_project" "basic" {
  project_id = "tf-test-my-project%{random_suffix}"
  name       = "tf-test-my-project%{random_suffix}"
  org_id     = "%{org_id}"
  billing_account =  "%{billing_acct}"
  deletion_policy = "DELETE"
  labels = {
    firebase = "enabled"
  }
}

resource "google_project_service" "identitytoolkit" {
  project = google_project.basic.project_id
  service = "identitytoolkit.googleapis.com"
}

resource "google_identity_platform_config" "basic" {
  project = google_project.basic.project_id
  sign_in {
    allow_duplicate_emails = false

    anonymous {
        enabled = false
    }
    email {
        enabled = true
        password_required = true
    }
    phone_number {
        enabled = true
        test_phone_numbers = {
	    "+17651212343" = "111111"
        }
    }
  }
  sms_region_config {
    allowlist_only {
      allowed_regions = [
        "AU",
        "NZ",
      ]
    }
  }

  client {
    permissions {
      disabled_user_deletion = false
      disabled_user_signup   = false
    }
  }

  mfa {
    enabled_providers = ["PHONE_SMS"]
    state = "DISABLED"
  }
  monitoring {
    request_logging {
      enabled = false
    }
  }

  depends_on = [google_project_service.identitytoolkit]
}
`, context)
}
