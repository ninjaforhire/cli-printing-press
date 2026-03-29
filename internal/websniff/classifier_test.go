package websniff

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyEntries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		entries          []EnrichedEntry
		wantAPIURLs      []string
		wantNoiseURLs    []string
		wantClassByURL   map[string]string
		wantIsNoiseByURL map[string]bool
	}{
		{
			name: "json api and analytics tracker are separated",
			entries: []EnrichedEntry{
				{
					Method:              "GET",
					URL:                 "https://example.com/api/users",
					ResponseContentType: "application/json; charset=utf-8",
					ResponseBody:        `{"users":[{"id":1}]}`,
				},
				{
					Method:              "GET",
					URL:                 "https://www.google-analytics.com/g/collect?v=2",
					ResponseContentType: "text/html",
				},
			},
			wantAPIURLs:   []string{"https://example.com/api/users"},
			wantNoiseURLs: []string{"https://www.google-analytics.com/g/collect?v=2"},
			wantClassByURL: map[string]string{
				"https://example.com/api/users":                  "api",
				"https://www.google-analytics.com/g/collect?v=2": "noise",
			},
			wantIsNoiseByURL: map[string]bool{
				"https://example.com/api/users":                  false,
				"https://www.google-analytics.com/g/collect?v=2": true,
			},
		},
		{
			name: "google analytics is noise",
			entries: []EnrichedEntry{
				{
					Method:              "POST",
					URL:                 "https://google-analytics.com/j/collect",
					ResponseContentType: "application/json",
					ResponseBody:        `{}`,
				},
			},
			wantNoiseURLs: []string{"https://google-analytics.com/j/collect"},
			wantClassByURL: map[string]string{
				"https://google-analytics.com/j/collect": "noise",
			},
			wantIsNoiseByURL: map[string]bool{
				"https://google-analytics.com/j/collect": true,
			},
		},
		{
			name: "post form with json response is api",
			entries: []EnrichedEntry{
				{
					Method:              "POST",
					URL:                 "https://example.com/session",
					ResponseContentType: "application/json",
					ResponseBody:        `{"ok":true}`,
					RequestHeaders: map[string]string{
						"Content-Type": "application/x-www-form-urlencoded",
					},
				},
			},
			wantAPIURLs: []string{"https://example.com/session"},
			wantClassByURL: map[string]string{
				"https://example.com/session": "api",
			},
			wantIsNoiseByURL: map[string]bool{
				"https://example.com/session": false,
			},
		},
		{
			name: "all noise entries produce empty api list",
			entries: []EnrichedEntry{
				{
					Method:              "GET",
					URL:                 "https://cdn.example.com/styles.css",
					ResponseContentType: "text/css",
				},
				{
					Method:              "GET",
					URL:                 "https://cdn.example.com/logo.png",
					ResponseContentType: "image/png",
				},
			},
			wantNoiseURLs: []string{
				"https://cdn.example.com/styles.css",
				"https://cdn.example.com/logo.png",
			},
			wantClassByURL: map[string]string{
				"https://cdn.example.com/styles.css": "noise",
				"https://cdn.example.com/logo.png":   "noise",
			},
			wantIsNoiseByURL: map[string]bool{
				"https://cdn.example.com/styles.css": true,
				"https://cdn.example.com/logo.png":   true,
			},
		},
		{
			name: "youtube internal endpoint is api",
			entries: []EnrichedEntry{
				{
					Method:              "POST",
					URL:                 "https://www.youtube.com/youtubei/v1/player?prettyPrint=false",
					ResponseContentType: "application/json",
					ResponseBody:        `{"videoDetails":{"videoId":"abc123"}}`,
				},
			},
			wantAPIURLs: []string{"https://www.youtube.com/youtubei/v1/player?prettyPrint=false"},
			wantClassByURL: map[string]string{
				"https://www.youtube.com/youtubei/v1/player?prettyPrint=false": "api",
			},
			wantIsNoiseByURL: map[string]bool{
				"https://www.youtube.com/youtubei/v1/player?prettyPrint=false": false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			api, noise := ClassifyEntries(tt.entries)

			assert.Equal(t, emptyStrings(tt.wantAPIURLs), entryURLs(api))
			assert.Equal(t, emptyStrings(tt.wantNoiseURLs), entryURLs(noise))

			for _, entry := range append(api, noise...) {
				assert.Equal(t, tt.wantClassByURL[entry.URL], entry.Classification)
				assert.Equal(t, tt.wantIsNoiseByURL[entry.URL], entry.IsNoise)
			}
		})
	}
}

func TestDeduplicateEndpoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		entries             []EnrichedEntry
		wantMethods         []string
		wantNormalizedPaths []string
		wantGroupSizes      []int
	}{
		{
			name: "numeric ids normalize to id placeholder",
			entries: []EnrichedEntry{
				{Method: "GET", URL: "https://example.com/users/123?expand=true"},
				{Method: "GET", URL: "https://example.com/users/456"},
			},
			wantMethods:         []string{"GET"},
			wantNormalizedPaths: []string{"/users/{id}"},
			wantGroupSizes:      []int{2},
		},
		{
			name: "uuid segment normalizes to uuid placeholder",
			entries: []EnrichedEntry{
				{Method: "GET", URL: "https://example.com/orders/550e8400-e29b-41d4-a716-446655440000"},
				{Method: "GET", URL: "https://example.com/orders/123e4567-e89b-12d3-a456-426614174000?include=items"},
			},
			wantMethods:         []string{"GET"},
			wantNormalizedPaths: []string{"/orders/{uuid}"},
			wantGroupSizes:      []int{2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			groups := DeduplicateEndpoints(tt.entries)

			assert.Equal(t, tt.wantMethods, groupMethods(groups))
			assert.Equal(t, tt.wantNormalizedPaths, groupPaths(groups))
			assert.Equal(t, tt.wantGroupSizes, groupSizes(groups))
		})
	}
}

func entryURLs(entries []EnrichedEntry) []string {
	urls := make([]string, 0, len(entries))
	for _, entry := range entries {
		urls = append(urls, entry.URL)
	}

	return urls
}

func groupMethods(groups []EndpointGroup) []string {
	methods := make([]string, 0, len(groups))
	for _, group := range groups {
		methods = append(methods, group.Method)
	}

	return methods
}

func groupPaths(groups []EndpointGroup) []string {
	paths := make([]string, 0, len(groups))
	for _, group := range groups {
		paths = append(paths, group.NormalizedPath)
	}

	return paths
}

func groupSizes(groups []EndpointGroup) []int {
	sizes := make([]int, 0, len(groups))
	for _, group := range groups {
		sizes = append(sizes, len(group.Entries))
	}

	return sizes
}

func emptyStrings(values []string) []string {
	if values == nil {
		return []string{}
	}

	return values
}
