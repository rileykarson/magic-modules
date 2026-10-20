package vmwareengine

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceVmwareengineNetwork() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceVmwareengineNetworkRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceVmwareengineNetwork().SchemaMap())
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "location", "name")
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func dataSourceVmwareengineNetworkRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	// Store the ID now
	id, err := tpgresource.ReplaceVars(d, config, "projects/{{project}}/locations/{{location}}/vmwareEngineNetworks/{{name}}")
	if err != nil {
		return fmt.Errorf("Error constructing id: %s", err)
	}
	d.SetId(id)
	err = resourceVmwareengineNetworkRead(d, meta)
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
		Name:        "google_vmwareengine_network",
		ProductName: "vmwareengine",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceVmwareengineNetwork(),
	}.Register()
}
