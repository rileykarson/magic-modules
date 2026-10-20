package vpcaccess

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceVPCAccessConnector() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceVPCAccessConnectorRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceVPCAccessConnector().SchemaMap())
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "name")
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project", "region")
			return dsSchema
		},
	}
}

func dataSourceVPCAccessConnectorRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	id, err := tpgresource.ReplaceVars(d, config, "projects/{{project}}/locations/{{region}}/connectors/{{name}}")
	if err != nil {
		return fmt.Errorf("Error constructing id: %s", err)
	}

	d.SetId(id)

	err = resourceVPCAccessConnectorRead(d, meta)
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
		Name:        "google_vpc_access_connector",
		ProductName: "vpcaccess",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceVPCAccessConnector(),
	}.Register()
}
