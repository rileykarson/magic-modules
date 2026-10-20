package gkehub2

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceGoogleGkeHubFeature() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceGoogleGkeHubFeatureRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceGKEHub2Feature().SchemaMap())
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "location")
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "name")
			return dsSchema
		},
	}
}

func dataSourceGoogleGkeHubFeatureRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	id, err := tpgresource.ReplaceVars(d, config, "projects/{{project}}/locations/{{location}}/features/{{name}}")
	if err != nil {
		return fmt.Errorf("Error constructing id: %s", err)
	}

	d.SetId(id)

	err = resourceGKEHub2FeatureRead(d, meta)
	if err != nil {
		return err
	}

	if err := tpgresource.SetDataSourceLabels(d); err != nil {
		return err
	}

	if err := tpgresource.SetDataSourceAnnotations(d); err != nil {
		return err
	}

	if d.Id() == "" {
		return fmt.Errorf("%s not found", id)
	}
	return nil
}

func init() {
	registry.Schema{
		Name:        "google_gke_hub_feature",
		ProductName: "gkehub2",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceGoogleGkeHubFeature(),
	}.Register()
}
