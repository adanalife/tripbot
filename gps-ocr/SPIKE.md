# Multi-frame GPS re-OCR — spike findings (2026-06-14)

**Question:** can we get more accurate per-clip GPS coords by OCR-ing *many*
frames per dashcam clip and reconciling them, instead of the single-frame read
the legacy `pkg/ocr` pass used (removed in #79, 2021)?

**Answer: yes, clearly.** `spike_ocr.py` ports the original pipeline (crop
bottom-left HUD → grayscale/contrast/sharpen/invert → Tesseract psm6 whitelist
`NSEW.1234567890MPH` → split on `MPH` → parse `W<lon>N<lat>`, continental-US
gate) onto a `frames.py`-style PyAV multi-frame sampler, then reconciles all
valid reads (median + MAD outlier rejection).

## How it was run

A throwaway `python:3.12-slim` pod on the minipc (`default/gps-ocr-spike`),
dashcam NFS mounted read-only at `/footage` (server **192.168.40.100**, the
live PV value — the gitignored `dashcam-nfs.local.yaml` in infra is stale at
`192.168.1.222`). `apt install tesseract-ocr` + `pip install av pillow
pytesseract`. Interval 3.0s → ~60 frames/clip.

## Results (10 random clips)

- **Most clips get 30–60 valid reads of 60 frames.** Even clips with a long
  GPS-lock gap (one had only 11 valid) still reconcile cleanly.
- **Within a clip the track is dense, smooth, monotonic** — the van actually
  driving. The legacy single read was one sample of this; any one of them could
  be a digit-flip.
- **Two failure modes, both handled:**
  - Gross OCR garbage (dropped decimal, letter-for-digit) → rejected by the
    per-frame US-bounds gate.
  - Digit-flip misreads that land *inside* the US (e.g. lat `43.x`→`33.x`) slip
    the gate but are tens/hundreds of km from the real track → rejected by the
    MAD filter (`> max(1.5km, 5·MAD)` from the median).
- **Spread before/after MAD rejection** (= the win):

  | clip | before | after | rej |
  |---|---|---|---|
  | 0522 | 590 km | 9.6 km | 4 |
  | 1015 | 1781 km | 2.6 km | 2 |
  | 1009 | 1002 km | 1.0 km | 3 |
  | 0521 | 24.9 km | 6.8 km | 1 |
  | 0518 | 14.0 km | 5.1 km | 1 |
  | 1206 | 95 m | 0.3 km | 0 |

  Post-rejection spreads are all 0.3–9.6 km — exactly what ~3 min of driving
  covers. No clip is poisoned by an outlier anymore.

## Confidence signal (free, per clip)

`n_kept` (frames that agreed) + post-rejection `spread` give a per-clip quality
score. High kept + low spread → trust the reconciled coord. Few kept (e.g. 9) →
low confidence; a real pipeline could fall back to interpolation
(`cmd/backfill-coords`) for those.

## Open / next steps (the real tool)

- **Compare reconciled vs. stored `videos` coords** to quantify how many the DB
  currently has wrong (needs DB access; is the first step of the real pipeline).
- Representative coord: median is robust, but for a *moving* clip we may prefer
  a defined anchor (first valid fix? midpoint?) — decide per use (`!guess` /
  `!location` / map).
- Productionize separate from the (finishing, won't-rerun) SigLIP2 embed pass:
  its own lightweight subtree (PyAV + Pillow + pytesseract, no torch), runs as a
  one-off Job on the minipc against the NFS corpus.
- Writeback shape: follow `cmd/backfill-coords` — dry-run report + `--apply` +
  idempotent slug-keyed SQL, stamping `videos.coord_source = 'ocr'` for
  re-derived rows, never destroying a coord it can't confidently replace.
