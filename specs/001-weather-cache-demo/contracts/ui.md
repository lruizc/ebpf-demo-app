# Contract — UI: `GET /`

**Feature**: Weather Cache Demo for eBPF Talk
**Date**: 2026-05-14

This contract specifies the dashboard page the audience sees on screen. It
is deliberately minimal so the **source badge** dominates the visual field.

---

## Endpoint

```
GET /
```

- Method: `GET`.
- Authentication: none.
- Response: `200 OK`, `Content-Type: text/html; charset=utf-8`.
- The page is rendered server-side from `web/templates/index.html` using
  Go `html/template`.

## Page elements (required)

| Element                  | Required | Notes                                                                   |
|--------------------------|----------|-------------------------------------------------------------------------|
| `<title>`                | yes      | `weather-app — eBPF demo`.                                              |
| Heading                  | yes      | `Weather Cache Demo` (or equivalent Spanish copy).                      |
| Subtitle                 | yes      | Brief one-liner stating what the badge means.                           |
| City `<select>`          | yes      | One `<option>` per city in the catalog; uses `slug` as `value`, `display_name` as label. |
| "Consultar" button       | yes      | Submits the lookup.                                                     |
| Result panel             | yes      | Initially empty; populated after the first lookup.                      |
| Source badge             | yes      | Big, projector-readable. CSS class drives color (see below).            |
| Error region             | yes      | Hidden by default; visible only when an error occurs.                   |
| Footer (optional)        | no       | Build info / Pod name (read from `HOSTNAME`); useful for live demos.    |

## Template data model

The handler renders `index.html` with the following Go struct as
`.` (root data):

```go
type IndexPage struct {
    Cities      []City   // catalog, in stable order
    PodName     string   // os.Hostname(); empty string if unavailable
    AppVersion  string   // ldflag-injected; "dev" by default
}
```

The "result panel" is **not** populated server-side on the initial GET.
The page uses a tiny vanilla JavaScript snippet (≤ 50 lines) that calls
`/api/weather?city=<slug>` on button click and mutates the result panel.
This keeps the server-side template trivial and lets the audience watch
the same HTTP call they would see in any browser devtools.

## Source badge contract

This is the most important visual element of the talk. It MUST behave
exactly like this:

| `source` (from JSON) | Badge text                        | CSS class           | Suggested color |
|----------------------|-----------------------------------|---------------------|-----------------|
| `"cache"`            | `cache HIT (internal)`            | `.badge--internal`  | green           |
| `"origin"`           | `cache MISS (external)`           | `.badge--external`  | orange          |
| `"bypass"`           | `BYPASS (always external)`        | `.badge--bypass`    | red             |

Constraints:

- Badge font-size SHOULD be ≥ 28 px (SC-001 — readable from the back of
  the room).
- Color and text MUST stay in sync; never show the orange/red colors with
  the "internal" wording or vice versa.
- The badge MUST appear within 100 ms of the JSON response landing in the
  browser; no skeleton, no animation longer than that.

## Error rendering

When `/api/weather` returns 4xx or 5xx, the JS snippet:

1. Hides the result panel.
2. Reveals the error region.
3. Displays the `message` field from the JSON envelope verbatim, prefixed
   with `Error:`.
4. Does NOT show a stack trace, console dump, or HTTP status code in the
   visible UI (audience-friendly per SC-007).

## Static assets

- `GET /static/styles.css` — small (≤ 4 KB) hand-written CSS. No CDN.
- `GET /static/app.js` — the ≤ 50-line vanilla JS that talks to
  `/api/weather`. No bundler, no framework.

Both assets are served by the same Go binary using `http.FileServer` with
the embedded filesystem (`go:embed web/static`).

## Accessibility / sanity

- The dropdown is a real `<select>`, navigable by keyboard.
- The button is a real `<button type="submit">`.
- No tracking pixels, no analytics, no fonts loaded from the internet
  (Constitution IV).

## Out of scope for v1

- Multi-language support (page is bilingual-friendly but not localized
  via i18n libs).
- A "show all cities" grid view (mentioned as user-story alternative B in
  the original brainstorm; deferred).
- Auto-refresh / websockets.
