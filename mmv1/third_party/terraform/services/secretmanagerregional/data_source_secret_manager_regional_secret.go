package secretmanagerregional

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceSecretManagerRegionalRegionalSecret() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceSecretManagerRegionalRegionalSecretRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceSecretManagerRegionalRegionalSecret().SchemaMap())
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "secret_id")
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "location")
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func dataSourceSecretManagerRegionalRegionalSecretRead(d *schema.ResourceData, meta interface{}) error {
	id, err := tpgresource.ReplaceVars(d, meta.(*transport_tpg.Config), "projects/{{project}}/locations/{{location}}/secrets/{{secret_id}}")
	if err != nil {
		return fmt.Errorf("Error constructing id: %s", err)
	}
	d.SetId(id)
	err = resourceSecretManagerRegionalRegionalSecretRead(d, meta)
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
		Name:        "google_secret_manager_regional_secret",
		ProductName: "secretmanagerregional",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceSecretManagerRegionalRegionalSecret(),
	}.Register()
}
