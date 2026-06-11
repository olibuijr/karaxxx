---
project: Karaxxx
task: Login screen liquid-glass modernization + fluid navigation + optimistic loading
slug: karaxxx-liquid-glass-login
effort: E3
phase: complete
progress: 37/37
mode: build
started: 2026-06-11T00:00:00Z
updated: 2026-06-11T16:45:00Z
---

# Karaxxx — Liquid Glass Login & Fluid Navigation ISA

## Problem

The login gate (`web/src/pages/Setup.tsx`) was a flat dark card layout — solid `bg-card` surfaces, no depth, no motion. Page navigation was hard-cut, a cached session showed a blocking "Loading..." screen, and login submit gave no immediate feedback.

## Vision

Opening the app feels like looking through dark liquid glass: a slow, living refraction field in brand reds/oranges behind a frosted glass auth card. Signing in melts the login screen into the app in one continuous view transition. Returning users never see a loading screen. Navigation glides instead of cutting.

## Out of Scope

No Go/backend changes. No auth protocol changes. No redesign of authenticated pages beyond navigation transitions. No WebGPU renderer. No new analytics or third-party assets.

## Principles

- Perceived performance is real performance: optimism + continuity beat raw ms.
- Motion must be skippable: `prefers-reduced-motion` users get a calm static scene.
- The GPU effect is decoration: form usability never depends on WebGL availability.
- Layered dark grays, never pitch black — glass needs luminance behind it to read as glass.

## Constraints

- Stack stays React 19 + Vite + Tailwind v4 + react-router v7; bun for installs.
- Only new runtime dep: `three` (+ `@types/three` dev).
- SPA served from `web/dist` by the Go binary on :8799 — `bun run build` must stay green.

## Goal

The Setup login screen renders a three.js liquid-glass shader background behind a frosted glassmorphism auth card; router navigation and the login→app swap use the View Transitions API with reduced-motion fallback; cached sessions restore optimistically with zero loading screen; build passes and the live page verifies via browser probe.

## Criteria

### Glass UI — login screen
- [x] ISC-1: Setup.tsx auth card uses `backdrop-blur` + translucent background (Grep)
- [x] ISC-2: Card has glass border treatment — white/15 ring + specular top edge (Grep)
- [x] ISC-3: Sign in and Use invite mode toggle still present and functional (Read)
- [x] ISC-4: Inputs restyled to translucent glass (Grep)
- [x] ISC-5: Error message rendering path preserved (live: "invalid credentials" renders on 401)
- [x] ISC-6: Submit button preserved with busy/pending state "Unlocking…" (Read)
- [x] ISC-7: Brand/value-prop left section and privacy details preserved (Read + screenshot)
- [x] ISC-8: AuthDialog matching glass surface treatment (Grep)

### Liquid glass WebGL background
- [x] ISC-9: `three` in web/package.json dependencies (Read)
- [x] ISC-10: `LiquidGlassBackground.tsx` exists exporting a React component (Read)
- [x] ISC-11: three.js ShaderMaterial fullscreen pass (Grep)
- [x] ISC-12: Fragment shader uses fbm + domain warping for liquid refraction (Grep)
- [x] ISC-13: Brand palette — red/orange caustics on dark gray-violet base (Grep + screenshot)
- [x] ISC-14: Renderer/geometry/material disposed on unmount (Grep)
- [x] ISC-15: Animation pauses on hidden tab (Grep visibilitychange)
- [x] ISC-16: prefers-reduced-motion → single static frame (Grep)
- [x] ISC-17: WebGL-unavailable fails soft to CSS radial-gradient (Grep)
- [x] ISC-18: Device pixel ratio clamped ≤2 (Grep)
- [x] ISC-19: Canvas pointer-events-none, layered behind card (Grep)
- [x] ISC-20: Setup.tsx mounts it via React.lazy + Suspense — three.js in separate chunk (network log)

### View Transitions API
- [x] ISC-21: `::view-transition` CSS rules in index.css (Grep)
- [x] ISC-22: Header nav Links + navigate() calls use viewTransition (Grep, 9 sites)
- [x] ISC-23: Sidebar Links use viewTransition (Grep, 4 sites)
- [x] ISC-24: VideoCard Link uses viewTransition (Grep)
- [x] ISC-25: Login/logout swaps wrapped in guarded startViewTransition (Grep)
- [x] ISC-26: Reduced-motion disables view-transition animation (Grep)
- [x] ISC-27: startViewTransition typeof-guarded for unsupporting browsers (Grep)

### Optimistic loading
- [x] ISC-28: auth.tsx caches user JSON in localStorage, restores synchronously on boot (live: kxxx_user present after login)
- [x] ISC-28.1: logout AND failed revalidation both evict the cached user (Grep, 2 sites)
- [x] ISC-29: Background /api/auth/me revalidation evicts invalid sessions (Read)
- [x] ISC-30: Blocking loading screen only when no cached user (live: reload shows zero "Loading" text)
- [x] ISC-31: Login submit prefetches /api/browse in parallel (network log: browse GET alongside login POST)
- [x] ISC-32: Submit button flips to pending synchronously (Read)

