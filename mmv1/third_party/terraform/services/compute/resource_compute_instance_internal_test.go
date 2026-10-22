package compute

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/errwrap"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
	"google.golang.org/api/googleapi"
)

func TestComputeInstance_networkIPCustomizedDiff(t *testing.T) {
	t.Parallel()

	d := &tpgresource.ResourceDiffMock{
		Before: map[string]interface{}{
			"network_interface.#": 0,
		},
		After: map[string]interface{}{
			"network_interface.#": 1,
		},
	}

	err := forceNewIfNetworkIPNotUpdatableFunc(d)
	if err != nil {
		t.Error(err)
	}

	if d.IsForceNew {
		t.Errorf("Expected not force new if network_interface array size changes")
	}

	type NetworkInterface struct {
		Network           string
		Subnetwork        string
		SubnetworkProject string
		NetworkIP         string
	}
	NIBefore := NetworkInterface{
		Network:           "a",
		Subnetwork:        "a",
		SubnetworkProject: "a",
		NetworkIP:         "a",
	}

	cases := map[string]struct {
		ExpectedForceNew bool
		Before           NetworkInterface
		After            NetworkInterface
	}{
		"NetworkIP only change": {
			ExpectedForceNew: true,
			Before:           NIBefore,
			After: NetworkInterface{
				Network:           "a",
				Subnetwork:        "a",
				SubnetworkProject: "a",
				NetworkIP:         "b",
			},
		},
		"NetworkIP and Network change": {
			ExpectedForceNew: false,
			Before:           NIBefore,
			After: NetworkInterface{
				Network:           "b",
				Subnetwork:        "a",
				SubnetworkProject: "a",
				NetworkIP:         "b",
			},
		},
		"NetworkIP and Subnetwork change": {
			ExpectedForceNew: false,
			Before:           NIBefore,
			After: NetworkInterface{
				Network:           "a",
				Subnetwork:        "b",
				SubnetworkProject: "a",
				NetworkIP:         "b",
			},
		},
		"NetworkIP and SubnetworkProject change": {
			ExpectedForceNew: false,
			Before:           NIBefore,
			After: NetworkInterface{
				Network:           "a",
				Subnetwork:        "a",
				SubnetworkProject: "b",
				NetworkIP:         "b",
			},
		},
		"All change": {
			ExpectedForceNew: false,
			Before:           NIBefore,
			After: NetworkInterface{
				Network:           "b",
				Subnetwork:        "b",
				SubnetworkProject: "b",
				NetworkIP:         "b",
			},
		},
		"No change": {
			ExpectedForceNew: false,
			Before:           NIBefore,
			After: NetworkInterface{
				Network:           "a",
				Subnetwork:        "a",
				SubnetworkProject: "a",
				NetworkIP:         "a",
			},
		},
	}

	for tn, tc := range cases {
		d := &tpgresource.ResourceDiffMock{
			Before: map[string]interface{}{
				"network_interface.#":                    1,
				"network_interface.0.network":            tc.Before.Network,
				"network_interface.0.subnetwork":         tc.Before.Subnetwork,
				"network_interface.0.subnetwork_project": tc.Before.SubnetworkProject,
				"network_interface.0.network_ip":         tc.Before.NetworkIP,
			},
			After: map[string]interface{}{
				"network_interface.#":                    1,
				"network_interface.0.network":            tc.After.Network,
				"network_interface.0.subnetwork":         tc.After.Subnetwork,
				"network_interface.0.subnetwork_project": tc.After.SubnetworkProject,
				"network_interface.0.network_ip":         tc.After.NetworkIP,
			},
		}
		err := forceNewIfNetworkIPNotUpdatableFunc(d)
		if err != nil {
			t.Error(err)
		}
		if tc.ExpectedForceNew != d.IsForceNew {
			t.Errorf("%v: expected d.IsForceNew to be %v, but was %v", tn, tc.ExpectedForceNew, d.IsForceNew)
		}
	}
}

