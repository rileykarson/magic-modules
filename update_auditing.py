import re

file_path = "/usr/local/google/home/rileykarson/.gemini/jetski/brain/bf13e8a7-1ca8-4be4-8a35-f547bf1156ce/scratch/w-alpha-workload/mm-auditing/mmv1/third_party/terraform/services/container/resource_container_cluster.go.tmpl"
with open(file_path, "r") as f:
    content = f.read()

schema_search = """			"allow_net_admin": {
				Type: schema.TypeBool,
				Optional: true,
				Description: `Enable NET_ADMIN for this cluster.`,
            },"""
schema_replace = """			"workload_policy_config": {
				Type:        schema.TypeList,
				Optional:    true,
				MaxItems:    1,
				Description: `Configuration for cluster workload policy.`,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"allow_net_admin": {
							Type:        schema.TypeBool,
							Optional:    true,
							Description: `If true, workloads can use NET_ADMIN capability.`,
						},
						"autopilot_compatibility_auditing_enabled": {
							Type:        schema.TypeBool,
							Optional:    true,
							Description: `If true, Autopilot compatibility auditing will be enabled for the cluster.`,
						},
					},
				},
			},"""
content = content.replace(schema_search, schema_replace)

expand_search = """	var workloadPolicyConfig *container.WorkloadPolicyConfig
	if allowed := d.Get("allow_net_admin").(bool); allowed {
		workloadPolicyConfig = &container.WorkloadPolicyConfig{
			AllowNetAdmin: allowed,
		}
	}"""
expand_replace = """	var workloadPolicyConfig *container.WorkloadPolicyConfig
	if v, ok := d.GetOk("workload_policy_config"); ok {
		workloadPolicyConfig = expandWorkloadPolicyConfig(v)
	}"""
content = content.replace(expand_search, expand_replace)

expand_func = """
func expandWorkloadPolicyConfig(v interface{}) *container.WorkloadPolicyConfig {
	l := v.([]interface{})
	if len(l) == 0 || l[0] == nil {
		return nil
	}
	raw := l[0].(map[string]interface{})
	return &container.WorkloadPolicyConfig{
		AllowNetAdmin:                         raw["allow_net_admin"].(bool),
		AutopilotCompatibilityAuditingEnabled: raw["autopilot_compatibility_auditing_enabled"].(bool),
	}
}
"""
# insert before func expandDataplaneV2Config
content = content.replace("func expandDataplaneV2Config", expand_func + "\nfunc expandDataplaneV2Config")

flatten_search = """		if autopilot.WorkloadPolicyConfig != nil {
            if err := d.Set("allow_net_admin", autopilot.WorkloadPolicyConfig.AllowNetAdmin); err != nil {
                return fmt.Errorf("Error setting allow_net_admin: %s", err)
            }
        }"""
flatten_replace = """		if autopilot.WorkloadPolicyConfig != nil {
			if err := d.Set("workload_policy_config", flattenWorkloadPolicyConfig(autopilot.WorkloadPolicyConfig)); err != nil {
				return fmt.Errorf("Error setting workload_policy_config: %s", err)
			}
		}"""
content = content.replace(flatten_search, flatten_replace)

flatten_func = """
func flattenWorkloadPolicyConfig(v *container.WorkloadPolicyConfig) []interface{} {
	if v == nil {
		return nil
	}
	return []interface{}{
		map[string]interface{}{
			"allow_net_admin":                          v.AllowNetAdmin,
			"autopilot_compatibility_auditing_enabled": v.AutopilotCompatibilityAuditingEnabled,
		},
	}
}
"""
content = content.replace("func flattenDataplaneV2Config", flatten_func + "\nfunc flattenDataplaneV2Config")

update_search = """	if d.HasChange("allow_net_admin") {
		allowed := d.Get("allow_net_admin").(bool)
		req := &container.UpdateClusterRequest{
		    Update: &container.ClusterUpdate{
		        DesiredAutopilotWorkloadPolicyConfig: &container.WorkloadPolicyConfig{
					AllowNetAdmin: allowed,
				},
		    },
		}

	    updateF := updateFunc(req, "updating net admin for GKE autopilot workload policy config")
		// Call update serially.
		if err := updateF(); err != nil {
		    return err
	    }

		log.Printf("[INFO] GKE cluster %s's autopilot workload policy config allow_net_admin has been set to %v", d.Id(), allowed)
	}"""
update_replace = """	if d.HasChange("workload_policy_config") {
		req := &container.UpdateClusterRequest{
		    Update: &container.ClusterUpdate{
		        DesiredAutopilotWorkloadPolicyConfig: expandWorkloadPolicyConfig(d.Get("workload_policy_config")),
		    },
		}

	    updateF := updateFunc(req, "updating workload policy config for GKE cluster")
		if err := updateF(); err != nil {
		    return err
	    }
		log.Printf("[INFO] GKE cluster %s's workload policy config has been updated", d.Id())
	}"""
content = content.replace(update_search, update_replace)

