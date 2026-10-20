package spanner

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceSpannerInstance() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceSpannerInstanceRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceSpannerInstance().SchemaMap())

			tpgresource.AddRequiredFieldsToSchema(dsSchema, "name")
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "config")       // not sure why this is configurable
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "display_name") // not sure why this is configurable
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func dataSourceSpannerInstanceRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	id, err := tpgresource.ReplaceVars(d, config, "{{project}}/{{name}}")
	if err != nil {
		return fmt.Errorf("Error constructing id: %s", err)
	}
	d.SetId(id)

	err = resourceSpannerInstanceRead(d, meta)
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
		Name:        "google_spanner_instance",
		ProductName: "spanner",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceSpannerInstance(),
	}.Register()
}
