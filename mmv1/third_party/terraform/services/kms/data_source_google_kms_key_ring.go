package kms

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceGoogleKmsKeyRing() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceGoogleKmsKeyRingRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceKMSKeyRing().SchemaMap())
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "name")
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "location")
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func dataSourceGoogleKmsKeyRingRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	project, err := tpgresource.GetProject(d, config)
	if err != nil {
		return err
	}

	keyRingId := KmsKeyRingId{
		Name:     d.Get("name").(string),
		Location: d.Get("location").(string),
		Project:  project,
	}
	id := keyRingId.KeyRingId()
	d.SetId(id)

	err = resourceKMSKeyRingRead(d, meta)
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
		Name:        "google_kms_key_ring",
		ProductName: "kms",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceGoogleKmsKeyRing(),
	}.Register()
}
