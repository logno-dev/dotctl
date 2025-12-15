# Smart Template Merge Algorithm

## Overview

The smart merge algorithm intelligently propagates changes from base config files back to template files while preserving conditional blocks (`{{#if system}}`).

## Problem Statement

When users edit generated config files (e.g., `~/.zshrc`) instead of templates (e.g., `.zshrc.template`), we need to:
1. Detect what changed
2. Determine where changes belong in the template
3. Insert changes without breaking conditional blocks
4. Preserve system-specific sections

## Algorithm Components

### 1. Template Parsing (`parseTemplateSections`)

Breaks down a template file into sections:

```go
type TemplateSection struct {
    Type         string   // "conditional" or "common"
    System       string   // e.g., "macos", "linux" (empty for common)
    StartLine    int
    EndLine      int
    Content      []string
    MatchedLines []string
}
```

**Example Template:**
```bash
# Common header
export EDITOR=nvim

{{#if macos}}
export PATH="/opt/homebrew/bin:$PATH"
eval "$(brew shellenv)"
{{/if}}

{{#if linux}}
export PATH="/usr/local/bin:$PATH"
{{/if}}

# Common aliases
alias ll='ls -la'
```

**Parsed Sections:**
1. Common section (lines 1-2): `export EDITOR=nvim`
2. Conditional section for "macos" (lines 4-5): homebrew paths
3. Conditional section for "linux" (line 8): linux paths
4. Common section (line 11-12): aliases

### 2. Line-by-Line Diff (`computeLineDiff`)

Compares the base file with template-generated output:

```go
type LineDiff struct {
    Type            string // "added", "removed", "modified"
    BaseLineNum     int
    TemplateLineNum int
    BaseContent     string
    TemplateContent string
}
```

**Example:**
- User adds: `alias myalias='echo hello'`
- User modifies: `alias ll='ls -la'` → `alias ll='ls -laF'`
- User adds (on macOS): `export HOMEBREW_NO_ANALYTICS=1`

**Detected Diffs:**
1. `{Type: "added", BaseContent: "alias myalias='echo hello'"}`
2. `{Type: "modified", TemplateContent: "alias ll='ls -la'", BaseContent: "alias ll='ls -laF'"}`
3. `{Type: "added", BaseContent: "export HOMEBREW_NO_ANALYTICS=1"}`

### 3. Change Placement Analysis (`analyzeChangePlacement`)

Determines which template section each change should go into:

```go
type ChangePlacement struct {
    LineDiff
    RecommendedSection *TemplateSection
    Confidence         string // "high", "medium", "low"
}
```

**Logic:**
- For **modified** lines: Find which section contains that line in template output
- For **added** lines: 
  - If the line appears near other lines from a conditional block → suggest that block
  - If the line contains system-specific keywords (e.g., "brew", "pacman") → suggest appropriate block
  - Otherwise → suggest common section
- For **removed** lines: Track but don't auto-remove (user should manually remove from template if desired)

**Example Analysis:**
```
1. ADD: alias myalias='echo hello'
   → Common section (default for aliases)
   → Confidence: medium

2. MODIFY: alias ll='ls -la' -> alias ll='ls -laF'
   → Common section (found in output line 12)
   → Confidence: high

3. ADD: export HOMEBREW_NO_ANALYTICS=1
   → Conditional block for 'macos' (contains "HOMEBREW")
   → Confidence: high
```

### 4. Auto-Merge (`autoMergeTemplate`)

Applies changes to the template:

**For Added Lines:**
```go
if placement.Type == "added" {
    insertPos := placement.RecommendedSection.EndLine
    
    // For conditional sections, insert before {{/if}}
    if placement.RecommendedSection.Type == "conditional" {
        insertPos--
    }
    
    templateLines = insertLine(templateLines, insertPos, placement.BaseContent)
}
```

**For Modified Lines:**
```go
if placement.Type == "modified" {
    // Find and replace the line
    for i, line := range templateLines {
        if strings.TrimSpace(line) == strings.TrimSpace(placement.TemplateContent) {
            templateLines[i] = placement.BaseContent
            break
        }
    }
}
```

## Example Walkthrough

### Input

