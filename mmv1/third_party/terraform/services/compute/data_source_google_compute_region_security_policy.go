package compute

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceGoogleComputeRegionSecurityPolicy() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceGoogleComputeRegionSecurityPolicyRead,
		SchemaFunc: func() map[string]*schema.Schema {
			// Generate datasource schema from resource
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceComputeRegionSecurityPolicy().SchemaMap())

			// Set 'Required' schema elements
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "name")
			// Set 'Optional' schema elements
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project", "region")
			return dsSchema
		},
	}
}

func dataSourceGoogleComputeRegionSecurityPolicyRead(d *schema.ResourceData, meta interface{}) error {
	id, err := tpgresource.ReplaceVars(d, meta.(*transport_tpg.Config), "projects/{{project}}/regions/{{region}}/securityPolicies/{{name}}")
	if err != nil {
		return fmt.Errorf("Error constructing id: %s", err)
	}
	d.SetId(id)

	err = resourceComputeRegionSecurityPolicyRead(d, meta)
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
		Name:        "google_compute_region_security_policy",
		ProductName: "compute",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceGoogleComputeRegionSecurityPolicy(),
	}.Register()
}