func TestForceNewIfStoppedUpdateFieldsNotAllowed(t *testing.T) {
	t.Parallel()

	fields := []struct {
		name   string
		before map[string]interface{}
		after  map[string]interface{}
	}{
		{
			name:   "guest_accelerator",
			before: map[string]interface{}{"guest_accelerator": "old"},
			after:  map[string]interface{}{"guest_accelerator": "new"},
		},
		{
			name:   "network_performance_config",
			before: map[string]interface{}{"network_performance_config": "DEFAULT"},
			after:  map[string]interface{}{"network_performance_config": "TIER_1"},
		},
		{
			name:   "reservation_affinity",
			before: map[string]interface{}{"reservation_affinity": "ANY_RESERVATION"},
			after:  map[string]interface{}{"reservation_affinity": "NO_RESERVATION"},
		},
		{
			name:   "boot_disk.0.device_name",
			before: map[string]interface{}{"boot_disk.0.device_name": "boot-old"},
			after:  map[string]interface{}{"boot_disk.0.device_name": "boot-new"},
		},
		{
			name: "network_interface.0.queue_count",
			before: map[string]interface{}{
				"network_interface.#":             1,
				"network_interface.0.queue_count": 2,
			},
			after: map[string]interface{}{
				"network_interface.#":             1,
				"network_interface.0.queue_count": 4,
			},
		},
	}

	for _, f := range fields {
		t.Run(f.name+"_disallowed_triggers_ForceNew", func(t *testing.T) {
			before := map[string]interface{}{
				"allow_stopping_for_update": false,
				"desired_status":            "RUNNING",
			}
			after := map[string]interface{}{
				"allow_stopping_for_update": false,
				"desired_status":            "RUNNING",
			}
			for k, v := range f.before {
				before[k] = v
			}
			for k, v := range f.after {
				after[k] = v
			}
			d := &tpgresource.ResourceDiffMock{Before: before, After: after}
			if err := forceNewIfStoppedUpdateFieldsNotAllowedFunc(d); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !d.IsForceNew {
				t.Errorf("expected ForceNew when %s changes without allow_stopping_for_update", f.name)
			}
		})

		t.Run(f.name+"_allowed_by_allow_stopping_for_update", func(t *testing.T) {
			before := map[string]interface{}{
				"allow_stopping_for_update": true,
				"desired_status":            "RUNNING",
			}
			after := map[string]interface{}{
				"allow_stopping_for_update": true,
				"desired_status":            "RUNNING",
			}
			for k, v := range f.before {
				before[k] = v
			}
			for k, v := range f.after {
				after[k] = v
			}
			d := &tpgresource.ResourceDiffMock{Before: before, After: after}
			if err := forceNewIfStoppedUpdateFieldsNotAllowedFunc(d); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if d.IsForceNew {
				t.Errorf("expected in-place update (not ForceNew) when %s changes with allow_stopping_for_update=true", f.name)
			}
		})

		t.Run(f.name+"_allowed_by_desired_status_TERMINATED", func(t *testing.T) {
			before := map[string]interface{}{
				"allow_stopping_for_update": false,
				"desired_status":            "TERMINATED",
			}
			after := map[string]interface{}{
				"allow_stopping_for_update": false,
				"desired_status":            "TERMINATED",
			}
			for k, v := range f.before {
				before[k] = v
			}
			for k, v := range f.after {
				after[k] = v
			}
			d := &tpgresource.ResourceDiffMock{Before: before, After: after}
			if err := forceNewIfStoppedUpdateFieldsNotAllowedFunc(d); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if d.IsForceNew {
				t.Errorf("expected in-place update (not ForceNew) when %s changes with desired_status=TERMINATED", f.name)
			}
		})

		t.Run(f.name+"_allowed_when_transitioning_from_TERMINATED_to_RUNNING", func(t *testing.T) {
			before := map[string]interface{}{
				"allow_stopping_for_update": false,
				"desired_status":            "TERMINATED",
			}
			after := map[string]interface{}{
				"allow_stopping_for_update": false,
				"desired_status":            "RUNNING",
			}
			for k, v := range f.before {
				before[k] = v
			}
			for k, v := range f.after {
				after[k] = v
			}
			d := &tpgresource.ResourceDiffMock{Before: before, After: after}
			if err := forceNewIfStoppedUpdateFieldsNotAllowedFunc(d); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if d.IsForceNew {
				t.Errorf("expected in-place update (not ForceNew) when %s changes while transitioning from TERMINATED to RUNNING", f.name)
			}
		})
	}

	t.Run("key_revocation_action_type_clearing_triggers_ForceNew", func(t *testing.T) {
		d := &tpgresource.ResourceDiffMock{
			Before: map[string]interface{}{
				"allow_stopping_for_update":  true,
				"key_revocation_action_type": "STOP",
			},
			After: map[string]interface{}{
				"allow_stopping_for_update":  true,
				"key_revocation_action_type": "",
			},
		}
		if err := forceNewIfStoppedUpdateFieldsNotAllowedFunc(d); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !d.IsForceNew {
			t.Errorf("expected ForceNew when clearing key_revocation_action_type to empty string")
		}
	})

	t.Run("ipv6_access_config_removal_without_ipv4_only_triggers_ForceNew", func(t *testing.T) {
		d := &tpgresource.ResourceDiffMock{
			Before: map[string]interface{}{
				"allow_stopping_for_update":                true,
				"network_interface.#":                      1,
				"network_interface.0.stack_type":           "IPV4_IPV6",
				"network_interface.0.ipv6_access_config.#": 1,
			},
			After: map[string]interface{}{
				"allow_stopping_for_update":                true,
				"network_interface.#":                      1,
				"network_interface.0.stack_type":           "IPV4_IPV6",
				"network_interface.0.ipv6_access_config.#": 0,
			},
		}
		if err := forceNewIfStoppedUpdateFieldsNotAllowedFunc(d); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !d.IsForceNew {
			t.Errorf("expected ForceNew when removing ipv6_access_config while stack_type is IPV4_IPV6")
		}
	})

	t.Run("queue_count_clearing_to_zero_triggers_ForceNew", func(t *testing.T) {
		d := &tpgresource.ResourceDiffMock{
			Before: map[string]interface{}{
				"allow_stopping_for_update":       true,
				"network_interface.#":             1,
				"network_interface.0.queue_count": 4,
			},
			After: map[string]interface{}{
				"allow_stopping_for_update":       true,
				"network_interface.#":             1,
				"network_interface.0.queue_count": 0,
			},
		}
		if err := forceNewIfStoppedUpdateFieldsNotAllowedFunc(d); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !d.IsForceNew {
			t.Errorf("expected ForceNew when clearing queue_count from non-zero to 0")
		}
	})

	t.Run("boot_disk_device_name_clearing_triggers_ForceNew", func(t *testing.T) {
		d := &tpgresource.ResourceDiffMock{
			Before: map[string]interface{}{
				"allow_stopping_for_update": true,
				"boot_disk.0.device_name":   "custom-boot-name",
			},
			After: map[string]interface{}{
				"allow_stopping_for_update": true,
				"boot_disk.0.device_name":   "",
			},
		}
		if err := forceNewIfStoppedUpdateFieldsNotAllowedFunc(d); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !d.IsForceNew {
			t.Errorf("expected ForceNew when clearing boot_disk.0.device_name to empty string")
		}
	})
}

