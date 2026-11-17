package acctest

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/hashicorp/terraform-provider-google/google/tpgiamresource"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
	"google.golang.org/api/cloudresourcemanager/v1"
)

func uint64Ptr(v uint64) *uint64 {
	return &v
}

func buildTestTFJSONState(res *tfjson.StateResource) *tfjson.State {
	return &tfjson.State{
		Values: &tfjson.StateValues{
			RootModule: &tfjson.StateModule{
				Resources: []*tfjson.StateResource{res},
			},
		},
	}
}

func TestExpectIamIdentityMatchesState(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		addr      string
		state     *tfjson.State
		wantError string
	}{
		{
			name: "valid iam_policy with multi-parameter parent",
			addr: "google_workstations_workstation_iam_policy.foo",
			state: buildTestTFJSONState(&tfjson.StateResource{
				Address: "google_workstations_workstation_iam_policy.foo",
				Type:    "google_workstations_workstation_iam_policy",
				AttributeValues: map[string]any{
					"id":                     "projects/my-proj/locations/us-central1/workstationClusters/c1/workstationConfigs/cfg1/workstations/ws1",
					"etag":                   "etag-1",
					"policy_data":            `{"bindings":[]}`,
					"project":                "my-proj",
					"location":               "us-central1",
					"workstation_cluster_id": "c1",
					"workstation_config_id":  "cfg1",
					"workstation_id":         "projects/my-proj/locations/us-central1/workstationClusters/c1/workstationConfigs/cfg1/workstations/ws1",
				},
				IdentitySchemaVersion: uint64Ptr(1),
				IdentityValues: map[string]any{
					"project":                "my-proj",
					"location":               "us-central1",
					"workstation_cluster_id": "c1",
					"workstation_config_id":  "cfg1",
					"workstation_id":         "projects/my-proj/locations/us-central1/workstationClusters/c1/workstationConfigs/cfg1/workstations/ws1",
				},
			}),
		},
		{
			name: "valid iam_binding without condition",
			addr: "google_pubsub_topic_iam_binding.foo",
			state: buildTestTFJSONState(&tfjson.StateResource{
				Address: "google_pubsub_topic_iam_binding.foo",
				Type:    "google_pubsub_topic_iam_binding",
				AttributeValues: map[string]any{
					"id":        "projects/my-proj/topics/my-topic/roles/viewer",
					"etag":      "etag-1",
					"project":   "my-proj",
					"topic":     "projects/my-proj/topics/my-topic",
					"role":      "roles/viewer",
					"members":   []any{"user:admin@hashicorptest.com"},
					"condition": []any{},
				},
				IdentitySchemaVersion: uint64Ptr(1),
				IdentityValues: map[string]any{
					"project":         "my-proj",
					"topic":           "projects/my-proj/topics/my-topic",
					"role":            "roles/viewer",
					"condition_title": nil,
				},
			}),
		},
		{
			name: "valid iam_binding with condition and optional unset zone",
			addr: "google_compute_instance_iam_binding.foo",
			state: buildTestTFJSONState(&tfjson.StateResource{
				Address: "google_compute_instance_iam_binding.foo",
				Type:    "google_compute_instance_iam_binding",
				AttributeValues: map[string]any{
					"id":            "projects/my-proj/zones/us-central1-a/instances/inst-1/roles/viewer/expires_after_2019_12_31",
					"etag":          "etag-1",
					"project":       "my-proj",
					"zone":          nil,
					"instance_name": "projects/my-proj/zones/us-central1-a/instances/inst-1",
					"role":          "roles/viewer",
					"members":       []any{"user:admin@hashicorptest.com"},
					"condition": []any{
						map[string]any{
							"title":       "expires_after_2019_12_31",
							"description": "Expiring",
							"expression":  "true",
						},
					},
				},
				IdentitySchemaVersion: uint64Ptr(1),
				IdentityValues: map[string]any{
					"project":         "my-proj",
					"zone":            "",
					"instance_name":   "projects/my-proj/zones/us-central1-a/instances/inst-1",
					"role":            "roles/viewer",
					"condition_title": "expires_after_2019_12_31",
				},
			}),
		},
		{
			name: "valid iam_member with case-normalized principal and condition",
			addr: "google_pubsub_topic_iam_member.foo",
			state: buildTestTFJSONState(&tfjson.StateResource{
				Address: "google_pubsub_topic_iam_member.foo",
				Type:    "google_pubsub_topic_iam_member",
				AttributeValues: map[string]any{
					"id":      "projects/my-proj/topics/my-topic/roles/viewer/user:admin@hashicorptest.com/cond-1",
					"etag":    "etag-1",
					"project": "my-proj",
					"topic":   "projects/my-proj/topics/my-topic",
					"role":    "roles/viewer",
					"member":  "user:Admin@HashiCorpTest.com",
					"condition": []any{
						map[string]any{
							"title":      "cond-1",
							"expression": "true",
						},
					},
				},
				IdentitySchemaVersion: uint64Ptr(1),
				IdentityValues: map[string]any{
					"project":         "my-proj",
					"topic":           "projects/my-proj/topics/my-topic",
					"role":            "roles/viewer",
					"member":          "user:admin@hashicorptest.com",
					"condition_title": "cond-1",
				},
			}),
		},
		{
			name:      "nil state",
			addr:      "google_pubsub_topic_iam_policy.foo",
			state:     nil,
			wantError: "state is nil",
		},
		{
			name:      "resource missing in state",
			addr:      "google_pubsub_topic_iam_policy.missing",
			state:     buildTestTFJSONState(&tfjson.StateResource{Address: "google_pubsub_topic_iam_policy.foo"}),
			wantError: "resource not found in state",
		},
		{
			name: "missing IdentitySchemaVersion",
			addr: "google_pubsub_topic_iam_policy.foo",
			state: buildTestTFJSONState(&tfjson.StateResource{
				Address:        "google_pubsub_topic_iam_policy.foo",
				Type:           "google_pubsub_topic_iam_policy",
				IdentityValues: map[string]any{"project": "p"},
			}),
			wantError: "IdentitySchemaVersion is nil",
		},
		{
			name: "parent attribute mismatch",
			addr: "google_pubsub_topic_iam_policy.foo",
			state: buildTestTFJSONState(&tfjson.StateResource{
				Address: "google_pubsub_topic_iam_policy.foo",
				Type:    "google_pubsub_topic_iam_policy",
				AttributeValues: map[string]any{
					"project":     "my-proj",
					"topic":       "projects/my-proj/topics/my-topic",
					"policy_data": "{}",
				},
				IdentitySchemaVersion: uint64Ptr(1),
				IdentityValues: map[string]any{
					"project": "my-proj",
					"topic":   "my-topic",
				},
			}),
			wantError: `parent attribute "topic" mismatch`,
		},
		{
			name: "extra or missing identity key",
			addr: "google_pubsub_topic_iam_binding.foo",
			state: buildTestTFJSONState(&tfjson.StateResource{
				Address: "google_pubsub_topic_iam_binding.foo",
				Type:    "google_pubsub_topic_iam_binding",
				AttributeValues: map[string]any{
					"project": "my-proj",
					"topic":   "projects/my-proj/topics/my-topic",
					"role":    "roles/viewer",
				},
				IdentitySchemaVersion: uint64Ptr(1),
				IdentityValues: map[string]any{
					"project": "my-proj",
					"topic":   "projects/my-proj/topics/my-topic",
					"role":    "roles/viewer",
				},
			}),
			wantError: "identity key mismatch",
		},
		{
			name: "role mismatch on binding",
			addr: "google_pubsub_topic_iam_binding.foo",
			state: buildTestTFJSONState(&tfjson.StateResource{
				Address: "google_pubsub_topic_iam_binding.foo",
				Type:    "google_pubsub_topic_iam_binding",
				AttributeValues: map[string]any{
					"project":   "my-proj",
					"topic":     "projects/my-proj/topics/my-topic",
					"role":      "roles/viewer",
					"condition": []any{},
				},
				IdentitySchemaVersion: uint64Ptr(1),
				IdentityValues: map[string]any{
					"project":         "my-proj",
					"topic":           "projects/my-proj/topics/my-topic",
					"role":            "roles/editor",
					"condition_title": nil,
				},
			}),
			wantError: `identity attribute "role" mismatch`,
		},
		{
			name: "unexpected non-nil condition_title when condition is unset",
			addr: "google_pubsub_topic_iam_binding.foo",
			state: buildTestTFJSONState(&tfjson.StateResource{
				Address: "google_pubsub_topic_iam_binding.foo",
				Type:    "google_pubsub_topic_iam_binding",
				AttributeValues: map[string]any{
					"project":   "my-proj",
					"topic":     "projects/my-proj/topics/my-topic",
					"role":      "roles/viewer",
					"condition": []any{},
				},
				IdentitySchemaVersion: uint64Ptr(1),
				IdentityValues: map[string]any{
					"project":         "my-proj",
					"topic":           "projects/my-proj/topics/my-topic",
					"role":            "roles/viewer",
					"condition_title": "stale-title",
				},
			}),
			wantError: `expected nil identity "condition_title"`,
		},
		{
			name: "mismatched condition_title when condition is set",
			addr: "google_pubsub_topic_iam_binding.foo",
			state: buildTestTFJSONState(&tfjson.StateResource{
				Address: "google_pubsub_topic_iam_binding.foo",
				Type:    "google_pubsub_topic_iam_binding",
				AttributeValues: map[string]any{
					"project": "my-proj",
					"topic":   "projects/my-proj/topics/my-topic",
					"role":    "roles/viewer",
					"condition": []any{
						map[string]any{"title": "cond-a", "expression": "true"},
					},
				},
				IdentitySchemaVersion: uint64Ptr(1),
				IdentityValues: map[string]any{
					"project":         "my-proj",
					"topic":           "projects/my-proj/topics/my-topic",
					"role":            "roles/viewer",
					"condition_title": nil,
				},
			}),
			wantError: `identity attribute "condition_title" mismatch`,
		},
		{
			name: "member mismatch on iam_member",
			addr: "google_pubsub_topic_iam_member.foo",
			state: buildTestTFJSONState(&tfjson.StateResource{
				Address: "google_pubsub_topic_iam_member.foo",
				Type:    "google_pubsub_topic_iam_member",
				AttributeValues: map[string]any{
					"project":   "my-proj",
					"topic":     "projects/my-proj/topics/my-topic",
					"role":      "roles/viewer",
					"member":    "user:alice@example.com",
					"condition": []any{},
				},
				IdentitySchemaVersion: uint64Ptr(1),
				IdentityValues: map[string]any{
					"project":         "my-proj",
					"topic":           "projects/my-proj/topics/my-topic",
					"role":            "roles/viewer",
					"member":          "user:bob@example.com",
					"condition_title": nil,
				},
			}),
			wantError: `identity attribute "member" mismatch`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var resp statecheck.CheckStateResponse
			ExpectIamIdentityMatchesState(tc.addr).CheckState(context.Background(), statecheck.CheckStateRequest{
				State: tc.state,
			}, &resp)
			if tc.wantError == "" {
				if resp.Error != nil {
					t.Fatalf("unexpected error: %v", resp.Error)
				}
				return
			}
			if resp.Error == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantError)
			}
			if !strings.Contains(resp.Error.Error(), tc.wantError) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantError, resp.Error)
			}
		})
	}
}

