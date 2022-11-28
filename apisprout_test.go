package apisprout

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var localServerTests = []struct {
	name string
	in   []string
	out  []string
}{
	{
		"No servers",
		[]string{},
		[]string{},
	},
	{
		"Same path",
		[]string{
			"https://api.example.com/v1",
			"https://beta.api.example.com/v1",
		},
		[]string{
			"https://api.example.com/v1",
			"https://beta.api.example.com/v1",
			"http://localhost:8000/v1",
		},
	},
	{
		"Includes localhost already",
		[]string{
			"https://api.example.com/v1",
			"http://localhost:8000/v1",
		},
		[]string{
			"https://api.example.com/v1",
			"http://localhost:8000/v1",
		},
	},
	{
		"Invalid URL",
		[]string{
			"http://192.168.0.%31/",
		},
		[]string{},
	},
}

func TestAddLocalServers(t *testing.T) {
	cr := &ConfigReloader{Port: 8000}
	for _, tt := range localServerTests {
		t.Run(tt.name, func(t *testing.T) {
			servers := make([]*openapi3.Server, len(tt.in))
			for i, u := range tt.in {
				servers[i] = &openapi3.Server{
					URL: u,
				}
			}

			s := &openapi3.T{
				Servers: servers,
			}

			err := cr.addLocalServers(s)
			if len(tt.in) > 0 && len(tt.out) == 0 {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			results := make([]string, 0, len(tt.out))
			for _, server := range s.Servers {
				results = append(results, server.URL)
			}

			assert.Equal(t, tt.out, results)
		})
	}
}

func TestParsePreferHeader(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   map[string]string
	}{
		{
			name:   "Single",
			header: "status=200",
			want: map[string]string{
				"status": "200",
			},
		},
		{
			name:   "Single Quotes",
			header: "status=\"200\"",
			want: map[string]string{
				"status": "200",
			},
		},
		{
			name:   "Single Quotes Space",
			header: "example=\"in progress\"",
			want: map[string]string{
				"example": "in progress",
			},
		},
		{
			name:   "Multiple Semicolon",
			header: "status=200;example=complete",
			want: map[string]string{
				"status":  "200",
				"example": "complete",
			},
		},
		{
			name:   "Multiple Semi Space",
			header: "status=200; example=complete",
			want: map[string]string{
				"status":  "200",
				"example": "complete",
			},
		},
		{
			name:   "Multiple Comma",
			header: "status=200,example=complete",
			want: map[string]string{
				"status":  "200",
				"example": "complete",
			},
		},
		{
			name:   "Multiple Comma Space",
			header: "status=200, example=complete",
			want: map[string]string{
				"status":  "200",
				"example": "complete",
			},
		},
		{
			name:   "Mixed Pairs",
			header: "example=complete; foo, status=\"200\",",
			want: map[string]string{
				"example": "complete",
				"foo":     "",
				"status":  "200",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parsePreferHeader(tt.header); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parsePreferHeader() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMediaTypes(t *testing.T) {
	const schema = `{
		"paths": {
			"/test": {
				"get": {
					"summary": "Test",
					"responses": {
						"200": {
							"content": {
								"%s": {
									"schema": {
										type": "boolean",
										"example": true
									}
								}
							}
						}
					}
				}
			}
		}
	}`

	tests := []struct {
		MediaType  string
		StatusCode int
	}{
		{
			MediaType:  "application/json",
			StatusCode: http.StatusOK,
		},
		{
			MediaType:  "application/vnd.test-api+json",
			StatusCode: http.StatusOK,
		},
		{
			MediaType:  "application/yaml",
			StatusCode: http.StatusOK,
		},
		{
			MediaType:  "application/x-yaml",
			StatusCode: http.StatusOK,
		},
		{
			MediaType:  "application/vnd.test-api+yaml",
			StatusCode: http.StatusOK,
		},
		{
			MediaType:  "text/yaml",
			StatusCode: http.StatusOK,
		},
		{
			MediaType:  "text/x-yaml",
			StatusCode: http.StatusOK,
		},
		{
			MediaType:  "text/vnd.test-api+yaml",
			StatusCode: http.StatusOK,
		},
		{
			MediaType:  "text/vnd.test-api+xml",
			StatusCode: http.StatusInternalServerError,
		},
		{
			MediaType:  "application/json-with-extensions",
			StatusCode: http.StatusInternalServerError,
		},
	}
	for _, test := range tests {
		t.Run(test.MediaType, func(t *testing.T) {
			swagger, err := openapi3.NewLoader().LoadFromData([]byte(fmt.Sprintf(schema, test.MediaType)))
			require.Nil(t, err)

			req, err := http.NewRequest("GET", "/test", nil)
			require.NoError(t, err)

			resp := httptest.NewRecorder()

			s, err := NewOpenAPIServer(swagger)
			require.Nil(t, err)

			s.ServeHTTP(resp, req)

			assert.Equal(t, test.StatusCode, resp.Code)
		})
	}
}

func TestOpenAPIServer(t *testing.T) {
	for _, test := range []struct {
		name   string
		method string
		path   string
		code   int
	}{
		{
			name:   "root",
			path:   "/",
			method: http.MethodGet,
			code:   http.StatusNotFound,
		},
		{
			name:   "find by tag",
			path:   "/v3/pet/findByStatus",
			method: http.MethodGet,
			code:   http.StatusOK,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, err := NewOpenAPIServer(loadPetStoreScheme(t))
			require.Nil(t, err)

			req := httptest.NewRequest(test.method, test.path, nil)
			// apisprout does not support xml, so we need to set the Accept header.
			req.Header.Add("Accept", "application/json")

			w := httptest.NewRecorder()

			s.ServeHTTP(w, req)

			assert.Equal(t, test.code, w.Code)
		})
	}
}

func loadPetStoreScheme(t *testing.T) *openapi3.T {
	buf, err := os.ReadFile("testdata/petstore.yaml")
	require.Nil(t, err)
	s, err := openapi3.NewLoader().LoadFromData(buf)
	require.Nil(t, err)

	return s
}

func TestConfigReloaderHandler(t *testing.T) {
	for _, test := range []struct {
		name            string
		method          string
		path            string
		code            int
		validateServer  bool
		validateRequest bool
		body            string
	}{
		{
			name:   "root",
			path:   "/",
			method: http.MethodGet,
			code:   http.StatusNotFound,
		},
		{
			name:           "find by tag with server validation",
			path:           "/v3/pet/findByStatus",
			method:         http.MethodGet,
			code:           http.StatusOK,
			validateServer: true,
		},
		{
			name:   "find by tag without server validation",
			path:   "/pet/findByStatus",
			method: http.MethodGet,
			code:   http.StatusOK,
		},
		{
			name:           "find by tag with server validation and wrong path",
			path:           "/pet/findByStatus",
			method:         http.MethodGet,
			code:           http.StatusNotFound,
			validateServer: true,
		},
		{
			name:   "new pet",
			path:   "/pet",
			method: http.MethodPost,
			code:   http.StatusOK,
		},
		{
			name:            "new pet with request validation",
			path:            "/pet",
			method:          http.MethodPost,
			code:            http.StatusOK,
			validateRequest: true,
			body: `{
				"name": "foo",
				"photoUrls": []
			}`,
		},
		{
			name:            "new pet with request validation and missing parameter",
			path:            "/pet",
			method:          http.MethodPost,
			code:            http.StatusBadRequest,
			validateRequest: true,
			body: `{
				"name": "foo"
			}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			cr := &ConfigReloader{
				Mux:             http.NewServeMux(),
				URI:             "testdata/petstore.yaml",
				ValidateServer:  test.validateServer,
				ValidateRequest: test.validateRequest,
			}
			require.Nil(t, cr.Run(ctx))

			req := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			// apisprout does not support xml, so we need to set the Accept header.
			req.Header.Add("Accept", "application/json")
			req.Header.Add("Content-Type", "application/json")

			w := httptest.NewRecorder()

			cr.ServeHTTP(w, req)

			assert.Equal(t, test.code, w.Code)
		})
	}
}
