package observability

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
)

func DataSourceObservabilityProjectSettings() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceObservabilityProjectSettingsRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceObservabilityProjectSettings().SchemaMap())

			tpgresource.AddRequiredFieldsToSchema(dsSchema, "project")
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "location")
			return dsSchema
		},
	}
}

func dataSourceObservabilityProjectSettingsRead(d *schema.ResourceData, meta interface{}) error {
	project := d.Get("project").(string)
	location := d.Get("location").(string)

	id := fmt.Sprintf("projects/%s/locations/%s/settings", project, location)
	d.SetId(id)

	err := resourceObservabilityProjectSettingsRead(d, meta)
	if err != nil {
		return nil
	}
	if d.Id() == "" {
		return fmt.Errorf("%s not found", id)
	}
	return nil
}

func init() {
	registry.Schema{
		Name:        "google_observability_project_settings",
		ProductName: "observability",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceObservabilityProjectSettings(),
	}.Register()
}
