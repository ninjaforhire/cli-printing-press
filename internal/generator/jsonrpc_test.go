package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/spec"
	"github.com/stretchr/testify/require"
)

func TestGeneratedJSONRPCClientPattern(t *testing.T) {
	t.Parallel()

	for _, envelope := range []string{spec.JSONRPCEnvelopePlain, spec.JSONRPCEnvelopeMCP} {
		envelope := envelope
		t.Run(envelope, func(t *testing.T) {
			apiSpec := minimalSpec("jsonrpc-" + envelope)
			apiSpec.ClientPattern = spec.ClientPatternJSONRPC
			apiSpec.JSONRPC = spec.JSONRPCConfig{Envelope: envelope}
			apiSpec.Resources = map[string]spec.Resource{
				"meetings": {
					Description: "Meetings",
					Endpoints: map[string]spec.Endpoint{
						"search": {
							Method:        "POST",
							Path:          "/rpc",
							Description:   "Search meetings",
							JSONRPCMethod: "search_meetings",
							Meta:          map[string]string{"mcp:read-only": "true"},
							Body:          []spec.Param{{Name: "query", Type: "string"}},
						},
						"archive": {
							Method:        "POST",
							Path:          "/rpc",
							Description:   "Archive a meeting",
							JSONRPCMethod: "archive_meeting",
							Body:          []spec.Param{{Name: "meeting_id", Type: "string", Required: true}},
						},
					},
				},
			}

			outputDir := filepath.Join(t.TempDir(), apiSpec.Name+"-pp-cli")
			require.NoError(t, New(apiSpec, outputDir).Generate())

			commandPath := filepath.Join(outputDir, "internal", "cli", "meetings_search.go")
			commandSrc, err := os.ReadFile(commandPath)
			require.NoError(t, err)
			require.Contains(t, string(commandSrc), `path := "search_meetings"`)
			require.Contains(t, string(commandSrc), "c.PostQueryWithParams(")

			requireGeneratedCompiles(t, outputDir)
			writeGeneratedJSONRPCRuntimeTest(t, outputDir, apiSpec.Name, envelope)
			runGoCommandRequired(t, outputDir, "test", "./internal/client")
		})
	}
}

func writeGeneratedJSONRPCRuntimeTest(t *testing.T, outputDir, apiName, envelope string) {
	t.Helper()

	testSource := fmt.Sprintf(`package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"%s-pp-cli/internal/config"
)

type jsonRPCRequestCapture struct {
	Header http.Header
	Body   map[string]any
}

type jsonRPCRecorder struct {
	captures []jsonRPCRequestCapture
}

func (r *jsonRPCRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	defer req.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return nil, err
	}
	r.captures = append(r.captures, jsonRPCRequestCapture{Header: req.Header.Clone(), Body: body})
	method, _ := body["method"].(string)
	if method == "tools/call" {
		params, _ := body["params"].(map[string]any)
		method, _ = params["name"].(string)
	}
	payload := `+"`"+`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`+"`"+`
	if method == "fail" {
		payload = `+"`"+`{"jsonrpc":"2.0","id":4,"error":{"code":-32001,"message":"denied"}}`+"`"+`
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(payload)),
	}, nil
}

func TestGeneratedJSONRPCWireContract(t *testing.T) {
	recorder := &jsonRPCRecorder{}
	client := New(&config.Config{BaseURL: "http://example.test"}, time.Second, 0)
	client.NoCache = true
	client.HTTPClient = &http.Client{Transport: recorder}

	result, _, err := client.Post(context.Background(), "search_meetings", map[string]any{"query": "renewal"})
	if err != nil {
		t.Fatalf("plain request: %%v", err)
	}
	if string(result) != `+"`"+`{"ok":true}`+"`"+` {
		t.Fatalf("result = %%s", result)
	}
	_, _, err = client.Post(context.Background(), "empty_input", map[string]any{})
	if err != nil {
		t.Fatalf("empty request: %%v", err)
	}

	t.Setenv("PRINTING_PRESS_VERIFY", "1")
	t.Setenv("PRINTING_PRESS_VERIFY_LIVE_HTTP", "")
	_, _, err = client.PostQueryWithParams(context.Background(), "search_meetings", nil, map[string]any{})
	if err != nil {
		t.Fatalf("read-only request: %%v", err)
	}
	beforeMutation := len(recorder.captures)
	synthetic, status, err := client.Post(context.Background(), "archive_meeting", map[string]any{})
	if err != nil {
		t.Fatalf("mutation request: %%v", err)
	}
	if status != http.StatusOK || !strings.Contains(string(synthetic), "verify_short_circuit") {
		t.Fatalf("mutation must short-circuit, status=%%d body=%%s", status, synthetic)
	}
	if len(recorder.captures) != beforeMutation {
		t.Fatalf("mutating JSON-RPC call dialed under verify: got %%d calls, want %%d", len(recorder.captures), beforeMutation)
	}
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	_, _, err = client.Post(context.Background(), "fail", map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "-32001") || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("JSON-RPC error = %%v", err)
	}

	if len(recorder.captures) != 4 {
		t.Fatalf("capture count = %%d, want 4", len(recorder.captures))
	}
	for index, capture := range recorder.captures {
		if capture.Header.Get("Accept") != "application/json, text/event-stream" {
			t.Fatalf("request %%d Accept = %%q", index+1, capture.Header.Get("Accept"))
		}
		if capture.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("request %%d Content-Type = %%q", index+1, capture.Header.Get("Content-Type"))
		}
		if got := capture.Body["id"]; got != float64(index+1) {
			t.Fatalf("request %%d id = %%v", index+1, got)
		}
	}

	first := recorder.captures[0].Body
	second := recorder.captures[1].Body
	if %q == "mcp" {
		if first["method"] != "tools/call" {
			t.Fatalf("MCP method = %%v", first["method"])
		}
		params, _ := first["params"].(map[string]any)
		if params["name"] != "search_meetings" {
			t.Fatalf("MCP name = %%v", params["name"])
		}
		arguments, _ := params["arguments"].(map[string]any)
		if arguments["query"] != "renewal" {
			t.Fatalf("MCP arguments = %%v", arguments)
		}
		emptyParams, _ := second["params"].(map[string]any)
		emptyArguments, _ := emptyParams["arguments"].(map[string]any)
		if len(emptyArguments) != 0 {
			t.Fatalf("MCP empty arguments = %%v", emptyArguments)
		}
	} else {
		if first["method"] != "search_meetings" {
			t.Fatalf("plain method = %%v", first["method"])
		}
		params, _ := first["params"].(map[string]any)
		if params["query"] != "renewal" {
			t.Fatalf("plain params = %%v", params)
		}
		emptyParams, _ := second["params"].(map[string]any)
		if len(emptyParams) != 0 {
			t.Fatalf("plain empty params = %%v", emptyParams)
		}
	}
}
`, apiName, envelope)

	testPath := filepath.Join(outputDir, "internal", "client", "jsonrpc_generated_test.go")
	require.NoError(t, os.WriteFile(testPath, []byte(testSource), 0o644))
}
