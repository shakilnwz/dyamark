# dyamark

A small, fast Markdown viewer and editor for Omarchy, built around drawing **Mermaid sequence diagrams natively and faithfully**.

## Features

- **Instant Preview:** Pure-Go GPU-rendered Wayland window (via Gio), launching in <150 ms.
- **Native Sequence Diagrams:** Faithful layout matching Mermaid without a browser engine or JavaScript runtime.
- **Omarchy Integration:** Follows system theme live (`~/.local/state/omarchy/current/theme/colors.toml`), matches Hyprland corner rounding, uses XDG Desktop Portals, and registers with `dyamark.desktop`.
- **Lightweight:** Single stripped Go binary (~9.5 MB) with bundled fonts included.

## Building & Testing

```bash
# Build the stripped binary
./bin/build

# Run automated tests
./bin/test

# Run with measurement benchmark
./build/dyamark --measure --exit-after
```

## Status

Phase 0 (Spike & Benchmarks) complete:
- Gio Wayland window on Hyprland (`app_id=dyamark`)
- Live Omarchy theme integration and Hyprland decoration rounding
- Bundled Liberation Sans fonts
- Hardcoded sequence diagram spike
- Benchmarked: 9.5 MB binary, ~138 ms cold start to first paint
