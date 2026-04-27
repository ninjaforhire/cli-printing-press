package reachability

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// RenderJSON writes the result to w as indented JSON.
func RenderJSON(w io.Writer, result *Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// RenderTable writes a human-readable table summarizing the probe rungs and
// final classification. Columns: Transport, Status, Elapsed, Evidence/Error.
func RenderTable(w io.Writer, result *Result) error {
	headers := []string{"Transport", "Status", "Elapsed", "Notes"}
	rows := make([][]string, 0, len(result.Probes))
	for _, pr := range result.Probes {
		status := "-"
		if pr.Status > 0 {
			status = fmt.Sprintf("%d", pr.Status)
		}
		notes := pr.Error
		if notes == "" {
			if len(pr.Evidence) == 0 {
				notes = "no protection signals"
			} else {
				notes = strings.Join(pr.Evidence, "; ")
			}
		}
		rows = append(rows, []string{
			string(pr.Transport),
			status,
			fmt.Sprintf("%dms", pr.ElapsedMS),
			notes,
		})
	}

	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	if err := writeRow(w, widths, headers); err != nil {
		return err
	}
	sep := make([]string, len(headers))
	for i := range sep {
		sep[i] = strings.Repeat("-", widths[i])
	}
	if err := writeRow(w, widths, sep); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writeRow(w, widths, row); err != nil {
			return err
		}
	}

	partial := ""
	if result.Partial {
		partial = " (partial — --probe-only restricted the ladder; ignore mode for skill decisions)"
	}
	if _, err := fmt.Fprintf(w, "\nMode: %s (confidence %.2f)%s\n", result.Mode, result.Confidence, partial); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Recommendation: runtime=%s, needs_browser_capture=%v, needs_clearance_cookie=%v\n",
		result.Recommendation.Runtime,
		result.Recommendation.NeedsBrowserCapture,
		result.Recommendation.NeedsClearanceCookie); err != nil {
		return err
	}
	if result.Recommendation.Rationale != "" {
		if _, err := fmt.Fprintf(w, "Rationale: %s\n", result.Recommendation.Rationale); err != nil {
			return err
		}
	}
	return nil
}

func writeRow(w io.Writer, widths []int, cells []string) error {
	var b strings.Builder
	for i, cell := range cells {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(cell)
		if i < len(cells)-1 {
			pad := widths[i] - len(cell)
			if pad > 0 {
				b.WriteString(strings.Repeat(" ", pad))
			}
		}
	}
	b.WriteString("\n")
	_, err := io.WriteString(w, b.String())
	return err
}
