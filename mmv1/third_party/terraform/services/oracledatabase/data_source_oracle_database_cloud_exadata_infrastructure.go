package oracledatabase

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceOracleDatabaseCloudExadataInfrastructure() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceOracleDatabaseCloudExadataInfrastructureRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceOracleDatabaseCloudExadataInfrastructure().SchemaMap())
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "location", "cloud_exadata_infrastructure_id")
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func dataSourceOracleDatabaseCloudExadataInfrastructureRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	id, err := tpgresource.ReplaceVars(d, config, "projects/{{project}}/locations/{{location}}/cloudExadataInfrastructures/{{cloud_exadata_infrastructure_id}}")
	if err != nil {
		return fmt.Errorf("Error constructing id: %s", err)
	}
	err = resourceOracleDatabaseCloudExadataInfrastructureRead(d, meta)
	if err != nil {
		return err
	}
	d.SetId(id)

	return nil
}

func init() {
	registry.Schema{
		Name:        "google_oracle_database_cloud_exadata_infrastructure",
		ProductName: "oracledatabase",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceOracleDatabaseCloudExadataInfrastructure(),
	}.Register()
}
