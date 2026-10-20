package sourcerepo

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceGoogleSourceRepoRepository() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceGoogleSourceRepoRepositoryRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceSourceRepoRepository().SchemaMap())

			tpgresource.AddRequiredFieldsToSchema(dsSchema, "name")
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func dataSourceGoogleSourceRepoRepositoryRead(d *schema.ResourceData, meta interface{}) error {

	config := meta.(*transport_tpg.Config)

	id, err := tpgresource.ReplaceVars(d, config, "projects/{{project}}/repos/{{name}}")
	if err != nil {
		return fmt.Errorf("Error constructing id: %s", err)
	}
	d.SetId(id)

	err = resourceSourceRepoRepositoryRead(d, meta)
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
		Name:        "google_sourcerepo_repository",
		ProductName: "sourcerepo",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceGoogleSourceRepoRepository(),
	}.Register()
}
