package oracledatabase

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceOracleDatabaseCloudVmCluster() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceOracleDatabaseCloudVmClusterRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceOracleDatabaseCloudVmCluster().SchemaMap())
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "location", "cloud_vm_cluster_id")
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func dataSourceOracleDatabaseCloudVmClusterRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	id, err := tpgresource.ReplaceVars(d, config, "projects/{{project}}/locations/{{location}}/cloudVmClusters/{{cloud_vm_cluster_id}}")
	if err != nil {
		return fmt.Errorf("Error constructing id: %s", err)
	}
	err = resourceOracleDatabaseCloudVmClusterRead(d, meta)
	if err != nil {
		return err
	}
	d.SetId(id)

	return nil
}

func init() {
	registry.Schema{
		Name:        "google_oracle_database_cloud_vm_cluster",
		ProductName: "oracledatabase",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceOracleDatabaseCloudVmCluster(),
	}.Register()
}
