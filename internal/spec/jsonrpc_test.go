package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestJSONRPCConfigYAMLAndValidation(t *testing.T) {
	t.Parallel()

	var api APISpec
	require.NoError(t, yaml.Unmarshal([]byte(`
name: meeting-tools
base_url: https://api.example.com/mcp
client_pattern: jsonrpc
x-jsonrpc:
  envelope: mcp
resources:
  meetings:
    endpoints:
      search:
        method: POST
        path: /tools/search
        x-jsonrpc-method: search_meetings
`), &api))
	require.NoError(t, api.Validate())
	assert.Equal(t, JSONRPCVersion, api.JSONRPC.EffectiveVersion())
	assert.True(t, api.JSONRPC.IsMCP())
}

func TestJSONRPCValidationRejectsMissingMethodAndInvalidEnvelope(t *testing.T) {
	t.Parallel()

	base := APISpec{
		Name:          "meeting-tools",
		BaseURL:       "https://api.example.com/mcp",
		ClientPattern: ClientPatternJSONRPC,
		Resources: map[string]Resource{
			"meetings": {Endpoints: map[string]Endpoint{"search": {Method: "POST", Path: "/tools/search"}}},
		},
	}
	require.ErrorContains(t, base.Validate(), "x-jsonrpc-method is required")

	base.Resources["meetings"] = Resource{Endpoints: map[string]Endpoint{
		"search": {Method: "POST", Path: "/tools/search", JSONRPCMethod: "search_meetings"},
	}}
	base.JSONRPC.Envelope = "stream"
	require.ErrorContains(t, base.Validate(), "x-jsonrpc.envelope must be one of: plain, mcp")
}
