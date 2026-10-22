package container

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func testNodeConfigResourceSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"node_config": schemaNodeConfig(),
	}
}

func testNodeConfigDiffResourceData(t *testing.T, oldNodeConfig, newNodeConfig map[string]interface{}) *schema.ResourceData {
	t.Helper()
	s := testNodeConfigResourceSchema()

	oldRaw := map[string]interface{}{
		"node_config": []interface{}{oldNodeConfig},
	}
	dOld := schema.TestResourceDataRaw(t, s, oldRaw)
	dOld.SetId("projects/test-proj/locations/us-central1/clusters/test-cluster/nodePools/np-1")
	state := dOld.State()

	newRaw := map[string]interface{}{
		"node_config": []interface{}{newNodeConfig},
	}
	diff, err := schema.InternalMap(s).Diff(context.Background(), state, terraform.NewResourceConfigRaw(newRaw), nil, nil, false)
	if err != nil {
		t.Fatalf("unexpected diff error: %v", err)
	}
	if diff == nil {
		diff = &terraform.InstanceDiff{Attributes: map[string]*terraform.ResourceAttrDiff{}}
	}
	// For Optional+Computed TypeList blocks (e.g. containerd_config, confidential_nodes),
	// schema.InternalMap.Diff suppresses list-length clearing from raw maps; populate the
	// explicit 1 -> 0 diff entry when a previously populated list block is removed in newNodeConfig.
	for k, oldVal := range oldNodeConfig {
		oldSlice, isSlice := oldVal.([]interface{})
		if !isSlice || len(oldSlice) == 0 {
			continue
		}
		newVal, exists := newNodeConfig[k]
		newSlice, _ := newVal.([]interface{})
		if !exists || len(newSlice) == 0 {
			attrKey := "node_config.0." + k + ".#"
			if _, hasDiff := diff.Attributes[attrKey]; !hasDiff {
				diff.Attributes[attrKey] = &terraform.ResourceAttrDiff{
					Old: "1",
					New: "0",
				}
			}
		}
	}
	d, err := schema.InternalMap(s).Data(state, diff)
	if err != nil {
		t.Fatalf("unexpected data error: %v", err)
	}
	return d
}

