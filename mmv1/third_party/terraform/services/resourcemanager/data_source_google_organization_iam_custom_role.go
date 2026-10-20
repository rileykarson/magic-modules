package resourcemanager

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
)

func DataSourceGoogleOrganizationIamCustomRole() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceOrganizationIamCustomRoleRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceGoogleOrganizationIamCustomRole().SchemaMap())

			tpgresource.AddRequiredFieldsToSchema(dsSchema, "org_id")
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "role_id")
			return dsSchema
		},
	}
}

func dataSourceOrganizationIamCustomRoleRead(d *schema.ResourceData, meta interface{}) error {
	orgId := d.Get("org_id").(string)
	roleId := d.Get("role_id").(string)
	d.SetId(fmt.Sprintf("organizations/%s/roles/%s", orgId, roleId))

	id := d.Id()

	if err := resourceGoogleOrganizationIamCustomRoleRead(d, meta); err != nil {
		return err
	}

	if d.Id() == "" {
		return fmt.Errorf("Role %s not found!", id)
	}

	return nil
}

func init() {
	registry.Schema{
		Name:        "google_organization_iam_custom_role",
		ProductName: "resourcemanager",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceGoogleOrganizationIamCustomRole(),
	}.Register()
}
