# Interface and themes

ImgNest's interface is built into the program, so there is nothing separate to deploy. It ships two layouts that can be switched at runtime, both with dark mode and Chinese/English.

## Two layouts

The **new layout** is the default:

- The sidebar floats over the page and can collapse to icons
- `Ctrl K` (`⌘ K` on macOS) opens search, for images or for jumping between pages
- On wide screens, **My images** shows a side panel with dimensions, size and EXIF. Copy existing links from the image-card context menu; see [Uploading and managing images](./upload-and-images.md#manage-existing-images)

The **classic layout** is an admin panel with a top bar and tabs.

Where to switch:

- In the new layout: the gear at the bottom of the sidebar → Settings → Layout → choose New or Classic.
- In the classic layout: the header gear, or the user menu at the top right → Settings → Layout → choose New or Classic. The user menu also offers Switch to the new layout as a shortcut.

The switch takes effect at once, with no rebuild and no page reload.

## Appearance settings

Both layouts open the same **Settings** dialog, which directly shows these options:

| Setting | Options |
| --- | --- |
| Layout | New, classic |
| Colour mode | Light, dark, follow the system |
| Accent colour | A set of presets |
| Corner radius | Small, medium, large |
| Compact sidebar | Icons only |
| Language | Follow browser, 简体中文, English |

The sign-in page offers colour-mode and language controls. Signed-in users can change the layout and other appearance preferences in the console.

Profile, password and API tokens are managed on the Account page. When signed in, click Open account at the bottom of the settings dialog, then choose API tokens or Profile & security. See [Account and usage](./account-and-overview.md) for the steps.

Preferences are saved in the browser, so another device starts from the defaults. Both layouts share the same default accent colour and corner radius, and switching layouts preserves your current choices. Reset appearance restores only the accent colour and corner radius to these shared defaults.

After upgrading, classic-layout preferences still using the old default accent colour or corner radius move to the shared defaults once; other custom values are preserved. You can then choose the old classic blue or medium corner radius again, and those choices survive a page reload.

## Working on the interface

The source is in `web-vben/` in the repository, built with Vue 3, Vite and Naive UI. Commands run inside the development image:

```bash
make fe-build   # build the interface
make fe-test    # run component tests
make fe-lint    # type check and lint
make release    # build the interface and compile bin/imgnest
```

More detail is in [`docs/development.md`](https://github.com/biliblihuorong/imgnest/blob/main/docs/development.md) in the repository.