func TestPerformanceMonitoringUnitDiffSuppress(t *testing.T) {
	t.Parallel()

	newResource := schema.TestResourceDataRaw(t, map[string]*schema.Schema{}, map[string]interface{}{})
	existingResource := schema.TestResourceDataRaw(t, map[string]*schema.Schema{}, map[string]interface{}{})
	existingResource.SetId("existing")

	cases := map[string]struct {
		old, new string
		data     *schema.ResourceData
		want     bool
	}{
		"new resource preserves standard":                       {old: "", new: "STANDARD", data: newResource, want: false},
		"existing resource suppresses empty to standard":        {old: "", new: "STANDARD", data: existingResource, want: true},
		"existing resource suppresses standard to empty":        {old: "STANDARD", new: "", data: existingResource, want: true},
		"existing resource preserves empty to architectural":    {old: "", new: "ARCHITECTURAL", data: existingResource, want: false},
		"existing resource preserves architectural to standard": {old: "ARCHITECTURAL", new: "STANDARD", data: existingResource, want: false},
		"nil resource does not suppress":                        {old: "", new: "STANDARD", data: nil, want: false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := performanceMonitoringUnitDiffSuppress("performance_monitoring_unit", tc.old, tc.new, tc.data); got != tc.want {
				t.Fatalf("performanceMonitoringUnitDiffSuppress(%q, %q) = %t, want %t", tc.old, tc.new, got, tc.want)
			}
		})
	}
}

type recordedInstanceHTTPRequest struct {
	Method string
	Path   string
	Body   map[string]interface{}
}

func buildInstanceResourceDataWithDiff(t *testing.T, before, after map[string]interface{}) *schema.ResourceData {
	t.Helper()
	s := ResourceComputeInstance().Schema
	dOld := schema.TestResourceDataRaw(t, s, before)
	dOld.SetId("projects/test-project/zones/us-central1-a/instances/test-instance")
	dNew := schema.TestResourceDataRaw(t, s, after)
	dNew.SetId("projects/test-project/zones/us-central1-a/instances/test-instance")

	oldState := dOld.State()
	newState := dNew.State()

	diff := &terraform.InstanceDiff{
		Attributes: map[string]*terraform.ResourceAttrDiff{},
	}
	seen := map[string]bool{}
	for k, oldVal := range oldState.Attributes {
		seen[k] = true
		newVal, ok := newState.Attributes[k]
		if !ok {
			diff.Attributes[k] = &terraform.ResourceAttrDiff{
				Old:        oldVal,
				NewRemoved: true,
			}
		} else if oldVal != newVal {
			diff.Attributes[k] = &terraform.ResourceAttrDiff{
				Old: oldVal,
				New: newVal,
			}
		}
	}
	for k, newVal := range newState.Attributes {
		if seen[k] {
			continue
		}
		diff.Attributes[k] = &terraform.ResourceAttrDiff{
			Old: "",
			New: newVal,
		}
	}

	d, err := schema.InternalMap(s).Data(oldState, diff)
	if err != nil {
		t.Fatalf("failed building ResourceData from state and diff: %v", err)
	}
	return d
}

func baseComputeInstanceConfigMap() map[string]interface{} {
	return map[string]interface{}{
		"name":                       "test-instance",
		"project":                    "test-project",
		"zone":                       "us-central1-a",
		"machine_type":               "e2-medium",
		"description":                "initial description",
		"can_ip_forward":             false,
		"deletion_protection":        false,
		"allow_stopping_for_update":  true,
		"desired_status":             "RUNNING",
		"min_cpu_platform":           "Intel Haswell",
		"enable_display":             false,
		"key_revocation_action_type": "NONE",
		"metadata": map[string]interface{}{
			"foo": "bar",
		},
		"tags":              []interface{}{"tag-a"},
		"effective_labels":  map[string]interface{}{"env": "dev"},
		"label_fingerprint": "lbl-fp-1",
		"resource_policies": []interface{}{"projects/test-project/regions/us-central1/resourcePolicies/policy-old"},
		"boot_disk": []interface{}{
			map[string]interface{}{
				"auto_delete": true,
				"device_name": "boot-disk-1",
				"interface":   "SCSI",
				"source":      "projects/test-project/zones/us-central1-a/disks/boot-disk-1",
				"initialize_params": []interface{}{
					map[string]interface{}{
						"size":  10,
						"image": "projects/debian-cloud/global/images/debian-11",
					},
				},
			},
		},
		"attached_disk": []interface{}{
			map[string]interface{}{
				"source":      "projects/test-project/zones/us-central1-a/disks/data-disk-old",
				"device_name": "data-disk-old",
				"mode":        "READ_WRITE",
			},
		},
		"network_interface": []interface{}{
			map[string]interface{}{
				"name":                        "nic0",
				"network":                     "projects/test-project/global/networks/default",
				"subnetwork":                  "projects/test-project/regions/us-central1/subnetworks/default",
				"subnetwork_project":          "test-project",
				"network_ip":                  "10.128.0.2",
				"stack_type":                  "IPV4_IPV6",
				"igmp_query":                  "IGMP_QUERY_DISABLED",
				"ipv6_address":                "2001:db8::1",
				"internal_ipv6_prefix_length": 96,
				"queue_count":                 2,
				"access_config": []interface{}{
					map[string]interface{}{
						"nat_ip":       "34.1.1.1",
						"network_tier": "PREMIUM",
					},
				},
				"ipv6_access_config": []interface{}{
					map[string]interface{}{
						"network_tier":           "PREMIUM",
						"public_ptr_domain_name": "old.example.com.",
					},
				},
				"alias_ip_range": []interface{}{
					map[string]interface{}{
						"ip_cidr_range":         "10.128.1.0/24",
						"subnetwork_range_name": "range-common",
					},
					map[string]interface{}{
						"ip_cidr_range":         "10.128.2.0/24",
						"subnetwork_range_name": "range-old",
					},
				},
			},
		},
		"scheduling": []interface{}{
			map[string]interface{}{
				"automatic_restart":   true,
				"on_host_maintenance": "MIGRATE",
				"preemptible":         false,
				"provisioning_model":  "STANDARD",
			},
		},
		"service_account": []interface{}{
			map[string]interface{}{
				"email":  "old-sa@test-project.iam.gserviceaccount.com",
				"scopes": []interface{}{"https://www.googleapis.com/auth/cloud-platform"},
			},
		},
		"shielded_instance_config": []interface{}{
			map[string]interface{}{
				"enable_secure_boot":          false,
				"enable_vtpm":                 true,
				"enable_integrity_monitoring": true,
			},
		},
		"advanced_machine_features": []interface{}{
			map[string]interface{}{
				"enable_nested_virtualization": false,
				"threads_per_core":             1,
			},
		},
		"guest_accelerator": []interface{}{
			map[string]interface{}{
				"count": 1,
				"type":  "nvidia-tesla-t4",
			},
		},
		"network_performance_config": []interface{}{
			map[string]interface{}{
				"total_egress_bandwidth_tier": "DEFAULT",
			},
		},
		"reservation_affinity": []interface{}{
			map[string]interface{}{
				"type": "ANY_RESERVATION",
			},
		},
	}
}