// TestMMv1IamResourcesIdentityParity verifies across the live compiled provider
// that every MMv1 IAM resource triple (*_iam_member, *_iam_binding, *_iam_policy)
// registers a Version 1 ResourceIdentity with exact parent attribute parity.
func TestMMv1IamResourcesIdentityParity(t *testing.T) {
	t.Parallel()

	// Handwritten IAM families where only *_iam_member currently wires IamWithParentResourceIdentity.
	handwrittenMemberOnlyPrefixes := map[string]bool{
		"google_bigquery_dataset":               true,
		"google_discovery_engine_search_engine": true,
		"google_folder":                         true,
		"google_pubsub_subscription":            true,
		"google_service_account":                true,
		"google_storage_bucket":                 true,
	}

	var checkedFamilies int
	for resName, memberRes := range testAccProvider.ResourcesMap {
		prefix, ok := strings.CutSuffix(resName, "_iam_member")
		if !ok || memberRes.Identity == nil {
			continue
		}
		if handwrittenMemberOnlyPrefixes[prefix] {
			continue
		}

		bindingName := prefix + "_iam_binding"
		policyName := prefix + "_iam_policy"

		bindingRes, ok := testAccProvider.ResourcesMap[bindingName]
		if !ok {
			t.Errorf("missing sibling %q for %q", bindingName, resName)
			continue
		}
		policyRes, ok := testAccProvider.ResourcesMap[policyName]
		if !ok {
			t.Errorf("missing sibling %q for %q", policyName, resName)
			continue
		}

		if bindingRes.Identity == nil {
			t.Errorf("%s is missing ResourceIdentity (sibling %s has Identity)", bindingName, resName)
			continue
		}
		if policyRes.Identity == nil {
			t.Errorf("%s is missing ResourceIdentity (sibling %s has Identity)", policyName, resName)
			continue
		}
		if bindingRes.Identity.Version != 1 {
			t.Errorf("%s Identity.Version = %d, want 1", bindingName, bindingRes.Identity.Version)
		}
		if policyRes.Identity.Version != 1 {
			t.Errorf("%s Identity.Version = %d, want 1", policyName, policyRes.Identity.Version)
		}

		memberIdSchema := memberRes.Identity.SchemaFunc()
		bindingIdSchema := bindingRes.Identity.SchemaFunc()
		policyIdSchema := policyRes.Identity.SchemaFunc()

		// Binding identity must equal member identity minus "member".
		if len(bindingIdSchema) != len(memberIdSchema)-1 {
			t.Errorf("%s identity schema len = %d, want %d (member keys=%v, binding keys=%v)",
				bindingName, len(bindingIdSchema), len(memberIdSchema)-1, memberIdSchema, bindingIdSchema)
		}
		for k, ms := range memberIdSchema {
			if k == "member" {
				if _, exists := bindingIdSchema[k]; exists {
					t.Errorf("%s identity schema unexpectedly contains %q", bindingName, k)
				}
				continue
			}
			bs, exists := bindingIdSchema[k]
			if !exists {
				t.Errorf("%s identity schema missing attribute %q", bindingName, k)
				continue
			}
			if bs.Type != ms.Type || bs.RequiredForImport != ms.RequiredForImport || bs.OptionalForImport != ms.OptionalForImport {
				t.Errorf("%s identity schema %q = %+v, want parity with %s %+v", bindingName, k, bs, resName, ms)
			}
		}

		// Policy identity must equal member identity minus {"member", "role", "condition_title"}.
		if len(policyIdSchema) != len(memberIdSchema)-3 {
			t.Errorf("%s identity schema len = %d, want %d (member keys=%v, policy keys=%v)",
				policyName, len(policyIdSchema), len(memberIdSchema)-3, memberIdSchema, policyIdSchema)
		}
		for k, ms := range memberIdSchema {
			if k == "member" || k == "role" || k == "condition_title" {
				if _, exists := policyIdSchema[k]; exists {
					t.Errorf("%s identity schema unexpectedly contains %q", policyName, k)
				}
				continue
			}
			ps, exists := policyIdSchema[k]
			if !exists {
				t.Errorf("%s identity schema missing parent attribute %q", policyName, k)
				continue
			}
			if ps.Type != ms.Type || ps.RequiredForImport != ms.RequiredForImport || ps.OptionalForImport != ms.OptionalForImport {
				t.Errorf("%s identity schema %q = %+v, want parity with %s %+v", policyName, k, ps, resName, ms)
			}
		}

		checkedFamilies++
	}

	if checkedFamilies < 120 {
		t.Fatalf("expected at least 120 IAM resource families with full ResourceIdentity parity, checked %d", checkedFamilies)
	}
}

