package apigee_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/hashicorp/terraform-provider-google/google/acctest"
	"github.com/hashicorp/terraform-provider-google/google/envvar"
	"github.com/hashicorp/terraform-provider-google/google/services/apigee"
	_ "github.com/hashicorp/terraform-provider-google/google/services/compute"
	_ "github.com/hashicorp/terraform-provider-google/google/services/resourcemanager"
	_ "github.com/hashicorp/terraform-provider-google/google/services/servicenetworking"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func testAccCheckApigeeSecurityActionDestroyProducer(t *testing.T) func(s *terraform.State) error {
	return func(s *terraform.State) error {
		for name, rs := range s.RootModule().Resources {
			if rs.Type != "google_apigee_security_action" {
				continue
			}
			if strings.HasPrefix(name, "data.") {
				continue
			}

			config := acctest.GoogleProviderConfig(t)

			url, err := tpgresource.ReplaceVarsForTest(config, rs, transport_tpg.BaseUrl(apigee.Product, config)+"organizations/{{org_id}}/environments/{{env_id}}/securityActions/{{security_action_id}}")
			if err != nil {
				return err
			}

			billingProject := ""

			if config.BillingProject != "" {
				billingProject = config.BillingProject
			}

			_, err = transport_tpg.SendRequest(transport_tpg.SendRequestOptions{
				Config:    config,
				Method:    "GET",
				Project:   billingProject,
				RawURL:    url,
				UserAgent: config.UserAgent,
			})
			if err == nil {
				return fmt.Errorf("ApigeeSecurityAction still exists at %s", url)
			}
		}

		return nil
	}
}

