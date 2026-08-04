# Tray icon placeholders

The tray icons are embedded into the application at build time. Replace the
generated files below and rebuild the application; no Go code changes are
required.

- `darwin/trayicon.png`: 36×36 RGBA PNG, monochrome template artwork for
  macOS Retina menu bars.
- `windows/trayicon.ico`: multi-image ICO containing 16, 20, 24, 32, 40, 48,
  64, and 256 pixel images.
- `linux/trayicon.png`: 64×64 RGBA PNG for Ubuntu/Linux status notifiers.

The adjacent SVG files are editable placeholder sources only. The application
embeds the PNG/ICO files, not the SVG sources.
