# Go-Specific Features for goowee Showcase

Ideas for the landing page and tutorial highlighting what Go + WASM
unlocks that JS alone cannot easily replicate.

## Ranked by wow-factor and practical value

### 1. SQLite in the browser

`modernc.org/sqlite` compiles to WASM with zero C dependencies. Ship a
full SQL database to the client — offline query, full-text search,
relational joins with no server round-trip. JS has `sql.js` (SQLite via
Emscripten) but Go's version is part of the same build and toolchain.

**Demo idea:** A searchable contact list or recipe manager that works
fully offline, syncs when connected.

### 2. Image processing (client-side)

`image/jpeg`, `image/png`, `github.com/disintegration/imaging` — drag an
image onto a drop zone, resize, crop, rotate, re-encode, and upload. In JS
this requires multiple libraries and OffscreenCanvas juggling. Go does it
in a dozen lines of stdlib code.

**Demo idea:** A profile-photo cropper that resizes to 256×256 and
re-encodes at 80% quality, all before the upload button is clicked.

### 3. PDF generation without a server

`github.com/jung-kurt/gofpdf` — generate invoices, receipts, tickets, or
reports entirely in the browser tab. JS alternatives (jsPDF) exist but are
slower and memory-heavy for complex multi-page documents.

**Demo idea:** A receipt generator — fill in line items, hit "download",
and get a server-quality PDF produced by the same Go binary that serves
the page.

### 4. Cryptography (bcrypt, TLS, ed25519)

Go's `crypto/...` standard library is among the most rigorous in any
language. Hash passwords client-side, verify ed25519 signatures, parse
X.509 certificates. JS's SubtleCrypto is limited to a handful of
primitives and is async-only.

**Demo idea:** A message-signing tool — type a message, sign it with an
ed25519 key, verify with a public key. All computation stays in the tab.

### 5. CSV/XLSX → interactive table

`encoding/csv` + `github.com/xuri/excelize` or `tealeg/xlsx` — drop a
spreadsheet onto a goowee list, parse it, render as a sortable/filterable
table. No JS library import overhead; the Go parser runs at native speed.

**Demo idea:** A CSV viewer with column sorting, search, and pagination —
built entirely with stdlib + one line to open the file.

### 6. Markdown → reactive document

`github.com/yuin/goldmark` — render markdown into a goowee component tree
with live preview. Goldmark is significantly faster than JS markdown
parsers on documents larger than a few kilobytes.

**Demo idea:** A notes editor with split-pane markdown preview, where
both the raw text and the rendered output update reactively.

### 7. Archive extraction (zip/tar/gzip)

`archive/zip`, `compress/gzip`, `archive/tar` — extract archives
in-browser without JS libraries. Useful for bulk-upload workflows where
the user uploads a `.zip` and the app processes each file.

**Demo idea:** A bulk photo uploader — drop a `.zip` of images, extract
client-side, preview thumbnails, then upload individually.

### 8. Template execution (Go templates in the browser)

`html/template` — render Go templates client-side, sharing the exact same
templates used by the SSR server. Zero duplication, guaranteed parity
between server-rendered and client-updated HTML.

**Demo idea:** A live template playground — edit a `.gotmpl` file and see
the rendered output update in real time, using the same template engine
as the server.

### 9. TOML/YAML config parsing

`github.com/BurntSushi/toml`, `gopkg.in/yaml.v3` — parse config files
in-browser for a web-based settings editor. JS equivalents exist but are
separate dependencies with different APIs.

**Demo idea:** A TOML config editor with schema validation, syntax
highlighting, and export.

### 10. Timezone math

Go's `time.LoadLocation` + `time.Time.In` — convert between timezones
correctly without a `moment.js` / `luxon` dependency. Go's time library
is famously saner than JS `Date`, and the `time/tzdata` embed makes
timezone data available even offline.

**Demo idea:** A world-clock widget showing the current time in 5 cities,
with correct DST handling, all in stdlib.
