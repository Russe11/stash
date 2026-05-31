---
name: fixes
description: Non-defect fixes — tech debt, contract hardening, code-quality adjustments
updated: 2026-05-31
---

# Fixes

Code that works but should be improved (security hardening, contract tightening, dead code, misleading comment) — things that "need fixing" but aren't behavioral bugs. Remove an entry when done.

<!--
### YYYY-MM-DD — <short title>
<one paragraph: what to change and why + file:line>
-->

### 2026-05-31 — Add a timeout to external scraper scripts
`pkg/scraper/script.go:260-270` starts scraper scripts and has a TODO for timeout handling. Before surfacing more client-side identify/scraper flows, run scripts with a configurable context timeout and report timeout failures distinctly from JSON decode/schema errors.
