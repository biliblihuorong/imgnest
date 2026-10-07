# Interface and themes

ImgNest's interface is built and embedded into the Go binary. The repository holds two frontends, and a build picks one of them. The newer frontend ships two layouts that can be switched at runtime.

## Two frontends

| | Classic frontend | New frontend |
| --- | --- | --- |
| Directory | `web/` | `web-vben/` |
| Build command | `make release` | `make release-vben` |
| Output | `bin/imgnest-legacy` | `bin/imgnest-vben` |
| Layouts | One | New and classic, switchable |
| Languages | Chinese | Chinese, English |

The default build uses the classic frontend. Each frontend has its own dependencies and build output, and a binary embeds only one of them.

Swapping the binary changes the interface only. The backend API, database and security settings stay the same.

## The two layouts of the new frontend

The **new layout** is the default:

- The sidebar floats over the page and can collapse to icons
- `Ctrl K` (`⌘ K` on macOS) opens search, for images or for jumping between pages
- On wide screens, **My images** shows a side panel with dimensions, size and links in every format

The **classic layout** is an admin panel with a top bar and tabs.

Where to switch:

- In the new layout: the gear at the bottom of the sidebar → Settings → Appearance → Layout
- In the classic layout: the user menu at the top right → Switch to the new layout

The switch takes effect at once, with no rebuild and no page reload.

## Appearance settings

**Settings → Appearance** offers:

| Setting | Options |
| --- | --- |
| Layout | New, classic |
| Colour mode | Light, dark, follow the system |
| Accent colour | A set of presets |
| Corner radius | Small, medium, large |
| Compact sidebar | Icons only |
| Language | Chinese, English |

Visitors who are not signed in can also switch colour mode, language and layout.

Preferences are saved in the browser, so another device starts from the defaults. Accent colour and corner radius follow the current layout's defaults until you choose your own. After that, both layouts share your choice.

## Working on the frontends

Frontend commands run inside the development image:

```bash
make fe-build-legacy   # build the classic frontend
make fe-build-vben     # build the new frontend
make fe-test           # run component tests
make fe-lint           # type check and lint
```

More detail is in [`docs/development.md`](https://github.com/biliblihuorong/imgnest/blob/main/docs/development.md) in the repository.
