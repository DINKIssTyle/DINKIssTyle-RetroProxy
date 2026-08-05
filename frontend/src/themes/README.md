# Theme structure

Each top-level CSS file is the entry point for one selectable RetroProxy theme.
Add OS-specific overrides at the end of that theme's file.

| Theme key | Entry CSS | Upstream theme and skin |
| --- | --- | --- |
| `macintosh` | `macintosh.css` | `@sakun/system.css` |
| `macos9` | `macos9.css` | `macos9/default` |
| `win31` | `windows31.css` | `win3x/3.1` |
| `win95` | `windows95.css` | `win9x/95` |
| `win98` | `windows98.css` | `win9x/98` |
| `win2000` | `windows2000.css` | `win9x/2000` |
| `winxp` | `windows-xp-blue.css` | `winxp/default` |

The `shared` directory contains only rules reused by multiple entry themes:

- `classic-app.css`: RetroProxy layout for themes from `classic-stylesheets`.
- `modern-windows-titlebar.css`: right-aligned window controls for Windows 95 and later.
- `win9x-select-focus.css`: neutral closed selects with selection colors only on focus.
