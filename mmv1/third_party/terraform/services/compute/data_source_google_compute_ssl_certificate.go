package compute

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func DataSourceGoogleComputeSslCertificate() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceComputeSslCertificateRead,
		SchemaFunc: func() map[string]*schema.Schema {
			// Generate datasource schema from resource
			dsSchema := tpgresource.DatasourceSchemaFromResourceSchema(ResourceComputeSslCertificate().SchemaMap())

			// Set 'Required' schema elements
			tpgresource.AddRequiredFieldsToSchema(dsSchema, "name")

			// Set 'Optional' schema elements
			tpgresource.AddOptionalFieldsToSchema(dsSchema, "project")
			return dsSchema
		},
	}
}

func dataSourceComputeSslCertificateRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*transport_tpg.Config)

	project, err := tpgresource.GetProject(d, config)
	if err != nil {
		return err
	}
	certificateName := d.Get("name").(string)

	id := fmt.Sprintf("projects/%s/global/sslCertificates/%s", project, certificateName)
	d.SetId(id)

	err = resourceComputeSslCertificateRead(d, meta)
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
		Name:        "google_compute_ssl_certificate",
		ProductName: "compute",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceGoogleComputeSslCertificate(),
	}.Register()
}
