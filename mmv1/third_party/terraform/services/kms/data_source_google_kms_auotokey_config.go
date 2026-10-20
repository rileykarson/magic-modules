package kms

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
)

func DataSourceGoogleKmsAutokeyConfig() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceGoogleKmsAutokeyConfigRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceKMSAutokeyConfig().SchemaMap())
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "folder")
			return dsSchema
		},
	}
}

func dataSourceGoogleKmsAutokeyConfigRead(d *schema.ResourceData, meta interface{}) error {
	configId := KmsAutokeyConfigId{
		Folder: d.Get("folder").(string),
	}
	id := configId.AutokeyConfigId()
	d.SetId(id)
	err := resourceKMSAutokeyConfigRead(d, meta)
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
		Name:        "google_kms_autokey_config",
		ProductName: "kms",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceGoogleKmsAutokeyConfig(),
	}.Register()
}
