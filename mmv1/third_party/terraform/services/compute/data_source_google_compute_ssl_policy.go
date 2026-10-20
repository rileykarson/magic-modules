package compute

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceGoogleComputeSslPolicy() *schema.Resource {
	return &schema.Resource{
		Read: datasourceComputeSslPolicyRead,
		SchemaFunc: func() map[string]*schema.Schema {
			// Generate datasource schema from resource
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceComputeSslPolicy().SchemaMap())

			// Set 'Required' schema elements
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "name")

			// Set 'Optional' schema elements
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func datasourceComputeSslPolicyRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	project, err := tpgresource.GetProject(d, config)
	if err != nil {
		return err
	}
	policyName := d.Get("name").(string)

	id := fmt.Sprintf("projects/%s/global/sslPolicies/%s", project, policyName)
	d.SetId(id)

	err = resourceComputeSslPolicyRead(d, meta)
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
		Name:        "google_compute_ssl_policy",
		ProductName: "compute",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceGoogleComputeSslPolicy(),
	}.Register()
}