func baseServerInstanceStateMap() map[string]interface{} {
	return map[string]interface{}{
		"name":                    "test-instance",
		"id":                      "1234567890",
		"status":                  "RUNNING",
		"fingerprint":             "inst-fp-1",
		"labelFingerprint":        "lbl-fp-1",
		"machineType":             "projects/test-project/zones/us-central1-a/machineTypes/e2-medium",
		"minCpuPlatform":          "Intel Haswell",
		"description":             "initial description",
		"canIpForward":            false,
		"deletionProtection":      false,
		"keyRevocationActionType": "NONE",
		"cpuPlatform":             "Intel Haswell",
		"selfLink":                "https://www.googleapis.com/compute/v1/projects/test-project/zones/us-central1-a/instances/test-instance",
		"metadata": map[string]interface{}{
			"fingerprint": "md-fp-1",
			"items": []interface{}{
				map[string]interface{}{
					"key":   "foo",
					"value": "bar",
				},
			},
		},
		"tags": map[string]interface{}{
			"fingerprint": "tags-fp-1",
			"items":       []interface{}{"tag-a"},
		},
		"labels": map[string]interface{}{
			"env": "dev",
		},
		"resourcePolicies": []interface{}{
			"projects/test-project/regions/us-central1/resourcePolicies/policy-old",
		},
		"disks": []interface{}{
			map[string]interface{}{
				"boot":       true,
				"autoDelete": true,
				"deviceName": "boot-disk-1",
				"interface":  "SCSI",
				"source":     "https://www.googleapis.com/compute/v1/projects/test-project/zones/us-central1-a/disks/boot-disk-1",
				"initializeParams": map[string]interface{}{
					"diskSizeGb": "10",
				},
			},
			map[string]interface{}{
				"boot":       false,
				"autoDelete": false,
				"deviceName": "data-disk-old",
				"mode":       "READ_WRITE",
				"type":       "PERSISTENT",
				"source":     "https://www.googleapis.com/compute/v1/projects/test-project/zones/us-central1-a/disks/data-disk-old",
			},
		},
		"networkInterfaces": []interface{}{
			map[string]interface{}{
				"name":                     "nic0",
				"fingerprint":              "nic-fp-1",
				"network":                  "https://www.googleapis.com/compute/v1/projects/test-project/global/networks/default",
				"subnetwork":               "https://www.googleapis.com/compute/v1/projects/test-project/regions/us-central1/subnetworks/default",
				"networkIP":                "10.128.0.2",
				"stackType":                "IPV4_IPV6",
				"igmpQuery":                "IGMP_QUERY_DISABLED",
				"ipv6Address":              "2001:db8::1",
				"internalIpv6PrefixLength": float64(96),
				"queueCount":               float64(2),
				"accessConfigs": []interface{}{
					map[string]interface{}{
						"name":        "External NAT",
						"type":        "ONE_TO_ONE_NAT",
						"natIP":       "34.1.1.1",
						"networkTier": "PREMIUM",
					},
				},
				"ipv6AccessConfigs": []interface{}{
					map[string]interface{}{
						"name":                "External IPv6",
						"type":                "DIRECT_IPV6",
						"networkTier":         "PREMIUM",
						"publicPtrDomainName": "old.example.com.",
					},
				},
				"aliasIpRanges": []interface{}{
					map[string]interface{}{
						"ipCidrRange":         "10.128.1.0/24",
						"subnetworkRangeName": "range-common",
					},
					map[string]interface{}{
						"ipCidrRange":         "10.128.2.0/24",
						"subnetworkRangeName": "range-old",
					},
				},
			},
		},
		"scheduling": map[string]interface{}{
			"automaticRestart":  true,
			"onHostMaintenance": "MIGRATE",
			"preemptible":       false,
			"provisioningModel": "STANDARD",
		},
		"serviceAccounts": []interface{}{
			map[string]interface{}{
				"email":  "old-sa@test-project.iam.gserviceaccount.com",
				"scopes": []interface{}{"https://www.googleapis.com/auth/cloud-platform"},
			},
		},
		"displayDevice": map[string]interface{}{
			"enableDisplay": false,
		},
		"shieldedInstanceConfig": map[string]interface{}{
			"enableSecureBoot":          false,
			"enableVtpm":                true,
			"enableIntegrityMonitoring": true,
		},
		"advancedMachineFeatures": map[string]interface{}{
			"enableNestedVirtualization": false,
			"threadsPerCore":             float64(1),
		},
		"guestAccelerators": []interface{}{
			map[string]interface{}{
				"acceleratorCount": float64(1),
				"acceleratorType":  "projects/test-project/zones/us-central1-a/acceleratorTypes/nvidia-tesla-t4",
			},
		},
		"networkPerformanceConfig": map[string]interface{}{
			"totalEgressBandwidthTier": "DEFAULT",
		},
		"reservationAffinity": map[string]interface{}{
			"consumeReservationType": "ANY_RESERVATION",
		},
	}
}

