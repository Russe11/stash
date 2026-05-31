---
name: optimizations
description: Performance / refactor improvements for the Stash NG server fork
updated: 2026-05-31
---

# Optimizations

Concrete performance wins or scoped refactor opportunities. Remove when done.

<!--
### YYYY-MM-DD — <short title>
<one paragraph: the win + file:line>
-->

### 2026-05-31 — Bound broad `find*` page sizes or split interactive queries from export scans
The deprecated `all*` resolvers are capped, but preferred `find*` queries still accept very large `per_page` values or `per_page=-1` for all results (`pkg/models/find_filter.go:96-118`; `pkg/sqlite/sql.go:33-47`). Add a generous hard cap for interactive GraphQL pages and a deliberate export/snapshot traversal path for manager use cases that really need whole-library scans. At minimum, log/measure `per_page=-1` and pages above the proposed cap before enforcing.
