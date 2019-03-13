package main

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
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
	viper.SetDefault("port", 8000)
	for _, tt := range localServerTests {
		t.Run(tt.name, func(t *testing.T) {
			servers := make([]*openapi3.Server, len(tt.in))
			for i, u := range tt.in {
				servers[i] = &openapi3.Server{
					URL: u,
				}
			}

			s := &openapi3.Swagger{
				Servers: servers,
			}

			err := addLocalServers(s)
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
