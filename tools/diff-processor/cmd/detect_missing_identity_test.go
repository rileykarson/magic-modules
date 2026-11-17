package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/GoogleCloudPlatform/magic-modules/tools/diff-processor/detector"
	"github.com/GoogleCloudPlatform/magic-modules/tools/diff-processor/diff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestDetectMissingIdentityCmdRun(t *testing.T) {
	servicesDir := t.TempDir()
	pubsubDir := filepath.Join(servicesDir, "pubsub")
	if err := os.MkdirAll(pubsubDir, 0755); err != nil {
		t.Fatalf("failed to create pubsub dir: %v", err)
	}

	// Covered MMv1 resource under <servicesDir>/pubsub/resource_pubsub_topic.go
	coveredSrc := `package pubsub

func ResourcePubsubTopic() *schema.Resource {
	return &schema.Resource{
		Create:   resourcePubsubTopicCreate,
		Read:     resourcePubsubTopicRead,
		Importer: &schema.ResourceImporter{},
		Identity: &schema.ResourceIdentity{Version: 1},
	}
}

func resourcePubsubTopicCreate(d *schema.ResourceData, meta interface{}) error {
	identity, _ := d.Identity()
	_ = identity.Set("name", "t")
	return nil
}

func resourcePubsubTopicRead(d *schema.ResourceData, meta interface{}) error {
	identity, _ := d.Identity()
	_ = identity.Set("name", "t")
	return nil
}
`
	coveredTestSrc := `package pubsub_test

func TestAccPubsubTopic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				ResourceName:    "google_pubsub_topic.example",
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
		},
	})
}
`
	// Uncovered resource under <servicesDir>/pubsub/resource_pubsub_subscription.go
	uncoveredSrc := `package pubsub

func ResourcePubsubSubscription() *schema.Resource {
	return &schema.Resource{
		Create:   resourcePubsubSubscriptionCreate,
		Read:     resourcePubsubSubscriptionRead,
		Importer: &schema.ResourceImporter{},
		Identity: &schema.ResourceIdentity{Version: 1},
	}
}

func resourcePubsubSubscriptionCreate(d *schema.ResourceData, meta interface{}) error {
	return nil
}

func resourcePubsubSubscriptionRead(d *schema.ResourceData, meta interface{}) error {
	identity, _ := d.Identity()
	_ = identity.Set("name", "s")
	return nil
}
`
	// Removed resource file still sitting in a flat dir shouldn't be checked if removed in SchemaDiff
	removedSrc := uncoveredSrc

	for path, content := range map[string]string{
		filepath.Join(pubsubDir, "resource_pubsub_topic.go"):            coveredSrc,
		filepath.Join(pubsubDir, "resource_pubsub_topic_test.go"):       coveredTestSrc,
		filepath.Join(pubsubDir, "resource_pubsub_subscription.go"):     uncoveredSrc,
		filepath.Join(pubsubDir, "resource_pubsub_removed_resource.go"): removedSrc,
	} {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", path, err)
		}
	}

	sd := diff.SchemaDiff{
		"google_pubsub_topic": {
			ResourceConfig: diff.ResourceConfigDiff{
				New: &schema.Resource{},
			},
		},
		"google_pubsub_subscription": {
			ResourceConfig: diff.ResourceConfigDiff{
				New: &schema.Resource{},
			},
		},
		"google_pubsub_removed_resource": {
			ResourceConfig: diff.ResourceConfigDiff{
				Old: &schema.Resource{},
				New: nil,
			},
		},
	}

	var buf bytes.Buffer
	o := &detectMissingIdentityOptions{
		computeSchemaDiff: func() diff.SchemaDiff { return sd },
		stdout:            &buf,
	}

	if err := o.run([]string{servicesDir}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got map[string]*detector.MissingIdentityInfo
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}

	want := map[string]*detector.MissingIdentityInfo{
		"google_pubsub_subscription": {
			MissingCRUD:       []string{"Create"},
			MissingImportTest: true,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("detect-missing-identity output = %#v, want %#v", got, want)
	}
}
