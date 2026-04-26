package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/mark3labs/mcp-go/server"
	"github.com/mvanhorn/cli-printing-press/v2/internal/megamcp"
	"github.com/mvanhorn/cli-printing-press/v2/internal/version"
)

const (
	defaultBaseURL = "https://raw.githubusercontent.com/mvanhorn/printing-press-library/main"
)

func main() {
	baseURL := os.Getenv("PRINTING_PRESS_MCP_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	cacheDir := os.Getenv("PRINTING_PRESS_MCP_CACHE_DIR")
	if cacheDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot determine home directory: %v\n", err)
			os.Exit(1)
		}
		cacheDir = filepath.Join(home, ".cache", "printing-press-mcp")
	}

	// Fetch registry from GitHub (or cache).
	registry, err := megamcp.FetchRegistry(baseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not fetch registry: %v\n", err)
		// Continue with empty entries — LoadManifests handles nil gracefully,
		// and cached manifests from a previous successful run will still be used.
	}

	// Load and cache tools manifests.
	var registryEntries []megamcp.RegistryEntry
	if registry != nil {
		registryEntries = registry.Entries
	}
	entries, warnings := megamcp.LoadManifests(registryEntries, cacheDir, baseURL)
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}

	// Create the MCP server with dynamic tool capabilities.
	s := server.NewMCPServer("printing-press-mcp", version.Version,
		server.WithToolCapabilities(true))

	// Set up activation manager and register meta-tools.
	am := megamcp.NewActivationManager(s, entries)
	megamcp.RegisterMetaTools(s, am)

	fmt.Fprintf(os.Stderr, "printing-press-mcp %s: %d APIs loaded, serving on stdio\n",
		version.Version, len(entries))

	// Handle graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintf(os.Stderr, "\nshutting down...\n")
		os.Exit(0)
	}()

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
		os.Exit(1)
	}
}
