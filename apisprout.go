package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/gobwas/glob"
	"github.com/pkg/errors"
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

	// ErrRecursive is when a schema is impossible to represent because it infinitely recurses.
	ErrRecursive = errors.New("Recursive schema")

	// ErrCannotMarshal is set when an example cannot be marshalled.
	ErrCannotMarshal = errors.New("Cannot marshal example")

	// ErrMissingAuth is set when no authorization header or key is present but
	// one is required by the API description.
	ErrMissingAuth = errors.New("Missing auth")

	// ErrInvalidAuth is set when the authorization scheme doesn't correspond
	// to the one required by the API description.
	ErrInvalidAuth = errors.New("Invalid auth")
)

var (
	marshalJSONMatcher = regexp.MustCompile(`^application/(vnd\..+\+)?json$`)
	marshalYAMLMatcher = regexp.MustCompile(`^(application|text)/(x-|vnd\..+\+)?yaml$`)
)

type RefreshableRouter struct {
	router *openapi3filter.Router
}

func (rr *RefreshableRouter) Set(router *openapi3filter.Router) {
	rr.router = router
}

func (rr *RefreshableRouter) Get() *openapi3filter.Router {
	return rr.router
}

func NewRefreshableRouter() *RefreshableRouter {
	return &RefreshableRouter{}
}

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
		Example: fmt.Sprintf("  # Basic usage\n  %s openapi.yaml\n\n  # Validate server name and use base path\n  %s --validate-server openapi.yaml\n\n  # Fetch API via HTTP with custom auth header\n  %s -H 'Authorization: abc123' http://example.com/openapi.yaml", cmd, cmd, cmd),
	}

	// Set up global options.
	flags := root.PersistentFlags()

	addParameter(flags, "port", "p", 8000, "HTTP port")
	addParameter(flags, "validate-server", "s", false, "Check scheme/hostname/basepath against configured servers")
	addParameter(flags, "validate-request", "", false, "Check request data structure")
	addParameter(flags, "watch", "w", false, "Reload when input file changes")
	addParameter(flags, "disable-cors", "", false, "Disable CORS headers")
	addParameter(flags, "header", "H", "", "Add a custom header when fetching API")
	addParameter(flags, "add-server", "", "", "Add a new valid server URL, use with --validate-server")
	addParameter(flags, "https", "", false, "Use HTTPS instead of HTTP")
	addParameter(flags, "public-key", "", "", "Public key for HTTPS, use with --https")
	addParameter(flags, "private-key", "", "", "Private key for HTTPS, use with --https")

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
// random unless an "example" item exists in the Prefer header
func getTypedExample(mt *openapi3.MediaType, prefer map[string]string) (interface{}, error) {
	if mt.Example != nil {
		return mt.Example, nil
	}

	if len(mt.Examples) > 0 {
		// If preferred example requested and it it exists, return it
		preferredExample := ""
		if mapContainsKey(prefer, "example") {
			preferredExample = prefer["example"]
			if _, ok := mt.Examples[preferredExample]; ok {
				return mt.Examples[preferredExample].Value.Value, nil
			}
		}

		// Choose a random example to return.
		keys := make([]string, 0, len(mt.Examples))
		for k := range mt.Examples {
			keys = append(keys, k)
		}

		if len(keys) > 0 {
			selected := keys[rand.Intn(len(keys))]
			return mt.Examples[selected].Value.Value, nil
		}
	}

	if mt.Schema != nil {
		return OpenAPIExample(ModeResponse, mt.Schema.Value)
	}
	// TODO: generate data from JSON schema, if no examples available?

	return nil, ErrNoExample
}

