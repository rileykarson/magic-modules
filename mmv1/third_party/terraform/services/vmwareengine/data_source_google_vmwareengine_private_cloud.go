package vmwareengine

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceVmwareenginePrivateCloud() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceVmwareenginePrivateCloudRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceVmwareenginePrivateCloud().SchemaMap())
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "name", "location")
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func dataSourceVmwareenginePrivateCloudRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	// Store the ID now
	id, err := tpgresource.ReplaceVars(d, config, "projects/{{project}}/locations/{{location}}/privateClouds/{{name}}")
	if err != nil {
		return fmt.Errorf("Error constructing id: %s", err)
	}
	d.SetId(id)
	err = resourceVmwareenginePrivateCloudRead(d, meta)
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
		Name:        "google_vmwareengine_private_cloud",
		ProductName: "vmwareengine",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceVmwareenginePrivateCloud(),
	}.Register()
}
