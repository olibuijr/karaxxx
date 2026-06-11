# Karaxxx — Category Overhaul + Full Audit (in progress 2026-06-11)

## User directives (verbatim intent)

1. **Filter bug**: "only two categories work as the filter" — root-cause and fix the category/sidebar filtering.
2. **10-agent review**: improvements, optimizations, features, gaps, security fixes across the codebase. *(running: workflow wf_3ccfc125-3b1)*
3. **Finish the backlog** the review produces — implement, don't just report.
4. **Unify tags → categories**: "only use categories — tags should be referenced as categories and treated as such." One concept. Tags are folded into categories everywhere (storage, API, UI, FTS).
5. **Complete extraction**: "get all data from all sources" — DrTuber + EPorner currently store `uncategorized`; every provider must extract real categories.
6. **Index everything**: "index every single part of the extracted data from the source pages" — all extracted metadata fields indexed (FTS + structured), nothing dropped.

## Workflow constraints (standing)

- **Worktrees always** for implementation; clean up after merge.
- **Deploy only via `./deploy.sh`** (memory: always-deploy-via-deploy-sh).
- Build must stay green; verify live via agent-browser before declaring done.

## Known evidence (pre-review)

- `videos.categories` is comma-joined TEXT; browse filter uses `categories LIKE '%cat%'` → substring false-positives + can't exact-match; `idx_categories` unusable by LIKE.
- Source category coverage: xnxx 62412 (real), eporner 7413 (uncategorized), tnaflix 2680 (real), xhamster 1417 (real), drtuber 8243 (uncategorized).
- Browse handler (~main.go:1633) SELECTs 12 cols incl. `source` but `rows.Scan` binds ~11 dests — column/scan mismatch suspected.
- `tags` column exists separately from `categories` — to be unified per directive #4.

## Plan

Review (10 agents) → verified prioritized backlog → implement in worktree (category-as-tag unification migration, per-source category extraction, full-field indexing, filter rewrite to a normalized category join or exact-match, plus P0/P1 fixes) → build → deploy via deploy.sh → verify live → clean up worktree.
