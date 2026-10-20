package sql

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
)

func DataSourceSqlDatabaseInstance() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceSqlDatabaseInstanceRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceSqlDatabaseInstance().SchemaMap())
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "name")
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func dataSourceSqlDatabaseInstanceRead(d *schema.ResourceData, meta interface{}) error {
	id := d.Get("name").(string)
	err := resourceSqlDatabaseInstanceRead(d, meta)
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
		Name:        "google_sql_database_instance",
		ProductName: "sql",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceSqlDatabaseInstance(),
	}.Register()
}
