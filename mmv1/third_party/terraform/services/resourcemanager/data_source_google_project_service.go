package resourcemanager

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceGoogleProjectService() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceGoogleProjectServiceRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceGoogleProjectService().SchemaMap())
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "service")
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func dataSourceGoogleProjectServiceRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	id, err := tpgresource.ReplaceVars(d, config, "{{project}}/{{service}}")
	if err != nil {
		return fmt.Errorf("Error constructing id: %s", err)
	}
	d.SetId(id)
	return resourceGoogleProjectServiceRead(d, meta)
}

func init() {
	registry.Schema{
		Name:        "google_project_service",
		ProductName: "resourcemanager",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceGoogleProjectService(),
	}.Register()
}
