

module Provider
  class Config < Api::Object
    class Function < Api::Object::Named

      # An argument required by the function being provided by the module.
      class Argument < Api::Object::Named
        attr_reader :description
        attr_reader :type

        def validate
          super
          check_property :description, String
          check_property :type, String
        end
      end
    end
  end
end
