package oracledatabase

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceOracleDatabaseExascaleDbStorageVault() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceOracleDatabaseExascaleDbStorageVaultRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceOracleDatabaseExascaleDbStorageVault().SchemaMap())
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "location", "exascale_db_storage_vault_id")
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func dataSourceOracleDatabaseExascaleDbStorageVaultRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	id, err := tpgresource.ReplaceVars(d, config, "projects/{{project}}/locations/{{location}}/exascaleDbStorageVaults/{{exascale_db_storage_vault_id}}")
	if err != nil {
		return fmt.Errorf("Error constructing id: %s", err)
	}
	d.SetId(id)
	err = resourceOracleDatabaseExascaleDbStorageVaultRead(d, meta)
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
		Name:        "google_oracle_database_exascale_db_storage_vault",
		ProductName: "oracledatabase",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceOracleDatabaseExascaleDbStorageVault(),
	}.Register()
}
