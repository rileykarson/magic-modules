package oracledatabase

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceOracleDatabaseAutonomousDatabase() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceOracleDatabaseAutonomousDatabaseRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceOracleDatabaseAutonomousDatabase().SchemaMap())
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "location", "autonomous_database_id")
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func dataSourceOracleDatabaseAutonomousDatabaseRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	id, err := tpgresource.ReplaceVars(d, config, "projects/{{project}}/locations/{{location}}/autonomousDatabases/{{autonomous_database_id}}")
	if err != nil {
		return fmt.Errorf("Error constructing id: %s", err)
	}

	d.SetId(id)

	err = resourceOracleDatabaseAutonomousDatabaseRead(d, meta)
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
		Name:        "google_oracle_database_autonomous_database",
		ProductName: "oracledatabase",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceOracleDatabaseAutonomousDatabase(),
	}.Register()
}