// inMemoryIamPolicyStore backs mock MMv1-shaped IAM resources for hermetic
// CRUD + ImportBlockWithResourceIdentity round-trip unit tests.
type inMemoryIamPolicyStore struct {
	mu       sync.Mutex
	policies map[string]*cloudresourcemanager.Policy
	nextEtag int
}

func newInMemoryIamPolicyStore() *inMemoryIamPolicyStore {
	return &inMemoryIamPolicyStore{
		policies: make(map[string]*cloudresourcemanager.Policy),
	}
}

func clonePolicy(p *cloudresourcemanager.Policy) *cloudresourcemanager.Policy {
	if p == nil {
		return &cloudresourcemanager.Policy{}
	}
	raw, err := json.Marshal(p)
	if err != nil {
		panic(err)
	}
	var out cloudresourcemanager.Policy
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(err)
	}
	return &out
}

type mockWidgetIamUpdater struct {
	store      *inMemoryIamPolicyStore
	project    string
	location   string
	clusterId  string
	widgetName string
}

func (u *mockWidgetIamUpdater) GetResourceId() string {
	return fmt.Sprintf("projects/%s/locations/%s/clusters/%s/widgets/%s", u.project, u.location, u.clusterId, u.widgetName)
}

func (u *mockWidgetIamUpdater) GetMutexKey() string {
	return "iam-mock-widget-" + u.GetResourceId()
}

