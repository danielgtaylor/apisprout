package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func exampleFixture(t *testing.T, name string) string {
	f, err := os.Open(path.Join("testdata/example", name))
	require.NoError(t, err)
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	require.NoError(t, err)

	return string(b)
}

var schemaTests = []struct {
	name string
	in   string
	out  string
}{
	// ----- Booleans -----
	{
		"Boolean",
		`{"type": "boolean"}`,
		`true`,
	},
	{
		"Boolean default",
		`{"type": "boolean", "default": false}`,
		`false`,
	},
	{
		"Boolean example",
		`{"type": "boolean", "example": false}`,
		`false`,
	},
	// ----- Numbers -----
	{
		"Integer",
		`{"type": "integer"}`,
		`0`,
	},
	{
		"Number",
		`{"type": "number"}`,
		`0.0`,
	},
	{
		"Number default",
		`{"type": "number", "default": 1.0}`,
		`1.0`,
	},
	{
		"Number example",
		`{"type": "number", "example": 2.0}`,
		`2.0`,
	},
	{
		"Number enum",
		`{"type": "number", "enum": [2.0, 4.0, 6.0]}`,
		`2.0`,
	},
	{
		"Number minimum",
		`{"type": "number", "minimum": 5.0}`,
		`5.0`,
	},
	{
		"Number exclusive minimum",
		`{"type": "number", "minimum": 5.0, "exclusiveMinimum": true}`,
		`6.0`,
	},
	{
		"Number maximum",
		`{"type": "number", "maximum": -1.0}`,
		`-1.0`,
	},
	{
		"Number exclusive maximum",
		`{"type": "number", "maximum": -1.0, "exclusiveMaximum": true}`,
		`-2.0`,
	},
	{
		"Number exclusinve minimum with max",
		`{"type": "number", "minimum": 5.0, "exclusiveMinimum": true, "maximum": 5.5}`,
		`5.25`,
	},
	{
		"Number exclusinve maximum with min",
		`{"type": "number", "minimum": -5.0, "maximum": -1.0, "exclusiveMaximum": true}`,
		`-3.0`,
	},
	{
		"Integer multiple of",
		`{"type": "integer", "minimum": 1, "multipleOf": 4}`,
		`4`,
	},
	// ----- Strings -----
	{
		"String",
		`{"type": "string"}`,
		`"string"`,
	},
	{
		"String default",
		`{"type": "string", "default": "def"}`,
		`"def"`,
	},
	{
		"String example",
		`{"type": "string", "example": "ex"}`,
		`"ex"`,
	},
	{
		"String enum",
		`{"type": "string", "enum": ["one", "two", "three"]}`,
		`"one"`,
	},
	{
		"String format date",
		`{"type": "string", "format": "date"}`,
		`"2018-07-23"`,
	},
	{
		"String format date-time",
		`{"type": "string", "format": "date-time"}`,
		`"2018-07-23T22:58:00-07:00"`,
	},
	{
		"String format time",
		`{"type": "string", "format": "time"}`,
		`"22:58:00-07:00"`,
	},
	{
		"String format email",
		`{"type": "string", "format": "email"}`,
		`"email@example.com"`,
	},
	{
		"String format hostname",
		`{"type": "string", "format": "hostname"}`,
		`"example.com"`,
	},
	{
		"String format ipv4",
		`{"type": "string", "format": "ipv4"}`,
		`"198.51.100.0"`,
	},
	{
		"String format ipv6",
		`{"type": "string", "format": "ipv6"}`,
		`"2001:0db8:85a3:0000:0000:8a2e:0370:7334"`,
	},
	{
		"String format uri",
		`{"type": "string", "format": "uri"}`,
		`"https://tools.ietf.org/html/rfc3986"`,
	},
	{
		"String format uri-template",
		`{"type": "string", "format": "uri-template"}`,
		`"http://example.com/dictionary/{term:1}/{term}"`,
	},
	{
		"String format json-pointer",
		`{"type": "string", "format": "json-pointer"}`,
		`"#/components/parameters/term"`,
	},
	{
		"String format regex",
		`{"type": "string", "format": "regex"}`,
		`"/^1?$|^(11+?)\\1+$/"`,
	},
	{
		"String format uuid",
		`{"type": "string", "format": "uuid"}`,
		`"f81d4fae-7dec-11d0-a765-00a0c91e6bf6"`,
	},
	{
		"String format password",
		`{"type": "string", "format": "password"}`,
		`"********"`,
	},
	{
		"String min length",
		`{"type": "string", "minLength": 10}`,
		`"stringstring"`,
	},
	{
		"String max length",
		`{"type": "string", "maxLength": 2}`,
		`"st"`,
	},
	{
		"String min & max length",
		`{"type": "string", "minLength": 8, "maxLength": 10}`,
		`"stringstri"`,
	},
	// ----- Arrays -----
	{
		"Array without items returns empty []",
		`{
			"type": "array"
		}`,
		`[]`,
	},
	{
		"Array of simple type",
		`{
			"type": "array",
			"items": {
				"type": "string"
			}
		}`,
		`["string"]`,
	},
	{
		"Array of simple type with example",
		`{
			"type": "array",
			"items": {
				"type": "string",
				"example": "I'm in an array"
			}
		}`,
		`["I'm in an array"]`,
	},
	{
		"Array of blank items fails",
		`{
			"type": "array",
			"items": {}
		}`,
		``,
	},
	{
		"Array with example",
		`{
			"type": "array",
			"example": [true, false, true],
			"items": {
				"type": "boolean"
			}
		}`,
		`[true, false, true]`,
	},
	{
		"Array of array of simple type",
		`{
			"type": "array",
			"items": {
				"type": "array",
				"items": {
					"type": "number"
				}
			}
		}`,
		`[[0]]`,
	},
	{
		"Array of objects",
		`{
			"type": "array",
			"items": {
				"type": "object",
				"required": ["foo", "bar"],
				"properties": {
					"foo": {
						"type": "boolean"
					},
					"bar": {
						"type": "string",
						"example": "baz"
					}
				}
			}
		}`,
		`[{"foo": true, "bar": "baz"}]`,
	},
	{
		"Array with min items (e.g. coordinates)",
		`{
			"type": "array",
			"minItems": 2,
			"items": {
				"type": "number"
			}
		}`,
		`[0, 0]`,
	},
	// ----- Objects -----
	{
		"Object without properties returns {}",
		`{
			"type": "object"
		}`,
		`{}`,
	},
	{
		"Object with example",
		`{
			"type": "object",
			"example": {
				"foo": 1
			},
			"properties": {
				"foo": {
					"type": "number"
				}
			}
		}`,
		`{"foo": 1}`,
	},
	{
		"Object with simple properties",
		`{
			"type": "object",
			"required": ["foo", "bar"],
			"properties": {
				"foo": {
					"type": "boolean"
				},
				"bar": {
					"type": "string",
					"example": "baz"
				}
			}
		}`,
		`{"foo": true, "bar": "baz"}`,
	},
	{
		"Object with complex properties",
		`{
			"type": "object",
			"properties": {
				"foo": {
					"type": "object",
					"properties": {
						"bar": {
							"type": "array",
							"items": {
								"type": "string"
							}
						}
					}
				}
			}
		}`,
		`{"foo": {"bar": ["string"]}}`,
	},
	{
		"Object with additional properties",
		`{
			"type": "object",
			"properties": {
				"foo": {
					"type": "number"
				}
			},
			"additionalProperties": {
				"type": "string"
			}
		}`,
		`{"foo": 0, "additionalPropertyName": "string"}`,
	},
	{
		"Object with additional properties error",
		`{
			"type": "object",
			"properties": {
				"foo": {
					"type": "number"
				}
			},
			"additionalProperties": {}
		}`,
		``,
	},
	// ----- Precedence -----
	{
		"Example before default",
		`{"type": "string", "default": "one", "example": "two"}`,
		`"two"`,
	},
	// ----- Modes -----
	{
		"Request mode",
		`{"type": "object", "required": ["normal", "readOnly", "writeOnly"],
			"properties": {
				"normal": {
					"type": "string"
				},
				"readOnly": {
					"type": "string",
					"readOnly": true
				},
				"writeOnly": {
					"type": "string",
					"writeOnly": true
				}
			}
		}`,
		`{"normal": "string", "writeOnly": "string"}`,
	},
	{
		"Response mode",
		`{"type": "object", "required": ["normal", "readOnly", "writeOnly"],
			"properties": {
				"normal": {
					"type": "string"
				},
				"readOnly": {
					"type": "string",
					"readOnly": true
				},
				"writeOnly": {
					"type": "string",
					"writeOnly": true
				}
			}
		}`,
		`{"normal": "string", "readOnly": "string"}`,
	},
	// ----- Combination keywords -----
	{
		"Combine with allOf",
		`{
			"allOf": [
				{
					"type": "object",
					"properties": {
						"foo": {"type": "string"}
					}
				},
				{
					"type": "object",
					"properties": {
						"bar": {"type": "boolean"}
					}
				}
			]
		}`,
		`{"foo": "string", "bar": true}`,
	},
	{
		"Combine with anyOf",
		`{
			"anyOf": [
				{
					"type": "object",
					"properties": {
						"foo": {"type": "string"}
					}
				},
				{
					"type": "object",
					"properties": {
						"bar": {"type": "boolean"}
					}
				}
			]
		}`,
		`{"foo": "string"}`,
	},
	{
		"Combine with oneOf",
		`{
			"oneOf": [
				{
					"type": "object",
					"properties": {
						"foo": {"type": "string"}
					}
				},
				{
					"type": "object",
					"properties": {
						"bar": {"type": "boolean"}
					}
				}
			]
		}`,
		`{"foo": "string"}`,
	},
}

