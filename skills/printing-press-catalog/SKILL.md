---
name: printing-press-catalog
description: Browse and install pre-built Go CLIs for popular APIs from the catalog
version: 0.4.0
allowed-tools:
  - Bash
  - Read
  - Write
  - Glob
  - Grep
  - WebFetch
  - AskUserQuestion
---

# /printing-press-catalog

Browse and install pre-built Go CLIs for popular APIs.

## Quick Start

```
/printing-press-catalog
/printing-press-catalog install stripe
/printing-press-catalog search auth
```

## Prerequisites

- Go 1.21+ installed
- Running from inside the cli-printing-press repo (or a worktree of it)

## Setup

Before any other commands, resolve and cd to the repo root. This ensures all relative paths work even from subdirectories or worktrees:

<!-- PRESS_SETUP_CONTRACT_START -->
```bash
REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

PRESS_BASE="$(basename "$REPO_ROOT" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9_-]/-/g; s/^-+//; s/-+$//')"
if [ -z "$PRESS_BASE" ]; then
  PRESS_BASE="workspace"
fi
PRESS_SCOPE="$PRESS_BASE-$(printf '%s' "$REPO_ROOT" | shasum -a 256 | cut -c1-8)"
PRESS_HOME="$HOME/printing-press"
PRESS_RUNSTATE="$PRESS_HOME/.runstate/$PRESS_SCOPE"
PRESS_LIBRARY="$PRESS_HOME/library"

mkdir -p "$PRESS_RUNSTATE" "$PRESS_LIBRARY"
```
<!-- PRESS_SETUP_CONTRACT_END -->

If `git rev-parse` fails, you are not inside a cli-printing-press checkout. Stop and tell the user.

Generated CLIs are published to `$PRESS_LIBRARY/`, not to the repo.

## Workflows

### List Catalog (no arguments)

When invoked with no arguments, list all available CLIs grouped by category.

1. Read all YAML files in catalog/ using Glob + Read
2. Parse each file's name, display_name, description, category fields
3. Group by category and display:

```
Available CLIs (12 entries):

Payments:
  stripe - Payment processing and financial infrastructure API
  square - Payment processing and commerce API

Auth:
  stytch - Authentication and user management API

Email:
  sendgrid - Email delivery and marketing API

Communication:
  discord - Chat and community platform API
  twilio - Communication APIs for SMS, voice, and messaging
  front - Customer communication platform API

Developer Tools:
  github - Software development platform API
  digitalocean - Cloud infrastructure and developer platform API

Project Management:
  asana - Work management and project tracking API

CRM:
  hubspot - CRM contacts API

Example:
  petstore - Canonical OpenAPI example

Install any CLI: /printing-press-catalog install <name>
```

### Install (install <name>)

When invoked with `install <name>`:

1. Read catalog/<name>.yaml
2. If file doesn't exist, show error: "No catalog entry for '<name>'. Run /printing-press-catalog to see available CLIs."
3. Extract spec_url from the catalog entry
4. Show preview: "Installing <display_name> CLI from <spec_url>"
5. Build the printing-press binary if needed:
   ```bash
   go build -o ./printing-press ./cmd/printing-press
   ```
6. Download the spec and generate:
   ```bash
   curl -sL -o /tmp/catalog-spec-$$.yaml "<spec_url>"
   OUTPUT_BASE="$PRESS_LIBRARY/<name>-pp-cli"
   OUTPUT_DIR="$OUTPUT_BASE"
   i=2
   while [ -e "$OUTPUT_DIR" ]; do
     OUTPUT_DIR="${OUTPUT_BASE}-$i"
     i=$((i + 1))
   done
   ./printing-press generate \
     --spec /tmp/catalog-spec-$$.yaml \
     --output "$OUTPUT_DIR" \
     --validate
   ```
7. If all quality gates pass, present the result:
   ```
   Generated <name>-pp-cli with X resources.

   Try it:
     cd "$OUTPUT_DIR"
     go install ./cmd/<name>-pp-cli
     <name>-pp-cli --help
     <name>-pp-cli doctor
   ```
8. If gates fail, show the error and suggest: "Try /printing-press <display_name> API for a custom generation with retry support."

### Search (search <query>)

When invoked with `search <query>`:

1. Read all YAML files in catalog/
2. Search name, display_name, description, and category for the query (case-insensitive)
3. Display matching entries

## Limitations

- Large API specs (Stripe, Discord, GitHub) take 30-60 seconds to generate and compile
- Generated CLIs are truncated to 50 resources / 20 endpoints per resource
- Catalog entries point to external URLs that may change
