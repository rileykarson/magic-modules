# Copyright 2017 Google Inc.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

require 'provider/abstract_core'
require 'provider/terraform/config_cloudshell'
require 'provider/terraform/import'
require 'provider/terraform/custom_code'
require 'provider/terraform/property_override'
require 'provider/terraform/resource_override'
require 'provider/terraform/sub_template'
require 'google/golang_utils'

module Provider
  # Code generator for Terraform Resources that manage Google Cloud Platform
  # resources.
  class TerraformCloudShell < Provider::AbstractCore

    private

    # This function uses the resource.erb template to create one file
    # per resource. The resource.erb template forms the basis of a single
    # GCP Resource on Terraform.
    def generate_resource(data)
      target_folder = File.join(data[:output_folder], 'google')
      FileUtils.mkpath target_folder
      name = data[:object].name.underscore
      product_name = data[:product_name].underscore
      filepath = File.join(target_folder, "resourcea_#{product_name}_#{name}.go")
      generate_resource_file data.clone.merge(
        default_template: 'templates/terraform/resource.erb',
        out_file: filepath
      )
      # TODO: error check goimports
      %x(goimports -w #{filepath})
    end
  end
end
