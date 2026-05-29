---
name: optimizations
description: Performance / refactor improvements for the Stash NG server fork
updated: 2026-05-29
---

# Optimizations

Concrete performance wins or scoped refactor opportunities. Remove when done.

<!--
### YYYY-MM-DD — <short title>
<one paragraph: the win + file:line>
-->

### 2026-05-29 — Folder recursive counts are N+1 (no dataloader batching)
`Folder.scene_count(depth)`/`image_count(depth)`/`total_size` each resolve via a separate `WITH RECURSIVE` CTE per folder per field (`internal/api/resolver_model_folder.go:19-52` → `pkg/sqlite/folder.go:492-600`). A folder list view showing counts issues up to 3N recursive queries. Add a dataloader (`loaders.From`) that batches `Count*InTree`/`TotalSizeInTree` by folder id, or a single windowed CTE keyed by a set of root folder ids. NG-only fields.
