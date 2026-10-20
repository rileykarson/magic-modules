package storage

import (
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceGoogleStorageBucket() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceGoogleStorageBucketRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceStorageBucket().SchemaMap())

			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "name")
			return dsSchema
		},
	}
}

func dataSourceGoogleStorageBucketRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)
	userAgent, err := tpgresource.GenerateUserAgentString(d, config.UserAgent)
	if err != nil {
		return err
	}

	// Get the bucket and acl
	bucket := d.Get("name").(string)

	res, err := NewClient(config, userAgent).Buckets.Get(bucket).Do()
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] Read bucket %v at location %v\n\n", res.Name, res.SelfLink)

	return setStorageBucket(d, config, res, bucket, userAgent)
}

func init() {
	registry.Schema{
		Name:        "google_storage_bucket",
		ProductName: "storage",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceGoogleStorageBucket(),
	}.Register()
}
