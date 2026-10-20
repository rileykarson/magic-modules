package resourcemanager

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
)

func DataSourceGoogleFolderOrganizationPolicy() *schema.Resource {
	return &schema.Resource{
		Read: datasourceGoogleFolderOrganizationPolicyRead,
		SchemaFunc: func() map[string]*schema.Schema {
			// Generate datasource schema from resource
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceGoogleFolderOrganizationPolicy().SchemaMap())

			tpgresource.AddRequiredFieldsToSchema(dsSchema, "folder")
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "constraint")
			return dsSchema
		},
	}
}

func datasourceGoogleFolderOrganizationPolicyRead(d *schema.ResourceData, meta interface{}) error {

	id := fmt.Sprintf("%s/%s", d.Get("folder"), d.Get("constraint"))
	d.SetId(id)

	err := resourceGoogleFolderOrganizationPolicyRead(d, meta)
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
		Name:        "google_folder_organization_policy",
		ProductName: "resourcemanager",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceGoogleFolderOrganizationPolicy(),
	}.Register()
}
