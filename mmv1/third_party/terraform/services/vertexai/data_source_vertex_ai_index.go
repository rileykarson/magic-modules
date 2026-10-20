package vertexai

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceVertexAIIndex() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceVertexAIIndexRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceVertexAIIndex().SchemaMap())

			tpgresource.AddRequiredFieldsToSchema(dsSchema, "name", "region")
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func dataSourceVertexAIIndexRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	id, err := tpgresource.ReplaceVars(d, config, "projects/{{project}}/locations/{{region}}/indexes/{{name}}")
	if err != nil {
		return fmt.Errorf("Error constructing id: %s", err)
	}
	d.SetId(id)
	err = resourceVertexAIIndexRead(d, meta)
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
		Name:        "google_vertex_ai_index",
		ProductName: "vertexai",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceVertexAIIndex(),
	}.Register()
}