**Template (`.zshrc.template`):**
```bash
{{#if macos}}
export PATH="/opt/homebrew/bin:$PATH"
{{/if}}

{{#if linux}}
export PATH="/usr/local/bin:$PATH"
{{/if}}

alias ll='ls -la'
```

**User Edits (`.zshrc` on macOS):**
```bash
export PATH="/opt/homebrew/bin:$PATH"
export HOMEBREW_NO_ANALYTICS=1

alias ll='ls -laF'
alias myalias='echo hello'
```

### Processing Steps

**Step 1: Parse Template**
```
Section 1: conditional for "macos"
  - export PATH="/opt/homebrew/bin:$PATH"

Section 2: conditional for "linux"
  - export PATH="/usr/local/bin:$PATH"

Section 3: common
  - alias ll='ls -la'
```

**Step 2: Generate Template Output (for macOS)**
```bash
export PATH="/opt/homebrew/bin:$PATH"
alias ll='ls -la'
```

**Step 3: Compute Diff**
```
1. ADDED: export HOMEBREW_NO_ANALYTICS=1 (after line 1)
2. MODIFIED: alias ll='ls -la' → alias ll='ls -laF' (line 2)
3. ADDED: alias myalias='echo hello' (after line 2)
```

**Step 4: Analyze Placement**
```
1. export HOMEBREW_NO_ANALYTICS=1
   → macos conditional (keyword match + proximity)
   → Insert before {{/if}} of macos block

2. alias ll='ls -laF'
   → common section (found at line 2 of output = common section)
   → Replace in place

3. alias myalias='echo hello'
   → common section (default for aliases)
   → Insert at end of common section
```

**Step 5: Auto-Merge**
```bash
{{#if macos}}
export PATH="/opt/homebrew/bin:$PATH"
export HOMEBREW_NO_ANALYTICS=1          # ← INSERTED
{{/if}}

{{#if linux}}
export PATH="/usr/local/bin:$PATH"
{{/if}}

alias ll='ls -laF'                      # ← MODIFIED
alias myalias='echo hello'              # ← INSERTED
```

### Output

Template is updated with all changes in the correct sections, conditionals preserved!

## Confidence Levels

**High Confidence:**
- Modified lines (we know exactly where they are)
- Added lines near modified lines in same section
- Added lines with system-specific keywords matching a conditional

**Medium Confidence:**
- Added lines with generic content
- Added lines with no clear section affinity

**Low Confidence:**
- Added lines when template structure is ambiguous
- Added lines when multiple sections could apply

## Limitations and Future Improvements

### Current Limitations
1. **Simple line-by-line diff**: Doesn't handle complex rearrangements
2. **Keyword matching is basic**: Could miss subtle system-specific content
3. **No handling for removed lines**: Users must manually remove from template
4. **Assumes single-system editing**: If base file has changes for multiple systems, may misplace

### Potential Improvements
1. **Myers diff algorithm**: More sophisticated change detection
2. **Semantic analysis**: Understand bash/shell syntax to better classify changes
3. **Multi-system detection**: Detect changes that should go in multiple conditional blocks
4. **Learning system**: Remember user's manual placement choices
5. **Conflict resolution**: Handle cases where changes conflict with template updates

## User Workflow

```bash
# User edits base file
vim ~/.zshrc

# Smart merge detects and analyzes
dotctl merge-resolve
# Choice: 3 (Smart merge)

# System analyzes:
# - 3 changes detected
# - Placement suggestions shown
# - Confidence levels displayed

# User chooses:
# 1. Auto-merge (apply suggestions)
# 2. Manual edit (review in editor)

# If auto-merge:
# - Changes inserted automatically
# - Template structure preserved
# - File staged for commit

# Result:
# - Template updated correctly
# - All conditionals intact
# - Base file regenerated
```

## Why This Approach Works

1. **Section-aware**: Understands template structure, not just text
2. **Context-sensitive**: Places changes based on content and location
3. **Non-destructive**: Never removes conditional blocks automatically
4. **Transparent**: Shows user exactly what will happen
5. **Fallback options**: Manual edit always available if auto-merge isn't confident

This algorithm bridges the gap between convenience (editing base files) and maintainability (keeping templates as source of truth).