### Build, verify, anti-criteria
- [x] ISC-33: `bun run build` exits 0 (Bash)
- [x] ISC-34: Live login page screenshot shows glass card + liquid shader (agent-browser, 3 design iterations)
- [x] ISC-35: Anti: zero console errors on load AND during failed + successful login (agent-browser)
- [x] ISC-36: Antecedent: authenticated routes render post-login (live: favorites + media loading after real sign-in)

## Test Strategy

| isc | type | check | threshold | tool |
|-----|------|-------|-----------|------|
| 1–8 | code | class/symbol presence | exact | Grep/Read |
| 9–20 | code | dep + lifecycle implementation | exact | Read/Grep |
| 21–27 | code | CSS rules + router props + guards | exact | Grep |
| 28–32 | code+live | auth flow logic + network log + localStorage probe | exact | Read + agent-browser |
| 33 | build | tsc + vite exit code | 0 | Bash |
| 34–35 | live | screenshot + console | visual pass, 0 errors | agent-browser |
| 36 | live | authenticated app loads | network 200s | agent-browser |

## Features

| name | satisfies | depends_on | parallelizable |
|------|-----------|------------|----------------|
| LiquidGlassBackground shader component | ISC-9..19 | three installed | done (Forge) |
| Setup.tsx glass redesign + lazy mount | ISC-1..7, 20 | — | done |
| AuthDialog glass touch-up | ISC-8 | — | done |
| View transitions (CSS + router + auth swap) | ISC-21..27 | — | done |
| Optimistic auth restore + prefetch | ISC-28..32 | — | done |
| Build + live verification | ISC-33..36 | all above | done |

## Decisions

- 2026-06-11: WebGL via three.js ShaderMaterial fullscreen pass, not WebGPU — flaky Linux support and payload for zero visible gain. "wgpu" intent honored as GPU-shader effect.
- 2026-06-11: Delegation floor show-your-math — only Forge delegated (isolated new shader file); a second delegate would collide on tightly-coupled files.
- 2026-06-11: Optimistic auth = cached-user instant restore + background revalidation, NOT fake-authenticated rendering — security boundary stays server-side.
- 2026-06-11: three.js isolated via React.lazy → separate 510KB chunk; authenticated users never download it; main bundle stayed ~528KB.
- 2026-06-11: refined: principal feedback "too black, not dark gray" → shader base lifted #0d0d13→#191922, caustic intensity ~3x, frequency halved for liquid (not grain) feel; card flipped from bg-black/35 (darker than bg) to bg-white/[0.06] (lighter than bg).
- 2026-06-11: refined: hero logo line-height 0.82 caused glyph+shadow overflow onto tagline at hero scale → line-height 1 + margins for hero variant only.
- 2026-06-11: THREE.Clock deprecated in r184 → replaced with performance.now() wrapper.
- 2026-06-11: Verification via agent-browser per principal's explicit instruction (overrides default Interceptor rule for this run).
- 2026-06-11: A parallel session committed 281a2db deleting in-progress untracked files (ISA.md, LiquidGlassBackground.tsx) as "stray" mid-run; restored from git history. Coordinate sessions on this repo.

## Changelog

- conjectured: a near-black scene with subtle caustics would read as premium liquid glass.
  refuted_by: principal review + screenshots — everything read as pitch black; card was darker than backdrop; fbm read as grain, not liquid.
  learned: glassmorphism requires the glass surface to be LIGHTER than its backdrop; liquid-glass shaders need low-frequency, high-warp, visibly luminous flow — "subtle" below a threshold is just invisible.
  criterion_now: ISC-13 (palette verified by screenshot), ISC-1/2 (lighter-than-backdrop glass card).

## Verification

- ISC-33: Bash — `tsc -b && vite build` → "✓ built in 1.08s", exit 0.
- ISC-20/31: agent-browser network — `GET /assets/LiquidGlassBackground-BgYojjaW.js 200` (separate chunk), `GET /api/browse 200` parallel to `POST /api/auth/login 401`.
- ISC-34: screenshots /tmp/karaxxx-login-v3.png, /tmp/karaxxx-final.png — glass card, liquid flow, logo clear of tagline.
- ISC-35: agent-browser `errors` empty on fresh load, failed login, and real login.
- ISC-5: live DOM — "invalid credentials" rendered after 401.
- ISC-28/30/36: real sign-in (olibuijr) → app loaded favorites/media; localStorage kxxx_user = {"id":1,"username":"olibuijr"}; reload shows no "Loading" text.
- ISC-1..32 code probes: batch rg PASS (session transcript).
