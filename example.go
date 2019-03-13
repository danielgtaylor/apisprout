package main

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
)

// Mode defines a mode of operation for example generation.
type Mode int

const (
	// ModeRequest is for the request body (writes to the server)
	ModeRequest Mode = iota
	// ModeResponse is for the response body (reads from the server)
	ModeResponse
)

func getSchemaExample(schema *openapi3.Schema) (interface{}, bool) {
	if schema.Example != nil {
		return schema.Example, true
	}

	if schema.Default != nil {
		return schema.Default, true
	}

	if schema.Enum != nil && len(schema.Enum) > 0 {
		return schema.Enum[0], true
	}

	return nil, false
}

// stringFormatExample returns an example string based on the given format.
// http://json-schema.org/latest/json-schema-validation.html#rfc.section.7.3
func stringFormatExample(format string) string {
	switch format {
	case "date":
		// https://tools.ietf.org/html/rfc3339
		return "2018-07-23"
	case "date-time":
		// This is the date/time of API Sprout's first commit! :-)
		return "2018-07-23T22:58:00-07:00"
	case "time":
		return "22:58:00-07:00"
	case "email":
		return "email@example.com"
	case "hostname":
		// https://tools.ietf.org/html/rfc2606#page-2
		return "example.com"
	case "ipv4":
		// https://tools.ietf.org/html/rfc5737
		return "198.51.100.0"
	case "ipv6":
		// https://tools.ietf.org/html/rfc3849
		return "2001:0db8:85a3:0000:0000:8a2e:0370:7334"
	case "uri":
		return "https://tools.ietf.org/html/rfc3986"
	case "uri-template":
		// https://tools.ietf.org/html/rfc6570
		return "http://example.com/dictionary/{term:1}/{term}"
	case "json-pointer":
		// https://tools.ietf.org/html/rfc6901
		return "#/components/parameters/term"
	case "regex":
		// https://stackoverflow.com/q/3296050/164268
		return "/^1?$|^(11+?)\\1+$/"
	case "uuid":
		// https://www.ietf.org/rfc/rfc4122.txt
		return "f81d4fae-7dec-11d0-a765-00a0c91e6bf6"
	case "password":
		return "********"
	}

	return ""
}

// excludeFromMode will exclude a schema if the mode is request and the schema
// is read-only, or if the mode is response and the schema is write only.
func excludeFromMode(mode Mode, schema *openapi3.Schema) bool {
	if schema == nil {
		return true
	}

	if mode == ModeRequest && schema.ReadOnly {
		return true
	} else if mode == ModeResponse && schema.WriteOnly {
		return true
	}

	return false
}

// OpenAPIExample creates an example structure from an OpenAPI 3 schema
// object, which is an extended subset of JSON Schema.
// https://github.com/OAI/OpenAPI-Specification/blob/master/versions/3.0.1.md#schemaObject
func OpenAPIExample(mode Mode, schema *openapi3.Schema) (interface{}, error) {
	if ex, ok := getSchemaExample(schema); ok {
		return ex, nil
	}

	switch {
	case schema.Type == "boolean":
		return true, nil
	case schema.Type == "number", schema.Type == "integer":
		value := 0.0

		if schema.Min != nil && *schema.Min > value {
			value = *schema.Min
			if schema.ExclusiveMin {
				if schema.Max != nil {
					// Make the value half way.
					value = (*schema.Min + *schema.Max) / 2.0
				} else {
					value++
				}
			}
		}

		if schema.Max != nil && *schema.Max < value {
			value = *schema.Max
			if schema.ExclusiveMax {
				if schema.Min != nil {
					// Make the value half way.
					value = (*schema.Min + *schema.Max) / 2.0
				} else {
					value--
				}
			}
		}

		if schema.MultipleOf != nil && int(value)%int(*schema.MultipleOf) != 0 {
			value += float64(int(*schema.MultipleOf) - (int(value) % int(*schema.MultipleOf)))
		}

		if schema.Type == "integer" {
			return int(value), nil
		}

		return value, nil
	case schema.Type == "string":
		if ex := stringFormatExample(schema.Format); ex != "" {
			return ex, nil
		}

		example := "string"

		for schema.MinLength > uint64(len(example)) {
			example += example
		}

		if schema.MaxLength != nil && *schema.MaxLength < uint64(len(example)) {
			example = example[:*schema.MaxLength]
		}

		return example, nil
	case schema.Type == "array", schema.Items != nil:
		example := []interface{}{}

		if schema.Items != nil && schema.Items.Value != nil {
			ex, err := OpenAPIExample(mode, schema.Items.Value)
			if err != nil {
				return nil, fmt.Errorf("can't get example for array item")
			}

			example = append(example, ex)

			for uint64(len(example)) < schema.MinItems {
				example = append(example, ex)
			}
		}

		return example, nil
	case schema.Type == "object", len(schema.Properties) > 0:
		example := map[string]interface{}{}

		for k, v := range schema.Properties {
			if excludeFromMode(mode, v.Value) {
				continue
			}

			ex, err := OpenAPIExample(mode, v.Value)
			if err != nil {
				return nil, fmt.Errorf("can't get example for '%s'", k)
			}

			example[k] = ex
		}

		if schema.AdditionalProperties != nil && schema.AdditionalProperties.Value != nil {
			addl := schema.AdditionalProperties.Value

			if !excludeFromMode(mode, addl) {
				ex, err := OpenAPIExample(mode, addl)
				if err != nil {
					return nil, fmt.Errorf("can't get example for additional properties")
				}

				example["additionalPropertyName"] = ex
			}
		}

		return example, nil
	}

	return nil, ErrNoExample
}
