package crowdsniff

import (
	"regexp"
	"strings"
)

var (
	// Concrete value patterns (from websniff/classifier.go).
	uuidSegmentPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	hashSegmentPattern = regexp.MustCompile(`(?i)^[0-9a-f]{32,}$`)
	numericPattern     = regexp.MustCompile(`^\d+$`)

	// Parameter syntax patterns — matches :id, {id}, {user_id}, <id>, <userId>, $id.
	colonParamPattern  = regexp.MustCompile(`^:[a-zA-Z_][a-zA-Z0-9_]*$`)
	bracketParamPattern = regexp.MustCompile(`^\{[a-zA-Z_][a-zA-Z0-9_]*\}$`)
	angleParamPattern  = regexp.MustCompile(`^<[a-zA-Z_][a-zA-Z0-9_]*>$`)
	dollarParamPattern = regexp.MustCompile(`^\$[a-zA-Z_][a-zA-Z0-9_]*$`)
)

// tierRank returns a numeric rank for tier ordering (higher = more authoritative).
func tierRank(tier string) int {
	switch tier {
	case TierOfficialSDK:
		return 4
	case TierCommunitySDK:
		return 3
	case TierCodeSearch:
		return 2
	case TierPostman:
		return 1
	default:
		return 0
	}
}

// Aggregate deduplicates endpoints from multiple sources, computing the highest
// source tier and distinct source count for each unique method+path combination.
// It also collects all base URL candidates from the sources.
func Aggregate(results []SourceResult) ([]AggregatedEndpoint, []string) {
	type endpointKey struct {
		method string
		path   string
	}

	type accumulator struct {
		bestTier string
		sources  map[string]struct{} // distinct source names
	}

	index := make(map[endpointKey]*accumulator)
	var order []endpointKey
	var baseURLs []string

	for _, result := range results {
		baseURLs = append(baseURLs, result.BaseURLCandidates...)

		for _, ep := range result.Endpoints {
			method := strings.ToUpper(strings.TrimSpace(ep.Method))
			path := NormalizePath(ep.Path)
			key := endpointKey{method: method, path: path}

			acc, exists := index[key]
			if !exists {
				acc = &accumulator{
					sources: make(map[string]struct{}),
				}
				index[key] = acc
				order = append(order, key)
			}

			if tierRank(ep.SourceTier) > tierRank(acc.bestTier) {
				acc.bestTier = ep.SourceTier
			}
			acc.sources[ep.SourceName] = struct{}{}
		}
	}

	aggregated := make([]AggregatedEndpoint, 0, len(order))
	for _, key := range order {
		acc := index[key]
		aggregated = append(aggregated, AggregatedEndpoint{
			Method:      key.method,
			Path:        key.path,
			SourceTier:  acc.bestTier,
			SourceCount: len(acc.sources),
		})
	}

	return aggregated, deduplicateStrings(baseURLs)
}

// NormalizePath unifies parameter syntax and replaces concrete values with placeholders.
// Step 1: Convert :id, {user_id}, <id>, $id → {id}
// Step 2: Replace UUIDs, numeric IDs, long hashes → {id}/{uuid}/{hash}
func NormalizePath(path string) string {
	// Strip query string if present.
	if idx := strings.Index(path, "?"); idx >= 0 {
		path = path[:idx]
	}

	parts := strings.Split(path, "/")
	segments := make([]string, 0, len(parts))
	for _, segment := range parts {
		if segment == "" {
			continue
		}

		// Step 1: Unify parameter syntax → {id}
		switch {
		case colonParamPattern.MatchString(segment):
			segment = "{id}"
		case bracketParamPattern.MatchString(segment):
			segment = "{id}"
		case angleParamPattern.MatchString(segment):
			segment = "{id}"
		case dollarParamPattern.MatchString(segment):
			segment = "{id}"
		// Step 2: Replace concrete values
		case numericPattern.MatchString(segment):
			segment = "{id}"
		case uuidSegmentPattern.MatchString(segment):
			segment = "{uuid}"
		case hashSegmentPattern.MatchString(segment):
			segment = "{hash}"
		}
		segments = append(segments, segment)
	}

	normalized := "/" + strings.Join(segments, "/")
	if normalized == "" {
		return "/"
	}
	return normalized
}

func deduplicateStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
