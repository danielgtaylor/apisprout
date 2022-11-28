package main

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/danielgtaylor/apisprout"
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
		Version: apisprout.GitSummary,
		Args:    cobra.MinimumNArgs(1),
		RunE:    apisprout.ServeCMD,
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
	if err := root.Execute(); err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		os.Exit(1)
	}
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
