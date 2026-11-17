package detector

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDetectMissingIdentityCoverage(t *testing.T) {
	cases := []struct {
		name            string
		resourceContent string
		testContent     string
		resourceName    string
		want            map[string]*MissingIdentityInfo
	}{
		{
			name:            "resource with identity and full coverage is not flagged",
			resourceContent: fakeResourceWithFullCoverage(),
			testContent:     fakeTestWithImportIdentity(),
			resourceName:    "foo_instance",
			want:            map[string]*MissingIdentityInfo{},
		},
		{
			name:            "resource with identity import step using RefreshState instead of ImportState is flagged",
			resourceContent: fakeResourceWithFullCoverage(),
			testContent:     fakeTestWithRefreshStateIdentity(),
			resourceName:    "foo_instance",
			want: map[string]*MissingIdentityInfo{
				"foo_instance": {
					MissingImportTest: true,
				},
			},
		},
		{
			name:            "resource with identity missing Create and Update CRUD and import test is flagged",
			resourceContent: fakeResourceMissingCreateUpdate(),
			testContent:     "",
			resourceName:    "foo_instance",
			want: map[string]*MissingIdentityInfo{
				"foo_instance": {
					MissingCRUD:       []string{"Create", "Update"},
					MissingImportTest: true,
				},
			},
		},
		{
			name:            "resource without identity block is not flagged",
			resourceContent: fakeResourceWithoutIdentity(),
			testContent:     "",
			resourceName:    "foo_instance",
			want:            map[string]*MissingIdentityInfo{},
		},
		{
			name:            "resource file does not exist is skipped",
			resourceContent: "",
			testContent:     "",
			resourceName:    "nonexistent_resource",
			want:            map[string]*MissingIdentityInfo{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			if tc.resourceContent != "" {
				err := os.WriteFile(filepath.Join(tmpDir, "resource_"+tc.resourceName+".go"), []byte(tc.resourceContent), 0644)
				if err != nil {
					t.Fatalf("failed to write resource file: %v", err)
				}
			}
			if tc.testContent != "" {
				err := os.WriteFile(filepath.Join(tmpDir, "resource_"+tc.resourceName+"_test.go"), []byte(tc.testContent), 0644)
				if err != nil {
					t.Fatalf("failed to write test file: %v", err)
				}
			}

			results, err := DetectMissingIdentityCoverage(tmpDir, []string{tc.resourceName})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(results, tc.want) {
				t.Errorf("got %+v, want %+v", results, tc.want)
			}
		})
	}
}

func TestDetectMissingIdentityCoverage_SkipsUnchangedResources(t *testing.T) {
	tmpDir := t.TempDir()

	err := os.WriteFile(filepath.Join(tmpDir, "resource_dns_record_set.go"), []byte(fakeResourceMissingCreateUpdate()), 0644)
	if err != nil {
		t.Fatalf("failed to write resource file: %v", err)
	}

	results, err := DetectMissingIdentityCoverage(tmpDir, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected no results when no resources changed, got %d", len(results))
	}
}