// getExample tries to return an example for a given operation.
// Using the Prefer http header, the consumer can specify the type of response they want.
func getExample(negotiator *ContentNegotiator, prefer map[string]string, op *openapi3.Operation) (int, string, map[string]*openapi3.HeaderRef, interface{}, error) {
	var responses []string
	var blankHeaders = make(map[string]*openapi3.HeaderRef)

	if !mapContainsKey(prefer, "status") {
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
	} else if op.Responses[prefer["status"]] != nil {
		responses = []string{prefer["status"]}
	} else if op.Responses["default"] != nil {
		responses = []string{"default"}
	} else {
		return 0, "", blankHeaders, nil, ErrNoExample
	}

	// Now try to find the first example we can and return it!
	for _, s := range responses {
		response := op.Responses[s]
		status, err := strconv.Atoi(s)
		if err != nil {
			// If we are using the default with prefer, we can use its status
			// code:
			status, err = strconv.Atoi(prefer["status"])
		}
		if err != nil {
			// Otherwise, treat default and other named statuses as 200.
			status = http.StatusOK
		}

		if response.Value.Content == nil {
			// This is a valid response but has no body defined.
			return status, "", blankHeaders, "", nil
		}

		for mt, content := range response.Value.Content {
			if negotiator != nil && !negotiator.Match(mt) {
				// This is not what the client asked for.
				continue
			}

			example, err := getTypedExample(content, prefer)
			if err == nil {
				return status, mt, response.Value.Headers, example, nil
			}

			fmt.Printf("Error getting example: %v\n", err)
		}
	}

	return 0, "", blankHeaders, nil, ErrNoExample
}

// addLocalServers will ensure that requests to localhost are always allowed
// even if not specified in the OpenAPI document.
func addLocalServers(swagger *openapi3.Swagger) error {
	seen := make(map[string]bool)
	for _, s := range swagger.Servers {
		seen[s.URL] = true
	}

	lservers := make([]*openapi3.Server, 0, len(swagger.Servers))
	for _, s := range swagger.Servers {
		u, err := url.Parse(s.URL)
		if err != nil {
			return err
		}

		if u.Hostname() != "localhost" {
			u.Scheme = "http"
			u.Host = fmt.Sprintf("localhost:%d", viper.GetInt("port"))

			ls := &openapi3.Server{
				URL:         u.String(),
				Description: s.Description,
				Variables:   s.Variables,
			}

			if !seen[ls.URL] {
				lservers = append(lservers, ls)
				seen[ls.URL] = true
			}
		}
	}

	if len(lservers) != 0 {
		swagger.Servers = append(swagger.Servers, lservers...)
	}

	return nil
}

// Load the OpenAPI document and create the router.
func load(uri string, data []byte) (swagger *openapi3.Swagger, router *openapi3filter.Router, err error) {
	defer func() {
		if r := recover(); r != nil {
			swagger = nil
			router = nil
			if e, ok := r.(error); ok {
				err = errors.Wrap(e, "Caught panic while trying to load")
			} else {
				err = fmt.Errorf("Caught panic while trying to load")
			}
		}
	}()

	loader := openapi3.NewSwaggerLoader()
	loader.IsExternalRefsAllowed = true

	var u *url.URL
	u, err = url.Parse(uri)
	if err != nil {
		return
	}

	swagger, err = loader.LoadSwaggerFromDataWithPath(data, u)
	if err != nil {
		return
	}

	if !viper.GetBool("validate-server") {
		// Clear the server list so no validation happens. Note: this has a side
		// effect of no longer parsing any server-declared parameters.
		swagger.Servers = make([]*openapi3.Server, 0)
	} else {
		// Special-case localhost to always be allowed for local testing.
		if err = addLocalServers(swagger); err != nil {
			return
		}

		if cs := viper.GetString("add-server"); cs != "" {
			swagger.Servers = append(swagger.Servers, &openapi3.Server{
				URL:         cs,
				Description: "Custom server from command line param",
				Variables:   make(map[string]*openapi3.ServerVariable),
			})
		}
	}

	// Create a new router using the OpenAPI document's declared paths.
	router = openapi3filter.NewRouter().WithSwagger(swagger)

	return
}

