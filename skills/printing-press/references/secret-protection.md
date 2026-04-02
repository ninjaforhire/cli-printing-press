# Secret & PII Protection — Implementation Details

Read this file during Phase 5.5 (Archive Manuscripts) and before any publish step.
The cardinal rules in SKILL.md apply at all times. This file has the implementation.

## Exact-value scan before archiving

The skill knows the API key if the user provided one. Before archiving manuscripts,
scan all artifacts for the exact key value. This has zero false positives — it checks
for the specific string, not guessed patterns.

Use `grep -F` (fixed string) and `awk` for replacement — NOT bare `grep`/`sed` —
because API keys often contain regex metacharacters (`+`, `/`, `.`, `=`) that would
cause `grep` to match wrong text and `sed` to corrupt files.

```bash
if [ -n "$API_KEY_VALUE" ]; then
  LEAK_FOUND=false
  for dir in "$RESEARCH_DIR" "$PROOFS_DIR" "$DISCOVERY_DIR"; do
    if [ -d "$dir" ] && grep -rF "$API_KEY_VALUE" "$dir" 2>/dev/null; then
      LEAK_FOUND=true
    fi
  done
  if [ "$LEAK_FOUND" = true ]; then
    echo "BLOCKING: API key value found in manuscript artifacts. Auto-redacting."
    REDACT_TO="\$${API_KEY_ENV_VAR:-API_KEY}"
    for dir in "$RESEARCH_DIR" "$PROOFS_DIR" "$DISCOVERY_DIR"; do
      [ -d "$dir" ] || continue
      find "$dir" -type f -print0 | while IFS= read -r -d '' f; do
        if grep -qF "$API_KEY_VALUE" "$f" 2>/dev/null; then
          # Use python for truly literal replacement — awk's gsub and perl's
          # s/// both interpret regex metacharacters (+, ., /) in the key,
          # which breaks on JWT tokens and base64-encoded secrets.
          REDACT_OLD="$API_KEY_VALUE" REDACT_NEW="$REDACT_TO" python3 -c "
import sys, os
old, new, path = os.environ['REDACT_OLD'], os.environ['REDACT_NEW'], sys.argv[1]
with open(path) as f: content = f.read()
with open(path, 'w') as f: f.write(content.replace(old, new))
" "$f"
        fi
      done
    done
    echo "Auto-redacted. Verify before proceeding."
  fi
fi
```

## Strip auth from HAR captures before archiving

Credentials can appear in three locations within HAR files:
- **Headers:** `Authorization: Bearer <token>`, `Cookie: session=...`
- **Query strings:** `?key=<value>`, `?api_key=<value>`, `?access_token=<value>`
- **Cookies:** session tokens, auth cookies

The archive step must strip all three, plus response bodies (for size):

```bash
jq 'del(.log.entries[].response.content.text) |
    # Remove auth headers
    (.log.entries[].request.headers) |= [.[] |
      select(.name | test("^(Authorization|Cookie|Set-Cookie|X-API-Key|X-Auth-Token)$"; "i") | not)
    ] |
    # Redact auth-like query string params
    (.log.entries[].request.queryString) |= [.[] |
      if (.name | test("^(key|api_key|apikey|token|secret|access_token|password)$"; "i"))
      then .value = "<REDACTED>"
      else . end
    ] |
    # Remove cookies entirely (they often contain session tokens)
    (.log.entries[].request.cookies) |= []
    ' "$har" > "${har}.stripped" 2>/dev/null && mv "${har}.stripped" "$har"
```

## API key handling during the run

When the user provides an API key (Phase 0 API Key Gate or inline):
- Store it only in a shell variable, never in a file
- Pass it to commands via environment variable, not via flags visible in process lists
- In dry-run output, the key may appear in query params — this is expected for
  debugging but must NOT be captured in proof artifacts
- When writing live smoke results to proofs, write the test outcomes (PASS/FAIL)
  but never the request URLs that contain the key in query params

## Session state cleanup

Session state files (`session-state.json`) contain browser cookies and auth tokens.
The Phase 5.5 archive block removes them with `rm -f "$DISCOVERY_DIR/session-state.json"`.
This removal is mandatory and must happen BEFORE the `cp -r "$DISCOVERY_DIR"` command.
If the order is reversed, cookies leak into manuscripts.