func TestDetectMissingIdentityCoverage_NestedServicesAndMMv1Identity(t *testing.T) {
	cases := []struct {
		name      string
		files     map[string]string
		resources []string
		want      map[string]*MissingIdentityInfo
	}{
		{
			name: "MMv1 resource in nested service dir with d.Identity()/identity.Set and generated import test is covered",
			files: map[string]string{
				"pubsub/resource_pubsub_topic.go": `package pubsub

func init() {
	registry.Schema{
		Name:        "google_pubsub_topic",
		ProductName: "pubsub",
		Type:        registry.SchemaTypeResource,
		Schema:      ResourcePubsubTopic(),
	}.Register()
}

func ResourcePubsubTopic() *schema.Resource {
	return &schema.Resource{
		Create: resourcePubsubTopicCreate,
		Read:   resourcePubsubTopicRead,
		Update: resourcePubsubTopicUpdate,
		Delete: resourcePubsubTopicDelete,
		Importer: &schema.ResourceImporter{
			State: resourcePubsubTopicImport,
		},
		Identity: &schema.ResourceIdentity{Version: 1},
	}
}

func resourcePubsubTopicCreate(d *schema.ResourceData, meta interface{}) error {
	identity, err := d.Identity()
	if err == nil && identity != nil {
		_ = identity.Set("name", "example")
	}
	return resourcePubsubTopicRead(d, meta)
}

func resourcePubsubTopicRead(d *schema.ResourceData, meta interface{}) error {
	identity, err := d.Identity()
	if err == nil && identity != nil {
		_ = identity.Set("name", "example")
	}
	return nil
}

func resourcePubsubTopicUpdate(d *schema.ResourceData, meta interface{}) error {
	identity, err := d.Identity()
	if err == nil && identity != nil {
		_ = identity.Set("name", "example")
	}
	return resourcePubsubTopicRead(d, meta)
}
`,
				"pubsub/resource_pubsub_topic_generated_test.go": `package pubsub_test

func TestAccPubsubTopic_example(t *testing.T) {
	acctest.VcrTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				ResourceName:       "google_pubsub_topic.example",
				ImportState:        true,
				ExpectNonEmptyPlan: true,
				ImportStateKind:    resource.ImportBlockWithResourceIdentity,
			},
		},
	})
}
`,
			},
			resources: []string{"google_pubsub_topic"},
			want:      map[string]*MissingIdentityInfo{},
		},
		{
			name: "nested MMv1 resource with custom_update early-returning Read only on clientSideOnly is flagged for Update",
			files: map[string]string{
				"foo/resource_foo_job.go": `package foo

func ResourceFooJob() *schema.Resource {
	return &schema.Resource{
		Create:   resourceFooJobCreate,
		Read:     resourceFooJobRead,
		Update:   resourceFooJobUpdate,
		Importer: &schema.ResourceImporter{},
		Identity: &schema.ResourceIdentity{Version: 1},
	}
}

func resourceFooJobCreate(d *schema.ResourceData, meta interface{}) error {
	identity, err := d.Identity()
	if err == nil && identity != nil {
		_ = identity.Set("job_id", "id")
	}
	return resourceFooJobRead(d, meta)
}

func resourceFooJobRead(d *schema.ResourceData, meta interface{}) error {
	identity, err := d.Identity()
	if err == nil && identity != nil {
		_ = identity.Set("job_id", "id")
	}
	return nil
}

func resourceFooJobUpdate(d *schema.ResourceData, meta interface{}) error {
	if clientSideOnly {
		return resourceFooJobRead(d, meta)
	}
	_ = d.Set("delete_protection", false)
	return nil
}
`,
				"foo/resource_foo_job_generated_test.go": `package foo_test

func TestAccFooJob(t *testing.T) {
	acctest.VcrTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				ResourceName:    "google_foo_job.primary",
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
		},
	})
}
`,
			},
			resources: []string{"google_foo_job"},
			want: map[string]*MissingIdentityInfo{
				"google_foo_job": {
					MissingCRUD:       []string{"Update"},
					MissingImportTest: false,
				},
			},
		},
		{
			name: "immutable resource with helper functions containing Create/Read/Update names does not false-positive",
			files: map[string]string{
				"bar/resource_bar_item.go": `package bar

func ResourceBarItem() *schema.Resource {
	return &schema.Resource{
		Create:   resourceBarItemCreate,
		Read:     resourceBarItemRead,
		Delete:   resourceBarItemDelete,
		Importer: &schema.ResourceImporter{},
		Identity: &schema.ResourceIdentity{Version: 1},
	}
}

func resourceBarItemCreate(d *schema.ResourceData, meta interface{}) error {
	return resourceBarItemRead(d, meta)
}

func resourceBarItemPollRead(d *schema.ResourceData, meta interface{}) interface{} {
	return nil
}

func resourceBarItemRead(d *schema.ResourceData, meta interface{}) error {
	return flattenBarItem(d)
}

func flattenBarItem(d *schema.ResourceData) error {
	return tpgresource.SetResourceIdentityAttributes(d, map[string]interface{}{"name": "x"})
}

func resourceBarItemPostCreateFailure(d *schema.ResourceData, meta interface{}, err error) error {
	return err
}

func resourceBarItemUpdateEncoder(d *schema.ResourceData, meta interface{}, obj map[string]interface{}) (map[string]interface{}, error) {
	return obj, nil
}

func resourceBarItemReadEncoder(d *schema.ResourceData, meta interface{}, obj map[string]interface{}) (map[string]interface{}, error) {
	return obj, nil
}

func resourceBarItemUpdateComputedFields(d *schema.ResourceData, meta interface{}) error {
	return nil
}

func expandBarItemUpdateTime(v interface{}, d interface{}, config interface{}) (interface{}, error) {
	return v, nil
}
`,
				"bar/resource_bar_item_test.go": `package bar_test

func TestAccBarItem(t *testing.T) {
	acctest.VcrTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				ResourceName:    "google_bar_item.primary",
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
		},
	})
}
`,
			},
			resources: []string{"google_bar_item"},
			want:      map[string]*MissingIdentityInfo{},
		},
		{
			name: "fallback minimal schema.Resource without CRUD fields also ignores PollRead and Encoder helpers",
			files: map[string]string{
				"resource_minimal_item.go": `package fake

func ResourceMinimalItem() *schema.Resource {
	return &schema.Resource{
		Identity: &schema.ResourceIdentity{},
	}
}

func resourceMinimalItemCreate(d *schema.ResourceData, meta interface{}) error {
	identity, _ := d.Identity()
	_ = identity.Set("id", "1")
	return nil
}

func resourceMinimalItemPollRead(d *schema.ResourceData, meta interface{}) interface{} {
	return nil
}

func resourceMinimalItemRead(d *schema.ResourceData, meta interface{}) error {
	identity, _ := d.Identity()
	_ = identity.Set("id", "1")
	return nil
}

func resourceMinimalItemPostCreateFailure(d *schema.ResourceData, meta interface{}, err error) error {
	return err
}

func resourceMinimalItemUpdateEncoder(d *schema.ResourceData, meta interface{}, obj map[string]interface{}) (map[string]interface{}, error) {
	return obj, nil
}

func resourceMinimalItemUpdateComputedFields(d *schema.ResourceData, meta interface{}) error {
	return nil
}
`,
				"resource_minimal_item_test.go": fakeTestWithImportIdentity(),
			},
			resources: []string{"minimal_item"},
			want:      map[string]*MissingIdentityInfo{},
		},
		{
			name: "special filenames (resource_google_*, *_connectivity_test_resource.go, registry-indexed) and sibling prefix isolation",
			files: map[string]string{
				"resourcemanager/resource_google_project.go": `package resourcemanager

func ResourceGoogleProject() *schema.Resource {
	return &schema.Resource{
		Create:   resourceGoogleProjectCreate,
		Read:     resourceGoogleProjectRead,
		Importer: &schema.ResourceImporter{},
		Identity: &schema.ResourceIdentity{Version: 1},
	}
}

func resourceGoogleProjectCreate(d *schema.ResourceData, meta interface{}) error {
	return tpgresource.SetResourceIdentityAttributes(d, map[string]interface{}{"project_id": "p"})
}

func resourceGoogleProjectRead(d *schema.ResourceData, meta interface{}) error {
	return tpgresource.SetResourceIdentityAttributes(d, map[string]interface{}{"project_id": "p"})
}
`,
				// Sibling resource_google_project_service_test.go must NOT satisfy google_project's import test check.
				"resourcemanager/resource_google_project_service_test.go": `package resourcemanager_test

func TestAccProjectService_identity(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				ResourceName:    "google_project_service.test",
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
		},
	})
}
`,
				"resourcemanager/resource_google_service_account_key.go": `package resourcemanager

// Non-importable resource (no Importer field) should not require ImportBlockWithResourceIdentity test.
func ResourceGoogleServiceAccountKey() *schema.Resource {
	return &schema.Resource{
		Create:   resourceGoogleServiceAccountKeyCreate,
		Read:     resourceGoogleServiceAccountKeyRead,
		Identity: &schema.ResourceIdentity{Version: 1},
	}
}

func resourceGoogleServiceAccountKeyCreate(d *schema.ResourceData, meta interface{}) error {
	return tpgresource.SetResourceIdentityAttributes(d, map[string]interface{}{"name": "k"})
}

func resourceGoogleServiceAccountKeyRead(d *schema.ResourceData, meta interface{}) error {
	return tpgresource.SetResourceIdentityAttributes(d, map[string]interface{}{"name": "k"})
}
`,
				"networkmanagement/resource_network_management_connectivity_test_resource.go": `package networkmanagement

func ResourceNetworkManagementConnectivityTest() *schema.Resource {
	return &schema.Resource{
		Create:   resourceNetworkManagementConnectivityTestCreate,
		Read:     resourceNetworkManagementConnectivityTestRead,
		Importer: &schema.ResourceImporter{},
		Identity: &schema.ResourceIdentity{Version: 1},
	}
}

func resourceNetworkManagementConnectivityTestCreate(d *schema.ResourceData, meta interface{}) error {
	identity, _ := d.Identity()
	_ = identity.Set("name", "c")
	return nil
}

func resourceNetworkManagementConnectivityTestRead(d *schema.ResourceData, meta interface{}) error {
	identity, _ := d.Identity()
	_ = identity.Set("name", "c")
	return nil
}
`,
				"networkmanagement/resource_network_management_connectivity_test_resource_generated_test.go": `package networkmanagement_test

func TestAccConnectivityTest(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				ResourceName:    "google_network_management_connectivity_test.primary",
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
		},
	})
}
`,
			},
			resources: []string{
				"google_project",
				"google_service_account_key",
				"google_network_management_connectivity_test",
			},
			want: map[string]*MissingIdentityInfo{
				"google_project": {
					MissingImportTest: true,
				},
			},
		},
		{
			name: "tpgiamresource IAM file sharing identity-enabled member and non-identity binding",
			files: map[string]string{
				"pubsub/iam_pubsub_topic.go": `package pubsub

func init() {
	registry.Schema{
		Name:        "google_pubsub_topic_iam_binding",
		ProductName: "Pubsub",
		Type:        registry.SchemaTypeIAMResource,
		Schema:      tpgiamresource.ResourceIamBinding(PubsubTopicIamSchema, PubsubTopicIamUpdaterProducer, PubsubTopicIdParseFunc),
	}.Register()
	registry.Schema{
		Name:        "google_pubsub_topic_iam_member",
		ProductName: "Pubsub",
		Type:        registry.SchemaTypeIAMResource,
		Schema:      NewPubsubTopicIamMemberResource(),
	}.Register()
	registry.Schema{
		Name:        "google_pubsub_topic_iam_policy",
		ProductName: "Pubsub",
		Type:        registry.SchemaTypeIAMResource,
		Schema:      tpgiamresource.ResourceIamPolicy(PubsubTopicIamSchema, PubsubTopicIamUpdaterProducer, PubsubTopicIdParseFunc, tpgiamresource.IamWithParentResourceIdentity(PubsubTopicIamParentResourceIdentityParser)),
	}.Register()
}

func NewPubsubTopicIamMemberResource() *schema.Resource {
	return tpgiamresource.ResourceIamMember(
		PubsubTopicIamSchema,
		PubsubTopicIamUpdaterProducer,
		PubsubTopicIdParseFunc,
		tpgiamresource.IamWithParentResourceIdentity(PubsubTopicIamParentResourceIdentityParser),
	)
}
`,
				"pubsub/iam_pubsub_topic_generated_test.go": `package pubsub_test

func TestAccPubsubTopicIamMemberGenerated(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				ResourceName:    "google_pubsub_topic_iam_member.foo",
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
		},
	})
}
`,
			},
			resources: []string{
				"google_pubsub_topic_iam_binding",
				"google_pubsub_topic_iam_member",
				"google_pubsub_topic_iam_policy",
			},
			want: map[string]*MissingIdentityInfo{
				"google_pubsub_topic_iam_policy": {
					MissingImportTest: true,
				},
			},
		},
		{
			name: "Plugin Framework resources: covered immutable stub vs updatable missing Update identity",
			files: map[string]string{
				"storage/fw_resource_storage_notification.go": `package storage

func init() {
	registry.FrameworkResource{
		Name:        "google_storage_notification",
		ProductName: "storage",
		Func:        NewStorageNotificationResource,
	}.Register()
}

func NewStorageNotificationResource() resource.Resource {
	return &storageNotificationResource{}
}

type storageNotificationResource struct{}

func (r *storageNotificationResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {}

func (r *storageNotificationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.refresh(ctx, resp.Identity)
	resp.Diagnostics.Append(resp.State.Set(ctx, nil)...)
}

func (r *storageNotificationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.refresh(ctx, resp.Identity)
	resp.Diagnostics.Append(resp.State.Set(ctx, nil)...)
}

func (r *storageNotificationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Immutable resource: all attributes have RequiresReplace.
}

func (r *storageNotificationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {}

func (r *storageNotificationResource) refresh(ctx context.Context, identity *tfsdk.ResourceIdentity) {
	_ = identity.Set(ctx, nil)
}
`,
				"storage/fw_resource_storage_notification_test.go": `package storage_test

func TestAccStorageNotification_identity(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				ResourceName:    "google_storage_notification.default",
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
		},
	})
}
`,
				"apigee/fw_resource_apigee_alias.go": `package apigee

func init() {
	registry.FrameworkResource{
		Name:        "google_apigee_alias",
		ProductName: "apigee",
		Func:        NewApigeeAliasResource,
	}.Register()
}

func NewApigeeAliasResource() resource.Resource {
	return &apigeeAliasResource{}
}

type apigeeAliasResource struct{}

func (r *apigeeAliasResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {}

func (r *apigeeAliasResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	resp.Diagnostics.Append(resp.Identity.Set(ctx, nil)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, nil)...)
}

func (r *apigeeAliasResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	resp.Diagnostics.Append(resp.Identity.Set(ctx, nil)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, nil)...)
}

func (r *apigeeAliasResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.Append(resp.State.Set(ctx, nil)...)
}

func (r *apigeeAliasResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {}
`,
			},
			resources: []string{"google_storage_notification", "google_apigee_alias"},
			want: map[string]*MissingIdentityInfo{
				"google_apigee_alias": {
					MissingCRUD:       []string{"Update"},
					MissingImportTest: true,
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			servicesDir := t.TempDir()
			for relPath, content := range tc.files {
				fullPath := filepath.Join(servicesDir, filepath.FromSlash(relPath))
				if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
					t.Fatalf("failed to create dir for %s: %v", relPath, err)
				}
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
					t.Fatalf("failed to write %s: %v", relPath, err)
				}
			}

			got, err := DetectMissingIdentityCoverage(servicesDir, tc.resources)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("DetectMissingIdentityCoverage() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// fakeResourceWithFullCoverage returns a resource file that declares ResourceIdentity
// and calls SetResourceIdentityAttributes in Create, Read, and Update.
func fakeResourceWithFullCoverage() string {
	return `package fake

func resourceFooInstanceCreate(d *schema.ResourceData, meta interface{}) error {
	tpgresource.SetResourceIdentityAttributes(d, map[string]interface{}{})
	return nil
}

func resourceFooInstanceRead(d *schema.ResourceData, meta interface{}) error {
	tpgresource.SetResourceIdentityAttributes(d, map[string]interface{}{})
	return nil
}

func resourceFooInstanceUpdate(d *schema.ResourceData, meta interface{}) error {
	tpgresource.SetResourceIdentityAttributes(d, map[string]interface{}{})
	return nil
}

func ResourceFooInstance() *schema.Resource {
	return &schema.Resource{
		Identity: &schema.ResourceIdentity{},
	}
}
`
}

// fakeResourceMissingCreateUpdate returns a resource file that declares ResourceIdentity
// but only calls SetResourceIdentityAttributes in Read, leaving Create and Update uncovered.
func fakeResourceMissingCreateUpdate() string {
	return `package fake

func resourceFooInstanceCreate(d *schema.ResourceData, meta interface{}) error {
	return nil
}

func resourceFooInstanceRead(d *schema.ResourceData, meta interface{}) error {
	tpgresource.SetResourceIdentityAttributes(d, map[string]interface{}{})
	return nil
}

func resourceFooInstanceUpdate(d *schema.ResourceData, meta interface{}) error {
	return nil
}

func ResourceFooInstance() *schema.Resource {
	return &schema.Resource{
		Identity: &schema.ResourceIdentity{},
	}
}
`
}

// fakeResourceWithoutIdentity returns a resource file with no ResourceIdentity block.
func fakeResourceWithoutIdentity() string {
	return `package fake

func resourceFooInstanceCreate(d *schema.ResourceData, meta interface{}) error {
	return nil
}

func resourceFooInstanceRead(d *schema.ResourceData, meta interface{}) error {
	return nil
}

func ResourceFooInstance() *schema.Resource {
	return &schema.Resource{}
}
`
}

// fakeTestWithImportIdentity returns a test file that exercises ImportBlockWithResourceIdentity.
func fakeTestWithImportIdentity() string {
	return `package fake

func TestAccFooInstance_importBlockWithResourceIdentity(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
		},
	})
}
`
}

// fakeTestWithRefreshStateIdentity returns a test file that mistakenly uses RefreshState: true with ImportBlockWithResourceIdentity.
func fakeTestWithRefreshStateIdentity() string {
	return `package fake

func TestAccFooInstance_importBlockWithResourceIdentity(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				RefreshState:    true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
		},
	})
}
`
}