func TestGenExample(t *testing.T) {
	for _, tt := range schemaTests {
		t.Run(tt.name, func(t *testing.T) {
			schema := &openapi3.Schema{}
			err := schema.UnmarshalJSON([]byte(tt.in))
			assert.NoError(t, err)
			m := ModeRequest
			if strings.Contains(tt.name, "Response") {
				m = ModeResponse
			}
			example, err := OpenAPIExample(m, schema)

			if tt.out == "" {
				// Expected to return an error.
				assert.Nil(t, example)
				assert.Error(t, err)
			} else {
				// Expected to match the output.
				var expected interface{}
				json.Unmarshal([]byte(tt.out), &expected)
				assert.EqualValues(t, expected, example)
			}
		})
	}
}

func TestRecursiveSchema(t *testing.T) {
	loader := openapi3.NewSwaggerLoader()

	tests := []struct {
		name   string
		in     string
		schema string
		out    string
	}{
		{
			"Valid recursive schema",
			exampleFixture(t, "recursive_ok.yml"),
			"Test",
			`{"something": "Hello"}`,
		},
		{
			"Infinitely recursive schema",
			exampleFixture(t, "recursive_infinite.yml"),
			"Test",
			``,
		},
		{
			"Seeing the same schema twice non-recursively",
			exampleFixture(t, "recursive_seen_twice.yml"),
			"Test",
			`{"ref_a": {"spud": "potato"}, "ref_b": {"spud": "potato"}}`,
		},
		{
			"Cyclical dependencies",
			exampleFixture(t, "recursive_cycles.yml"),
			"Front",
			``,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			swagger, err := loader.LoadSwaggerFromData([]byte(test.in))
			require.NoError(t, err)

			ex, err := OpenAPIExample(ModeResponse, swagger.Components.Schemas[test.schema].Value)
			if test.out == "" {
				assert.Error(t, err)
				assert.Nil(t, ex)
			} else {
				assert.Nil(t, err)
				// Expected to match the output.
				var expected interface{}
				json.Unmarshal([]byte(test.out), &expected)
				assert.EqualValues(t, expected, ex)
			}
		})
	}
}
