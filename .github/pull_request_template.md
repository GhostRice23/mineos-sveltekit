<!--
  Base this PR on `vibing` (staging), not `master`. See CONTRIBUTING.md / AGENTS.md.
-->

## What & why



## Checklist

- [ ] Based on **`vibing`** (not `master`)
- [ ] `.NET` tests pass — `dotnet test apps/MineOS.Tests/MineOS.Tests.csproj`
- [ ] `apps/web` checks pass if the frontend changed — `cd apps/web && npm run check && npm run test:unit`
- [ ] `tools/mineos-cli` tests pass if the CLI changed — `cd tools/mineos-cli && go test ./... -race`
- [ ] Stays within the Clean Architecture layer rules (`AGENTS.md`) — no outward/framework deps in Domain or Application
- [ ] No change to auth/access behavior — or, if there is, it's called out explicitly below

## Auth / access impact

<!-- None, or describe exactly what changes about who-can-do-what. -->
None.