func (u *mockWidgetIamUpdater) DescribeResource() string {
	return fmt.Sprintf("mock widget %q", u.GetResourceId())
}

func (u *mockWidgetIamUpdater) GetResourceIamPolicy() (*cloudresourcemanager.Policy, error) {
	u.store.mu.Lock()
	defer u.store.mu.Unlock()
	p, ok := u.store.policies[u.GetResourceId()]
	if !ok {
		return &cloudresourcemanager.Policy{Etag: "etag-0", Version: tpgiamresource.IamPolicyVersion}, nil
	}
	return clonePolicy(p), nil
}

func (u *mockWidgetIamUpdater) SetResourceIamPolicy(policy *cloudresourcemanager.Policy) error {
	u.store.mu.Lock()
	defer u.store.mu.Unlock()
	u.store.nextEtag++
	cp := clonePolicy(policy)
	cp.Etag = fmt.Sprintf("etag-%d", u.store.nextEtag)
	cp.Version = tpgiamresource.IamPolicyVersion
	u.store.policies[u.GetResourceId()] = cp
	return nil
}

var mockWidgetIamSchema = map[string]*schema.Schema{
	"project": {
		Type:     schema.TypeString,
		Computed: true,
		Optional: true,
		ForceNew: true,
	},
	"location": {
		Type:     schema.TypeString,
		Computed: true,
		Optional: true,
		ForceNew: true,
	},
	"cluster_id": {
		Type:     schema.TypeString,
		Required: true,
		ForceNew: true,
	},
	"widget": {
		Type:             schema.TypeString,
		Required:         true,
		ForceNew:         true,
		DiffSuppressFunc: tpgresource.CompareSelfLinkOrResourceName,
	},
}

var mockWidgetImportRegexes = []string{
	"projects/(?P<project>[^/]+)/locations/(?P<location>[^/]+)/clusters/(?P<cluster_id>[^/]+)/widgets/(?P<widget>[^/]+)",
	"(?P<project>[^/]+)/(?P<location>[^/]+)/(?P<cluster_id>[^/]+)/(?P<widget>[^/]+)",
	"(?P<location>[^/]+)/(?P<cluster_id>[^/]+)/(?P<widget>[^/]+)",
	"(?P<widget>[^/]+)",
}

func makeMockWidgetUpdaterProducer(store *inMemoryIamPolicyStore) tpgiamresource.NewResourceIamUpdaterFunc {
	return func(d tpgresource.TerraformResourceData, config *transport_tpg.Config) (tpgiamresource.ResourceIamUpdater, error) {
		values := make(map[string]string)

		project, _ := tpgresource.GetProject(d, config)
		if project != "" {
			if err := d.Set("project", project); err != nil {
				return nil, err
			}
		}
		values["project"] = project

		location, _ := tpgresource.GetLocation(d, config)
		if location != "" {
			if err := d.Set("location", location); err != nil {
				return nil, err
			}
		}
		values["location"] = location

		if v, ok := d.GetOk("cluster_id"); ok {
			values["cluster_id"] = v.(string)
		}
		if v, ok := d.GetOk("widget"); ok {
			values["widget"] = v.(string)
		}

		m, err := tpgresource.GetImportIdQualifiers(mockWidgetImportRegexes, d, config, d.Get("widget").(string))
		if err != nil {
			return nil, err
		}
		for k, v := range m {
			values[k] = v
		}

		u := &mockWidgetIamUpdater{
			store:      store,
			project:    values["project"],
			location:   values["location"],
			clusterId:  values["cluster_id"],
			widgetName: values["widget"],
		}
		if err := d.Set("project", u.project); err != nil {
			return nil, err
		}
		if err := d.Set("location", u.location); err != nil {
			return nil, err
		}
		if err := d.Set("cluster_id", u.clusterId); err != nil {
			return nil, err
		}
		if err := d.Set("widget", u.GetResourceId()); err != nil {
			return nil, err
		}
		return u, nil
	}
}

