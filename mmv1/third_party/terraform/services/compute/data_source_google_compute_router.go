package compute

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
)

func DataSourceGoogleComputeRouter() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceComputeRouterRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceComputeRouter().SchemaMap())
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "name")
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "network")
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "region")
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func dataSourceComputeRouterRead(d *schema.ResourceData, meta interface{}) error {
	routerName := d.Get("name").(string)

	d.SetId(routerName)
	err := resourceComputeRouterRead(d, meta)
	if err != nil {
		return err
	}

	if d.Id() == "" {
		return fmt.Errorf("%s not found", routerName)
	}
	return nil
}

func init() {
	registry.Schema{
		Name:        "google_compute_router",
		ProductName: "compute",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceGoogleComputeRouter(),
	}.Register()
}