// TestAccApigeeSecurityAction_update verifies that mutable fields can be updated
// in place (via PATCH) without destroying and recreating the resource, now that
// the Apigee Security Actions API supports mutations.
func TestAccApigeeSecurityAction_update(t *testing.T) {
	acctest.SkipIfVcr(t)
	t.Parallel()

	context := map[string]interface{}{
		"billing_account": envvar.GetTestBillingAccountFromEnv(t),
		"org_id":          envvar.GetTestOrgFromEnv(t),
		"random_suffix":   acctest.RandString(t, 10),
	}

	contextBaseCondition := map[string]interface{}{
		"billing_account": context["billing_account"],
		"org_id":          context["org_id"],
		"random_suffix":   context["random_suffix"],
		"http_methods":    nil,
		"api_keys":        nil,
		"access_tokens":   nil,
		"api_products":    nil,
		"developer_apps":  nil,
		"developers":      nil,
		"user_agents":     nil,
		"region_codes":    nil,
		"asns":            nil,
		"expire_time":     `"2032-12-31T23:59:59Z"`,
		"ttl":             nil,
	}

	contextHttpMethods := map[string]interface{}{}
	for k, v := range contextBaseCondition {
		contextHttpMethods[k] = v
	}
	contextHttpMethods["http_methods"] = `[ "GET", "POST", "PUT" ]`

	contextApiKeys := map[string]interface{}{}
	for k, v := range contextBaseCondition {
		contextApiKeys[k] = v
	}
	contextApiKeys["api_keys"] = `[ "foo-key", "bar-key" ]`

	contextAccessTokens := map[string]interface{}{}
	for k, v := range contextBaseCondition {
		contextAccessTokens[k] = v
	}
	contextAccessTokens["access_tokens"] = `[ "foo-token", "bar-token" ]`

	contextApiProducts := map[string]interface{}{}
	for k, v := range contextBaseCondition {
		contextApiProducts[k] = v
	}
	contextApiProducts["api_products"] = `[ "foo-product", "bar-product" ]`

	contextDeveloperApps := map[string]interface{}{}
	for k, v := range contextBaseCondition {
		contextDeveloperApps[k] = v
	}
	contextDeveloperApps["developer_apps"] = `[ "foo-app", "bar-app" ]`

	contextDevelopers := map[string]interface{}{}
	for k, v := range contextBaseCondition {
		contextDevelopers[k] = v
	}
	contextDevelopers["developers"] = `[ "foo-developer", "bar-developer" ]`

	contextUserAgents := map[string]interface{}{}
	for k, v := range contextBaseCondition {
		contextUserAgents[k] = v
	}
	contextUserAgents["user_agents"] = `[ "Mozilla/5.0", "curl/7.64.1" ]`

	contextRegionCodes := map[string]interface{}{}
	for k, v := range contextBaseCondition {
		contextRegionCodes[k] = v
	}
	contextRegionCodes["region_codes"] = `[ "US", "CA" ]`

	contextAsns := map[string]interface{}{}
	for k, v := range contextBaseCondition {
		contextAsns[k] = v
	}
	contextAsns["asns"] = `[ "23", "42" ]`

	contextTTL := map[string]interface{}{}
	for k, v := range contextBaseCondition {
		contextTTL[k] = v
	}
	contextTTL["asns"] = `[ "23", "42" ]`
	contextTTL["expire_time"] = nil
	contextTTL["ttl"] = `"3600s"`

	resourceName := "google_apigee_security_action.default"

	acctest.VcrTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.AccTestPreCheck(t) },
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		ExternalProviders: map[string]resource.ExternalProvider{
			"time": {},
		},
		CheckDestroy: testAccCheckApigeeSecurityActionDestroyProducer(t),
		Steps: []resource.TestStep{
			{
				Config: testAccApigeeSecurityAction_updateBefore(context),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "description", "Apigee Security Action"),
					resource.TestCheckResourceAttr(resourceName, "state", "ENABLED"),
					resource.TestCheckResourceAttr(resourceName, "api_proxies.#", "1"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// Modify mutable fields: description and the deny response code
				// are updated via PATCH; state (ENABLED -> DISABLED) is updated
				// via the dedicated :disable endpoint. The resource must be
				// updated in place (not recreated).
				Config: testAccApigeeSecurityAction_updateAfter(context),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "description", "Apigee Security Action updated"),
					resource.TestCheckResourceAttr(resourceName, "state", "DISABLED"),
					resource.TestCheckResourceAttr(resourceName, "deny.0.response_code", "429"),
					resource.TestCheckResourceAttr(resourceName, "api_proxies.#", "1"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccApigeeSecurityAction_updateBefore(context map[string]interface{}) string {
	return testAccApigeeSecurityAction_apigeeBase(context) + acctest.Nprintf(`
resource "google_apigee_security_action" "default" {
    security_action_id = "tf-test-%{random_suffix}"
    org_id             = google_apigee_organization.apigee_org.name
    env_id             = google_apigee_environment.env.name
    description        = "Apigee Security Action"
    state              = "ENABLED"
    api_proxies        = [google_apigee_api.proxy.name]

    condition_config {
        ip_address_ranges = [
            "100.0.220.1",
            "200.0.0.1",
        ]
    }

    deny {
        response_code = 403
    }

    expire_time = "2032-12-31T23:59:59Z"
    depends_on  = [
        google_apigee_addons_config.apigee_org_security_addons_config
    ]
}
`, context)
}

func testAccApigeeSecurityAction_updateAfter(context map[string]interface{}) string {
	return testAccApigeeSecurityAction_apigeeBase(context) + acctest.Nprintf(`
resource "google_apigee_security_action" "default" {
    security_action_id = "tf-test-%{random_suffix}"
    org_id             = google_apigee_organization.apigee_org.name
    env_id             = google_apigee_environment.env.name
    description        = "Apigee Security Action updated"
    state              = "DISABLED"
    api_proxies        = [google_apigee_api.proxy.name]

    condition_config {
        ip_address_ranges = [
            "100.0.220.1",
            "200.0.0.1",
        ]
    }

    deny {
        response_code = 429
    }

    expire_time = "2032-12-31T23:59:59Z"
    depends_on  = [
        google_apigee_addons_config.apigee_org_security_addons_config
    ]
}
`, context)
}

func TestAccApigeeSecurityAction_apigeeSecurityActionFull(t *testing.T) {
	acctest.SkipIfVcr(t)
	t.Parallel()

	context := map[string]interface{}{
		"billing_account": envvar.GetTestBillingAccountFromEnv(t),
		"org_id":          envvar.GetTestOrgFromEnv(t),
		"random_suffix":   acctest.RandString(t, 10),
	}

	contextBaseCondition := map[string]interface{}{
		"billing_account": context["billing_account"],
		"org_id":          context["org_id"],
		"random_suffix":   context["random_suffix"],
		"http_methods":    nil,
		"api_keys":        nil,
		"access_tokens":   nil,
		"api_products":    nil,
		"developer_apps":  nil,
		"developers":      nil,
		"user_agents":     nil,
		"region_codes":    nil,
		"asns":            nil,
		"expire_time":     `"2032-12-31T23:59:59Z"`,
		"ttl":             nil,
	}

	contextHttpMethods := map[string]interface{}{}
	for k, v := range contextBaseCondition {
		contextHttpMethods[k] = v
	}
	contextHttpMethods["http_methods"] = `[ "GET", "POST", "PUT" ]`

	contextApiKeys := map[string]interface{}{}
	for k, v := range contextBaseCondition {
		contextApiKeys[k] = v
	}
	contextApiKeys["api_keys"] = `[ "foo-key", "bar-key" ]`

	contextAccessTokens := map[string]interface{}{}
	for k, v := range contextBaseCondition {
		contextAccessTokens[k] = v
	}
	contextAccessTokens["access_tokens"] = `[ "foo-token", "bar-token" ]`

	contextApiProducts := map[string]interface{}{}
	for k, v := range contextBaseCondition {
		contextApiProducts[k] = v
	}
	contextApiProducts["api_products"] = `[ "foo-product", "bar-product" ]`

	contextDeveloperApps := map[string]interface{}{}
	for k, v := range contextBaseCondition {
		contextDeveloperApps[k] = v
	}
	contextDeveloperApps["developer_apps"] = `[ "foo-app", "bar-app" ]`

	contextDevelopers := map[string]interface{}{}
	for k, v := range contextBaseCondition {
		contextDevelopers[k] = v
	}
	contextDevelopers["developers"] = `[ "foo-developer", "bar-developer" ]`

	contextUserAgents := map[string]interface{}{}
	for k, v := range contextBaseCondition {
		contextUserAgents[k] = v
	}
	contextUserAgents["user_agents"] = `[ "Mozilla/5.0", "curl/7.64.1" ]`

	contextRegionCodes := map[string]interface{}{}
	for k, v := range contextBaseCondition {
		contextRegionCodes[k] = v
	}
	contextRegionCodes["region_codes"] = `[ "US", "CA" ]`

	contextAsns := map[string]interface{}{}
	for k, v := range contextBaseCondition {
		contextAsns[k] = v
	}
	contextAsns["asns"] = `[ "23", "42" ]`

	contextTTL := map[string]interface{}{}
	for k, v := range contextBaseCondition {
		contextTTL[k] = v
	}
	contextTTL["asns"] = `[ "23", "42" ]`
	contextTTL["expire_time"] = nil
	contextTTL["ttl"] = `"3600s"`

	acctest.VcrTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.AccTestPreCheck(t) },
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		ExternalProviders: map[string]resource.ExternalProvider{
			"time": {},
		},
		CheckDestroy: testAccCheckApigeeSecurityActionDestroyProducer(t),
		/* allow, deny and flag are mutually exclusive, so we test them in sequence */
		/* also all conditions except ip_address_ranges and bot_reasons seem to be mutually exclusive, so we test them in sequence */
		Steps: []resource.TestStep{
			{
				Config: testAccApigeeSecurityAction_apigeeSecurityActionFullAllow(context),
			},
			{
				ResourceName:      "google_apigee_security_action.default",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccApigeeSecurityAction_apigeeSecurityActionFullDeny(context),
			},
			{
				ResourceName:      "google_apigee_security_action.default",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccApigeeSecurityAction_apigeeSecurityActionFullCondition(contextHttpMethods),
			},
			{
				ResourceName:      "google_apigee_security_action.default",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccApigeeSecurityAction_apigeeSecurityActionFullFlag(context),
			},
			{
				ResourceName:      "google_apigee_security_action.default",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccApigeeSecurityAction_apigeeSecurityActionFullCondition(contextApiKeys),
			},
			{
				ResourceName:      "google_apigee_security_action.default",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccApigeeSecurityAction_apigeeSecurityActionFullCondition(contextAccessTokens),
			},
			{
				ResourceName:      "google_apigee_security_action.default",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccApigeeSecurityAction_apigeeSecurityActionFullCondition(contextApiProducts),
			},

			{
				ResourceName:      "google_apigee_security_action.default",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccApigeeSecurityAction_apigeeSecurityActionFullCondition(contextDeveloperApps),
			},
			{
				ResourceName:      "google_apigee_security_action.default",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccApigeeSecurityAction_apigeeSecurityActionFullCondition(contextDevelopers),
			},
			{
				ResourceName:      "google_apigee_security_action.default",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccApigeeSecurityAction_apigeeSecurityActionFullCondition(contextUserAgents),
			},
			{
				ResourceName:      "google_apigee_security_action.default",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccApigeeSecurityAction_apigeeSecurityActionFullCondition(contextRegionCodes),
			},
			{
				ResourceName:      "google_apigee_security_action.default",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccApigeeSecurityAction_apigeeSecurityActionFullCondition(contextAsns),
			},
			{
				ResourceName:      "google_apigee_security_action.default",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config:             testAccApigeeSecurityAction_apigeeSecurityActionFullCondition(contextTTL),
				ExpectNonEmptyPlan: true, // ttl change enforces recreation of the resource
			},
		},
	})
}

