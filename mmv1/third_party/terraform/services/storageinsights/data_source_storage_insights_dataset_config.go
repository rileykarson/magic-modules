package storageinsights

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceGoogleStorageInsightsDatasetConfig() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceGoogleStorageInsightsDatasetConfigRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceStorageInsightsDatasetConfig().SchemaMap())
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "location", "dataset_config_id")
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func dataSourceGoogleStorageInsightsDatasetConfigRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	id, err := tpgresource.ReplaceVars(d, config, "projects/{{project}}/locations/{{location}}/datasetConfigs/{{dataset_config_id}}")
	if err != nil {
		return fmt.Errorf("Error constructing id: %s", err)
	}
	d.SetId(id)
	err = resourceStorageInsightsDatasetConfigRead(d, meta)
	if err != nil {
		return err
	}

	if d.Id() == "" {
		return fmt.Errorf("%s not found", id)
	}

	return nil
}

func init() {
	registry.Schema{
		Name:        "google_storage_insights_dataset_config",
		ProductName: "storageinsights",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceGoogleStorageInsightsDatasetConfig(),
	}.Register()
}
