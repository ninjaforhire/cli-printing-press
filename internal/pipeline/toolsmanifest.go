package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/mvanhorn/cli-printing-press/v2/internal/spec"
)

// ToolsManifestFilename is the name of the tools manifest file written to each
// published CLI directory for use by the mega MCP server.
const ToolsManifestFilename = "tools-manifest.json"

// ToolsManifest describes every MCP tool for an API, along with API-level
// metadata needed by the mega MCP to register and execute tools without
// runtime spec parsing. These types will move to internal/megamcp/types.go
// in Unit 3.
type ToolsManifest struct {
	APIName         string           `json:"api_name"`
	BaseURL         string           `json:"base_url"`
	Description     string           `json:"description"`
	MCPReady        string           `json:"mcp_ready"`
	HTTPTransport   string           `json:"http_transport,omitempty"`
	Auth            ManifestAuth     `json:"auth"`
	RequiredHeaders []ManifestHeader `json:"required_headers"`
	Tools           []ManifestTool   `json:"tools"`
}

// ManifestAuth captures the auth configuration needed to make authenticated
// API requests at runtime.
type ManifestAuth struct {
	Type                           string   `json:"type"`
	Header                         string   `json:"header,omitempty"`
	Format                         string   `json:"format,omitempty"`
	In                             string   `json:"in,omitempty"`
	EnvVars                        []string `json:"env_vars,omitempty"`
	KeyURL                         string   `json:"key_url,omitempty"`
	CookieDomain                   string   `json:"cookie_domain,omitempty"`
	RequiresBrowserSession         bool     `json:"requires_browser_session,omitempty"`
	BrowserSessionValidationPath   string   `json:"browser_session_validation_path,omitempty"`
	BrowserSessionValidationMethod string   `json:"browser_session_validation_method,omitempty"`
}

// ManifestTool describes a single MCP tool derived from an API endpoint.
type ManifestTool struct {
	Name            string           `json:"name"`
	Description     string           `json:"description"`
	Method          string           `json:"method"`
	Path            string           `json:"path"`
	NoAuth          bool             `json:"no_auth,omitempty"`
	Params          []ManifestParam  `json:"params"`
	HeaderOverrides []ManifestHeader `json:"header_overrides,omitempty"`
}

