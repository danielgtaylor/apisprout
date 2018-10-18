package main

import (
	"encoding/json"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
)

func Test_GetTypedExampleFromExampleField(t *testing.T) {
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