func testAccApigeeSecurityAction_apigeeBase(context map[string]interface{}) string {
	return acctest.Nprintf(`
resource "google_project" "project" {
  project_id      = "tf-test-%{random_suffix}"
  name            = "tf-test-%{random_suffix}"
  org_id          = "%{org_id}"
  billing_account = "%{billing_account}"
  deletion_policy = "DELETE"
}

resource "time_sleep" "wait_60_seconds" {
  create_duration = "60s"
  depends_on = [google_project.project]
}

resource "google_project_service" "apigee" {
  project = google_project.project.project_id
  service = "apigee.googleapis.com"
  depends_on = [time_sleep.wait_60_seconds]
}

resource "google_project_service" "compute" {
  project = google_project.project.project_id
  service = "compute.googleapis.com"
  depends_on = [google_project_service.apigee]
}

resource "google_project_service" "servicenetworking" {
  project = google_project.project.project_id
  service = "servicenetworking.googleapis.com"
  depends_on = [google_project_service.compute]
}

resource "time_sleep" "wait_300_seconds" {
  create_duration = "300s"
  depends_on = [google_project_service.servicenetworking]
}

resource "google_compute_network" "apigee_network" {
    name = "tf-test-network-%{random_suffix}"
    depends_on = [time_sleep.wait_300_seconds]
}

resource "google_compute_global_address" "apigee_range" {
    name          = "tf-test-address-%{random_suffix}"
    purpose       = "VPC_PEERING"
    address_type  = "INTERNAL"
    prefix_length = 16
    network       = google_compute_network.apigee_network.id
}

resource "google_service_networking_connection" "apigee_vpc_connection" {
    network                 = google_compute_network.apigee_network.id
    service                 = "servicenetworking.googleapis.com"
    reserved_peering_ranges = [google_compute_global_address.apigee_range.name]
}

resource "google_apigee_organization" "apigee_org" {
    analytics_region   = "us-central1"
    project_id         = google_project.project.project_id
    authorized_network = google_compute_network.apigee_network.id
    depends_on         = [google_service_networking_connection.apigee_vpc_connection]
}

resource "google_apigee_environment" "env" {
    name         = "tf-test-env-%{random_suffix}"
    description  = "Apigee Environment"
    display_name = "environment-1"
    org_id       = google_apigee_organization.apigee_org.id
}

resource "google_apigee_addons_config" "apigee_org_security_addons_config" {
    org = google_apigee_organization.apigee_org.name
    addons_config {
        api_security_config {
            enabled = true
        }
    }
}

resource "google_apigee_api" "proxy" {
    name          = "tf-test-proxy-%{random_suffix}"
    org_id        = google_apigee_organization.apigee_org.name
    config_bundle = "./test-fixtures/apigee_api_bundle.zip"
}
`, context)
}

