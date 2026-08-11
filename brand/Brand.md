# ThruBox Brand Kit

This folder is the canonical source for ThruBox's visual identity: logos, favicons/icons, and color palette. All assets referenced below live in this `brand/` folder. `README.md` embeds its own copies of the two logo SVGs under `public/` — keep those in sync with the originals here if the brand mark changes.

## Logo

| Asset | File |
| --- | --- |
| ThruBox logo (SVG, with wordmark) | [`thrubox-logo.svg`](./thrubox-logo.svg) |
| AOSSIE org logo (SVG) | [`aossie-logo.svg`](./aossie-logo.svg) |

The ThruBox mark is a Menger-sponge-style cube made of green tessellated tiles wrapped around a padlock, representing an encrypted "box" relaying data between clients.

## Favicons & Icons

Generated from `thrubox-logo.svg` at the standard sizes used across browsers, bookmarks, and mobile home screens:

| File | Size | Use |
| --- | --- | --- |
| [`favicon.ico`](./favicon.ico) | 16/32/48 (multi-res) | Classic browser favicon |
| [`favicon-16x16.png`](./favicon-16x16.png) | 16×16 | Browser tab |
| [`favicon-32x32.png`](./favicon-32x32.png) | 32×32 | Browser tab (HiDPI) |
| [`favicon-48x48.png`](./favicon-48x48.png) | 48×48 | Windows taskbar |
| [`apple-touch-icon.png`](./apple-touch-icon.png) | 180×180 | iOS home screen |
| [`icon-512.png`](./icon-512.png) | 512×512 | PWA manifest / app icon |

This server itself has no bundled web UI (it's a headless REST API) — the favicons/icons here aren't consumed by any application HTML, only by the repo's own README and any external dashboards, status pages, or documentation sites built around this server.

## Color Palette

Sourced directly from `thrubox-logo.svg` (shared with [ThruBox-Client](https://github.com/AOSSIE-Org/ThruBox-Client)):

| Swatch | Name | Hex | Usage in logo |
| --- | --- | --- | --- |
| 🟩 | ThruBox Green (light) | `#3eb03e` | Sponge tile — top face |
| 🟩 | ThruBox Green (mid) | `#228B22` | Sponge tile — front face, wordmark |
| 🟩 | ThruBox Green (dark) | `#145A14` | Sponge tile — side face |
| ⬛ | Outline | `#0f420f` | Tile stroke |
| 🟨 | Lock Gold | `#FFC517` | Padlock accent (shared AOSSIE brand color) |

## Typography

This is a non-UI project (headless Go relay server) — there is no application typography to document. The wordmark in `thrubox-logo.svg` uses `'Arial Black', system-ui, sans-serif` at weight 900 as a logotype only.
