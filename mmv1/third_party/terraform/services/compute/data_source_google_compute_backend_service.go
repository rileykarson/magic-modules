package compute

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceGoogleComputeBackendService() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceComputeBackendServiceRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceComputeBackendService().SchemaMap())

			// Set 'Required' schema elements
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "name")

			// Set 'Optional' schema elements
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func dataSourceComputeBackendServiceRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	serviceName := d.Get("name").(string)

	project, err := tpgresource.GetProject(d, config)
	if err != nil {
		return err
	}

	id := fmt.Sprintf("projects/%s/global/backendServices/%s", project, serviceName)
	d.SetId(id)

	err = resourceComputeBackendServiceRead(d, meta)
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
		Name:        "google_compute_backend_service",
		ProductName: "compute",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceGoogleComputeBackendService(),
	}.Register()
}
