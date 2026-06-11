---
project: Karaxxx
task: Login screen liquid-glass modernization + fluid navigation + optimistic loading
slug: karaxxx-liquid-glass-login
effort: E3
phase: execute
progress: 0/37
mode: build
started: 2026-06-11T00:00:00Z
updated: 2026-06-11T00:00:00Z
---

# Karaxxx — Liquid Glass Login & Fluid Navigation ISA

## Problem

The login gate (`web/src/pages/Setup.tsx`) is a competent but flat dark card layout — solid `bg-card` surfaces, no depth, no motion. Page navigation inside the SPA is hard-cut (no transition continuity), the first paint after a cached session shows a blocking "Loading..." text screen, and login submit gives no immediate spatial feedback. The UI reads dated next to the cinematic brand treatment already present in the 3D logo.

## Vision

Opening the app feels like looking through dark liquid glass: a slow, living refraction field in brand reds/oranges breathing behind a frosted glass auth card with a specular edge. Signing in melts the login screen into the app shell in one continuous view transition. Returning users never see a loading screen — the app is just *there*. Navigation between pages glides instead of cutting. Euphoric surprise: "this feels like a native Apple-grade product, not a self-hosted side project."

## Out of Scope

No Go/backend changes. No auth protocol changes (token + /api/auth/me revalidation stays). No redesign of the authenticated app pages beyond adding view transitions to navigation. No WebGPU renderer (three.js WebGL shader is the implementation; WebGPU adds payload and instability for zero visible gain here). No new analytics or third-party assets.

## Principles

- Perceived performance is real performance: optimism + continuity beat raw ms.
- Motion must be skippable: `prefers-reduced-motion` users get a calm static scene.
- The GPU effect is decoration: form usability never depends on WebGL availability.

## Constraints

- Stack stays React 19 + Vite + Tailwind v4 + react-router v7; bun for installs.
- Only new runtime dep allowed: `three` (+ `@types/three` dev).
- SPA is served embedded from `web/dist` by the Go binary on :8799 — `bun run build` must stay green.
- Existing auth API contract unchanged.

## Goal

The Setup login screen renders a three.js liquid-glass shader background behind a frosted glassmorphism auth card; router navigation and the login→app swap use the View Transitions API with reduced-motion fallback; cached sessions restore optimistically with zero loading screen; `bun run build` passes and the live page verifies via browser probe.

## Criteria

### Glass UI — login screen
- [ ] ISC-1: Setup.tsx auth card uses `backdrop-blur` + translucent background (Grep)
- [ ] ISC-2: Card has glass border treatment — white/10 ring + inner top highlight gradient (Grep/Read)
- [ ] ISC-3: Sign in and Use invite mode toggle still present and functional (Read)
- [ ] ISC-4: Inputs restyled to translucent glass (`bg-white/5` or equivalent) (Grep)
- [ ] ISC-5: Error message rendering path preserved (Grep `error`)
- [ ] ISC-6: Submit button preserved with busy/pending state (Read)
- [ ] ISC-7: Brand/value-prop left section and privacy details preserved (Read)
- [ ] ISC-8: AuthDialog gets matching glass surface treatment (Grep backdrop-blur in AuthDialog.tsx)

### Liquid glass WebGL background
- [ ] ISC-9: `three` present in web/package.json dependencies (Read)
- [ ] ISC-10: `LiquidGlassBackground.tsx` exists exporting a React component (Read)
- [ ] ISC-11: Uses three.js ShaderMaterial/RawShaderMaterial fullscreen pass (Grep)
- [ ] ISC-12: Fragment shader contains noise/fbm-based liquid refraction (Grep `fbm|noise`)
- [ ] ISC-13: Brand palette in shader — red/orange caustics on near-black (Grep color constants)
- [ ] ISC-14: Renderer + geometry + material disposed on unmount (Grep `dispose`)
- [ ] ISC-15: Animation pauses when tab hidden (Grep `visibilitychange|hidden`)
- [ ] ISC-16: `prefers-reduced-motion` → static frame, no RAF loop (Grep `prefers-reduced-motion`)
- [ ] ISC-17: WebGL-unavailable path fails soft (try/catch → null/CSS fallback) (Read)
- [ ] ISC-18: Device pixel ratio clamped ≤2 (Grep `Math.min` + pixelRatio)
- [ ] ISC-19: Canvas is `pointer-events-none` and absolutely layered behind card (Grep)
- [ ] ISC-20: Setup.tsx mounts LiquidGlassBackground (Grep import)

