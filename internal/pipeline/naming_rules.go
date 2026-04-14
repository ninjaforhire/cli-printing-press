package pipeline

// NamingRule captures a banned name and the canonical alternative for
// agent-facing CLI surfaces (command verbs and flag names). Agents expect
// consistency: if one printed CLI uses `info` and another uses `get`,
// trained behavior on one hallucinates commands on the other.
//
// The rule catalog is intentionally small at the start. Retros add entries
// as real drift patterns surface.
//
// Categories:
//   - "verb": applies to the first token of a cobra `Use:` declaration
//   - "flag": applies to long-form flag names as registered (e.g. `--force`)
type NamingRule struct {
	Banned    string
	Preferred string
	Category  string
	Reason    string
}

// namingRules is the canonical set of banned → preferred naming pairs
// enforced by the dogfood naming-consistency check. Inspired by Cloudflare's
// schema-layer guardrails documented in their 2026-04-13 Wrangler CLI post:
// "It's always get, never info. Always --force, never --skip-confirmations."
var namingRules = []NamingRule{
	{
		Banned:    "info",
		Preferred: "get",
		Category:  "verb",
		Reason:    "agents expect `get` to retrieve a single resource; `info` collides with help-style surfaces",
	},
	{
		Banned:    "ls",
		Preferred: "list",
		Category:  "verb",
		Reason:    "unix shorthand `ls` is inconsistent with the rest of the command vocabulary; agents trained on `list` miss it",
	},
	{
		Banned:    "--skip-confirmations",
		Preferred: "--force",
		Category:  "flag",
		Reason:    "`--force` is the cross-CLI convention for bypassing confirmation prompts",
	},
	{
		Banned:    "--skip-confirmation",
		Preferred: "--force",
		Category:  "flag",
		Reason:    "`--force` is the cross-CLI convention for bypassing confirmation prompts",
	},
	// NOTE: `--yes` is NOT banned. It is a long-standing Unix convention
	// (apt, dnf, etc.) and printed CLIs use it as the canonical skip-prompt
	// flag today. Cloudflare's article bans only `--skip-confirmations`,
	// not `--yes`. If a retro shows agents getting confused between `--yes`
	// and `--force`, consolidate then — not speculatively here.
}
