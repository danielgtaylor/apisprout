package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/gobwas/glob"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	yaml "gopkg.in/yaml.v2"
)

// GitSummary is filled in by `govvv` for version info.
var GitSummary string

var (
	// ErrNoExample is sent when no example was found for an operation.
	ErrNoExample = errors.New("No example found")

	// ErrCannotMarshal is set when an example cannot be marshalled.
	ErrCannotMarshal = errors.New("Cannot marshal example")

	// ErrMissingAuth is set when no authorization header or key is present but
	// one is required by the API description.
	ErrMissingAuth = errors.New("Missing auth")
)

// ContentNegotiator is used to match a media type during content negotiation
// of HTTP requests.
type ContentNegotiator struct {
	globs []glob.Glob
}

// NewContentNegotiator creates a new negotiator from an HTTP Accept header.
func NewContentNegotiator(accept string) *ContentNegotiator {
	// The HTTP Accept header is parsed and converted to simple globs, which
	// can be used to match an incoming mimetype. Example:
	// Accept: text/html, text/*;q=0.9, */*;q=0.8
	// Will be turned into the following globs:
	// - text/html
	// - text/*
	// - */*
	globs := make([]glob.Glob, 0)
	for _, mt := range strings.Split(accept, ",") {
		parsed, _, _ := mime.ParseMediaType(mt)
		globs = append(globs, glob.MustCompile(parsed))
	}

	return &ContentNegotiator{
		globs: globs,
	}
}

// Match returns true if the given mediatype string matches any of the allowed
// types in the accept header.
func (cn *ContentNegotiator) Match(mediatype string) bool {
	for _, glob := range cn.globs {
		if glob.Match(mediatype) {
			return true
		}
	}

	return false
}

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
		Version: GitSummary,
		Args:    cobra.MinimumNArgs(1),
		Run:     server,
		Example: fmt.Sprintf("  %s openapi.yaml", cmd),
	}

	// Set up global options.
	flags := root.PersistentFlags()

	addParameter(flags, "port", "p", 8000, "HTTP port")
	addParameter(flags, "validate-server", "", false, "Check hostname against configured servers")
	addParameter(flags, "validate-request", "", false, "Check request data structure")
	addParameter(flags, "enableCors", "c", false, "Enable CORS on all requests from all origins")
	// Run the app!
	root.Execute()
}

// addParameter adds a new global parameter with a default value that can be
// configured using configuration files, the environment, or commandline flags.
func addParameter(flags *pflag.FlagSet, name, short string, def interface{}, desc string) {
	viper.SetDefault(name, def)
	switch v := def.(type) {
	case bool:
		flags.BoolP(name, short, v, desc)
	case int:
		flags.IntP(name, short, v, desc)
	case string:
		flags.StringP(name, short, v, desc)
	}
	viper.BindPFlag(name, flags.Lookup(name))
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
		return mt.Examples[selected].Value.Value, nil
	}

	// TODO: generate data from JSON schema, if available?

	return nil, ErrNoExample
}

// getExample tries to return an example for a given operation.
func getExample(negotiator *ContentNegotiator, prefer string, op *openapi3.Operation) (int, string, interface{}, error) {
	var responses []string
	if prefer == "" {
		// First, make a list of responses ordered by successful (200-299 status code)
		// before other types.
		success := make([]string, 0)
		other := make([]string, 0)
		for s := range op.Responses {
			if status, err := strconv.Atoi(s); err == nil && status >= 200 && status < 300 {
				success = append(success, s)
				continue
			}
			other = append(other, s)
		}
		responses = append(success, other...)
	} else {
		if op.Responses[prefer] == nil {
			return 0, "", nil, ErrNoExample
		}
		responses = []string{prefer}
	}

	// Now try to find the first example we can and return it!
	for _, s := range responses {
		response := op.Responses[s]
		status, err := strconv.Atoi(s)
		if err != nil {
			// Treat default and other named statuses as 200.
			status = http.StatusOK
		}

		if response.Value.Content == nil {
			// This is a valid response but has no body defined.
			return status, "", "", nil
		}

		for mt, content := range response.Value.Content {
			if negotiator != nil && !negotiator.Match(mt) {
				// This is not what the client asked for.
				continue
			}

			example, err := getTypedExample(content)
			if err == nil {
				return status, mt, example, nil
			}
		}
	}

	return 0, "", nil, ErrNoExample
}