func testAccApigeeSecurityAction_apigeeSecurityActionFullAllow(context map[string]interface{}) string {
	return testAccApigeeSecurityAction_apigeeBase(context) + acctest.Nprintf(`
resource "google_apigee_security_action" "default" {
    security_action_id = "tf-test-%{random_suffix}"
    org_id             = google_apigee_organization.apigee_org.name
    env_id             = google_apigee_environment.env.name
    description        = "Apigee Security Action"
    state              = "ENABLED"

    condition_config {
        ip_address_ranges = [
            "100.0.220.1",
            "200.0.0.1",
        ]

        bot_reasons = [
            "Flooder",
            "Public Cloud Azure",
            "Public Cloud AWS",
        ]
    }

    allow {}

    expire_time = "2032-12-31T23:59:59Z"
    depends_on  = [
        google_apigee_addons_config.apigee_org_security_addons_config
    ]
}
`, context)
}

func testAccApigeeSecurityAction_apigeeSecurityActionFullFlag(context map[string]interface{}) string {
	return testAccApigeeSecurityAction_apigeeBase(context) + acctest.Nprintf(`
resource "google_apigee_security_action" "default" {
    security_action_id = "tf-test-%{random_suffix}"
    org_id             = google_apigee_organization.apigee_org.name
    env_id             = google_apigee_environment.env.name
    description        = "Apigee Security Action"
    state              = "ENABLED"

    condition_config {
        ip_address_ranges = [
            "100.0.220.1",
            "200.0.0.1",
        ]

        bot_reasons = [
            "Flooder",
            "Public Cloud Azure",
            "Public Cloud AWS",
        ]
    }

    flag {
        headers {
			name  = "X-Flag-Header"
			value = "flag-value"
		}
        headers {
			name  = "X-Flag-Header-2"
			value = "flag-value-2"
		}
    }

    expire_time = "2032-12-31T23:59:59Z"
    depends_on  = [
        google_apigee_addons_config.apigee_org_security_addons_config
    ]
}
`, context)
}

