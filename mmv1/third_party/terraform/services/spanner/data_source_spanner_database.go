package spanner

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceSpannerDatabase() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceSpannerDatabaseRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceSpannerDatabase().SchemaMap())

			tpgresource.AddRequiredFieldsToSchema(dsSchema, "name")
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "instance")

			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func dataSourceSpannerDatabaseRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	id, err := tpgresource.ReplaceVars(d, config, "{{instance}}/{{name}}")
	if err != nil {
		return fmt.Errorf("Error constructing id: %s", err)
	}

	d.SetId(id)

	err = resourceSpannerDatabaseRead(d, meta)
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
		Name:        "google_spanner_database",
		ProductName: "spanner",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceSpannerDatabase(),
	}.Register()
}
