package generator

import (
	"testing"

	"github.com/mvanhorn/cli-printing-press/v2/internal/spec"
	"github.com/stretchr/testify/assert"
)

func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"camelCase", "camel_case"},
		{"kebab-case", "kebab_case"},
		{"snake_case", "snake_case"},
		{"PascalCase", "pascal_case"},
		{"movie_id", "movie_id"},
		// Dot-notation params (TMDb, Elasticsearch style)
		{"primary_release_date.gte", "primary_release_date_gte"},
		{"vote_average.gte", "vote_average_gte"},
		{"vote_average.lte", "vote_average_lte"},
		{"vote_count.gte", "vote_count_gte"},
		{"field.nested.deep", "field_nested_deep"},
		// Combined dots and hyphens
		{"with.dots-and-hyphens", "with_dots_and_hyphens"},
		// No transformation needed
		{"simple", "simple"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, toSnakeCase(tt.input))
		})
	}
}

func TestCollectTextFieldNames(t *testing.T) {
	// Fields like tag/label/category/metadata should be picked up for FTS5
	// alongside the core text fields. Motivated by the ESPN retro where
	// "notes" (event tags) were unsearchable until manually added.
	mkResource := func(paramNames ...string) spec.Resource {
		params := make([]spec.Param, 0, len(paramNames))
		for _, n := range paramNames {
			params = append(params, spec.Param{Name: n, Type: "string"})
		}
		return spec.Resource{
			Endpoints: map[string]spec.Endpoint{
				"get": {Params: params},
			},
		}
	}

	tests := []struct {
		name     string
		params   []string
		wantIncl []string
		wantExcl []string
	}{
		{
			name:     "picks up core text fields",
			params:   []string{"title", "description", "body"},
			wantIncl: []string{"title", "description", "body"},
		},
		{
			name:     "picks up tag-family fields",
			params:   []string{"name", "tag", "tags", "label", "labels"},
			wantIncl: []string{"name", "tag", "tags", "label", "labels"},
		},
		{
			name:     "picks up category and metadata fields",
			params:   []string{"title", "category", "categories", "metadata"},
			wantIncl: []string{"title", "category", "categories", "metadata"},
		},
		{
			name:     "picks up notes and note",
			params:   []string{"note", "notes"},
			wantIncl: []string{"note", "notes"},
		},
		{
			name:     "ignores non-text fields",
			params:   []string{"id", "created_at", "price"},
			wantExcl: []string{"id", "created_at", "price"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collectTextFieldNames(mkResource(tt.params...))
			for _, want := range tt.wantIncl {
				assert.Contains(t, got, want)
			}
			for _, exc := range tt.wantExcl {
				assert.NotContains(t, got, exc)
			}
		})
	}
}
