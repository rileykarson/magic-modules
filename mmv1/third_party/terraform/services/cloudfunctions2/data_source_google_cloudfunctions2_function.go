package cloudfunctions2

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceGoogleCloudFunctions2Function() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceGoogleCloudFunctions2FunctionRead,
		SchemaFunc: func() map[string]*schema.Schema {
			// Generate datasource schema from resource
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceCloudfunctions2function().SchemaMap())

			// Set 'Required' schema elements
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "name", "location")

			// Set 'Optional' schema elements
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func dataSourceGoogleCloudFunctions2FunctionRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	project, err := tpgresource.GetProject(d, config)
	if err != nil {
		return err
	}

	id := fmt.Sprintf("projects/%s/locations/%s/functions/%s", project, d.Get("location").(string), d.Get("name").(string))
	d.SetId(id)

	err = resourceCloudfunctions2functionRead(d, meta)
	if err != nil {
		return err
	}

	if err := tpgresource.SetDataSourceLabels(d); err != nil {
		return err
	}

	if d.Id() == "" {
		return fmt.Errorf("%s not found", id)
	}

	return nil
}

func init() {
	registry.Schema{
		Name:        "google_cloudfunctions2_function",
		ProductName: "cloudfunctions2",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceGoogleCloudFunctions2Function(),
	}.Register()
}