// parsePreferHeader takes the value of a prefer header and splits it out into key value pairs
//
// HTTP Prefer header specification examples:
// - Prefer: status=200; example="something"
// - Prefer: example=something;status=200;
// - Prefer: example="somet,;hing";status=200;
//
// As part of the Prefer specification, it is completely valid to specify
// multiple Prefer headers in a single request, however we won't be
// supporting that for the moment and only the first Prefer header
// will be used.
func parsePreferHeader(value string) map[string]string {
	prefer := map[string]string{}
	if value != "" {
		// In the event that something is quoted, we want to pull those items out of the string
		// and save them for later, so they don't conflict with other splitting logic.

		quotedRegex := regexp.MustCompile(`"[^"]*"`)
		splitRegex := regexp.MustCompile(`(,|;| )`)
		wilcardRegex := regexp.MustCompile(`%%([0-9]+)%%`)

		quotedStrings := quotedRegex.FindAllString(value, -1)
		if len(quotedStrings) > 0 {
			// replace each quoted string with a placehoder
			for idx, quotedString := range quotedStrings {
				value = strings.Replace(value, quotedString, fmt.Sprintf("%%%%%v%%%%", idx), 1)
			}
		}

		pairs := splitRegex.Split(value, -1)
		for _, pair := range pairs {
			pair = strings.TrimSpace(pair)
			if pair != "" {
				// Put any wildcards back
				wildcardStrings := wilcardRegex.FindAllStringSubmatch(pair, -1)
				for _, wildcard := range wildcardStrings {
					quotedIdx, _ := strconv.Atoi(wildcard[1])
					pair = strings.Replace(pair, wildcard[0], quotedStrings[quotedIdx], 1)
				}

				// Determine the key and valid for this argument
				if strings.Contains(pair, "=") {
					eqIdx := strings.Index(pair, "=")
					prefer[pair[:eqIdx]] = strings.Trim(pair[eqIdx+1:], `"`)
				} else {
					prefer[pair] = ""
				}
			}
		}
	}
	return prefer
}

func mapContainsKey(dict map[string]string, key string) bool {
	if _, ok := dict[key]; ok {
		return true
	}
	return false
}