func runNodePoolNodeConfigUpdateCapture(t *testing.T, d *schema.ResourceData) []map[string]interface{} {
	t.Helper()
	var capturedBodies []map[string]interface{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("failed reading request body: %v", err)
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &payload); err != nil {
				t.Errorf("failed unmarshaling request body %q: %v", string(bodyBytes), err)
			}
			capturedBodies = append(capturedBodies, payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"op-1","status":"DONE"}`))
	}))
	defer ts.Close()

	cfg := &transport_tpg.Config{
		Context:         context.Background(),
		Client:          ts.Client(),
		CustomEndpoints: map[string]string{"container_custom_endpoint": ts.URL + "/v1/"},
		PollInterval:    time.Millisecond,
	}
	npInfo := &NodePoolInformation{
		project:  "test-proj",
		location: "us-central1",
		cluster:  "test-cluster",
	}

	if err := nodePoolNodeConfigUpdate(d, cfg, npInfo, "", "np-1", time.Minute); err != nil {
		t.Fatalf("nodePoolNodeConfigUpdate failed: %v", err)
	}
	return capturedBodies
}

func marshalToMap(t *testing.T, v interface{}) map[string]interface{} {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	return m
}

func TestNodePoolNodeConfigUpdate_BlockRemoval(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		oldConfig map[string]interface{}
		newConfig map[string]interface{}
		wantBody  map[string]interface{}
	}{
		{
			name: "remove taint_config block sends empty taintConfig",
			oldConfig: map[string]interface{}{
				"taint_config": []interface{}{
					map[string]interface{}{
						"architecture_taint_behavior": "ARM",
					},
				},
			},
			newConfig: map[string]interface{}{},
			wantBody: map[string]interface{}{
				"name":        "np-1",
				"taintConfig": map[string]interface{}{},
			},
		},
		{
			name: "remove containerd_config block sends empty containerdConfig",
			oldConfig: map[string]interface{}{
				"containerd_config": []interface{}{
					map[string]interface{}{
						"writable_cgroups": []interface{}{
							map[string]interface{}{
								"enabled": true,
							},
						},
					},
				},
			},
			newConfig: map[string]interface{}{},
			wantBody: map[string]interface{}{
				"name":             "np-1",
				"containerdConfig": map[string]interface{}{},
			},
		},
		{
			name: "remove confidential_nodes block sends enabled false",
			oldConfig: map[string]interface{}{
				"confidential_nodes": []interface{}{
					map[string]interface{}{
						"enabled":                    true,
						"confidential_instance_type": "SEV",
					},
				},
			},
			newConfig: map[string]interface{}{},
			wantBody: map[string]interface{}{
				"name": "np-1",
				"confidentialNodes": map[string]interface{}{
					"enabled": false,
				},
			},
		},
		{
			name: "set confidential_nodes enabled false sends enabled false",
			oldConfig: map[string]interface{}{
				"confidential_nodes": []interface{}{
					map[string]interface{}{
						"enabled": true,
					},
				},
			},
			newConfig: map[string]interface{}{
				"confidential_nodes": []interface{}{
					map[string]interface{}{
						"enabled": false,
					},
				},
			},
			wantBody: map[string]interface{}{
				"name": "np-1",
				"confidentialNodes": map[string]interface{}{
					"enabled": false,
				},
			},
		},
		{
			name: "update logging_variant sends variantConfig",
			oldConfig: map[string]interface{}{
				"logging_variant": "DEFAULT",
			},
			newConfig: map[string]interface{}{
				"logging_variant": "MAX_THROUGHPUT",
			},
			wantBody: map[string]interface{}{
				"name": "np-1",
				"loggingConfig": map[string]interface{}{
					"variantConfig": map[string]interface{}{
						"variant": "MAX_THROUGHPUT",
					},
				},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := testNodeConfigDiffResourceData(t, tc.oldConfig, tc.newConfig)
			bodies := runNodePoolNodeConfigUpdateCapture(t, d)
			if len(bodies) != 1 {
				t.Fatalf("expected 1 UpdateNodePool request, got %d: %#v", len(bodies), bodies)
			}
			if !reflect.DeepEqual(bodies[0], tc.wantBody) {
				t.Errorf("got request body %#v, want %#v", bodies[0], tc.wantBody)
			}
		})
	}
}

func TestNodePoolNodeConfigUpdate_ClearMapsAndSlices(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		oldConfig map[string]interface{}
		newConfig map[string]interface{}
		wantBody  map[string]interface{}
	}{
		{
			name: "clear resource_labels sends empty labels map",
			oldConfig: map[string]interface{}{
				"resource_labels": map[string]interface{}{
					"env": "prod",
				},
			},
			newConfig: map[string]interface{}{},
			wantBody: map[string]interface{}{
				"name": "np-1",
				"resourceLabels": map[string]interface{}{
					"labels": map[string]interface{}{},
				},
			},
		},
		{
			name: "clear labels sends empty labels map",
			oldConfig: map[string]interface{}{
				"labels": map[string]interface{}{
					"app": "web",
				},
			},
			newConfig: map[string]interface{}{
				"labels": map[string]interface{}{},
			},
			wantBody: map[string]interface{}{
				"name": "np-1",
				"labels": map[string]interface{}{
					"labels": map[string]interface{}{},
				},
			},
		},
		{
			name: "clear taint sends empty taints slice",
			oldConfig: map[string]interface{}{
				"taint": []interface{}{
					map[string]interface{}{
						"key":    "dedicated",
						"value":  "gpu",
						"effect": "NO_SCHEDULE",
					},
				},
			},
			newConfig: map[string]interface{}{
				"taint": []interface{}{},
			},
			wantBody: map[string]interface{}{
				"name": "np-1",
				"taints": map[string]interface{}{
					"taints": []interface{}{},
				},
			},
		},
		{
			name: "clear tags sends empty tags slice",
			oldConfig: map[string]interface{}{
				"tags": []interface{}{"tag-a", "tag-b"},
			},
			newConfig: map[string]interface{}{},
			wantBody: map[string]interface{}{
				"name": "np-1",
				"tags": map[string]interface{}{
					"tags": []interface{}{},
				},
			},
		},
		{
			name: "clear resource_manager_tags sends empty tags map",
			oldConfig: map[string]interface{}{
				"resource_manager_tags": map[string]interface{}{
					"tagKeys/123": "tagValues/456",
				},
			},
			newConfig: map[string]interface{}{},
			wantBody: map[string]interface{}{
				"name": "np-1",
				"resourceManagerTags": map[string]interface{}{
					"tags": map[string]interface{}{},
				},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := testNodeConfigDiffResourceData(t, tc.oldConfig, tc.newConfig)
			bodies := runNodePoolNodeConfigUpdateCapture(t, d)
			if len(bodies) != 1 {
				t.Fatalf("expected 1 UpdateNodePool request, got %d: %#v", len(bodies), bodies)
			}
			if !reflect.DeepEqual(bodies[0], tc.wantBody) {
				t.Errorf("got request body %#v, want %#v", bodies[0], tc.wantBody)
			}
		})
	}
}

func TestNodePoolNodeConfigUpdate_GcfsGvnicFastSocketForceSend(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		oldConfig map[string]interface{}
		newConfig map[string]interface{}
		wantBody  map[string]interface{}
	}{
		{
			name: "gcfs_config enabled false sends enabled false",
			oldConfig: map[string]interface{}{
				"gcfs_config": []interface{}{
					map[string]interface{}{"enabled": true},
				},
			},
			newConfig: map[string]interface{}{
				"gcfs_config": []interface{}{
					map[string]interface{}{"enabled": false},
				},
			},
			wantBody: map[string]interface{}{
				"nodePoolId": "np-1",
				"gcfsConfig": map[string]interface{}{
					"enabled": false,
				},
			},
		},
		{
			name: "gvnic enabled false sends enabled false",
			oldConfig: map[string]interface{}{
				"gvnic": []interface{}{
					map[string]interface{}{"enabled": true},
				},
			},
			newConfig: map[string]interface{}{
				"gvnic": []interface{}{
					map[string]interface{}{"enabled": false},
				},
			},
			wantBody: map[string]interface{}{
				"nodePoolId": "np-1",
				"gvnic": map[string]interface{}{
					"enabled": false,
				},
			},
		},
		{
			name: "gvnic block removed sends enabled false",
			oldConfig: map[string]interface{}{
				"gvnic": []interface{}{
					map[string]interface{}{"enabled": true},
				},
			},
			newConfig: map[string]interface{}{},
			wantBody: map[string]interface{}{
				"nodePoolId": "np-1",
				"gvnic": map[string]interface{}{
					"enabled": false,
				},
			},
		},
		{
			name: "fast_socket enabled false sends enabled false",
			oldConfig: map[string]interface{}{
				"fast_socket": []interface{}{
					map[string]interface{}{"enabled": true},
				},
			},
			newConfig: map[string]interface{}{
				"fast_socket": []interface{}{
					map[string]interface{}{"enabled": false},
				},
			},
			wantBody: map[string]interface{}{
				"nodePoolId": "np-1",
				"fastSocket": map[string]interface{}{
					"enabled": false,
				},
			},
		},
		{
			name: "fast_socket block removed sends enabled false",
			oldConfig: map[string]interface{}{
				"fast_socket": []interface{}{
					map[string]interface{}{"enabled": true},
				},
			},
			newConfig: map[string]interface{}{},
			wantBody: map[string]interface{}{
				"nodePoolId": "np-1",
				"fastSocket": map[string]interface{}{
					"enabled": false,
				},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := testNodeConfigDiffResourceData(t, tc.oldConfig, tc.newConfig)
			bodies := runNodePoolNodeConfigUpdateCapture(t, d)
			if len(bodies) != 1 {
				t.Fatalf("expected 1 UpdateNodePool request, got %d: %#v", len(bodies), bodies)
			}
			if !reflect.DeepEqual(bodies[0], tc.wantBody) {
				t.Errorf("got request body %#v, want %#v", bodies[0], tc.wantBody)
			}
		})
	}
}

func TestFlattenNodeConfig_GvnicAndFastSocketPermadiff(t *testing.T) {
	t.Parallel()

	s := testNodeConfigResourceSchema()
	d := schema.TestResourceDataRaw(t, s, map[string]interface{}{
		"node_config": []interface{}{map[string]interface{}{}},
	})
	nc := expandNodeConfig(d, "", d.Get("node_config"))
	nc.Gvnic = expandGvnic([]interface{}{map[string]interface{}{"enabled": false}})
	nc.FastSocket = expandFastSocket([]interface{}{map[string]interface{}{"enabled": false}})

	// 1. When prior state has node_config WITHOUT gvnic/fast_socket (e.g. removed from HCL),
	// API returning enabled=false should flatten to empty slice to avoid permadiff.
	priorWithoutBlocks := []interface{}{
		map[string]interface{}{
			"gvnic":       []interface{}{},
			"fast_socket": []interface{}{},
		},
	}
	flattenedRemoved := flattenNodeConfig(nc, priorWithoutBlocks)
	if got := len(flattenedRemoved[0]["gvnic"].([]map[string]interface{})); got != 0 {
		t.Errorf("expected gvnic to flatten to empty when unconfigured and disabled, got %#v", flattenedRemoved[0]["gvnic"])
	}
	if got := len(flattenedRemoved[0]["fast_socket"].([]map[string]interface{})); got != 0 {
		t.Errorf("expected fast_socket to flatten to empty when unconfigured and disabled, got %#v", flattenedRemoved[0]["fast_socket"])
	}

	// 2. When prior state explicitly configured gvnic/fast_socket { enabled = false },
	// flattenNodeConfig must preserve [{enabled: false}] in state.
	priorWithFalseBlocks := []interface{}{
		map[string]interface{}{
			"gvnic":       []interface{}{map[string]interface{}{"enabled": false}},
			"fast_socket": []interface{}{map[string]interface{}{"enabled": false}},
		},
	}
	flattenedConfigured := flattenNodeConfig(nc, priorWithFalseBlocks)
	if got := flattenedConfigured[0]["gvnic"].([]map[string]interface{}); len(got) != 1 || got[0]["enabled"] != false {
		t.Errorf("expected gvnic [{enabled: false}] when configured in state, got %#v", got)
	}
	if got := flattenedConfigured[0]["fast_socket"].([]map[string]interface{}); len(got) != 1 || got[0]["enabled"] != false {
		t.Errorf("expected fast_socket [{enabled: false}] when configured in state, got %#v", got)
	}

	// 3. During ImportState (prior state is empty []interface{}{}), flattenNodeConfig
	// preserves API values for ImportStateVerify compatibility.
	flattenedImport := flattenNodeConfig(nc, []interface{}{})
	if got := flattenedImport[0]["gvnic"].([]map[string]interface{}); len(got) != 1 || got[0]["enabled"] != false {
		t.Errorf("expected gvnic [{enabled: false}] on import, got %#v", got)
	}
	if got := flattenedImport[0]["fast_socket"].([]map[string]interface{}); len(got) != 1 || got[0]["enabled"] != false {
		t.Errorf("expected fast_socket [{enabled: false}] on import, got %#v", got)
	}
}

func TestExpandNodeConfig_SubfieldForceSendAndLoggingVariant(t *testing.T) {
	t.Parallel()

	// 1. Unset logging_variant on Create should not populate LoggingConfig.
	s := testNodeConfigResourceSchema()
	dUnset := schema.TestResourceDataRaw(t, s, map[string]interface{}{
		"node_config": []interface{}{map[string]interface{}{}},
	})
	ncUnset := expandNodeConfig(dUnset, "", dUnset.Get("node_config"))
	if ncUnset.LoggingConfig != nil {
		t.Errorf("expected LoggingConfig to be nil when logging_variant is unset, got %#v", ncUnset.LoggingConfig)
	}

	// 2. Explicit logging_variant on Create should populate LoggingConfig.
	dSet := schema.TestResourceDataRaw(t, s, map[string]interface{}{
		"node_config": []interface{}{
			map[string]interface{}{
				"logging_variant": "MAX_THROUGHPUT",
			},
		},
	})
	ncSet := expandNodeConfig(dSet, "", dSet.Get("node_config"))
	if ncSet.LoggingConfig == nil || ncSet.LoggingConfig.VariantConfig == nil || ncSet.LoggingConfig.VariantConfig.Variant != "MAX_THROUGHPUT" {
		t.Errorf("expected LoggingConfig MAX_THROUGHPUT, got %#v", ncSet.LoggingConfig)
	}

	// 3. LinuxNodeConfig with empty sysctls and accurate_time_config=false serializes both fields.
	linuxCfg := expandLinuxNodeConfig([]interface{}{
		map[string]interface{}{
			"sysctls": map[string]interface{}{},
			"accurate_time_config": []interface{}{
				map[string]interface{}{
					"enable_ptp_kvm_time_sync": false,
				},
			},
		},
	})
	linuxMap := marshalToMap(t, linuxCfg)
	if _, ok := linuxMap["sysctls"]; !ok {
		t.Errorf("expected empty sysctls map to be serialized via ForceSendFields, got %#v", linuxMap)
	}
	atcMap, ok := linuxMap["accurateTimeConfig"].(map[string]interface{})
	if !ok || atcMap["enablePtpKvmTimeSync"] != false {
		t.Errorf("expected enablePtpKvmTimeSync=false to be serialized via ForceSendFields, got %#v", linuxMap)
	}

	// 4. ContainerdConfig with false booleans serializes enabled=false and overridePath=false.
	containerdCfg := expandContainerdConfig([]interface{}{
		map[string]interface{}{
			"private_registry_access_config": []interface{}{
				map[string]interface{}{
					"enabled": false,
				},
			},
			"writable_cgroups": []interface{}{
				map[string]interface{}{
					"enabled": false,
				},
			},
			"registry_hosts": []interface{}{
				map[string]interface{}{
					"server": "docker.io",
					"hosts": []interface{}{
						map[string]interface{}{
							"host":          "mirror.gcr.io",
							"override_path": false,
						},
					},
				},
			},
		},
	})
	containerdMap := marshalToMap(t, containerdCfg)
	pracMap, ok := containerdMap["privateRegistryAccessConfig"].(map[string]interface{})
	if !ok || pracMap["enabled"] != false {
		t.Errorf("expected privateRegistryAccessConfig.enabled=false, got %#v", containerdMap)
	}
	wcgMap, ok := containerdMap["writableCgroups"].(map[string]interface{})
	if !ok || wcgMap["enabled"] != false {
		t.Errorf("expected writableCgroups.enabled=false, got %#v", containerdMap)
	}
	rhList, ok := containerdMap["registryHosts"].([]interface{})
	if !ok || len(rhList) != 1 {
		t.Fatalf("expected 1 registryHost, got %#v", containerdMap)
	}
	hostsList := rhList[0].(map[string]interface{})["hosts"].([]interface{})
	hostMap := hostsList[0].(map[string]interface{})
	if hostMap["overridePath"] != false {
		t.Errorf("expected host.overridePath=false, got %#v", hostMap)
	}
}
