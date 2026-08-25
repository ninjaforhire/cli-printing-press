package openapi

import (
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseJSONRPCExtensions(t *testing.T) {
	t.Parallel()

	parsed, err := Parse([]byte(`
openapi: 3.0.3
x-jsonrpc:
  version: "2.0"
  envelope: mcp
info:
  title: Meeting Tools
  version: "1.0"
servers:
  - url: https://api.example.com/mcp
paths:
  /tools/search:
    post:
      operationId: searchMeetings
      x-jsonrpc-method: search_meetings
      responses:
        "200":
          description: ok
`))
	require.NoError(t, err)
	assert.Equal(t, spec.JSONRPCVersion, parsed.JSONRPC.EffectiveVersion())
	assert.Equal(t, spec.JSONRPCEnvelopeMCP, parsed.JSONRPC.EffectiveEnvelope())

	var endpoint spec.Endpoint
	for _, resource := range parsed.Resources {
		for _, candidate := range resource.Endpoints {
			endpoint = candidate
		}
	}
	assert.Equal(t, "search_meetings", endpoint.JSONRPCMethod)
}

func TestParseJSONRPCMethodRejectsNonString(t *testing.T) {
	t.Parallel()

	_, err := Parse([]byte(`
openapi: 3.0.3
info:
  title: Meeting Tools
  version: "1.0"
servers:
  - url: https://api.example.com/mcp
paths:
  /tools/search:
    post:
      x-jsonrpc-method: 42
      responses:
        "200":
          description: ok
`))
	require.ErrorContains(t, err, "x-jsonrpc-method must be a string")
}