func mockWidgetIdParseFunc(d *schema.ResourceData, config *transport_tpg.Config) error {
	values := make(map[string]string)
	if project, _ := tpgresource.GetProject(d, config); project != "" {
		values["project"] = project
	}
	if location, _ := tpgresource.GetLocation(d, config); location != "" {
		values["location"] = location
	}
	m, err := tpgresource.GetImportIdQualifiers(mockWidgetImportRegexes, d, config, d.Id())
	if err != nil {
		return err
	}
	for k, v := range m {
		values[k] = v
	}
	u := &mockWidgetIamUpdater{
		project:    values["project"],
		location:   values["location"],
		clusterId:  values["cluster_id"],
		widgetName: values["widget"],
	}
	if err := d.Set("widget", u.GetResourceId()); err != nil {
		return err
	}
	d.SetId(u.GetResourceId())
	return nil
}

func mockWidgetParentIdentityParser(d *schema.ResourceData, identity *schema.IdentityData, config *transport_tpg.Config) (string, error) {
	return tpgiamresource.ParseIamResourceIdentity(d, identity, config, tpgiamresource.IamResourceIdentityConfig{
		Params: []tpgiamresource.IamIdentityParam{
			{Key: "project", IdentityKey: "project"},
			{Key: "location", IdentityKey: "location"},
			{Key: "clusterId", IdentityKey: "cluster_id"},
			{Key: "widget", IdentityKey: "widget"},
		},
		UriFormat: "projects/%s/locations/%s/clusters/%s/widgets/%s",
	})
}

func newMockIamTestProvider(store *inMemoryIamPolicyStore) *schema.Provider {
	producer := makeMockWidgetUpdaterProducer(store)
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"project": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "default-proj",
			},
			"region": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "us-central1",
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"google_mock_widget_iam_binding": tpgiamresource.ResourceIamBinding(
				mockWidgetIamSchema,
				producer,
				mockWidgetIdParseFunc,
				tpgiamresource.IamWithParentResourceIdentity(mockWidgetParentIdentityParser),
			),
			"google_mock_widget_iam_member": tpgiamresource.ResourceIamMember(
				mockWidgetIamSchema,
				producer,
				mockWidgetIdParseFunc,
				tpgiamresource.IamWithParentResourceIdentity(mockWidgetParentIdentityParser),
			),
			"google_mock_widget_iam_policy": tpgiamresource.ResourceIamPolicy(
				mockWidgetIamSchema,
				producer,
				mockWidgetIdParseFunc,
				tpgiamresource.IamWithParentResourceIdentity(mockWidgetParentIdentityParser),
			),
		},
		ConfigureContextFunc: func(_ context.Context, d *schema.ResourceData) (any, diag.Diagnostics) {
			return &transport_tpg.Config{
				Project:   d.Get("project").(string),
				Region:    d.Get("region").(string),
				SleepUnit: time.Millisecond,
			}, nil
		},
	}
}

func mockIamProtoV5Factories(store *inMemoryIamPolicyStore) map[string]func() (tfprotov5.ProviderServer, error) {
	return map[string]func() (tfprotov5.ProviderServer, error){
		"google": func() (tfprotov5.ProviderServer, error) {
			return schema.NewGRPCProviderServer(newMockIamTestProvider(store)), nil
		},
	}
}

