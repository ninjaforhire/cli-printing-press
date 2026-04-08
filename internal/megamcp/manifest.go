package megamcp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

// LoadManifests fetches and caches tools manifests for all eligible registry entries.
// It applies the PRINTING_PRESS_APIS env var filter, loads manifests in parallel,
// and returns successfully loaded API entries plus any warnings.
// Failed fetches produce warnings but do not block other APIs.
// If entries is nil (registry fetch failed), falls back to cached manifests.
func LoadManifests(entries []RegistryEntry, cacheDir, baseURL string) ([]*APIEntry, []string) {
	// If no registry was provided (fetch failed), try loading from cache.
	if entries == nil {
		return loadFromCacheOnly(cacheDir)
	}

	// Apply PRINTING_PRESS_APIS filter if set.
	filtered := filterEntries(entries)

	var (
		mu       sync.Mutex
		results  []*APIEntry
		warnings []string
	)

	var g errgroup.Group

	for _, entry := range filtered {
		g.Go(func() error {
			apiEntry, warn := loadSingleManifest(entry, cacheDir, baseURL)
			mu.Lock()
			defer mu.Unlock()
			if warn != "" {
				warnings = append(warnings, warn)
			}
			if apiEntry != nil {
				results = append(results, apiEntry)
			}
			return nil // never fail the group; warnings are collected
		})
	}

	// errgroup.Group (not WithContext) — errors don't cancel siblings.
	_ = g.Wait()

	return results, warnings
}

// loadFromCacheOnly scans the cache directory for previously cached manifests.
// Used as a fallback when the registry fetch fails.
func loadFromCacheOnly(cacheDir string) ([]*APIEntry, []string) {
	manifestsDir := filepath.Join(cacheDir, "manifests")
	dirEntries, err := os.ReadDir(manifestsDir)
	if err != nil {
		return nil, []string{"no cached manifests available (registry fetch failed)"}
	}

	apiFilter := parseAPIFilter()
	var results []*APIEntry
	var warnings []string

	for _, de := range dirEntries {
		if !de.IsDir() {
			continue
		}
		slug := de.Name()
		if err := ValidateSlug(slug); err != nil {
			continue
		}
		if apiFilter != nil && !apiFilter[slug] {
			continue
		}

		cachePath := filepath.Join(manifestsDir, slug, "tools-manifest.json")
		data, err := os.ReadFile(cachePath)
		if err != nil {
			continue
		}

		manifest, err := parseManifest(data)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("cached manifest for %q is corrupt: %v", slug, err))
			continue
		}

		// Validate base URL even for cached manifests.
		if err := ValidateBaseURL(manifest.BaseURL); err != nil {
			warnings = append(warnings, fmt.Sprintf("skipping cached %q: unsafe base URL: %v", slug, err))
			continue
		}

		prefix, err := slugToToolPrefix(slug)
		if err != nil {
			continue
		}

		results = append(results, &APIEntry{
			Slug:             slug,
			Dir:              filepath.Join(manifestsDir, slug),
			Manifest:         manifest,
			NormalizedPrefix: prefix,
		})
	}

	if len(results) > 0 {
		warnings = append(warnings, fmt.Sprintf("registry fetch failed; loaded %d APIs from cache", len(results)))
	}

	return results, warnings
}