// server loads an OpenAPI file and runs a mock server using the paths and
// examples defined in the file.
func server(cmd *cobra.Command, args []string) {
	uri := args[0]

	var err error
	var data []byte

	// Load either from an HTTP URL or from a local file depending on the passed
	// in value.
	if strings.HasPrefix(uri, "http") {
		resp, err := http.Get(uri)
		if err != nil {
			log.Fatal(err)
		}

		data, err = ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Fatal(err)
		}
	} else {
		data, err = ioutil.ReadFile(uri)
		if err != nil {
			log.Fatal(err)
		}
	}

	// Load the OpenAPI document.
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

	// Create a new router using the OpenAPI document's declared paths.
	var router = openapi3filter.NewRouter().WithSwagger(swagger)

	// Register our custom HTTP handler that will use the router to find
	// the appropriate OpenAPI operation and try to return an example.
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		//Enable CORS
		if viper.GetBool("enableCors") {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

			// Handle pre-flight OPTIONS request
			if (*req).Method == "OPTIONS" {
				return
			}
		}

		info := fmt.Sprintf("%s %v", req.Method, req.URL)
		route, _, err := router.FindRoute(req.Method, req.URL)
		if err != nil {
			log.Printf("ERROR: %s => %v", info, err)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if viper.GetBool("validate-request") {
			err = openapi3filter.ValidateRequest(nil, &openapi3filter.RequestValidationInput{
				Request: req,
				Route:   route,
				Options: &openapi3filter.Options{
					AuthenticationFunc: func(c context.Context, input *openapi3filter.AuthenticationInput) error {
						// TODO: support more schemes
						sec := input.SecurityScheme
						if sec.Type == "http" && sec.Scheme == "bearer" {
							if req.Header.Get("Authorization") == "" {
								return ErrMissingAuth
							}
						}
						return nil
					},
				},
			})
			if err != nil {
				log.Printf("ERROR: %s => %v", info, err)
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(fmt.Sprintf("%v", err)))
				return
			}
		}

		var negotiator *ContentNegotiator
		if accept := req.Header.Get("Accept"); accept != "" {
			negotiator = NewContentNegotiator(accept)
			if accept != "*/*" {
				info = fmt.Sprintf("%s (Accept %s)", info, accept)
			}
		}

		prefer := req.Header.Get("Prefer")
		if strings.HasPrefix(prefer, "status=") {
			prefer = prefer[7:10]
		} else {
			prefer = ""
		}

		status, mediatype, example, err := getExample(negotiator, prefer, route.Operation)
		if err != nil {
			log.Printf("%s => Missing example", info)
			w.WriteHeader(http.StatusTeapot)
			w.Write([]byte("No example available."))
			return
		}

		log.Printf("%s => %d (%s)", info, status, mediatype)

		var encoded []byte

		if s, ok := example.(string); ok {
			encoded = []byte(s)
		} else if _, ok := example.([]byte); ok {
			encoded = example.([]byte)
		} else {
			switch mediatype {
			case "application/json":
				encoded, err = json.MarshalIndent(example, "", "  ")
			case "application/x-yaml", "application/yaml", "text/x-yaml", "text/yaml", "text/vnd.yaml":
				encoded, err = yaml.Marshal(example)
			default:
				log.Printf("Cannot marshal as '%s'!", mediatype)
				err = ErrCannotMarshal
			}

			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Unable to marshal response"))
				return
			}
		}

		if mediatype != "" {
			w.Header().Add("Content-Type", mediatype)
		}
		w.WriteHeader(status)
		w.Write(encoded)
	})

	fmt.Printf("🌱 Sprouting %s on port %d\n", swagger.Info.Title, viper.GetInt("port"))
	if (viper.GetBool("enableCors")) {
		fmt.Printf("🌱 - CORS enabled on all requests from all origins")
	}
	http.ListenAndServe(fmt.Sprintf(":%d", viper.GetInt("port")), nil)
}