with open(file_path, "w") as f:
    f.write(content)

# Test file update
test_path = "/usr/local/google/home/rileykarson/.gemini/jetski/brain/bf13e8a7-1ca8-4be4-8a35-f547bf1156ce/scratch/w-alpha-workload/mm-auditing/mmv1/third_party/terraform/services/container/resource_container_cluster_test.go.tmpl"
with open(test_path, "r") as f:
    test_content = f.read()

test_search = """  enable_autopilot = true
  allow_net_admin  = %t"""
test_replace = """  enable_autopilot = true
  workload_policy_config {
    allow_net_admin = %t
  }"""
test_content = test_content.replace(test_search, test_replace)

test_check_search = """					resource.TestCheckResourceAttr("google_container_cluster.primary", "allow_net_admin", "true"),"""
test_check_replace = """					resource.TestCheckResourceAttr("google_container_cluster.primary", "workload_policy_config.0.allow_net_admin", "true"),"""
test_content = test_content.replace(test_check_search, test_check_replace)

test_check_search_false = """					resource.TestCheckResourceAttr("google_container_cluster.primary", "allow_net_admin", "false"),"""
test_check_replace_false = """					resource.TestCheckResourceAttr("google_container_cluster.primary", "workload_policy_config.0.allow_net_admin", "false"),"""
test_content = test_content.replace(test_check_search_false, test_check_replace_false)

# Add auditing test
auditing_test = """
func TestAccContainerCluster_autopilotCompatibilityAuditing(t *testing.T) {
	t.Parallel()

	clusterName := fmt.Sprintf("tf-test-cluster-%s", acctest.RandString(t, 10))
	networkName := tpgcompute.BootstrapSharedTestNetwork(t, "gke-cluster")
	subnetworkName := tpgcompute.BootstrapSubnetInRegion(t, "gke-cluster", networkName, "us-east1", "10.79.0.0/20")

	acctest.VcrTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.AccTestPreCheck(t) },
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		CheckDestroy:             testAccCheckContainerClusterDestroyProducer(t),
		Steps: []resource.TestStep{
			{
				Config: testAccContainerCluster_workloadPolicyConfig(clusterName, networkName, subnetworkName, false),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("google_container_cluster.primary", "workload_policy_config.0.autopilot_compatibility_auditing_enabled", "false"),
				),
			},
			{
				Config: testAccContainerCluster_workloadPolicyConfig(clusterName, networkName, subnetworkName, true),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("google_container_cluster.primary", "workload_policy_config.0.autopilot_compatibility_auditing_enabled", "true"),
				),
			},
			{
				ResourceName:            "google_container_cluster.primary",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"deletion_protection"},
			},
		},
	})
}

func testAccContainerCluster_workloadPolicyConfig(name, networkName, subnetworkName string, enabled bool) string {
	return fmt.Sprintf(`
resource "google_container_cluster" "primary" {
  name             = "%s"
  location         = "us-east1"

  network          = "%s"
  subnetwork       = "%s"
  
  initial_node_count = 1

  workload_policy_config {
    autopilot_compatibility_auditing_enabled = %t
  }

  deletion_protection = false
}
`, name, networkName, subnetworkName, enabled)
}
"""
test_content += auditing_test

with open(test_path, "w") as f:
    f.write(test_content)

meta_path = "/usr/local/google/home/rileykarson/.gemini/jetski/brain/bf13e8a7-1ca8-4be4-8a35-f547bf1156ce/scratch/w-alpha-workload/mm-auditing/mmv1/third_party/terraform/services/container/resource_container_cluster_meta.yaml.tmpl"
with open(meta_path, "r") as f:
    meta_content = f.read()

if "workload_policy_config" not in meta_content:
    meta_search = "        - addons_config"
    meta_replace = "        - workload_policy_config\n        - addons_config"
    meta_content = meta_content.replace(meta_search, meta_replace)
    # remove allow_net_admin
    meta_content = meta_content.replace("        - allow_net_admin\n", "")
    with open(meta_path, "w") as f:
        f.write(meta_content)

docs_path = "/usr/local/google/home/rileykarson/.gemini/jetski/brain/bf13e8a7-1ca8-4be4-8a35-f547bf1156ce/scratch/w-alpha-workload/mm-auditing/mmv1/third_party/terraform/website/docs/r/container_cluster.html.markdown"
with open(docs_path, "r") as f:
    docs_content = f.read()

docs_search = """* `allow_net_admin` - (Optional) Enable NET_ADMIN for this cluster."""
docs_replace = """* `workload_policy_config` - (Optional) Configuration for cluster workload policy. Structure is documented below.

The `workload_policy_config` block supports:

* `allow_net_admin` - (Optional) If true, workloads can use NET_ADMIN capability.

* `autopilot_compatibility_auditing_enabled` - (Optional) If true, Autopilot compatibility auditing will be enabled for the cluster."""
docs_content = docs_content.replace(docs_search, docs_replace)

with open(docs_path, "w") as f:
    f.write(docs_content)