// filterEntries applies the PRINTING_PRESS_APIS env var filter and excludes
// entries that are cli-only or have no ManifestURL.
func filterEntries(entries []RegistryEntry) []RegistryEntry {
	apiFilter := parseAPIFilter()

	var filtered []RegistryEntry
	for _, entry := range entries {
		// Skip entries without manifest URL.
		if entry.MCP.ManifestURL == "" {
			continue
		}
		// Skip cli-only entries.
		if entry.MCP.MCPReady == "cli-only" {
			continue
		}
		// Apply PRINTING_PRESS_APIS filter if set.
		if apiFilter != nil {
			slug := extractSlug(entry)
			if !apiFilter[slug] {
				continue
			}
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

// parseAPIFilter returns a set of allowed API slugs from PRINTING_PRESS_APIS,
// or nil if the env var is not set (meaning all APIs are allowed).
func parseAPIFilter() map[string]bool {
	val := os.Getenv("PRINTING_PRESS_APIS")
	if val == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	filter := make(map[string]bool, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			filter[p] = true
		}
	}
	return filter
}

// extractSlug derives the API slug from a registry entry's API field.
func extractSlug(entry RegistryEntry) string {
	return entry.API
}

// loadSingleManifest loads and caches a single API's tools manifest.
// Returns the APIEntry on success, or a warning string on failure.
func loadSingleManifest(entry RegistryEntry, cacheDir, baseURL string) (*APIEntry, string) {
	slug := extractSlug(entry)

	// Validate slug before constructing any paths.
	if err := ValidateSlug(slug); err != nil {
		return nil, fmt.Sprintf("skipping %q: %v", slug, err)
	}

	prefix, err := slugToToolPrefix(slug)
	if err != nil {
		return nil, fmt.Sprintf("skipping %q: invalid tool prefix: %v", slug, err)
	}

	// Construct cache path and validate it.
	cacheSubDir := filepath.Join(cacheDir, "manifests", slug)
	cachePath := filepath.Join(cacheSubDir, "tools-manifest.json")

	if err := ValidateCachePath(cachePath, cacheDir); err != nil {
		return nil, fmt.Sprintf("skipping %q: %v", slug, err)
	}

	// Try cached manifest first.
	manifest, err := tryCache(cachePath, entry.MCP.ManifestChecksum)
	if err == nil && manifest != nil {
		// Validate base URL even for cached manifests.
		if urlErr := ValidateBaseURL(manifest.BaseURL); urlErr != nil {
			return nil, fmt.Sprintf("skipping %q: unsafe base URL %q: %v", slug, manifest.BaseURL, urlErr)
		}
		return &APIEntry{
			Slug:             slug,
			Dir:              cacheSubDir,
			Manifest:         manifest,
			NormalizedPrefix: prefix,
		}, ""
	}

	// Cache miss or checksum mismatch — fetch from remote.
	manifestURL := baseURL + "/" + entry.MCP.ManifestURL
	data, err := fetchManifestData(manifestURL)
	if err != nil {
		return nil, fmt.Sprintf("skipping %q: fetch failed: %v", slug, err)
	}

	// Verify checksum if provided.
	if entry.MCP.ManifestChecksum != "" {
		if err := VerifyChecksum(data, entry.MCP.ManifestChecksum); err != nil {
			return nil, fmt.Sprintf("skipping %q: %v", slug, err)
		}
	}

	// Parse manifest.
	manifest, err = parseManifest(data)
	if err != nil {
		return nil, fmt.Sprintf("skipping %q: %v", slug, err)
	}

	// Validate the manifest's base URL against SSRF protections.
	if err := ValidateBaseURL(manifest.BaseURL); err != nil {
		return nil, fmt.Sprintf("skipping %q: unsafe base URL %q: %v", slug, manifest.BaseURL, err)
	}

	// Write to cache via temp-then-rename.
	if err := writeCache(cacheSubDir, cachePath, data); err != nil {
		// Cache write failure is a warning, not a blocker.
		return &APIEntry{
			Slug:             slug,
			Dir:              cacheSubDir,
			Manifest:         manifest,
			NormalizedPrefix: prefix,
		}, fmt.Sprintf("warning: %q: cache write failed: %v", slug, err)
	}

	return &APIEntry{
		Slug:             slug,
		Dir:              cacheSubDir,
		Manifest:         manifest,
		NormalizedPrefix: prefix,
	}, ""
}

// tryCache attempts to read and verify a cached manifest.
// Returns nil, nil if cache doesn't exist.
// Returns nil, error if cache exists but checksum mismatches or is unreadable.
func tryCache(cachePath, expectedChecksum string) (*ToolsManifest, error) {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading cache: %w", err)
	}

	// Verify checksum if expected is provided.
	if expectedChecksum != "" {
		if err := VerifyChecksum(data, expectedChecksum); err != nil {
			return nil, fmt.Errorf("cache checksum mismatch: %w", err)
		}
	}

	manifest, err := parseManifest(data)
	if err != nil {
		return nil, fmt.Errorf("parsing cached manifest: %w", err)
	}

	return manifest, nil
}

// fetchManifestData downloads manifest data from a URL.
func fetchManifestData(manifestURL string) ([]byte, error) {
	resp, err := http.Get(manifestURL)
	if err != nil {
		return nil, fmt.Errorf("HTTP request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	return data, nil
}

// parseManifest unmarshals JSON data into a ToolsManifest.
func parseManifest(data []byte) (*ToolsManifest, error) {
	var manifest ToolsManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest JSON: %w", err)
	}
	return &manifest, nil
}

// writeCache writes manifest data to disk via temp-then-rename for atomicity.
// Creates the directory structure with 0700 permissions and writes files with 0600.
func writeCache(dir, cachePath string, data []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	tmpPath := cachePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, cachePath); err != nil {
		_ = os.Remove(tmpPath) // clean up on failure
		return fmt.Errorf("renaming temp file: %w", err)
	}

	return nil
}

// slugToToolPrefix converts an API slug to a tool name prefix.
// Hyphens become underscores, consecutive underscores are collapsed,
// and the result is rejected if it still contains "__".
func slugToToolPrefix(slug string) (string, error) {
	// Replace hyphens with underscores.
	result := strings.ReplaceAll(slug, "-", "_")

	// Collapse consecutive underscores.
	for strings.Contains(result, "__") {
		result = strings.ReplaceAll(result, "__", "_")
	}

	// Reject if empty after normalization.
	if result == "" || result == "_" {
		return "", fmt.Errorf("slug %q normalizes to empty prefix", slug)
	}

	return result, nil
}