var handler = func(rr *RefreshableRouter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !viper.GetBool("disable-cors") {
			corsOrigin := req.Header.Get("Origin")
			if corsOrigin == "" {
				corsOrigin = "*"
			}
			w.Header().Set("Access-Control-Allow-Origin", corsOrigin)

			if corsOrigin != "*" {
				// Allow credentials to be sent if an origin has  been specified.
				// This is done *outside* of an OPTIONS request since it might be
				// required for a non-preflighted GET/POST request.
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			// Handle pre-flight OPTIONS request
			if (*req).Method == "OPTIONS" {
				corsMethod := req.Header.Get("Access-Control-Request-Method")
				if corsMethod == "" {
					corsMethod = "POST, GET, OPTIONS, PUT, DELETE"
				}

				corsHeaders := req.Header.Get("Access-Control-Request-Headers")
				if corsHeaders == "" {
					corsHeaders = "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization"
				}

				w.Header().Set("Access-Control-Allow-Methods", corsMethod)
				w.Header().Set("Access-Control-Allow-Headers", corsHeaders)
				return
			}
		}

		info := fmt.Sprintf("%s %v", req.Method, req.URL)

		// Set up the request, handling potential proxy headers
		req.URL.Host = req.Host
		fHost := req.Header.Get("X-Forwarded-Host")
		if fHost != "" {
			req.URL.Host = fHost
		}

		req.URL.Scheme = "http"
		if req.Header.Get("X-Forwarded-Proto") == "https" ||
			req.Header.Get("X-Forwarded-Scheme") == "https" ||
			strings.Contains(req.Header.Get("Forwarded"), "proto=https") {
			req.URL.Scheme = "https"
		}

		if viper.GetBool("validate-server") {
			// Use the scheme/host in the log message since we are validating it.
			info = fmt.Sprintf("%s %v", req.Method, req.URL)
		}

		route, pathParams, err := rr.Get().FindRoute(req.Method, req.URL)
		if err != nil {
			log.Printf("ERROR: %s => %v", info, err)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if viper.GetBool("validate-request") {
			err = openapi3filter.ValidateRequest(nil, &openapi3filter.RequestValidationInput{
				Request:    req,
				Route:      route,
				PathParams: pathParams,
				Options: &openapi3filter.Options{
					AuthenticationFunc: func(c context.Context, input *openapi3filter.AuthenticationInput) error {
						// TODO: support more schemes
						sec := input.SecurityScheme
						if sec.Type == "http" {
							// Prefixes for each scheme.
							prefixes := map[string]string{
								"bearer": "BEARER ",
								"basic":  "BASIC ",
							}
							if prefix, ok := prefixes[sec.Scheme]; ok {
								auth := req.Header.Get("Authorization")
								// If the auth is missing
								if len(auth) == 0 {
									return ErrMissingAuth
								}
								// If the auth doesn't have a value or doesn't start with the case insensitive prefix
								if len(auth) <= len(prefix) || !strings.HasPrefix(strings.ToUpper(auth), prefix) {
									return ErrInvalidAuth
								}
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

		prefer := parsePreferHeader(req.Header.Get("Prefer"))

		status, mediatype, headers, example, err := getExample(negotiator, prefer, route.Operation)
		if err != nil {
			log.Printf("%s => Missing example", info)
			w.WriteHeader(http.StatusTeapot)
			w.Write([]byte("No example available."))
			return
		}

		id := route.Operation.OperationID
		if id == "" {
			id = route.Operation.Summary
		}

		log.Printf("%s (%s) => %d (%s)", info, id, status, mediatype)

		var encoded []byte

		if s, ok := example.(string); ok {
			encoded = []byte(s)
		} else if _, ok := example.([]byte); ok {
			encoded = example.([]byte)
		} else {
			if marshalJSONMatcher.MatchString(mediatype) {
				encoded, err = json.MarshalIndent(example, "", "  ")
			} else if marshalYAMLMatcher.MatchString(mediatype) {
				encoded, err = yaml.Marshal(example)
			} else {
				log.Printf("Cannot marshal as '%s'!", mediatype)
				err = ErrCannotMarshal
			}

			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Unable to marshal response"))
				return
			}
		}

		for name, header := range headers {
			if header.Value != nil {
				example := name

				if header.Value.Schema != nil && header.Value.Schema.Value != nil {
					if v, err := OpenAPIExample(ModeResponse, header.Value.Schema.Value); err == nil {
						if vs, ok := v.(string); ok {
							example = vs
						} else {
							fmt.Printf("Could not convert example value '%v' to string", v)
						}
					}
				}

				w.Header().Set(name, example)
			}
		}

		if mediatype != "" {
			w.Header().Set("Content-Type", mediatype)
		}

		w.WriteHeader(status)
		w.Write(encoded)
	})
}

//
func loadSwaggerFromUri(uri string) (data []byte, err error) {
	if strings.HasPrefix(uri, "http") {
		req, httpErr := http.NewRequest("GET", uri, nil)
		if httpErr != nil {
			err = httpErr
			return
		}
		if customHeader := viper.GetString("header"); customHeader != "" {
			header := strings.Split(customHeader, ":")
			if len(header) != 2 {
				err = errors.New("Header format is invalid")
			} else {
				req.Header.Add(strings.TrimSpace(header[0]), strings.TrimSpace(header[1]))
			}
		}
		if err != nil {
			return
		}

		client := &http.Client{}
		resp, httpErr := client.Do(req)
		if httpErr != nil {
			err = httpErr
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			err = fmt.Errorf("Server at %s reported %d status code", uri, resp.StatusCode)
			return
		}
		data, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return
		}
	} else {
		data, err = ioutil.ReadFile(uri)
	}

	return data, err
}

// server loads an OpenAPI file and runs a mock server using the paths and
// examples defined in the file.
func server(cmd *cobra.Command, args []string) {
	var swagger *openapi3.Swagger
	rr := NewRefreshableRouter()

	uri := args[0]

	var err error
	var data []byte
	dataType := strings.Trim(strings.ToLower(filepath.Ext(uri)), ".")

	// Load either from an HTTP URL or from a local file depending on the passed
	// in value.
	data, err = loadSwaggerFromUri(uri)
	if err != nil {
		log.Fatal(err)
	}

	if viper.GetBool("watch") {
		if strings.HasPrefix(uri, "http") {
			log.Fatal(errors.New("Watching a URL is not supported."))
		}

		// Set up a new filesystem watcher and reload the router every time
		// the file has changed on disk.
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			log.Fatal(err)
		}
		defer watcher.Close()

		go func() {
			// Since waiting for events or errors is blocking, we do this in a
			// goroutine. It loops forever here but will exit when the process
			// is finished, e.g. when you `ctrl+c` to exit.
			for {
				select {
				case event, ok := <-watcher.Events:
					if !ok {
						return
					}
					if event.Op&fsnotify.Write == fsnotify.Write {
						fmt.Printf("🌙 Reloading %s\n", uri)
						data, err = loadSwaggerFromUri(uri)
						if err != nil {
							log.Printf("ERROR: %s", err)
						}

						if s, r, err := load(uri, data); err == nil {
							swagger = s
							rr.Set(r)
						} else {
							log.Printf("ERROR: Unable to load OpenAPI document: %s", err)
						}
					}
				case err, ok := <-watcher.Errors:
					if !ok {
						return
					}
					fmt.Println("error:", err)
				}
			}
		}()

		watcher.Add(uri)
	}

	swagger, router, err := load(uri, data)
	if err != nil {
		log.Fatal(err)
	}

	rr.Set(router)

	if strings.HasPrefix(uri, "http") {
		http.HandleFunc("/__reload", func(w http.ResponseWriter, r *http.Request) {
			log.Printf("🌙 Reloading %s\n", uri)
			data, err = loadSwaggerFromUri(uri)
			if err == nil {
				if s, r, err := load(uri, data); err == nil {
					swagger = s
					rr.Set(r)
				}
			}
			if err == nil {
				log.Printf("Reloaded from %s", uri)
				w.WriteHeader(200)
				w.Write([]byte("reloaded"))
			} else {
				log.Printf("ERROR: %s", err)
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("error while reloading"))
			}
		})
	}

	// Add a health check route which returns 200
	http.HandleFunc("/__health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		log.Printf("Health check")
	})

	// Another custom handler to return the exact swagger document given to us
	http.HandleFunc("/__schema", func(w http.ResponseWriter, req *http.Request) {
		if !viper.GetBool("disable-cors") {
			corsOrigin := req.Header.Get("Origin")
			if corsOrigin == "" {
				corsOrigin = "*"
			}
			w.Header().Set("Access-Control-Allow-Origin", corsOrigin)
		}

		w.Header().Set("Content-Type", fmt.Sprintf("application/%v; charset=utf-8", dataType))
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, string(data))
	})

	// Register our custom HTTP handler that will use the router to find
	// the appropriate OpenAPI operation and try to return an example.
	http.Handle("/", handler(rr))

	format := "🌱 Sprouting %s on port %d"
	if viper.GetBool("https") {
		format = "🌱 Securely sprouting %s on port %d"
	}
	fmt.Printf(format, swagger.Info.Title, viper.GetInt("port"))

	if viper.GetBool("validate-server") && len(swagger.Servers) != 0 {
		fmt.Printf(" with valid servers:\n")
		for _, s := range swagger.Servers {
			fmt.Println("• " + s.URL)
		}
	} else {
		fmt.Printf("\n")
	}

	port := fmt.Sprintf(":%d", viper.GetInt("port"))
	if viper.GetBool("https") {
		err = http.ListenAndServeTLS(port, viper.GetString("public-key"),
			viper.GetString("private-key"), nil)
	} else {
		err = http.ListenAndServe(port, nil)
	}
	if err != nil {
		log.Fatal(err)
	}
}
