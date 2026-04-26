package megamcp

import (
	"github.com/mvanhorn/cli-printing-press/v2/internal/pipeline"
)

// Re-export manifest types from pipeline to avoid duplication.
// These were defined in internal/pipeline/toolsmanifest.go (Unit 1).
type ToolsManifest = pipeline.ToolsManifest
type ManifestTool = pipeline.ManifestTool
type ManifestParam = pipeline.ManifestParam
type ManifestAuth = pipeline.ManifestAuth
type ManifestHeader = pipeline.ManifestHeader

// RegistryEntry matches the schema of a single entry in registry.json
// from the public library repo.
type RegistryEntry struct {
	Name        string      `json:"name"`
	Category    string      `json:"category"`
	API         string      `json:"api"`
	Description string      `json:"description"`
	Path        string      `json:"path"`
	MCP         RegistryMCP `json:"mcp"`
}

// RegistryMCP holds MCP-specific metadata within a registry entry.
type RegistryMCP struct {
	Binary           string   `json:"binary"`
	Transport        string   `json:"transport"`
	ToolCount        int      `json:"tool_count"`
	PublicToolCount  int      `json:"public_tool_count"`
	AuthType         string   `json:"auth_type"`
	EnvVars          []string `json:"env_vars"`
	MCPReady         string   `json:"mcp_ready"`
	ManifestChecksum string   `json:"manifest_checksum"`
	SpecFormat       string   `json:"spec_format"`
	ManifestURL      string   `json:"manifest_url"`
}

// Registry represents the top-level registry.json structure.
type Registry struct {
	SchemaVersion int             `json:"schema_version"`
	Entries       []RegistryEntry `json:"entries"`
}

// APIEntry holds aggregated runtime state for a loaded API.
type APIEntry struct {
	Slug             string
	Dir              string
	Manifest         *ToolsManifest
	NormalizedPrefix string
}
