package websniff

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateFixtures(t *testing.T) {
	t.Parallel()

	sampleCapture, err := ParseEnriched(filepath.Join("..", "..", "testdata", "sniff", "sample-enriched.json"))
	require.NoError(t, err)
	require.NotNil(t, sampleCapture)

	tests := []struct {
		name       string
		capture    *EnrichedCapture
		assertions func(t *testing.T, fixtures *FixtureSet)
	}{
		{
			name:    "post api endpoint with json body produces sanitized fixture",
			capture: sampleCapture,
			assertions: func(t *testing.T, fixtures *FixtureSet) {
				require.NotNil(t, fixtures)
				assert.Equal(t, "hn-algolia", fixtures.APIName)
				assert.Equal(t, "https://uj5wyc0l7x-dsn.algolia.net", fixtures.BaseURL)
				require.Len(t, fixtures.Fixtures, 1)

				fixture := fixtures.Fixtures[0]
				assert.Equal(t, "create_query", fixture.EndpointName)
				assert.Equal(t, "POST", fixture.Method)
				assert.Equal(t, "/1/indexes/Item_dev/query", fixture.Path)
				assert.Equal(t, []string{"hitsPerPage", "page", "query", "x-algolia-api-key", "x-algolia-application-id"}, fixture.ParamNames)
				assert.Equal(t, []string{"hitsPerPage", "page", "query"}, fixture.BodyFields)
			},
		},
		{
			name: "authenticated request only exposes auth presence",
			capture: &EnrichedCapture{
				TargetURL: "https://api.example.com",
				Entries: []EnrichedEntry{
					{
						Method:              "POST",
						URL:                 "https://api.example.com/v1/search",
						RequestBody:         `{"query":"secret","page":2}`,
						ResponseBody:        `{"items":[]}`,
						ResponseContentType: "application/json",
						RequestHeaders: map[string]string{
							"Content-Type":  "application/json",
							"Authorization": "Bearer super-secret-token",
							"Cookie":        "session=secret-cookie",
						},
					},
				},
			},
			assertions: func(t *testing.T, fixtures *FixtureSet) {
				require.NotNil(t, fixtures)
				require.Len(t, fixtures.Fixtures, 1)

				fixture := fixtures.Fixtures[0]
				assert.True(t, fixture.HasAuth)
				assert.NotContains(t, fixture.ParamNames, "Bearer")
				assert.NotContains(t, fixture.BodyFields, "super-secret-token")
				assert.NotContains(t, fixture.BodyFields, "session")
			},
		},
		{
			name: "noise-only capture produces empty fixtures",
			capture: &EnrichedCapture{
				TargetURL: "https://example.com",
				Entries: []EnrichedEntry{
					{
						Method:              "GET",
						URL:                 "https://cdn.example.com/styles.css",
						ResponseContentType: "text/css",
					},
					{
						Method:              "GET",
						URL:                 "https://www.google-analytics.com/collect?v=1",
						ResponseContentType: "image/gif",
					},
				},
			},
			assertions: func(t *testing.T, fixtures *FixtureSet) {
				require.NotNil(t, fixtures)
				assert.Empty(t, fixtures.Fixtures)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.assertions(t, GenerateFixtures(tt.capture))
		})
	}
}

func TestSanitizeForFixture(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		entry EnrichedEntry
		want  TestFixture
	}{
		{
			name: "get request uses query params",
			entry: EnrichedEntry{
				Method: "GET",
				URL:    "https://api.example.com/v1/search?q=kubernetes&page=2",
			},
			want: TestFixture{
				Method:     "GET",
				Path:       "/v1/search",
				ParamNames: []string{"page", "q"},
			},
		},
		{
			name: "empty request body has no body fields",
			entry: EnrichedEntry{
				Method: "POST",
				URL:    "https://api.example.com/v1/widgets",
				RequestHeaders: map[string]string{
					"Content-Type": "application/json",
				},
				RequestBody: "",
			},
			want: TestFixture{
				Method: "POST",
				Path:   "/v1/widgets",
			},
		},
		{
			name: "form encoded body uses form keys",
			entry: EnrichedEntry{
				Method: "POST",
				URL:    "https://api.example.com/session",
				RequestHeaders: map[string]string{
					"Content-Type": "application/x-www-form-urlencoded",
				},
				RequestBody: "client_id=abc&grant_type=client_credentials",
			},
			want: TestFixture{
				Method:     "POST",
				Path:       "/session",
				ParamNames: []string{"client_id", "grant_type"},
				BodyFields: []string{"client_id", "grant_type"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fixture := SanitizeForFixture(tt.entry)
			assert.Equal(t, tt.want.Method, fixture.Method)
			assert.Equal(t, tt.want.Path, fixture.Path)
			assert.Equal(t, tt.want.ParamNames, fixture.ParamNames)
			assert.Equal(t, tt.want.BodyFields, fixture.BodyFields)
		})
	}
}
