# KaraXXX Feature Plan

## Priority Tiers

| Tier | Epics | Rationale |
|------|-------|-----------|
| **P0 — Critical** | Watch History, Infrastructure | Existential (DB maintenance) + highest user impact |
| **P1 — High** | Content Discovery, User Engagement 1 (Watch Later, Playlists, Ratings) | Core feature gaps, high consensus |
| **P2 — Medium** | UI/UX Polish, Smart Recommendations | Quality-of-life, personalization |
| **P3 — Nice-to-have** | User Engagement 2 (Profile, Notifications), PWA, Adaptive Quality | Caps completion |

## Epic Index

| # | Epic | Tier | Subtasks | Files |
|---|------|------|----------|-------|
| EPIC-1 | Watch History & Continue Watching | P0 | 8 | `epics/EPIC-1-watch-history.md` |
| EPIC-2 | Infrastructure Hardening | P0 | 7 | `epics/EPIC-2-infra.md` |
| EPIC-3 | Content Discovery | P1 | 6 | `epics/EPIC-3-discovery.md` |
| EPIC-4 | User Engagement Core | P1 | 5 | `epics/EPIC-4-engagement.md` |
| EPIC-5 | UI/UX Polish | P2 | 7 | `epics/EPIC-5-uiux.md` |
| EPIC-6 | Smart Recommendations | P2 | 4 | `epics/EPIC-6-recommendations.md` |

## Dependency Order

```
EPIC-2 (Infra — DB maintenance, security first)
   ↓
EPIC-1 (Watch History — foundational for EPIC-4 and EPIC-6)
   ↓
EPIC-3 (Content Discovery — standalone features)
   + 
EPIC-4 (User Engagement — depends on EPIC-1 for watch history)
   ↓
EPIC-5 (UI/UX Polish — wraps around everything)
   +
EPIC-6 (Recommendations — depends on EPIC-1 + EPIC-4 for data)
```
