package datalineage

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceDataLineageConfig() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceDataLineageConfigRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceDataLineageConfig().SchemaMap())
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "parent", "location")
			return dsSchema
		},
	}
}

func dataSourceDataLineageConfigRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	id, err := tpgresource.ReplaceVars(d, config, "{{parent}}/locations/{{location}}/config")
	if err != nil {
		return fmt.Errorf("error constructing id: %s", err)
	}
	d.SetId(id)

	err = resourceDataLineageConfigRead(d, meta)
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
		Name:        "google_data_lineage_config",
		ProductName: "datalineage",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceDataLineageConfig(),
	}.Register()
}
