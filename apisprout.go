package apisprout

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
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/fsnotify/fsnotify"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/gobwas/glob"
	"github.com/oklog/run"
	"github.com/spf13/cobra"
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
	blankHeaders := make(map[string]*openapi3.HeaderRef)

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
func (cr *ConfigReloader) addLocalServers(swagger *openapi3.T) error {
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
			u.Host = fmt.Sprintf("localhost:%d", cr.Port)

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

func (s *OpenAPIServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !s.DisableCORS {
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

		route, pathParams, err := s.r.FindRoute(req)
		if err != nil {
			log.Printf("ERROR: %s => %v", info, err)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if s.ValidateRequest {
			err = openapi3filter.ValidateRequest(req.Context(), &openapi3filter.RequestValidationInput{
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
				fmt.Fprintf(w, "%v", err)
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
			fmt.Fprint(w, "No example available.")
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
				err = ErrCannotMarshal
			}

			if err != nil {
				log.Printf("Cannot marshal as '%s'!: %s", mediatype, err.Error())
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, "Unable to marshal response")
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

type ConfigReloader struct {
	OpenAPIServer *OpenAPIServer
	Mux           *http.ServeMux

	URI             string
	WithServer      string
	Watch           bool
	HTTPS           bool
	DisableCORS     bool
	ValidateServer  bool
	ValidateRequest bool
	Header          string
	PublicKey       string
	PrivateKey      string
	Port            int
}

type Option func(c *config)

func WithValidateRequest(s bool) Option {
	return func(c *config) {
		c.ValidateRequest = s
	}
}

func WithDisableCORS(b bool) Option {
	return func(c *config) {
		c.DisableCORS = b
	}
}

func NewOpenAPIServer(swagger *openapi3.T, options ...Option) (*OpenAPIServer, error) {
	c := &config{}
	for _, o := range options {
		o(c)
	}
	r, err := gorillamux.NewRouter(swagger)
	if err != nil {
		return nil, err
	}

	s := &OpenAPIServer{
		Swagger: swagger,
		r:       r,
		config:  *c,
	}

	return s, nil
}

type config struct {
	DisableCORS     bool
	ValidateRequest bool
}

type OpenAPIServer struct {
	Swagger *openapi3.T
	r       routers.Router
	config
}

func (s *OpenAPIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler().ServeHTTP(w, r)
}

func ServeCMD(cmd *cobra.Command, args []string) error {
	r := &ConfigReloader{
		Mux:             http.NewServeMux(),
		URI:             args[0],
		Header:          viper.GetString("header"),
		WithServer:      viper.GetString("add-server"),
		Watch:           viper.GetBool("watch"),
		Port:            viper.GetInt("port"),
		HTTPS:           viper.GetBool("https"),
		DisableCORS:     viper.GetBool("disable-cors"),
		ValidateServer:  viper.GetBool("validate-server"),
		ValidateRequest: viper.GetBool("validate-request"),
		PublicKey:       viper.GetString("public-key"),
		PrivateKey:      viper.GetString("private-key"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g := run.Group{}
	g.Add(run.SignalHandler(ctx, os.Interrupt, syscall.SIGTERM))
	g.Add(func() error {
		return r.Serve(ctx)
	}, func(err error) {
		cancel()
	})

	sErr := new(run.SignalError)
	if err := g.Run(); errors.As(err, sErr) {
		log.Println(sErr.Signal)
	} else {
		return err
	}
	return nil
}

func (s *ConfigReloader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Mux.ServeHTTP(w, r)
}

// server loads an OpenAPI file and runs a mock server using the paths and
// examples defined in the file.
func (s *ConfigReloader) Serve(ctx context.Context) error {
	if err := s.Run(ctx); err != nil {
		return err
	}

	format := "🌱 Sprouting %s on port %d"
	if s.HTTPS {
		format = "🌱 Securely sprouting %s on port %d"
	}
	fmt.Printf(format, s.OpenAPIServer.Swagger.Info.Title, s.Port)

	if s.ValidateServer && len(s.OpenAPIServer.Swagger.Servers) != 0 {
		fmt.Printf(" with valid servers:\n")
		for _, s := range s.OpenAPIServer.Swagger.Servers {
			fmt.Println("• " + s.URL)
		}
	} else {
		fmt.Printf("\n")
	}
	port := fmt.Sprintf(":%d", s.Port)
	var err error
	server := http.Server{Addr: port, Handler: s}
	go func() {
		<-ctx.Done()
		server.Close()
	}()

	if s.HTTPS {
		err = server.ListenAndServeTLS(s.PublicKey,
			s.PrivateKey)
	} else {
		err = server.ListenAndServe()
	}
	return err
}

func (r *ConfigReloader) Reload(ctx context.Context) error {
	swagger, err := r.Load(ctx)
	if err != nil {
		return err
	}
	if r.ValidateServer {
		// Special-case localhost to always be allowed for local testing.
		if err = r.addLocalServers(swagger); err != nil {
			return err
		}

		if cs := r.WithServer; cs != "" {
			swagger.Servers = append(swagger.Servers, &openapi3.Server{
				URL:         cs,
				Description: "Custom server from command line param",
				Variables:   make(map[string]*openapi3.ServerVariable),
			})
		}
	} else {
		swagger.AddServer(&openapi3.Server{URL: "/", Description: "server to listen on all addresses"})
	}

	o, err := NewOpenAPIServer(
		swagger,
		WithDisableCORS(r.DisableCORS),
		WithValidateRequest(r.ValidateRequest),
	)
	if err != nil {
		return err
	}

	if r.OpenAPIServer == nil {
		r.OpenAPIServer = o

		return nil
	}

	*r.OpenAPIServer = *o

	return nil
}

func (s *ConfigReloader) Load(ctx context.Context) (swagger *openapi3.T, err error) {
	var data []byte
	if strings.HasPrefix(s.URI, "http") {
		if s.Watch {
			return nil, errors.New("Watching a URL is not supported.")
		}
		req, err := http.NewRequest("GET", s.URI, nil)
		if err != nil {
			return nil, err
		}
		if customHeader := s.Header; customHeader != "" {
			header := strings.Split(customHeader, ":")
			if len(header) != 2 {
				return nil, err
			}
			req.Header.Add(strings.TrimSpace(header[0]), strings.TrimSpace(header[1]))
		}
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		data, err = ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
	} else {

		data, err = ioutil.ReadFile(s.URI)
		if err != nil {
			return nil, err
		}
	}

	loader := &openapi3.Loader{Context: ctx, IsExternalRefsAllowed: true}

	var u *url.URL
	u, err = url.Parse(s.URI)
	if err != nil {
		return nil, fmt.Errorf("failed parse URI: %w", err)
	}

	swagger, err = loader.LoadFromDataWithPath(data, u)
	if err != nil {
		return nil, fmt.Errorf("failed to load openapi spec: %w", err)
	}

	return swagger, nil
}

// Run loads the swagger spec from the URI and adds some utility routes to the server mux
func (s *ConfigReloader) Run(ctx context.Context) error {
	// Load either from an HTTP URL or from a local file depending on the passed
	// in value.
	if err := s.Reload(ctx); err != nil {
		return err
	}

	if s.Watch {
		// Set up a new filesystem watcher and reload the router every time
		// the file has changed on disk.
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return err
		}

		go func() {
			// Since waiting for events or errors is blocking, we do this in a
			// goroutine. It loops forever here but will exit when the process
			// is finished, e.g. when you `ctrl+c` to exit.
			for {
				select {
				case event, ok := <-watcher.Events:
					fmt.Println(event)
					if !ok {
						log.Fatal("watcher closed")
					}
					// Many IDEs (eg.: vim, neovim) rename and delete the file on changes.
					// We will stop watching the events for the file name that we care for,
					// but only watch the renamed path. In case the renamed file was deleted,
					// it will also be deteled from the watched paths, causing us to watch nothing.
					// In this implementation we would continue running the server for the renamed file
					// until it is deleted.
					// The hidden "feature" would be that renamed files would still be watched.
					// We can't remove it from the watch list because we can't get the new name.
					// At least not with this version of fsnotify.
					if event.Op&fsnotify.Rename == fsnotify.Rename {
						if err := watcher.Add(s.URI); err != nil {
							log.Fatal(err)
						}
					}
					if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Remove == fsnotify.Remove {
						fmt.Printf("🌙 Reloading %s\n", s.URI)
						if err := s.Reload(ctx); err != nil {
							log.Printf("ERROR: Unable to load OpenAPI document: %s", err)
						}
					}
				case err, ok := <-watcher.Errors:
					if !ok {
						watcher.Close()
						return
					}
					fmt.Println("error:", err)
				case <-ctx.Done():
					watcher.Close()
					return
				}
			}
		}()

		if err := watcher.Add(s.URI); err != nil {
			return err
		}
	}

	if strings.HasPrefix(s.URI, "http") {
		s.Mux.HandleFunc("/__reload", func(w http.ResponseWriter, r *http.Request) {
			if err := s.Reload(ctx); err != nil {
				log.Printf("ERROR: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("error while reloading"))
				return

			}

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("reloaded"))
			log.Printf("Reloaded from %s", s.URI)
		})
	}

	// Add a health check route which returns 200
	s.Mux.HandleFunc("/__health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		log.Printf("Health check")
	})

	// Another custom handler to return the exact swagger document given to us
	s.Mux.HandleFunc("/__schema", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(s.OpenAPIServer.Swagger)
	})
	// Register our custom HTTP handler that will use the router to find
	// the appropriate OpenAPI operation and try to return an example.
	if s.DisableCORS {
		s.Mux.Handle("/", disableCORS(s.OpenAPIServer))
	} else {
		s.Mux.Handle("/", s.OpenAPIServer)
	}
	return nil
}

func disableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corsOrigin := r.Header.Get("Origin")
		if corsOrigin == "" {
			corsOrigin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", corsOrigin)
		next.ServeHTTP(w, r)
	})
}
