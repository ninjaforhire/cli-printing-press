package crowdsniff

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "plain path", path: "/v1/users", want: "/v1/users"},
		{name: "colon param", path: "/users/:id", want: "/users/{id}"},
		{name: "bracket param", path: "/users/{user_id}", want: "/users/{id}"},
		{name: "angle param", path: "/users/<userId>", want: "/users/{id}"},
		{name: "dollar param", path: "/users/$id", want: "/users/{id}"},
		{name: "numeric ID", path: "/users/123", want: "/users/{id}"},
		{name: "UUID", path: "/users/550e8400-e29b-41d4-a716-446655440000", want: "/users/{uuid}"},
		{name: "long hash", path: "/blobs/abc123def456abc123def456abc123def456", want: "/blobs/{hash}"},
		{name: "strip query string", path: "/users?page=1&limit=10", want: "/users"},
		{name: "mixed params", path: "/v1/users/:id/posts/{post_id}", want: "/v1/users/{id}/posts/{id}"},
		{name: "extra slashes", path: "///v1///users///", want: "/v1/users"},
		{name: "double slashes", path: "/v1//users", want: "/v1/users"},
		{name: "empty path", path: "", want: "/"},
		{name: "root", path: "/", want: "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, NormalizePath(tt.path))
		})
	}
}

func TestAggregate(t *testing.T) {
	t.Parallel()

	t.Run("two sources same endpoint", func(t *testing.T) {
		t.Parallel()
		results := []SourceResult{
			{
				Endpoints: []DiscoveredEndpoint{
					{Method: "GET", Path: "/v1/users", SourceTier: TierOfficialSDK, SourceName: "@notionhq/client"},
				},
				BaseURLCandidates: []string{"https://api.notion.com"},
			},
			{
				Endpoints: []DiscoveredEndpoint{
					{Method: "GET", Path: "/v1/users", SourceTier: TierCodeSearch, SourceName: "github-code-search"},
				},
				BaseURLCandidates: []string{"https://api.notion.com"},
			},
		}

		aggregated, baseURLs := Aggregate(results)

		assert.Len(t, aggregated, 1)
		assert.Equal(t, TierOfficialSDK, aggregated[0].SourceTier)
		assert.Equal(t, 2, aggregated[0].SourceCount)
		assert.Equal(t, []string{"https://api.notion.com"}, baseURLs)
	})

	t.Run("different endpoints from different sources", func(t *testing.T) {
		t.Parallel()
		results := []SourceResult{
			{
				Endpoints: []DiscoveredEndpoint{
					{Method: "GET", Path: "/v1/users", SourceTier: TierOfficialSDK, SourceName: "sdk"},
				},
			},
			{
				Endpoints: []DiscoveredEndpoint{
					{Method: "POST", Path: "/v1/users", SourceTier: TierCodeSearch, SourceName: "github"},
				},
			},
		}

		aggregated, _ := Aggregate(results)
		assert.Len(t, aggregated, 2)
	})

	t.Run("parameter syntax normalization deduplicates", func(t *testing.T) {
		t.Parallel()
		results := []SourceResult{
			{
				Endpoints: []DiscoveredEndpoint{
					{Method: "GET", Path: "/users/:id", SourceTier: TierCommunitySDK, SourceName: "npm-sdk"},
				},
			},
			{
				Endpoints: []DiscoveredEndpoint{
					{Method: "GET", Path: "/users/{user_id}", SourceTier: TierCodeSearch, SourceName: "github"},
				},
			},
			{
				Endpoints: []DiscoveredEndpoint{
					{Method: "GET", Path: "/users/<id>", SourceTier: TierCodeSearch, SourceName: "github2"},
				},
			},
		}

		aggregated, _ := Aggregate(results)
		assert.Len(t, aggregated, 1)
		assert.Equal(t, "/users/{id}", aggregated[0].Path)
		assert.Equal(t, TierCommunitySDK, aggregated[0].SourceTier)
		assert.Equal(t, 3, aggregated[0].SourceCount)
	})

	t.Run("single source", func(t *testing.T) {
		t.Parallel()
		results := []SourceResult{
			{
				Endpoints: []DiscoveredEndpoint{
					{Method: "GET", Path: "/users", SourceTier: TierCodeSearch, SourceName: "github"},
					{Method: "POST", Path: "/users", SourceTier: TierCodeSearch, SourceName: "github"},
				},
			},
		}

		aggregated, _ := Aggregate(results)
		assert.Len(t, aggregated, 2)
		for _, ep := range aggregated {
			assert.Equal(t, 1, ep.SourceCount)
		}
	})

	t.Run("same source same endpoint deduplicates to count 1", func(t *testing.T) {
		t.Parallel()
		results := []SourceResult{
			{
				Endpoints: []DiscoveredEndpoint{
					{Method: "GET", Path: "/users", SourceTier: TierCodeSearch, SourceName: "github"},
					{Method: "GET", Path: "/users", SourceTier: TierCodeSearch, SourceName: "github"},
				},
			},
		}

		aggregated, _ := Aggregate(results)
		assert.Len(t, aggregated, 1)
		assert.Equal(t, 1, aggregated[0].SourceCount)
	})

	t.Run("empty results", func(t *testing.T) {
		t.Parallel()
		aggregated, baseURLs := Aggregate(nil)
		assert.Empty(t, aggregated)
		assert.Empty(t, baseURLs)
	})

	t.Run("base URL candidates deduplicated", func(t *testing.T) {
		t.Parallel()
		results := []SourceResult{
			{BaseURLCandidates: []string{"https://api.example.com", "https://api.example.com"}},
			{BaseURLCandidates: []string{"https://api.example.com", "https://other.example.com"}},
		}

		_, baseURLs := Aggregate(results)
		assert.Equal(t, []string{"https://api.example.com", "https://other.example.com"}, baseURLs)
	})
}