// TestIamBindingAndPolicyDirectSDKIdentityRoundTrip exercises Create, Update, Read,
// and Importer.State via ResourceIdentity directly in Go (independent of the terraform CLI).
func TestIamBindingAndPolicyDirectSDKIdentityRoundTrip(t *testing.T) {
	t.Parallel()

	cfg := &transport_tpg.Config{
		Project:   "sdk-proj",
		Region:    "us-east1",
		SleepUnit: time.Millisecond,
	}

	t.Run("ResourceIamBinding conditional and unconditional identity round-trip", func(t *testing.T) {
		t.Parallel()
		store := newInMemoryIamPolicyStore()
		p := newMockIamTestProvider(store)
		res := p.ResourcesMap["google_mock_widget_iam_binding"]

		// 1. Create conditional binding with spaces in condition_title using short widget name + provider defaults.
		dCreate := res.TestResourceData()
		if err := dCreate.Set("cluster_id", "clus-1"); err != nil {
			t.Fatal(err)
		}
		if err := dCreate.Set("widget", "wid-1"); err != nil {
			t.Fatal(err)
		}
		if err := dCreate.Set("role", "roles/viewer"); err != nil {
			t.Fatal(err)
		}
		if err := dCreate.Set("members", []any{"user:Admin@Example.com"}); err != nil {
			t.Fatal(err)
		}
		if err := dCreate.Set("condition", []any{
			map[string]any{
				"title":       "expires after 2030",
				"description": "desc",
				"expression":  "true",
			},
		}); err != nil {
			t.Fatal(err)
		}

		if err := res.Create(dCreate, cfg); err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		ident, err := dCreate.Identity()
		if err != nil {
			t.Fatal(err)
		}
		wantCanonical := "projects/sdk-proj/locations/us-east1/clusters/clus-1/widgets/wid-1"
		if got, _ := ident.GetOk("widget"); got != wantCanonical {
			t.Fatalf("identity widget = %v, want %q", got, wantCanonical)
		}
		if got, _ := ident.GetOk("project"); got != "sdk-proj" {
			t.Fatalf("identity project = %v, want %q", got, "sdk-proj")
		}
		if got, _ := ident.GetOk("location"); got != "us-east1" {
			t.Fatalf("identity location = %v, want %q", got, "us-east1")
		}
		if got, _ := ident.GetOk("cluster_id"); got != "clus-1" {
			t.Fatalf("identity cluster_id = %v, want %q", got, "clus-1")
		}
		if got, _ := ident.GetOk("role"); got != "roles/viewer" {
			t.Fatalf("identity role = %v, want %q", got, "roles/viewer")
		}
		if got, _ := ident.GetOk("condition_title"); got != "expires after 2030" {
			t.Fatalf("identity condition_title = %v, want %q", got, "expires after 2030")
		}

		// 2. Import from ResourceIdentity alone (dImport.Id() == "") using short widget name + omitted optional defaults.
		dImport := res.TestResourceData()
		impIdent, err := dImport.Identity()
		if err != nil {
			t.Fatal(err)
		}
		if err := impIdent.Set("cluster_id", "clus-1"); err != nil {
			t.Fatal(err)
		}
		if err := impIdent.Set("widget", "wid-1"); err != nil {
			t.Fatal(err)
		}
		if err := impIdent.Set("role", "roles/viewer"); err != nil {
			t.Fatal(err)
		}
		if err := impIdent.Set("condition_title", "expires after 2030"); err != nil {
			t.Fatal(err)
		}

		imported, err := res.Importer.State(dImport, cfg)
		if err != nil {
			t.Fatalf("Importer.State failed: %v", err)
		}
		if len(imported) != 1 {
			t.Fatalf("len(imported) = %d, want 1", len(imported))
		}
		if err := res.Read(imported[0], cfg); err != nil {
			t.Fatalf("post-import Read failed: %v", err)
		}
		if imported[0].Id() != dCreate.Id() {
			t.Fatalf("imported Id = %q, want %q", imported[0].Id(), dCreate.Id())
		}
		postIdent, err := imported[0].Identity()
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := postIdent.GetOk("condition_title"); got != "expires after 2030" {
			t.Fatalf("post-import identity condition_title = %v, want %q", got, "expires after 2030")
		}
	})

	t.Run("ResourceIamPolicy Create Update and ResourceIdentity import round-trip", func(t *testing.T) {
		t.Parallel()
		store := newInMemoryIamPolicyStore()
		p := newMockIamTestProvider(store)
		res := p.ResourcesMap["google_mock_widget_iam_policy"]

		dCreate := res.TestResourceData()
		if err := dCreate.Set("cluster_id", "clus-p"); err != nil {
			t.Fatal(err)
		}
		if err := dCreate.Set("widget", "wid-p"); err != nil {
			t.Fatal(err)
		}
		if err := dCreate.Set("policy_data", `{"bindings":[{"members":["user:alice@example.com"],"role":"roles/viewer"}]}`); err != nil {
			t.Fatal(err)
		}

		if err := res.Create(dCreate, cfg); err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		wantCanonical := "projects/sdk-proj/locations/us-east1/clusters/clus-p/widgets/wid-p"
		ident, err := dCreate.Identity()
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := ident.GetOk("widget"); got != wantCanonical {
			t.Fatalf("identity widget = %v, want %q", got, wantCanonical)
		}

		// Import from ResourceIdentity using canonical URI on widget.
		dImport := res.TestResourceData()
		impIdent, err := dImport.Identity()
		if err != nil {
			t.Fatal(err)
		}
		if err := impIdent.Set("widget", wantCanonical); err != nil {
			t.Fatal(err)
		}

		imported, err := res.Importer.State(dImport, cfg)
		if err != nil {
			t.Fatalf("Importer.State failed: %v", err)
		}
		if err := res.Read(imported[0], cfg); err != nil {
			t.Fatalf("post-import Read failed: %v", err)
		}
		if imported[0].Id() != wantCanonical {
			t.Fatalf("imported Id = %q, want %q", imported[0].Id(), wantCanonical)
		}
		postIdent, err := imported[0].Identity()
		if err != nil {
			t.Fatal(err)
		}
		for k, want := range map[string]string{
			"project":    "sdk-proj",
			"location":   "us-east1",
			"cluster_id": "clus-p",
			"widget":     wantCanonical,
		} {
			if got, _ := postIdent.GetOk(k); got != want {
				t.Fatalf("post-import identity[%q] = %v, want %q", k, got, want)
			}
		}
	})
}

