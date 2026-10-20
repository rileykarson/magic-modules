package kms

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceGoogleKmsCryptoKey() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceGoogleKmsCryptoKeyRead,
		SchemaFunc: func() map[string]*schema.Schema {
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceKMSCryptoKey().SchemaMap())
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "name")
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "key_ring")
			return dsSchema
		},
	}
}

func dataSourceGoogleKmsCryptoKeyRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	keyRingId, err := parseKmsKeyRingId(d.Get("key_ring").(string), config)
	if err != nil {
		return err
	}

	cryptoKeyId := KmsCryptoKeyId{
		KeyRingId: *keyRingId,
		Name:      d.Get("name").(string),
	}

	id := cryptoKeyId.CryptoKeyId()
	d.SetId(id)

	err = resourceKMSCryptoKeyRead(d, meta)
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
		Name:        "google_kms_crypto_key",
		ProductName: "kms",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceGoogleKmsCryptoKey(),
	}.Register()
}
