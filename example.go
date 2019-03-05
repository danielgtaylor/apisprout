package main

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
)

// getTypedExampleFromSchema will return an example from a given schema
func getTypedExampleFromSchema(schema *openapi3.Schema) (interface{}, error) {
	if schema.Example != nil {
		return schema.Example, nil
	}

	if schema.Type == "number" {
		return 0, nil
	}
	if schema.Type == "integer" {
		return 0, nil
	}
	if schema.Type == "boolean" {
		return true, nil
	}
	if schema.Type == "string" {
		return "string", nil
	}
	if schema.Type == "array" {
		example := []interface{}{}
		if schema.Items != nil && schema.Items.Value != nil {
			ex, err := getTypedExampleFromSchema(schema.Items.Value)
			if err != nil {
				return nil, fmt.Errorf("can't get example for array item")
			}
			example = append(example, ex)
		}
		return example, nil
	}

	if schema.Type == "object" || len(schema.Properties) > 0 {
		example := map[string]interface{}{}
		for k, v := range schema.Properties {
			ex, err := getTypedExampleFromSchema(v.Value)
			if err != nil {
				return nil, fmt.Errorf("can't get example for '%s'", k)
			}
			example[k] = ex
		}

		if schema.AdditionalProperties != nil && schema.AdditionalProperties.Value != nil {
			addl := schema.AdditionalProperties.Value
			ex, err := getTypedExampleFromSchema(addl)
			if err != nil {
				return nil, fmt.Errorf("can't get example for additional properties")
			}

			example["additionalPropertyName"] = ex
		}

		return example, nil
	}

	return nil, ErrNoExample
}