// ManifestParam describes a tool parameter with an explicit location
// (path, query, or body).
type ManifestParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Location    string `json:"location"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// ManifestHeader represents a header name/value pair used for both
// API-level required headers and per-tool header overrides.
type ManifestHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// WriteToolsManifest generates a tools-manifest.json from a parsed API spec.
// It iterates Resources/SubResources/Endpoints in sorted key order (matching
// the MCP template's RegisterTools pattern) and writes deterministic JSON.
func WriteToolsManifest(dir string, parsed *spec.APISpec) error {
	if parsed == nil {
		return fmt.Errorf("parsed spec is nil")
	}

	total, public := parsed.CountMCPTools()
	mcpReady := computeMCPReady(parsed.Auth.Type, public)

	// For cookie/composed auth, only include NoAuth endpoints.
	cookieOrComposed := parsed.Auth.Type == "cookie" || parsed.Auth.Type == "composed"

	manifest := ToolsManifest{
		APIName:       parsed.Name,
		BaseURL:       parsed.BaseURL,
		Description:   parsed.Description,
		MCPReady:      mcpReady,
		HTTPTransport: parsed.EffectiveHTTPTransport(),
		Auth: ManifestAuth{
			Type:                           parsed.Auth.Type,
			Header:                         parsed.Auth.Header,
			Format:                         normalizeAuthFormat(parsed.Auth.Format, parsed.Auth.EnvVars),
			In:                             parsed.Auth.In,
			EnvVars:                        parsed.Auth.EnvVars,
			KeyURL:                         parsed.Auth.KeyURL,
			CookieDomain:                   parsed.Auth.CookieDomain,
			RequiresBrowserSession:         parsed.Auth.RequiresBrowserSession,
			BrowserSessionValidationPath:   parsed.Auth.BrowserSessionValidationPath,
			BrowserSessionValidationMethod: parsed.Auth.BrowserSessionValidationMethod,
		},
		RequiredHeaders: make([]ManifestHeader, 0, len(parsed.RequiredHeaders)),
		Tools:           make([]ManifestTool, 0),
	}

	for _, rh := range parsed.RequiredHeaders {
		manifest.RequiredHeaders = append(manifest.RequiredHeaders, ManifestHeader{
			Name:  rh.Name,
			Value: rh.Value,
		})
	}

	// Iterate resources in sorted order for deterministic output.
	resourceNames := sortedResourceKeys(parsed.Resources)
	for _, rName := range resourceNames {
		resource := parsed.Resources[rName]

		// Top-level endpoints
		endpointNames := sortedEndpointKeys(resource.Endpoints)
		for _, eName := range endpointNames {
			endpoint := resource.Endpoints[eName]
			if cookieOrComposed && !endpoint.NoAuth {
				continue
			}
			toolName := toolSnake(rName) + "_" + toolSnake(eName)
			desc := mcpDescriptionForManifest(endpoint.Description, endpoint.NoAuth, parsed.Auth.Type, public, total)
			tool := buildManifestTool(toolName, desc, endpoint)
			manifest.Tools = append(manifest.Tools, tool)
		}

		// Sub-resources
		subNames := sortedResourceKeys(resource.SubResources)
		for _, subName := range subNames {
			subResource := resource.SubResources[subName]
			subEndpointNames := sortedEndpointKeys(subResource.Endpoints)
			for _, eName := range subEndpointNames {
				endpoint := subResource.Endpoints[eName]
				if cookieOrComposed && !endpoint.NoAuth {
					continue
				}
				toolName := toolSnake(rName) + "_" + toolSnake(subName) + "_" + toolSnake(eName)
				desc := mcpDescriptionForManifest(endpoint.Description, endpoint.NoAuth, parsed.Auth.Type, public, total)
				tool := buildManifestTool(toolName, desc, endpoint)
				manifest.Tools = append(manifest.Tools, tool)
			}
		}
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling tools manifest: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(filepath.Join(dir, ToolsManifestFilename), data, 0o644); err != nil {
		return fmt.Errorf("writing tools manifest: %w", err)
	}
	return nil
}

// buildManifestTool creates a ManifestTool from an endpoint, classifying
// each parameter's location.
func buildManifestTool(name, description string, ep spec.Endpoint) ManifestTool {
	tool := ManifestTool{
		Name:        name,
		Description: description,
		Method:      strings.ToUpper(ep.Method),
		Path:        ep.Path,
		NoAuth:      ep.NoAuth,
		Params:      make([]ManifestParam, 0, len(ep.Params)+len(ep.Body)),
	}

	// Regular params: positional → path, others → query.
	for _, p := range ep.Params {
		loc := "query"
		if p.Positional {
			loc = "path"
		}
		tool.Params = append(tool.Params, ManifestParam{
			Name:        p.Name,
			Type:        normalizeParamType(p.Type),
			Location:    loc,
			Description: p.Description,
			Required:    p.Required,
		})
	}

	// Body params → body.
	for _, p := range ep.Body {
		tool.Params = append(tool.Params, ManifestParam{
			Name:        p.Name,
			Type:        normalizeParamType(p.Type),
			Location:    "body",
			Description: p.Description,
			Required:    p.Required,
		})
	}

	// Per-endpoint header overrides.
	if len(ep.HeaderOverrides) > 0 {
		tool.HeaderOverrides = make([]ManifestHeader, 0, len(ep.HeaderOverrides))
		for _, ho := range ep.HeaderOverrides {
			tool.HeaderOverrides = append(tool.HeaderOverrides, ManifestHeader{
				Name:  ho.Name,
				Value: ho.Value,
			})
		}
	}

	return tool
}

// normalizeAuthFormat rewrites the auth format string so that derived
// placeholders (like {token} from DUB_TOKEN) become the actual env var
// name ({DUB_TOKEN}). This way the mega MCP's runtime expansion only needs
// to handle env var names, not the derived semantic aliases that the
// generated config template uses.
func normalizeAuthFormat(format string, envVars []string) string {
	if format == "" || len(envVars) == 0 {
		return format
	}
	result := format
	for _, envVar := range envVars {
		derived := envVarPlaceholder(envVar)
		if derived != strings.ToLower(envVar) {
			// Replace the derived placeholder with the env var name.
			result = strings.ReplaceAll(result, "{"+derived+"}", "{"+envVar+"}")
		}
	}
	// Also replace common semantic aliases with the first env var.
	first := envVars[0]
	for _, alias := range []string{"token", "access_token", "api_key"} {
		// Only replace if it's not already the env var name.
		if alias != strings.ToLower(first) {
			result = strings.ReplaceAll(result, "{"+alias+"}", "{"+first+"}")
		}
	}
	return result
}

// envVarPlaceholder derives the placeholder name from an env var.
// DUB_TOKEN -> token, STYTCH_PROJECT_ID -> project_id.
// Mirrors the logic in internal/generator/generator.go:1331.
func envVarPlaceholder(envVar string) string {
	parts := strings.Split(envVar, "_")
	if len(parts) <= 1 {
		return strings.ToLower(envVar)
	}
	lower := make([]string, 0, len(parts)-1)
	for _, p := range parts[1:] {
		lower = append(lower, strings.ToLower(p))
	}
	return strings.Join(lower, "_")
}

// normalizeParamType ensures a consistent type string. Empty types default
// to "string".
func normalizeParamType(t string) string {
	if t == "" {
		return "string"
	}
	return t
}

// toolSnake converts CamelCase to snake_case for tool name segments.
// This is a copy of toSnake from internal/generator/generator.go:794.
// It does NOT convert hyphens, matching the MCP template's {{snake}} function.
func toolSnake(s string) string {
	var result strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			result.WriteRune('_')
		}
		result.WriteRune(unicode.ToLower(r))
	}
	return result.String()
}

// mcpDescriptionForManifest replicates the mcpDescription template function
// from internal/generator/generator.go:1078 — adds minority-side annotations
// for mixed auth/no-auth APIs.
func mcpDescriptionForManifest(desc string, noAuth bool, authType string, publicCount, totalCount int) string {
	authCount := totalCount - publicCount
	mixed := publicCount > 0 && authCount > 0

	if mixed {
		if noAuth && publicCount < authCount {
			desc = desc + " (public)"
		} else if !noAuth && authCount < publicCount {
			switch authType {
			case "api_key":
				desc = desc + " (requires API key)"
			case "cookie", "composed":
				desc = desc + " (requires browser login)"
			case "oauth2", "bearer_token":
				desc = desc + " (requires auth)"
			default:
				desc = desc + " (requires auth)"
			}
		}
	}

	return onelineForManifest(desc)
}

// onelineForManifest replicates the oneline template function from
// internal/generator/generator.go:1051.
func onelineForManifest(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, `"`, `'`)
	s = strings.ReplaceAll(s, "\\", "")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	s = strings.TrimSpace(s)
	if len(s) > 120 {
		cut := s[:117]
		if idx := strings.LastIndex(cut, " "); idx > 60 {
			s = cut[:idx] + "..."
		} else {
			s = cut + "..."
		}
	}
	return s
}

// sortedResourceKeys returns sorted keys from a map[string]spec.Resource.
func sortedResourceKeys(m map[string]spec.Resource) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sortedEndpointKeys returns sorted keys from a map[string]spec.Endpoint.
func sortedEndpointKeys(m map[string]spec.Endpoint) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ComputeToolsManifestChecksum returns the SHA-256 checksum of manifest data
// in "sha256:<hex>" format, matching the format used in registry.json.
func ComputeToolsManifestChecksum(data []byte) string {
	h := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(h[:])
}
