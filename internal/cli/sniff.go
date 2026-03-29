package cli

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/mvanhorn/cli-printing-press/internal/websniff"
	"github.com/spf13/cobra"
)

func newSniffCmd() *cobra.Command {
	var harPath string
	var outputPath string
	var name string
	var blocklist string
	var authFrom string

	cmd := &cobra.Command{
		Use:   "sniff",
		Short: "Analyze captured web traffic to discover API endpoints and generate a spec",
		RunE: func(cmd *cobra.Command, args []string) error {
			websniff.SetAdditionalBlocklist(splitCSV(blocklist))

			capture, err := websniff.LoadCapture(harPath)
			if err != nil {
				return fmt.Errorf("loading capture: %w", err)
			}

			if authFrom != "" {
				authCapture, err := websniff.ParseEnriched(authFrom)
				if err != nil {
					return fmt.Errorf("reading auth capture: %w", err)
				}
				if err := validateAuthDomainBinding(authCapture, capture); err != nil {
					return err
				}
				capture.Auth = authCapture.Auth
			}

			apiSpec, err := websniff.AnalyzeCapture(capture)
			if err != nil {
				return fmt.Errorf("analyzing capture: %w", err)
			}

			if name != "" {
				apiSpec.Name = name
				apiSpec.Config.Path = fmt.Sprintf("~/.config/%s-pp-cli/config.toml", name)
			}

			if outputPath == "" {
				outputPath = websniff.DefaultCachePath(apiSpec.Name)
			}

			if err := websniff.WriteSpec(apiSpec, outputPath); err != nil {
				return fmt.Errorf("writing spec: %w", err)
			}

			endpoints := 0
			for _, resource := range apiSpec.Resources {
				endpoints += len(resource.Endpoints)
			}

			fmt.Printf("Spec written to %s (%d endpoints across %d resources)\n", outputPath, endpoints, len(apiSpec.Resources))
			fmt.Printf("Run 'printing-press generate --spec %s' to build the CLI\n", outputPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&harPath, "har", "", "Path to HAR or enriched capture file")
	cmd.Flags().StringVar(&outputPath, "output", "", "Output path for generated spec YAML")
	cmd.Flags().StringVar(&name, "name", "", "Override the auto-detected API name")
	cmd.Flags().StringVar(&blocklist, "blocklist", "", "Comma-separated additional domains to filter")
	cmd.Flags().StringVar(&authFrom, "auth-from", "", "Path to an enriched capture file to import auth from")
	_ = cmd.MarkFlagRequired("har")

	return cmd
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}

	return out
}

func validateAuthDomainBinding(authCapture *websniff.EnrichedCapture, targetCapture *websniff.EnrichedCapture) error {
	if authCapture == nil || authCapture.Auth == nil || strings.TrimSpace(authCapture.Auth.BoundDomain) == "" {
		return nil
	}

	targetDomain := captureDomain(targetCapture)
	boundDomain := normalizeDomain(authCapture.Auth.BoundDomain)
	if targetDomain == "" || boundDomain == "" {
		return nil
	}
	if targetDomain == boundDomain || strings.HasSuffix(targetDomain, "."+boundDomain) {
		return nil
	}

	return fmt.Errorf("auth captured for %s cannot be used with %s (domain mismatch)", authCapture.Auth.BoundDomain, targetDomain)
}

func captureDomain(capture *websniff.EnrichedCapture) string {
	if capture == nil {
		return ""
	}

	if capture.TargetURL != "" {
		parsed, err := url.Parse(capture.TargetURL)
		if err == nil && parsed.Hostname() != "" {
			return normalizeDomain(parsed.Hostname())
		}
	}

	baseURL := commonCaptureBaseURL(capture)
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}

	return normalizeDomain(parsed.Hostname())
}

func commonCaptureBaseURL(capture *websniff.EnrichedCapture) string {
	counts := make(map[string]int)
	best := ""
	bestCount := 0

	for _, entry := range capture.Entries {
		parsed, err := url.Parse(entry.URL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			continue
		}

		base := parsed.Scheme + "://" + parsed.Host
		counts[base]++
		if counts[base] > bestCount {
			best = base
			bestCount = counts[base]
		}
	}

	return best
}

func normalizeDomain(domain string) string {
	return strings.ToLower(strings.TrimSpace(domain))
}
