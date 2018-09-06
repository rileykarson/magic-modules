require 'provider/config/function/argument'

module Provider
  class Config < Api::Object

    # A Function is a custom client side function provided by the module.
    class Function < Api::Object::Named
      attr_reader :description
      attr_reader :arguments
      attr_reader :requires
      attr_reader :code
      attr_reader :helpers
      attr_reader :examples
      attr_reader :notes

      def validate
        super
        check_property_list :requires, String
        check_property :code, String
        check_property_list :arguments, Provider::Config::Function::Argument
        check_optional_property :helpers, String
      end
    end
  end
end
