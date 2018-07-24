package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	yaml "gopkg.in/yaml.v2"
)

var (
	// ErrNoExample is sent when no example was found for an operation.
	ErrNoExample = errors.New("No example found")

	// ErrCannotMarshal is set when an example cannot be marshalled.
	ErrCannotMarshal = errors.New("Cannot marshal example")
)

func main() {
	rand.Seed(time.Now().UnixNano())

	// Load configuration from file(s) if provided.
	viper.SetConfigName("config")
	viper.AddConfigPath("/etc/apisprout/")
	viper.AddConfigPath("$HOME/.apisprout/")
	viper.ReadInConfig()

	// Load configuration from the environment if provided. Flags below get
	// transformed automatically, e.g. `foo-bar` -> `SPROUT_FOO_BAR`.
	viper.SetEnvPrefix("SPROUT")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	// Build the root command. This is the application's entry point.
	cmd := filepath.Base(os.Args[0])
	root := &cobra.Command{
		Use:     fmt.Sprintf("%s [flags] FILE", cmd),
		Version: "1.0",
		Args:    cobra.MinimumNArgs(1),
		Run:     server,
		Example: fmt.Sprintf("  %s openapi.yaml", cmd),
	}

	// Set up global options.
	flags := root.PersistentFlags()

	viper.SetDefault("port", 8000)
	flags.IntP("port", "p", 8000, "HTTP port")
	viper.BindPFlag("port", flags.Lookup("port"))

	viper.SetDefault("validate-server", false)
	flags.BoolP("validate-server", "", false, "Check hostname against configured servers")
	viper.BindPFlag("validate-server", flags.Lookup("validate-server"))

	// Run the app!
	root.Execute()
}

// getTypedExample will return an example from a given media type, if such an
// example exists. If multiple examples are given, then one is selected at
// random.
func getTypedExample(mt *openapi3.MediaType) (interface{}, error) {
	if mt.Example != nil {
		return mt.Example, nil
	}

	if len(mt.Examples) > 0 {
		// Choose a random example to return.
		keys := make([]string, 0, len(mt.Examples))
		for k := range mt.Examples {
			keys = append(keys, k)
		}

		selected := keys[rand.Intn(len(keys))]
		return mt.Examples[selected].Value, nil
	}

	// TODO: generate data from JSON schema, if available?

	return nil, ErrNoExample
}

// getExample tries to return an example for a given operation.
func getExample(op *openapi3.Operation) (int, string, interface{}, error) {
	for s, response := range op.Responses {

		status, _ := strconv.Atoi(s)

		// Prefer successful status codes, if available.
		if status >= 200 && status < 300 {
			for mime, content := range response.Value.Content {
				example, err := getTypedExample(content)
				if err == nil {
					return status, mime, example, nil
				}
			}
		}

		// TODO: support other status codes.
	}

	return 0, "", nil, ErrNoExample
}

func server(cmd *cobra.Command, args []string) {
	data, err := ioutil.ReadFile(args[0])
	if err != nil {
		log.Fatal(err)
	}

	loader := openapi3.NewSwaggerLoader()
	var swagger *openapi3.Swagger
	if strings.HasSuffix(args[0], ".yaml") || strings.HasSuffix(args[0], ".yml") {
		swagger, err = loader.LoadSwaggerFromYAMLData(data)
	} else {
		swagger, err = loader.LoadSwaggerFromData(data)
	}
	if err != nil {
		log.Fatal(err)
	}

	if !viper.GetBool("validate-server") {
		// Clear the server list so no validation happens. Note: this has a side
		// effect of no longer parsing any server-declared parameters.
		swagger.Servers = make([]*openapi3.Server, 0)
	}

	var router = openapi3filter.NewRouter().WithSwagger(swagger)

	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		info := fmt.Sprintf("%s %v", req.Method, req.URL)
		route, _, err := router.FindRoute(req.Method, req.URL)
		if err != nil {
			log.Printf("ERROR: %s => %v", info, err)
			w.WriteHeader(404)
			return
		}

		status, mime, example, err := getExample(route.Operation)
		if err != nil {
			log.Printf("%s => Missing example", info)
			w.WriteHeader(http.StatusTeapot)
			w.Write([]byte("No example available."))
			return
		}

		log.Printf("%s => %d (%s)", info, status, mime)

		var encoded []byte

		if s, ok := example.(string); ok {
			encoded = []byte(s)
		} else if _, ok := example.([]byte); ok {
			encoded = example.([]byte)
		} else {
			switch mime {
			case "application/json":
				encoded, err = json.MarshalIndent(example, "", "  ")
			case "application/x-yaml", "application/yaml", "text/x-yaml", "text/yaml", "text/vnd.yaml":
				encoded, err = yaml.Marshal(example)
			default:
				log.Printf("Cannot marshal as %s!", mime)
				err = ErrCannotMarshal
			}

			if err != nil {
				w.WriteHeader(500)
				w.Write([]byte("Unable to marshal response"))
				return
			}
		}

		w.Header().Add("Content-Type", mime)
		w.WriteHeader(status)
		w.Write(encoded)
	})

	fmt.Printf("Starting server on port %d\n", viper.GetInt("port"))
	http.ListenAndServe(fmt.Sprintf(":%d", viper.GetInt("port")), nil)
}
