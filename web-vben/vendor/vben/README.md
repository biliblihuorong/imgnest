# Vben source workspace used by ImageNest

This directory vendors real runtime sources from the official
[vue-vben-admin repository](https://github.com/vbenjs/vue-vben-admin), version
**5.8.0**, commit **50f4ede309d4450c7dd417399cb8d5c02346d2d2**.
The complete upstream MIT notice is in [LICENSE](./LICENSE).

## Scope and provenance

The 24 packages listed in [UPSTREAM.json](./UPSTREAM.json) are the recursive
runtime dependency closure of `@vben/layouts`, `@vben/common-ui`,
`@vben/preferences`, `@vben/stores`, `@vben/styles`, and `@vben/icons`, plus
Vben's own Tailwind 4 theme/configuration package. These include the genuine
Vben shadcn/Reka UI primitives, shell, menu, tabs, dialogs, forms, preferences,
translations, stores, design tokens and icon components. They are not
ImageNest replacements or facade packages.

Only runtime source/assets, package manifests and relevant declarations are
included. Upstream apps, playground, documentation, demo data, tests,
changelogs, CI, release tooling and package-build configurations are omitted.
The tiny Tailwind `@reference` injection plugin is copied from upstream's
`internal/vite-config/src/plugins/tailwind-reference.ts`.

`UPSTREAM.json` records every copied source file's original SHA-256 and the
package paths, allowing comparison against the immutable upstream commit.

## Deliberate integration adaptations

- Package manifests are private source-only workspace packages. Publishing
  scripts and unused package-build/test tooling are removed. The normal
  `exports` conditions point to genuine upstream `src` entries, including
  their default condition, so no untracked upstream `dist` is required.
- Upstream `catalog:` references are resolved to exact versions from the
  upstream lockfile, except the existing ImageNest versions of Vue, Vue Router,
  Pinia, VueUse and test/build tools remain pinned. `@vue/shared` tracks the
  existing Vue 3.5.43; VueUse integrations use the existing core major 15.
- `@vben-core/design` explicitly depends on `@vben/tailwind-config`, which its
  CSS imports. Upstream relied on the monorepo's root dependency visibility.
- `internal/tailwind-config/src/theme.css` scans this application and the
  vendored runtime packages instead of the omitted upstream apps/docs/demos.
- `packages/@core/base/typings/vue-router.d.ts` refers to the source type
  exports rather than upstream generated declaration output.

- Eight obsolete `@ts-expect-error unused` comments are removed from the
  original form, modal, drawer and tab Vue SFCs. Vue 3.5.43/vue-tsc 3.3.12
  correctly typecheck those expressions; preserving the comments fails TS6.
- `common-ui/.../icon-picker/icons.ts` accurately models the pending-request
  cache as possibly absent and narrows a cached Promise before returning it.
- `utils/src/helpers/generate-menus.ts` narrows Vue Router 5 redirect unions:
  string targets remain unchanged, static object targets use `router.resolve`,
  and function redirects remain routed through the original menu path.
- The app TypeScript DOM compilation includes ES2023, matching upstream's
  `toSorted`/`toReversed` usage. JSX tooling is omitted because none of this
  runtime closure or the ImageNest application contains JSX/TSX.

`adaptedSourceFiles` in the manifest records both upstream and integrated
hashes for each deliberately patched source file; unlisted source files are
byte-for-byte upstream copies.

Application tests still use `src/**/*.test.ts`, and application lint excludes
this vendor snapshot. Imported source is checked by `vue-tsc`; it is not
replaced by ambient `any` module declarations. Build output remains `web/dist`
for the existing Go embed.

## Updating

Choose and review an explicit official upstream commit. Recompute the package
closure, copy runtime sources, record their original hashes, reapply only the
integration adaptations above, update notices/version documentation and the
pnpm lockfile, and run application tests, lint, typecheck and production build.
Do not pull in the upstream sample application or change ImageNest API/auth
contracts. From `web`, use `pnpm install --frozen-lockfile` in normal CI.
The existing `/go/pnpm-store` setting is for the Docker development volume;
other environments may supply `--store-dir` and `--state-dir` explicitly.

## ImageNest language lifecycle adapter

BasicLayout adds optional `refreshOnLocaleChange` (upstream behavior defaults true). ImageNest passes false because all business text reacts to the shared locale; refreshing the route would discard active forms, upload queues and one-time credentials. A mounted-shell regression test verifies both language directions preserve the component instance. No layout component is replaced.
