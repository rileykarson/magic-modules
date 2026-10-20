package apphub

import (
	"fmt"

	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func DataSourceGoogleApphubApplication() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceGoogleApphubApplicationRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceApphubApplication().SchemaMap())
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "project")
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "application_id")
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "location")
			return dsSchema
		},
	}
}

func dataSourceGoogleApphubApplicationRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	id, err := tpgresource.ReplaceVars(d, config, "projects/{{project}}/locations/{{location}}/applications/{{application_id}}")
	if err != nil {
		return fmt.Errorf("Error constructing id: %s", err)
	}
	d.SetId(id)
	err = resourceApphubApplicationRead(d, meta)
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
		Name:        "google_apphub_application",
		ProductName: "apphub",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceGoogleApphubApplication(),
	}.Register()
}