// TestIamBindingPolicyMemberEndToEndIdentityCLI runs real Terraform CLI
// Create -> ExpectIdentity / ExpectIamIdentityMatchesState -> ImportStateVerify ->
// ImportBlockWithResourceIdentity -> Update -> ImportBlockWithResourceIdentity
// against in-memory ResourceIamBinding, ResourceIamPolicy, and ResourceIamMember.
func TestIamBindingPolicyMemberEndToEndIdentityCLI(t *testing.T) {
	if _, err := exec.LookPath("terraform"); err != nil {
		t.Skip("terraform binary not found on PATH; skipping CLI ResourceIdentity unit test")
	}
	t.Parallel()

	t.Run("iam_binding_basic_and_update", func(t *testing.T) {
		t.Parallel()
		store := newInMemoryIamPolicyStore()
		resource.UnitTest(t, resource.TestCase{
			TerraformVersionChecks: []tfversion.TerraformVersionCheck{
				tfversion.SkipBelow(tfversion.Version1_12_0),
			},
			ProtoV5ProviderFactories: mockIamProtoV5Factories(store),
			Steps: []resource.TestStep{
				{
					Config: `
resource "google_mock_widget_iam_binding" "foo" {
  project    = "cli-proj"
  location   = "us-central1"
  cluster_id = "c1"
  widget     = "w1"
  role       = "roles/viewer"
  members    = ["user:admin@hashicorptest.com"]
}
`,
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectIdentity("google_mock_widget_iam_binding.foo", map[string]knownvalue.Check{
							"project":         knownvalue.StringExact("cli-proj"),
							"location":        knownvalue.StringExact("us-central1"),
							"cluster_id":      knownvalue.StringExact("c1"),
							"widget":          knownvalue.StringExact("projects/cli-proj/locations/us-central1/clusters/c1/widgets/w1"),
							"role":            knownvalue.StringExact("roles/viewer"),
							"condition_title": knownvalue.Null(),
						}),
						ExpectIamIdentityMatchesState("google_mock_widget_iam_binding.foo"),
					},
				},
				{
					ResourceName:      "google_mock_widget_iam_binding.foo",
					ImportStateId:     "projects/cli-proj/locations/us-central1/clusters/c1/widgets/w1 roles/viewer",
					ImportState:       true,
					ImportStateVerify: true,
				},
				{
					ResourceName:    "google_mock_widget_iam_binding.foo",
					ImportState:     true,
					ImportStateKind: resource.ImportBlockWithResourceIdentity,
				},
				{
					Config: `
resource "google_mock_widget_iam_binding" "foo" {
  project    = "cli-proj"
  location   = "us-central1"
  cluster_id = "c1"
  widget     = "w1"
  role       = "roles/viewer"
  members    = ["user:admin@hashicorptest.com", "user:gterraformtest1@gmail.com"]
}
`,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction("google_mock_widget_iam_binding.foo", plancheck.ResourceActionUpdate),
						},
					},
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectIdentity("google_mock_widget_iam_binding.foo", map[string]knownvalue.Check{
							"project":         knownvalue.StringExact("cli-proj"),
							"location":        knownvalue.StringExact("us-central1"),
							"cluster_id":      knownvalue.StringExact("c1"),
							"widget":          knownvalue.StringExact("projects/cli-proj/locations/us-central1/clusters/c1/widgets/w1"),
							"role":            knownvalue.StringExact("roles/viewer"),
							"condition_title": knownvalue.Null(),
						}),
						ExpectIamIdentityMatchesState("google_mock_widget_iam_binding.foo"),
					},
				},
				{
					ResourceName:    "google_mock_widget_iam_binding.foo",
					ImportState:     true,
					ImportStateKind: resource.ImportBlockWithResourceIdentity,
				},
			},
		})
	})

	t.Run("iam_binding_with_and_without_condition", func(t *testing.T) {
		t.Parallel()
		store := newInMemoryIamPolicyStore()
		resource.UnitTest(t, resource.TestCase{
			TerraformVersionChecks: []tfversion.TerraformVersionCheck{
				tfversion.SkipBelow(tfversion.Version1_12_0),
			},
			ProtoV5ProviderFactories: mockIamProtoV5Factories(store),
			Steps: []resource.TestStep{
				{
					Config: `
resource "google_mock_widget_iam_binding" "foo" {
  project    = "cli-proj"
  location   = "us-central1"
  cluster_id = "c1"
  widget     = "w-cond"
  role       = "roles/viewer"
  members    = ["user:admin@hashicorptest.com"]
}

resource "google_mock_widget_iam_binding" "foo2" {
  project    = "cli-proj"
  location   = "us-central1"
  cluster_id = "c1"
  widget     = "w-cond"
  role       = "roles/viewer"
  members    = ["user:admin@hashicorptest.com"]
  condition {
    title       = "expires_after_2019_12_31"
    description = "Expiring at midnight of 2019-12-31"
    expression  = "request.time < timestamp(\"2020-01-01T00:00:00Z\")"
  }
}

resource "google_mock_widget_iam_binding" "foo3" {
  project    = "cli-proj"
  location   = "us-central1"
  cluster_id = "c1"
  widget     = "w-cond"
  role       = "roles/viewer"
  members    = ["user:admin@hashicorptest.com"]
  condition {
    title      = "expires_after_2019_12_31-no-description"
    expression = "request.time < timestamp(\"2020-01-01T00:00:00Z\")"
  }
}
`,
					ConfigStateChecks: []statecheck.StateCheck{
						ExpectIamIdentityMatchesState("google_mock_widget_iam_binding.foo"),
						ExpectIamIdentityMatchesState("google_mock_widget_iam_binding.foo2"),
						ExpectIamIdentityMatchesState("google_mock_widget_iam_binding.foo3"),
					},
				},
				{
					ResourceName:    "google_mock_widget_iam_binding.foo",
					ImportState:     true,
					ImportStateKind: resource.ImportBlockWithResourceIdentity,
				},
				{
					ResourceName:    "google_mock_widget_iam_binding.foo2",
					ImportState:     true,
					ImportStateKind: resource.ImportBlockWithResourceIdentity,
				},
				{
					ResourceName:    "google_mock_widget_iam_binding.foo3",
					ImportState:     true,
					ImportStateKind: resource.ImportBlockWithResourceIdentity,
				},
			},
		})
	})

	t.Run("iam_policy_create_update_and_identity_import", func(t *testing.T) {
		t.Parallel()
		store := newInMemoryIamPolicyStore()
		resource.UnitTest(t, resource.TestCase{
			TerraformVersionChecks: []tfversion.TerraformVersionCheck{
				tfversion.SkipBelow(tfversion.Version1_12_0),
			},
			ProtoV5ProviderFactories: mockIamProtoV5Factories(store),
			Steps: []resource.TestStep{
				{
					Config: `
resource "google_mock_widget_iam_policy" "foo" {
  project     = "cli-proj"
  location    = "us-central1"
  cluster_id  = "c1"
  widget      = "w-pol"
  policy_data = "{\"bindings\":[{\"members\":[\"user:admin@hashicorptest.com\"],\"role\":\"roles/viewer\"}]}"
}
`,
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectIdentity("google_mock_widget_iam_policy.foo", map[string]knownvalue.Check{
							"project":    knownvalue.StringExact("cli-proj"),
							"location":   knownvalue.StringExact("us-central1"),
							"cluster_id": knownvalue.StringExact("c1"),
							"widget":     knownvalue.StringExact("projects/cli-proj/locations/us-central1/clusters/c1/widgets/w-pol"),
						}),
						ExpectIamIdentityMatchesState("google_mock_widget_iam_policy.foo"),
					},
				},
				{
					ResourceName:      "google_mock_widget_iam_policy.foo",
					ImportStateId:     "projects/cli-proj/locations/us-central1/clusters/c1/widgets/w-pol",
					ImportState:       true,
					ImportStateVerify: true,
				},
				{
					ResourceName:    "google_mock_widget_iam_policy.foo",
					ImportState:     true,
					ImportStateKind: resource.ImportBlockWithResourceIdentity,
				},
				{
					Config: `
resource "google_mock_widget_iam_policy" "foo" {
  project     = "cli-proj"
  location    = "us-central1"
  cluster_id  = "c1"
  widget      = "w-pol"
  policy_data = "{}"
}
`,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction("google_mock_widget_iam_policy.foo", plancheck.ResourceActionUpdate),
						},
					},
					ConfigStateChecks: []statecheck.StateCheck{
						ExpectIamIdentityMatchesState("google_mock_widget_iam_policy.foo"),
					},
				},
				{
					ResourceName:    "google_mock_widget_iam_policy.foo",
					ImportState:     true,
					ImportStateKind: resource.ImportBlockWithResourceIdentity,
				},
			},
		})
	})

	t.Run("iam_member_conditional_identity_import", func(t *testing.T) {
		t.Parallel()
		store := newInMemoryIamPolicyStore()
		resource.UnitTest(t, resource.TestCase{
			TerraformVersionChecks: []tfversion.TerraformVersionCheck{
				tfversion.SkipBelow(tfversion.Version1_12_0),
			},
			ProtoV5ProviderFactories: mockIamProtoV5Factories(store),
			Steps: []resource.TestStep{
				{
					Config: `
resource "google_mock_widget_iam_member" "foo" {
  project    = "cli-proj"
  location   = "us-central1"
  cluster_id = "c1"
  widget     = "w-mem"
  role       = "roles/viewer"
  member     = "user:admin@hashicorptest.com"
  condition {
    title       = "expires_after_2019_12_31"
    description = "Expiring at midnight of 2019-12-31"
    expression  = "request.time < timestamp(\"2020-01-01T00:00:00Z\")"
  }
}
`,
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectIdentity("google_mock_widget_iam_member.foo", map[string]knownvalue.Check{
							"project":         knownvalue.StringExact("cli-proj"),
							"location":        knownvalue.StringExact("us-central1"),
							"cluster_id":      knownvalue.StringExact("c1"),
							"widget":          knownvalue.StringExact("projects/cli-proj/locations/us-central1/clusters/c1/widgets/w-mem"),
							"role":            knownvalue.StringExact("roles/viewer"),
							"member":          knownvalue.StringExact("user:admin@hashicorptest.com"),
							"condition_title": knownvalue.StringExact("expires_after_2019_12_31"),
						}),
						ExpectIamIdentityMatchesState("google_mock_widget_iam_member.foo"),
					},
				},
				{
					ResourceName:    "google_mock_widget_iam_member.foo",
					ImportState:     true,
					ImportStateKind: resource.ImportBlockWithResourceIdentity,
				},
			},
		})
	})
}