func testAccApigeeSecurityAction_apigeeSecurityActionFullDeny(context map[string]interface{}) string {
	return testAccApigeeSecurityAction_apigeeBase(context) + acctest.Nprintf(`
resource "google_apigee_security_action" "default" {
    security_action_id = "tf-test-%{random_suffix}"
    org_id             = google_apigee_organization.apigee_org.name
    env_id             = google_apigee_environment.env.name
    description        = "Apigee Security Action"
    state              = "ENABLED"

    condition_config {
        ip_address_ranges = [
            "100.0.220.1",
            "200.0.0.1",
        ]

        bot_reasons = [
            "Flooder",
            "Public Cloud Azure",
            "Public Cloud AWS",
        ]
    }
	
	deny {
		response_code = 403
	}

    expire_time = "2032-12-31T23:59:59Z"
    depends_on  = [
        google_apigee_addons_config.apigee_org_security_addons_config
    ]
}
`, context)
}

func testAccApigeeSecurityAction_apigeeSecurityActionFullCondition(context map[string]interface{}) string {
	return testAccApigeeSecurityAction_apigeeBase(context) + acctest.Nprintf(`
resource "google_apigee_security_action" "default" {
    security_action_id = "tf-test-%{random_suffix}"
    org_id             = google_apigee_organization.apigee_org.name
    env_id             = google_apigee_environment.env.name
    description        = "Apigee Security Action"
    state              = "ENABLED"

    condition_config {
        http_methods   = %{http_methods}
        api_keys       = %{api_keys}
        access_tokens  = %{access_tokens}
        api_products   = %{api_products}
        developer_apps = %{developer_apps}
        developers     = %{developers}
        user_agents    = %{user_agents}
        region_codes   = %{region_codes}
        asns           = %{asns}
    }
	
	deny {
		response_code = 403
	}

    expire_time = %{expire_time}
    ttl         = %{ttl}

    depends_on  = [
        google_apigee_addons_config.apigee_org_security_addons_config
    ]
}
`, context)
}
