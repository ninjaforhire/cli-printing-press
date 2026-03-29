package websniff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func LoadCapture(path string) (*EnrichedCapture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	if bytes.Contains(data, []byte(`"log"`)) {
		var har HAR
		if err := json.Unmarshal(data, &har); err != nil {
			return nil, fmt.Errorf("parsing har json: %w", err)
		}

		capture := &EnrichedCapture{
			Entries: make([]EnrichedEntry, 0, len(har.Log.Entries)),
		}
		for _, entry := range har.Log.Entries {
			capture.Entries = append(capture.Entries, convertHAREntry(entry))
		}
		if len(har.Log.Entries) > 0 {
			capture.TargetURL = har.Log.Entries[0].Request.URL
		}

		return capture, nil
	}

	if bytes.Contains(data, []byte(`"target_url"`)) {
		var capture EnrichedCapture
		if err := json.Unmarshal(data, &capture); err != nil {
			return nil, fmt.Errorf("parsing enriched json: %w", err)
		}

		return &capture, nil
	}

	return nil, fmt.Errorf("unknown capture format")
}

func WriteEnrichedCapture(capture *EnrichedCapture, outputPath string) error {
	if capture == nil {
		return fmt.Errorf("capture is required")
	}

	data, err := json.MarshalIndent(capture, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling enriched json: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("opening output file: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("writing enriched json: %w", err)
	}

	return nil
}
