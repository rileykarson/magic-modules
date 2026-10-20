package compute

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceGoogleRegionComputeSslCertificate() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceComputeRegionSslCertificateRead,
		SchemaFunc: func() map[string]*schema.Schema {
			// Generate datasource schema from resource
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceComputeRegionSslCertificate().SchemaMap())

			// Set 'Required' schema elements
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "name")

			// Set 'Optional' schema elements
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "region")
			return dsSchema
		},
	}
}

func dataSourceComputeRegionSslCertificateRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	project, region, name, err := tpgresource.GetRegionalResourcePropertiesFromSelfLinkOrSchema(d, config)
	if err != nil {
		return err
	}

	id := fmt.Sprintf("projects/%s/regions/%s/sslCertificates/%s", project, region, name)
	d.SetId(id)

	err = resourceComputeRegionSslCertificateRead(d, meta)
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
		Name:        "google_compute_region_ssl_certificate",
		ProductName: "compute",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceGoogleRegionComputeSslCertificate(),
	}.Register()
}
