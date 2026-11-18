# goowee/canvas frame time

`make canvasbench` (`test/e2e/canvasbench.mjs`) builds `examples/graph`, opens
it in headless Chrome at 1280×800, and times 240 frames after a warm-up. Each
frame records 10,000 moving nodes (`Circles`, one path per color) and 10,000
edges (`Segments`, one path), about 280 KB of commands, and replays them.

- **record (Go)**: recording the picture into the `canvas.Ctx`, as the demo
  measures it.
- **Go up to the crossing**: the animation frame callback until `goowee.draw`
  starts: frame callbacks (recording), the flush, and the copy into JS.
- **replay**: `goowee.draw`, issuing the canvas calls.
- **frame interval**: time between frames that drew.

Budget on the reference machine: Go + replay at most 4 ms per frame.

## 2026-11-18, Apple M1, Go 1.25, Chrome headless

GPU raster (the default):

| ms per frame | median | p95 |
|---|---|---|
| record (Go) | 1.09 | |
| Go up to the crossing | 1.20 | 2.60 |
| replay (goowee.draw) | 2.70 | 6.20 |
| Go + replay | 3.90 | 8.60 |
| frame interval | 16.70 | 16.80 |

Software raster (`SOFTWARE=1`): Go + replay 3.80 ms median, but rasterizing
the 10,000 circles after the frame holds it to a 33.3 ms interval (30 fps).

Timings are wall-clock: compare runs on one machine, and run on a quiet one.
