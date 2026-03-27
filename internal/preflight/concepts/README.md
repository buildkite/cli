# Bubble Tea `preflight` Prototype

This is a small runnable example that demonstrates the Bubble Tea integration shape we discussed for `bk preflight`.

It intentionally runs in normal screen mode, not alt-screen, and shows:

- static sections mixed with independently updated sections
- external updates injected with `Program.Send`
- spinner-driven status updates
- a `viewport` for the live build output area
- resize-aware reflow using `tea.WindowSizeMsg`

Run it with:

```bash
cd internal/preflight/concepts
go run .
```

Resize the terminal while it is running to see the content reflow.

Use `up` / `down` or `pgup` / `pgdn` to scroll the viewport.

Press `q` or `ctrl+c` to quit.
