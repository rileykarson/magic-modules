package compute

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceGoogleComputeHealthCheck() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceGoogleComputeHealthCheckRead,
		SchemaFunc: func() map[string]*schema.Schema {
			// Generate datasource schema from resource
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceComputeHealthCheck().SchemaMap())

			// Set 'Required' schema elements
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "name")

			// Set 'Optional' schema elements
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func dataSourceGoogleComputeHealthCheckRead(d *schema.ResourceData, meta interface{}) error {
	id, err := tpgresource.ReplaceVars(d, meta.(*transport_tpg.Config), "projects/{{project}}/global/healthChecks/{{name}}")
	if err != nil {
		return err
	}
	d.SetId(id)

	err = resourceComputeHealthCheckRead(d, meta)
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
		Name:        "google_compute_health_check",
		ProductName: "compute",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceGoogleComputeHealthCheck(),
	}.Register()
}
