package beyondcorp

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceGoogleBeyondcorpSecurityGateway() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceGoogleBeyondcorpSecurityGatewayRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceBeyondcorpSecurityGateway().SchemaMap())
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "security_gateway_id")

			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func dataSourceGoogleBeyondcorpSecurityGatewayRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	name := d.Get("security_gateway_id").(string)

	project, err := tpgresource.GetProject(d, config)
	if err != nil {
		return err
	}

	id := fmt.Sprintf("projects/%s/locations/global/securityGateways/%s", project, name)
	d.SetId(id)
	// Remove after deprecated location variable is removed.
	d.Set("location", "global")

	err = resourceBeyondcorpSecurityGatewayRead(d, meta)
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
		Name:        "google_beyondcorp_security_gateway",
		ProductName: "beyondcorp",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceGoogleBeyondcorpSecurityGateway(),
	}.Register()
}