### View Transitions API
- [ ] ISC-21: `::view-transition` CSS rules added to index.css (Grep)
- [ ] ISC-22: Header nav Links use `viewTransition` prop (Grep)
- [ ] ISC-23: Sidebar Links use `viewTransition` prop (Grep)
- [ ] ISC-24: VideoCard Link uses `viewTransition` prop (Grep)
- [ ] ISC-25: Login success → app swap wrapped in guarded `document.startViewTransition` (Grep)
- [ ] ISC-26: Reduced-motion media query disables view-transition animation (Grep)
- [ ] ISC-27: `startViewTransition` guarded for unsupporting browsers (Grep typeof/optional check)

### Optimistic loading
- [ ] ISC-28: auth.tsx caches user JSON in localStorage and restores it synchronously on boot (Read)
- [ ] ISC-28.1: logout AND failed revalidation both evict the cached user from localStorage (Read)
- [ ] ISC-29: Background `/api/auth/me` revalidation still runs and evicts invalid sessions (Read)
- [ ] ISC-30: Blocking "Loading..." screen only shown when no cached user exists (Read App.tsx)
- [ ] ISC-31: Login submit prefetches initial browse data in parallel with auth round-trip (Grep prefetch)
- [ ] ISC-32: Submit button flips to pending state synchronously on click (Read)

### Build, verify, anti-criteria
- [ ] ISC-33: `bun run build` (tsc -b && vite build) exits 0 (Bash)
- [ ] ISC-34: Live login page screenshot shows glass card + shader background (Interceptor)
- [ ] ISC-35: Anti: no console errors on login page load (Interceptor console)
- [ ] ISC-36: Antecedent: authenticated routes still compile and render — build includes all pages unchanged in function (Bash build + Read)

## Test Strategy

| isc | type | check | threshold | tool |
|-----|------|-------|-----------|------|
| 1–8 | code | class/symbol presence + structure | exact | Grep/Read |
| 9–20 | code | dep + component implementation details | exact | Read/Grep |
| 21–27 | code | CSS rules + router props + guards | exact | Grep |
| 28–32 | code | auth flow logic | exact | Read |
| 33 | build | tsc + vite exit code | 0 | Bash |
| 34–35 | live | screenshot + console log | visual pass, 0 errors | Interceptor |
| 36 | build | dist contains app bundle | build green | Bash |

## Features

| name | satisfies | depends_on | parallelizable |
|------|-----------|------------|----------------|
| LiquidGlassBackground shader component | ISC-9..19 | three installed | yes (Forge) |
| Setup.tsx glass redesign + mount | ISC-1..7, 20 | — | yes (primary) |
| AuthDialog glass touch-up | ISC-8 | — | yes (primary) |
| View transitions (CSS + router + auth swap) | ISC-21..27 | — | yes (primary) |
| Optimistic auth restore + prefetch | ISC-28..32 | — | yes (primary) |
| Build + live verification | ISC-33..36 | all above | no |

## Decisions

- 2026-06-11: WebGL via three.js ShaderMaterial fullscreen pass, not WebGPU — WebGPU renderer adds larger payload and flaky Linux support for identical visual outcome at this fidelity. "wgpu" intent honored as GPU-shader effect.
- 2026-06-11: Delegation floor show-your-math — only Forge delegated (shader component, isolated new file). A second delegate would collide on the same tightly-coupled files (Setup/auth/css); Interceptor skill covers the verification surface instead.
- 2026-06-11: ISA written directly via Write tool (ISA skill CLI tools are v6.2.x-deferred per doctrine); structure follows E3 completeness gate.
- 2026-06-11: Optimistic auth = cached-user instant restore + background revalidation, NOT fake-authenticated rendering — security boundary stays server-side.
- 2026-06-11: ISA.md vanished from repo root mid-run (CHANGELOG.md/VERSION also show modifications — likely an external release script or git clean ran concurrently). Rewritten from context; watch for recurrence.

## Changelog

## Verification
