package catalog

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var namePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

var validCategories = map[string]struct{}{
	"developer-tools":      {},
	"monitoring":           {},
	"cloud":                {},
	"project-management":   {},
	"productivity":         {},
	"social-and-messaging": {},
	"sales-and-crm":       {},
	"marketing":            {},
	"payments":             {},
	"auth":                 {},
	"commerce":             {},
	"ai":                   {},
	"devices":              {},
	"other":                {},
	"example":              {},
}

var validSpecFormats = map[string]struct{}{
	"yaml": {},
	"json": {},
}

var validTiers = map[string]struct{}{
	"official":  {},
	"community": {},
}

type KnownAlt struct {
	Name     string `yaml:"name"`
	URL      string `yaml:"url"`
	Language string `yaml:"language"`
}

type Entry struct {
	Name              string     `yaml:"name"`
	DisplayName       string     `yaml:"display_name"`
	Description       string     `yaml:"description"`
	Category          string     `yaml:"category"`
	SpecURL           string     `yaml:"spec_url"`
	SpecFormat        string     `yaml:"spec_format"`
	OpenAPIVersion    string     `yaml:"openapi_version"`
	Tier              string     `yaml:"tier"`
	VerifiedDate      string     `yaml:"verified_date"`
	Homepage          string     `yaml:"homepage"`
	Notes             string     `yaml:"notes"`
	KnownAlternatives []KnownAlt `yaml:"known_alternatives,omitempty"`
	SandboxEndpoint   string     `yaml:"sandbox_endpoint,omitempty"`
}

func ParseEntry(data []byte) (*Entry, error) {
	var e Entry
	if err := yaml.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("parsing yaml: %w", err)
	}
	if err := e.Validate(); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}
	return &e, nil
}

func ParseDir(dir string) ([]Entry, error) {
	return ParseFS(os.DirFS(dir))
}

// ParseFS reads all YAML catalog entries from an fs.FS (e.g., an embedded filesystem).
// It mirrors ParseDir but operates on the fs.FS interface instead of the OS filesystem.
func ParseFS(fsys fs.FS) ([]Entry, error) {
	dirEntries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("reading fs: %w", err)
	}

	sort.Slice(dirEntries, func(i, j int) bool {
		return dirEntries[i].Name() < dirEntries[j].Name()
	})

	entries := make([]Entry, 0, len(dirEntries))
	for _, de := range dirEntries {
		if de.IsDir() {
			continue
		}
		if filepath.Ext(de.Name()) != ".yaml" {
			continue
		}

		data, err := fs.ReadFile(fsys, de.Name())
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", de.Name(), err)
		}

		entry, err := ParseEntry(data)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", de.Name(), err)
		}
		entries = append(entries, *entry)
	}

	return entries, nil
}

// LookupFS finds a single catalog entry by name from an fs.FS.
// Returns an error if the entry is not found.
func LookupFS(fsys fs.FS, name string) (*Entry, error) {
	data, err := fs.ReadFile(fsys, name+".yaml")
	if err != nil {
		return nil, fmt.Errorf("catalog entry %q not found", name)
	}
	return ParseEntry(data)
}

func (e *Entry) Validate() error {
	if e.Name == "" {
		return fmt.Errorf("name is required")
	}
	if !namePattern.MatchString(e.Name) {
		return fmt.Errorf("name must be lowercase kebab-case (letters, digits, hyphens only)")
	}
	if e.DisplayName == "" {
		return fmt.Errorf("display_name is required")
	}
	if e.Description == "" {
		return fmt.Errorf("description is required")
	}
	if e.Category == "" {
		return fmt.Errorf("category is required")
	}
	if _, ok := validCategories[e.Category]; !ok {
		return fmt.Errorf("category must be one of: developer-tools, monitoring, cloud, project-management, productivity, social-and-messaging, sales-and-crm, marketing, payments, auth, commerce, ai, devices, other")
	}
	if e.SpecURL == "" {
		return fmt.Errorf("spec_url is required")
	}
	if !strings.HasPrefix(e.SpecURL, "https://") {
		return fmt.Errorf(`spec_url must start with "https://"`)
	}
	if e.SpecFormat == "" {
		return fmt.Errorf("spec_format is required")
	}
	if _, ok := validSpecFormats[e.SpecFormat]; !ok {
		return fmt.Errorf("spec_format must be one of: yaml, json")
	}
	if e.Tier == "" {
		return fmt.Errorf("tier is required")
	}
	if _, ok := validTiers[e.Tier]; !ok {
		return fmt.Errorf("tier must be one of: official, community")
	}

	return nil
}
