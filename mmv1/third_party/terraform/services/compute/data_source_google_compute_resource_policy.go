package compute

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceGoogleComputeResourcePolicy() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceGoogleComputeResourcePolicyRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceComputeResourcePolicy().SchemaMap())

			tpgresource.AddRequiredFieldsToSchema(dsSchema, "name")
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "region")
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func dataSourceGoogleComputeResourcePolicyRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	name := d.Get("name").(string)

	project, err := tpgresource.GetProject(d, config)
	if err != nil {
		return err
	}
	region, err := tpgresource.GetRegion(d, config)
	if err != nil {
		return err
	}

	id := fmt.Sprintf("projects/%s/regions/%s/resourcePolicies/%s", project, region, name)
	d.SetId(id)

	err = resourceComputeResourcePolicyRead(d, meta)
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
		Name:        "google_compute_resource_policy",
		ProductName: "compute",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceGoogleComputeResourcePolicy(),
	}.Register()
}
