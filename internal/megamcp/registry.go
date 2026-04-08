package megamcp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// DefaultBaseURL is the default GitHub raw content URL for the public library repo.
const DefaultBaseURL = "https://raw.githubusercontent.com/mvanhorn/printing-press-library/main"

// FetchRegistry fetches registry.json from baseURL and parses it into a Registry.
// baseURL is injectable for testing (use httptest.NewServer).
func FetchRegistry(baseURL string) (*Registry, error) {
	registryURL := baseURL + "/registry.json"

	resp, err := http.Get(registryURL)
	if err != nil {
		return nil, fmt.Errorf("fetching registry: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching registry: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		return nil, fmt.Errorf("reading registry body: %w", err)
	}

	var registry Registry
	if err := json.Unmarshal(body, &registry); err != nil {
		return nil, fmt.Errorf("parsing registry JSON: %w", err)
	}

	return &registry, nil
}
