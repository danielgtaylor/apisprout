package main

import (
	"encoding/json"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
)

type getTypedExampleTestData struct {
	name                string
	generateInputSchema func() *openapi3.Schema
	validateResult      func(*testing.T, string)
}

func Test_GetTypedExampleShouldGetFromExampleField(t *testing.T) {
	exampleData := map[string]string{"name1": "value1", "name2": "value2"}

	mediaType := openapi3.NewMediaType()
	mediaType.WithExample("example1", exampleData)

	selectedExample, err := getTypedExample(mediaType)
	assert.Nil(t, err)

	selectedExampleJSON, err := json.Marshal(selectedExample)
	assert.Nil(t, err)
	assert.NotEmpty(t, string(selectedExampleJSON))
	assert.Equal(t, `{"name1":"value1","name2":"value2"}`, string(selectedExampleJSON))
}

var getTypedExampleTestDataEntries = []getTypedExampleTestData{
	getTypedExampleTestData{
		name: "SchemaExampleField",
		generateInputSchema: func() *openapi3.Schema {
			exampleData := map[string]string{"name1": "value1", "name2": "value2"}
			schema := openapi3.NewSchema()
			schema.Example = exampleData
			return schema
		},
		validateResult: func(t *testing.T, s string) {
			assert.Equal(t, `{"name1":"value1","name2":"value2"}`, s)
		},
	},
	getTypedExampleTestData{
		name: "SchemaPropertiesExampleField",
		generateInputSchema: func() *openapi3.Schema {
			schema := openapi3.NewSchema()

			parameterSchema1 := openapi3.NewStringSchema()
			parameterSchema1.Example = "testvalue"

			parameterSchema2 := openapi3.NewObjectSchema()
			nestedParameterSchema := openapi3.NewBoolSchema()
			nestedParameterSchema.Example = true
			parameterSchema2.WithProperty("nestedProperty", nestedParameterSchema)

			schema.WithProperties(map[string]*openapi3.Schema{
				"name1": parameterSchema1,
				"name2": parameterSchema2,
			})
			return schema
		},
		validateResult: func(t *testing.T, s string) {
			assert.Equal(t, `{"name1":"testvalue","name2":{"nestedProperty":true}}`, s)
		},
	},
	getTypedExampleTestData{
		name: "SchemaArrayItemsStringExampleField",
		generateInputSchema: func() *openapi3.Schema {
			schema := openapi3.NewArraySchema()
			itemSchema := openapi3.NewStringSchema()
			itemSchema.Example = "testvalue"
			schema.WithItems(itemSchema)
			return schema
		},
		validateResult: func(t *testing.T, s string) {
			assert.Equal(t, `["testvalue"]`, s)
		},
	},
	getTypedExampleTestData{
		name: "SchemaArrayItemsObjectExampleField",
		generateInputSchema: func() *openapi3.Schema {
			schema := openapi3.NewArraySchema()

			itemSchema := openapi3.NewObjectSchema()

			parameterSchema1 := openapi3.NewStringSchema()
			parameterSchema1.Example = "testvalue"

			itemSchema.WithProperties(map[string]*openapi3.Schema{
				"name1": parameterSchema1,
			})

			schema.WithItems(itemSchema)
			return schema
		},
		validateResult: func(t *testing.T, s string) {
			assert.Equal(t, `[{"name1":"testvalue"}]`, s)
		},
	},
	getTypedExampleTestData{
		name: "StringSchemaWithExample",
		generateInputSchema: func() *openapi3.Schema {
			schema := openapi3.NewStringSchema()
			schema.Example = "examplestr"
			return schema
		},
		validateResult: func(t *testing.T, s string) {
			assert.Equal(t, `"examplestr"`, s)
		},
	},
	getTypedExampleTestData{
		name: "StringSchemaWithoutExample",
		generateInputSchema: func() *openapi3.Schema {
			schema := openapi3.NewStringSchema()
			return schema
		},
		validateResult: func(t *testing.T, s string) {
			assert.Equal(t, `"string"`, s)
		},
	},
	getTypedExampleTestData{
		name: "BooleanSchema",
		generateInputSchema: func() *openapi3.Schema {
			schema := openapi3.NewBoolSchema()
			return schema
		},
		validateResult: func(t *testing.T, s string) {
			assert.Equal(t, `true`, s)
		},
	},
}

func Test_GetTypedExampleTest(t *testing.T) {

	for _, td := range getTypedExampleTestDataEntries {
		t.Logf("testcase: '%s'", td.name)

		mediaType := openapi3.NewMediaType()
		mediaType.WithSchema(td.generateInputSchema())

		selectedExample, err := getTypedExample(mediaType)
		assert.Nil(t, err)

		selectedExampleJSON, err := json.Marshal(selectedExample)
		assert.Nil(t, err)
		assert.NotEmpty(t, string(selectedExampleJSON))
		td.validateResult(t, string(selectedExampleJSON))
	}
}

func Test_GetTypedExampleShouldReturnErrorIfCannotGetFullExample(t *testing.T) {
	schema := openapi3.NewSchema()

	parameterSchema1 := openapi3.NewStringSchema()
	parameterSchema1.Example = "testvalue"

	parameterSchema2 := openapi3.NewObjectSchema()
	nestedParameterSchemaWithoutExample := openapi3.NewObjectSchema()
	parameterSchema2.WithProperty("nestedProperty", nestedParameterSchemaWithoutExample)

	schema.WithProperties(map[string]*openapi3.Schema{
		"name1": parameterSchema1,
		"name2": parameterSchema2,
	})

	mediaType := openapi3.NewMediaType()
	mediaType.WithSchema(schema)

	selectedExample, err := getTypedExample(mediaType)
	assert.NotNil(t, err)
	assert.Nil(t, selectedExample)
}
