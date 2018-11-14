package main

import (
	"encoding/json"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
)

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

func Test_GetTypedExampleShouldGetFromSchemaExampleField(t *testing.T) {
	exampleData := map[string]string{"name1": "value1", "name2": "value2"}

	schema := openapi3.NewSchema()
	schema.Example = exampleData

	mediaType := openapi3.NewMediaType()
	mediaType.WithSchema(schema)

	selectedExample, err := getTypedExample(mediaType)
	assert.Nil(t, err)

	selectedExampleJSON, err := json.Marshal(selectedExample)
	assert.Nil(t, err)
	assert.NotEmpty(t, string(selectedExampleJSON))
	assert.Equal(t, `{"name1":"value1","name2":"value2"}`, string(selectedExampleJSON))
}

func Test_GetTypedExampleShouldGetFromSchemaPropertiesExampleField(t *testing.T) {
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

	mediaType := openapi3.NewMediaType()
	mediaType.WithSchema(schema)

	selectedExample, err := getTypedExample(mediaType)
	assert.Nil(t, err)

	selectedExampleJSON, err := json.Marshal(selectedExample)
	assert.Nil(t, err)
	assert.NotEmpty(t, string(selectedExampleJSON))
	assert.Equal(t, `{"name1":"testvalue","name2":{"nestedProperty":true}}`, string(selectedExampleJSON))
}

func Test_GetTypedExampleShouldGetFromSchemaArrayItemsStringExampleField(t *testing.T) {
	schema := openapi3.NewArraySchema()

	itemSchema := openapi3.NewStringSchema()
	itemSchema.Example = "testvalue"

	schema.WithItems(itemSchema)

	mediaType := openapi3.NewMediaType()
	mediaType.WithSchema(schema)

	selectedExample, err := getTypedExample(mediaType)
	assert.Nil(t, err)

	selectedExampleJSON, err := json.Marshal(selectedExample)
	assert.Nil(t, err)
	assert.NotEmpty(t, string(selectedExampleJSON))
	assert.Equal(t, `["testvalue"]`, string(selectedExampleJSON))
}

func Test_GetTypedExampleShouldGetFromSchemaArrayItemsObjectExampleField(t *testing.T) {
	schema := openapi3.NewArraySchema()

	itemSchema := openapi3.NewObjectSchema()

	parameterSchema1 := openapi3.NewStringSchema()
	parameterSchema1.Example = "testvalue"

	itemSchema.WithProperties(map[string]*openapi3.Schema{
		"name1": parameterSchema1,
	})

	schema.WithItems(itemSchema)

	mediaType := openapi3.NewMediaType()
	mediaType.WithSchema(schema)

	selectedExample, err := getTypedExample(mediaType)
	assert.Nil(t, err)

	selectedExampleJSON, err := json.Marshal(selectedExample)
	assert.Nil(t, err)
	assert.NotEmpty(t, string(selectedExampleJSON))
	assert.Equal(t, `[{"name1":"testvalue"}]`, string(selectedExampleJSON))
}

func Test_GetTypedExampleShouldReturnErrorIfCannotGetFullExample(t *testing.T) {
	schema := openapi3.NewSchema()

	parameterSchema1 := openapi3.NewStringSchema()
	parameterSchema1.Example = "testvalue"

	parameterSchema2 := openapi3.NewObjectSchema()
	nestedParameterSchemaWithoutExample := openapi3.NewBoolSchema()
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
