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
require 'provider/config'

module Provider
  class TerraformCloudShell < Provider::Terraform
    # Settings for the provider
    class Config < Provider::Terraform::Config
      def provider
        Provider::TerraformCloudShell
      end

      def resource_override
        Provider::Terraform::ResourceOverride
      end

      def property_override
        Provider::Terraform::PropertyOverride
      end

      def self.parse(cfg_file, api = nil, _version_name = nil)
        source = Provider::TerraformCloudShell::Config.compile(cfg_file)
        config = Google::YamlValidator.parse(source)
        puts config.class
        config.default_overrides
        config.spread_api config, api, [], '' unless api.nil?
        config.validate
        config
      end
    end
  end
end