func TestIsInstanceUpdateFingerprintError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "http 412 conditionNotMet",
			err: &googleapi.Error{
				Code:    412,
				Message: "Precondition check failed.",
				Errors:  []googleapi.ErrorItem{{Reason: "conditionNotMet"}},
			},
			want: true,
		},
		{
			name: "http 400 GCE instance update fingerprint mismatch",
			err: &googleapi.Error{
				Code:    400,
				Message: "Fingerprint mismatch, some instance properties may have changed or you are using a different API version to get the instance resource.",
			},
			want: true,
		},
		{
			name: "wrapped http 400 fingerprint mismatch via errwrap",
			err: errwrap.Wrapf("Error updating instance: {{err}}", &googleapi.Error{
				Code:    400,
				Message: "Fingerprint mismatch, some instance properties may have changed.",
			}),
			want: true,
		},
		{
			name: "async LRO fingerprint mismatch",
			err:  errwrap.Wrapf("Error waiting for Updating Instance: {{err}}", &googleapi.Error{Code: 400, Message: "Cannot start update due to fingerprint mismatch."}),
			want: true,
		},
		{
			name: "non-fingerprint 400 error",
			err: &googleapi.Error{
				Code:    400,
				Message: "Invalid value for field 'resource.machineType'",
			},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := transport_tpg.IsRetryableError(tc.err, []transport_tpg.RetryErrorPredicateFunc{isInstanceUpdateFingerprintError}, nil)
			if got != tc.want {
				t.Fatalf("IsRetryableError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestComputeInstanceUpdate_WholeResourceLiveUpdates(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var recorded []recordedInstanceHTTPRequest
	serverState := baseServerInstanceStateMap()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		var bodyMap map[string]interface{}
		if r.Body != nil {
			raw, _ := io.ReadAll(r.Body)
			if len(raw) > 0 {
				_ = json.Unmarshal(raw, &bodyMap)
			}
		}
		recorded = append(recorded, recordedInstanceHTTPRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Body:   bodyMap,
		})

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/instances/test-instance"):
			_ = json.NewEncoder(w).Encode(serverState)
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/disks/boot-disk-1"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name":              "boot-disk-1",
				"zone":              "https://www.googleapis.com/compute/v1/projects/test-project/zones/us-central1-a",
				"sizeGb":            "10",
				"diskEncryptionKey": map[string]interface{}{},
				"labelFingerprint":  "disk-lbl-fp",
			})
		case r.Method == "PUT" && strings.HasSuffix(r.URL.Path, "/instances/test-instance"):
			// Verify initializeParams was stripped on every PUT
			if disks, ok := bodyMap["disks"].([]interface{}); ok {
				for _, rawDisk := range disks {
					if disk, ok := rawDisk.(map[string]interface{}); ok {
						if _, hasInit := disk["initializeParams"]; hasInit {
							w.WriteHeader(http.StatusBadRequest)
							_ = json.NewEncoder(w).Encode(map[string]interface{}{
								"error": map[string]interface{}{
									"code":    400,
									"message": "initializeParams must not be sent on instances.update",
								},
							})
							return
						}
					}
				}
			}
			serverState = bodyMap
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name":   "op-live",
				"status": "DONE",
				"zone":   "https://www.googleapis.com/compute/v1/projects/test-project/zones/us-central1-a",
			})
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/instances/test-instance/setSecurityPolicy"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name":   "op-sec-pol",
				"status": "DONE",
				"zone":   "https://www.googleapis.com/compute/v1/projects/test-project/zones/us-central1-a",
			})
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/operations/"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name":   "op-live",
				"status": "DONE",
				"zone":   "https://www.googleapis.com/compute/v1/projects/test-project/zones/us-central1-a",
			})
		default:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"code":    400,
					"message": "unexpected endpoint: " + r.Method + " " + r.URL.Path,
				},
			})
		}
	}))
	defer srv.Close()

	before := baseComputeInstanceConfigMap()
	after := baseComputeInstanceConfigMap()
	after["description"] = "updated description"
	after["key_revocation_action_type"] = "STOP"
	after["metadata"] = map[string]interface{}{"foo": "baz", "new-key": "new-val"}
	after["tags"] = []interface{}{"tag-b", "tag-c"}
	after["effective_labels"] = map[string]interface{}{"env": "prod"}
	after["params"] = []interface{}{
		map[string]interface{}{
			"resource_manager_tags": map[string]interface{}{"tagKeys/123": "tagValues/456"},
		},
	}
	after["resource_policies"] = []interface{}{"projects/test-project/regions/us-central1/resourcePolicies/policy-new"}
	after["scheduling"] = []interface{}{
		map[string]interface{}{
			"automatic_restart":   false,
			"on_host_maintenance": "TERMINATE",
			"preemptible":         false,
			"provisioning_model":  "STANDARD",
		},
	}
	after["network_interface"] = []interface{}{
		map[string]interface{}{
			"name":                        "nic0",
			"network":                     "projects/test-project/global/networks/default",
			"subnetwork":                  "projects/test-project/regions/us-central1/subnetworks/default",
			"subnetwork_project":          "test-project",
			"network_ip":                  "10.128.0.2",
			"stack_type":                  "IPV4_IPV6",
			"igmp_query":                  "IGMP_QUERY_V2",
			"ipv6_address":                "2001:db8::2",
			"internal_ipv6_prefix_length": 96,
			"queue_count":                 2,
			"access_config": []interface{}{
				map[string]interface{}{
					"nat_ip":       "34.1.1.2",
					"network_tier": "PREMIUM",
				},
			},
			"ipv6_access_config": []interface{}{
				map[string]interface{}{
					"network_tier":           "PREMIUM",
					"public_ptr_domain_name": "new.example.com.",
				},
			},
			"alias_ip_range": []interface{}{
				map[string]interface{}{
					"ip_cidr_range":         "10.128.1.0/24",
					"subnetwork_range_name": "range-common",
				},
				map[string]interface{}{
					"ip_cidr_range":         "10.128.3.0/24",
					"subnetwork_range_name": "range-new",
				},
			},
		},
	}
	after["attached_disk"] = []interface{}{
		map[string]interface{}{
			"source":      "projects/test-project/zones/us-central1-a/disks/data-disk-new",
			"device_name": "data-disk-new",
			"mode":        "READ_ONLY",
		},
	}
	after["boot_disk"] = []interface{}{
		map[string]interface{}{
			"auto_delete": false,
			"device_name": "boot-disk-1",
			"interface":   "SCSI",
			"source":      "projects/test-project/zones/us-central1-a/disks/boot-disk-1",
			"initialize_params": []interface{}{
				map[string]interface{}{
					"size":  10,
					"image": "projects/debian-cloud/global/images/debian-11",
				},
			},
		},
	}
	after["deletion_protection"] = true
	after["can_ip_forward"] = true

	d := buildInstanceResourceDataWithDiff(t, before, after)
	cfg := &transport_tpg.Config{
		Client: srv.Client(),
		CustomEndpoints: map[string]string{
			Product.CustomEndpointField: srv.URL + "/compute/v1/",
			"compute":                   srv.URL + "/compute/v1/",
		},
		Project:      "test-project",
		PollInterval: time.Millisecond,
	}

	if err := resourceComputeInstanceUpdate(d, cfg); err != nil {
		t.Fatalf("resourceComputeInstanceUpdate failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	putCount := 0
	for _, req := range recorded {
		if req.Method == "POST" && !strings.HasSuffix(req.Path, "/setSecurityPolicy") {
			t.Fatalf("unexpected fine-grained POST request to %s; all live instance mutations must use PUT", req.Path)
		}
		if req.Method == "PATCH" {
			t.Fatalf("unexpected fine-grained PATCH request to %s; all live instance mutations must use PUT", req.Path)
		}
		if req.Method == "PUT" {
			putCount++
		}
	}
	// Coalesced live update: 1 pre-step PUT (for clearing replaced policy/access_config/alias_ip_range/attached_disk) + 1 final coalesced PUT = 2 PUTs total
	if putCount != 2 {
		t.Fatalf("expected 2 coalesced PUT requests (pre-step + final), got %d", putCount)
	}

	// Verify final server state has all updated fields applied, including newly unlocked live fields
	if got := serverState["description"]; got != "updated description" {
		t.Errorf("description = %v, want updated description", got)
	}
	if got := serverState["keyRevocationActionType"]; got != "STOP" {
		t.Errorf("keyRevocationActionType = %v, want STOP", got)
	}
	if got := serverState["canIpForward"]; got != true {
		t.Errorf("canIpForward = %v, want true", got)
	}
	if got := serverState["deletionProtection"]; got != true {
		t.Errorf("deletionProtection = %v, want true", got)
	}
	if labels, ok := serverState["labels"].(map[string]interface{}); !ok || labels["env"] != "prod" {
		t.Errorf("labels = %v, want env=prod", serverState["labels"])
	}
	nics, _ := serverState["networkInterfaces"].([]interface{})
	nic0, _ := nics[0].(map[string]interface{})
	ipv6Acs, _ := nic0["ipv6AccessConfigs"].([]interface{})
	if len(ipv6Acs) != 1 {
		t.Fatalf("expected 1 ipv6AccessConfig, got %d", len(ipv6Acs))
	}
	ipv6Ac0, _ := ipv6Acs[0].(map[string]interface{})
	if got := ipv6Ac0["publicPtrDomainName"]; got != "new.example.com." {
		t.Errorf("ipv6AccessConfigs[0].publicPtrDomainName = %v, want new.example.com.", got)
	}
}

func TestComputeInstanceUpdate_WholeResourceStoppedUpdates(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var recorded []recordedInstanceHTTPRequest
	serverState := baseServerInstanceStateMap()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		var bodyMap map[string]interface{}
		if r.Body != nil {
			raw, _ := io.ReadAll(r.Body)
			if len(raw) > 0 {
				_ = json.Unmarshal(raw, &bodyMap)
			}
		}
		recorded = append(recorded, recordedInstanceHTTPRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Body:   bodyMap,
		})

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/instances/test-instance"):
			_ = json.NewEncoder(w).Encode(serverState)
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/disks/boot-disk-1"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name":   "boot-disk-1",
				"zone":   "https://www.googleapis.com/compute/v1/projects/test-project/zones/us-central1-a",
				"sizeGb": "10",
			})
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/subnetworks/subnet-new"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"network": "https://www.googleapis.com/compute/v1/projects/test-project/global/networks/default",
			})
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/instances/test-instance/stop"):
			serverState["status"] = "TERMINATED"
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name":   "op-stop",
				"status": "DONE",
				"zone":   "https://www.googleapis.com/compute/v1/projects/test-project/zones/us-central1-a",
			})
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/instances/test-instance/start"):
			serverState["status"] = "RUNNING"
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name":   "op-start",
				"status": "DONE",
				"zone":   "https://www.googleapis.com/compute/v1/projects/test-project/zones/us-central1-a",
			})
		case r.Method == "PUT" && strings.HasSuffix(r.URL.Path, "/instances/test-instance"):
			if serverState["status"] != "TERMINATED" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]interface{}{
						"code":    400,
						"message": "instance must be TERMINATED before updating stopped-only fields",
					},
				})
				return
			}
			serverState = bodyMap
			serverState["status"] = "TERMINATED"
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name":   "op-put",
				"status": "DONE",
				"zone":   "https://www.googleapis.com/compute/v1/projects/test-project/zones/us-central1-a",
			})
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/operations/"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name":   "op-done",
				"status": "DONE",
				"zone":   "https://www.googleapis.com/compute/v1/projects/test-project/zones/us-central1-a",
			})
		default:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"code":    400,
					"message": "unexpected endpoint: " + r.Method + " " + r.URL.Path,
				},
			})
		}
	}))
	defer srv.Close()

	before := baseComputeInstanceConfigMap()
	after := baseComputeInstanceConfigMap()
	after["min_cpu_platform"] = "Intel Cascade Lake"
	after["machine_type"] = "e2-standard-4"
	after["enable_display"] = true
	after["service_account"] = []interface{}{
		map[string]interface{}{
			"email":  "new-sa@test-project.iam.gserviceaccount.com",
			"scopes": []interface{}{"https://www.googleapis.com/auth/compute"},
		},
	}
	after["shielded_instance_config"] = []interface{}{
		map[string]interface{}{
			"enable_secure_boot":          true,
			"enable_vtpm":                 true,
			"enable_integrity_monitoring": true,
		},
	}
	after["scheduling"] = []interface{}{
		map[string]interface{}{
			"automatic_restart":   true,
			"on_host_maintenance": "MIGRATE",
			"preemptible":         false,
			"provisioning_model":  "STANDARD",
			"termination_time":    "2030-01-01T00:00:00Z",
		},
	}
	after["advanced_machine_features"] = []interface{}{
		map[string]interface{}{
			"enable_nested_virtualization": true,
			"threads_per_core":             2,
		},
	}
	after["guest_accelerator"] = []interface{}{
		map[string]interface{}{
			"count": 2,
			"type":  "nvidia-tesla-t4",
		},
	}
	after["network_performance_config"] = []interface{}{
		map[string]interface{}{
			"total_egress_bandwidth_tier": "TIER_1",
		},
	}
	after["reservation_affinity"] = []interface{}{
		map[string]interface{}{
			"type": "NO_RESERVATION",
		},
	}
	after["boot_disk"] = []interface{}{
		map[string]interface{}{
			"auto_delete": true,
			"device_name": "boot-disk-renamed",
			"interface":   "SCSI",
			"source":      "projects/test-project/zones/us-central1-a/disks/boot-disk-1",
			"initialize_params": []interface{}{
				map[string]interface{}{
					"size":  10,
					"image": "projects/debian-cloud/global/images/debian-11",
				},
			},
		},
	}
	after["network_interface"] = []interface{}{
		map[string]interface{}{
			"name":                        "nic0",
			"network":                     "projects/test-project/global/networks/default",
			"subnetwork":                  "projects/test-project/regions/us-central1/subnetworks/subnet-new",
			"subnetwork_project":          "test-project",
			"network_ip":                  "10.128.0.2",
			"stack_type":                  "IPV4_IPV6",
			"igmp_query":                  "IGMP_QUERY_DISABLED",
			"ipv6_address":                "2001:db8::1",
			"internal_ipv6_prefix_length": 96,
			"queue_count":                 4,
			"access_config": []interface{}{
				map[string]interface{}{
					"nat_ip":       "34.1.1.1",
					"network_tier": "PREMIUM",
				},
			},
			"ipv6_access_config": []interface{}{
				map[string]interface{}{
					"network_tier":           "PREMIUM",
					"public_ptr_domain_name": "old.example.com.",
				},
			},
			"alias_ip_range": []interface{}{},
		},
	}

	d := buildInstanceResourceDataWithDiff(t, before, after)
	cfg := &transport_tpg.Config{
		Client: srv.Client(),
		CustomEndpoints: map[string]string{
			Product.CustomEndpointField: srv.URL + "/compute/v1/",
			"compute":                   srv.URL + "/compute/v1/",
		},
		Project:      "test-project",
		PollInterval: time.Millisecond,
	}

	if err := resourceComputeInstanceUpdate(d, cfg); err != nil {
		t.Fatalf("resourceComputeInstanceUpdate failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	sawStop := false
	sawStart := false
	putCount := 0
	for _, req := range recorded {
		if req.Method == "POST" {
			if strings.HasSuffix(req.Path, "/stop") {
				sawStop = true
			} else if strings.HasSuffix(req.Path, "/start") {
				sawStart = true
			} else {
				t.Fatalf("unexpected POST endpoint %s", req.Path)
			}
		}
		if req.Method == "PATCH" {
			t.Fatalf("unexpected PATCH endpoint %s", req.Path)
		}
		if req.Method == "PUT" {
			putCount++
		}
	}
	if !sawStop || !sawStart {
		t.Fatalf("expected stop (%v) and start (%v) around stopped updates", sawStop, sawStart)
	}
	// 1 coalesced PUT for all stopped mutations + 1 separate PUT for queueCount (GCE verifyUnsupportedCombinedUpdateWithQueueCount)
	if putCount != 2 {
		t.Fatalf("expected 2 PUT requests for stopped updates (coalesced stopped + separate queueCount), got %d", putCount)
	}

	// Verify networkIP and aliasIpRanges were cleared during subnetwork migration, and queueCount was updated
	nics, _ := serverState["networkInterfaces"].([]interface{})
	nic0, _ := nics[0].(map[string]interface{})
	if _, hasNetIP := nic0["networkIP"]; hasNetIP {
		t.Errorf("expected networkIP to be cleared on subnetwork migration when network_ip did not change, got %v", nic0["networkIP"])
	}
	if _, hasAlias := nic0["aliasIpRanges"]; hasAlias {
		t.Errorf("expected aliasIpRanges to be cleared when removed during subnetwork migration, got %v", nic0["aliasIpRanges"])
	}
	if got := nic0["queueCount"]; got != float64(4) {
		t.Errorf("networkInterfaces[0].queueCount = %v, want 4", got)
	}

	// Verify newly unlocked stopped fields
	accs, _ := serverState["guestAccelerators"].([]interface{})
	if len(accs) != 1 {
		t.Fatalf("expected 1 guestAccelerator, got %d", len(accs))
	}
	acc0, _ := accs[0].(map[string]interface{})
	if got := acc0["acceleratorCount"]; got != float64(2) {
		t.Errorf("guestAccelerators[0].acceleratorCount = %v, want 2", got)
	}
	npc, _ := serverState["networkPerformanceConfig"].(map[string]interface{})
	if got := npc["totalEgressBandwidthTier"]; got != "TIER_1" {
		t.Errorf("networkPerformanceConfig.totalEgressBandwidthTier = %v, want TIER_1", got)
	}
	ra, _ := serverState["reservationAffinity"].(map[string]interface{})
	if got := ra["consumeReservationType"]; got != "NO_RESERVATION" {
		t.Errorf("reservationAffinity.consumeReservationType = %v, want NO_RESERVATION", got)
	}
	disks, _ := serverState["disks"].([]interface{})
	bootDisk, _ := disks[0].(map[string]interface{})
	if got := bootDisk["deviceName"]; got != "boot-disk-renamed" {
		t.Errorf("disks[0].deviceName = %v, want boot-disk-renamed", got)
	}
}

func TestCheckForCommonAliasIp(t *testing.T) {
	t.Parallel()

	oldNic := map[string]interface{}{
		"aliasIpRanges": []interface{}{
			map[string]interface{}{
				"ipCidrRange":         "10.128.0.5/32",
				"subnetworkRangeName": "range-a",
			},
			map[string]interface{}{
				"ipCidrRange":         "/24",
				"subnetworkRangeName": "range-b",
			},
		},
	}
	newNic := map[string]interface{}{
		"aliasIpRanges": []interface{}{
			// Host IP without /32 should match old /32 entry on same subnetworkRangeName
			map[string]interface{}{
				"ipCidrRange":         "10.128.0.5",
				"subnetworkRangeName": "range-a",
			},
			// Same CIDR "/24" but different subnetworkRangeName must NOT be treated as common
			map[string]interface{}{
				"ipCidrRange":         "/24",
				"subnetworkRangeName": "range-c",
			},
		},
	}

	common := CheckForCommonAliasIp(oldNic, newNic)
	if len(common) != 1 {
		t.Fatalf("expected 1 common aliasIpRange, got %d: %v", len(common), common)
	}
	kept := common[0].(map[string]interface{})
	if kept["subnetworkRangeName"] != "range-a" {
		t.Errorf("expected range-a to be kept, got %v", kept)
	}
}

func TestComputeInstanceUpdate_FingerprintConflictRetry(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	putAttempts := 0
	serverState := baseServerInstanceStateMap()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		var bodyMap map[string]interface{}
		if r.Body != nil {
			raw, _ := io.ReadAll(r.Body)
			if len(raw) > 0 {
				_ = json.Unmarshal(raw, &bodyMap)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/instances/test-instance"):
			_ = json.NewEncoder(w).Encode(serverState)
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/disks/boot-disk-1"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name":   "boot-disk-1",
				"zone":   "https://www.googleapis.com/compute/v1/projects/test-project/zones/us-central1-a",
				"sizeGb": "10",
			})
		case r.Method == "PUT" && strings.HasSuffix(r.URL.Path, "/instances/test-instance"):
			putAttempts++
			if putAttempts == 1 {
				serverState["fingerprint"] = "inst-fp-2"
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]interface{}{
						"code":    400,
						"message": "Fingerprint mismatch, some instance properties may have changed or you are using a different API version to get the instance resource.",
					},
				})
				return
			}
			if bodyMap["fingerprint"] != "inst-fp-2" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]interface{}{
						"code":    400,
						"message": "expected refreshed fingerprint inst-fp-2",
					},
				})
				return
			}
			serverState = bodyMap
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name":   "op-retry",
				"status": "DONE",
				"zone":   "https://www.googleapis.com/compute/v1/projects/test-project/zones/us-central1-a",
			})
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/operations/"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name":   "op-retry",
				"status": "DONE",
				"zone":   "https://www.googleapis.com/compute/v1/projects/test-project/zones/us-central1-a",
			})
		}
	}))
	defer srv.Close()

	before := baseComputeInstanceConfigMap()
	after := baseComputeInstanceConfigMap()
	after["tags"] = []interface{}{"tag-retried"}

	d := buildInstanceResourceDataWithDiff(t, before, after)
	cfg := &transport_tpg.Config{
		Client: srv.Client(),
		CustomEndpoints: map[string]string{
			Product.CustomEndpointField: srv.URL + "/compute/v1/",
			"compute":                   srv.URL + "/compute/v1/",
		},
		Project:      "test-project",
		PollInterval: time.Millisecond,
	}

	if err := resourceComputeInstanceUpdate(d, cfg); err != nil {
		t.Fatalf("resourceComputeInstanceUpdate failed on retryable fingerprint mismatch: %v", err)
	}
	if putAttempts != 2 {
		t.Fatalf("expected 2 PUT attempts, got %d", putAttempts)
	}
}
