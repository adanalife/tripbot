#!/usr/bin/env python3
"""Spike: multi-frame GPS re-OCR of dashcam clips.

Goal of THIS script is accuracy validation, not production. It re-derives a
clip's GPS coordinate by OCR-ing many frames (not the single frame the legacy
`pkg/ocr` pass used) and reconciling them, then prints enough diagnostics to
judge whether the approach is sound before we build the real pipeline.

Pipeline per frame, faithful to the removed Go pipeline (pkg/ocr/ocr.go +
helpers.ParseLatLng):

  sample frame  -> crop bottom-left overlay -> grayscale/contrast/sharpen/invert
  -> tesseract (psm 6, whitelist "NSEW.1234567890MPH", dawg off)
  -> split on "MPH", take the GPS half -> parse  W<lon>N<lat>  (W hemisphere)
  -> gate to continental-US bounds (a free per-frame validity filter)

Then reconcile all valid reads for the clip into one robust coordinate
(median), reporting spread + agreement so we can see how much multi-frame
voting buys us over a single read.

Usage (inside a pod on the minipc, NFS mounted at /footage):
  python spike_ocr.py /footage/2018_0514_224801_013.MP4
  python spike_ocr.py --interval 1.0 --dump-crops /tmp/crops /footage/*.MP4
  python spike_ocr.py --sample 8 /footage   # 8 random clips from a dir

Deps: av, Pillow, pytesseract  (+ the tesseract binary).
"""

from __future__ import annotations

import argparse
import random
import re
import statistics
import sys
from collections.abc import Iterator
from dataclasses import dataclass
from pathlib import Path

import av
import pytesseract
from PIL import Image, ImageEnhance, ImageFilter, ImageOps

# ----- legacy OCR constants (from pkg/ocr + configs/tesseract.cfg) -----------

# bottom-left strip the dashcam burns the HUD into; absolute px, BottomLeft anchor
CROP_W = 600
CROP_H = 60

# tesseract: single uniform block, restrict the alphabet, kill the dictionaries
TESS_CONFIG = (
    "--psm 6 "
    "-c tessedit_char_whitelist=NSEW.1234567890MPH "
    "-c load_system_dawg=0 -c load_freq_dawg=0"
)

# continental-US gate from helpers.ParseLatLng — doubles as a misread filter
US_LAT_MIN, US_LAT_MAX = 25.7, 49.23
US_LON_MIN, US_LON_MAX = -124.44, -66.57

DEFAULT_INTERVAL_SEC = 2.0


# ----- frame sampling (modeled on cv/dashcam_cv/frames.py) -------------------


def sample_frames(
    path: str | Path, interval_sec: float = DEFAULT_INTERVAL_SEC
) -> Iterator[tuple[float, Image.Image]]:
    """Yield (ts_sec, frame) roughly every `interval_sec` seconds."""
    with av.open(str(path)) as container:
        stream = container.streams.video[0]
        stream.thread_type = "AUTO"
        next_at = 0.0
        for frame in container.decode(stream):
            if frame.pts is None:
                continue
            ts = float(frame.pts * stream.time_base)
            if ts + 1e-6 < next_at:
                continue
            yield ts, frame.to_image()
            next_at = ts + interval_sec


# ----- per-frame OCR ---------------------------------------------------------


def crop_overlay(
    img: Image.Image, *, crop_w: int = CROP_W, crop_h: int = CROP_H
) -> Image.Image:
    """Crop to the bottom-left HUD strip and apply the legacy read-prep filters."""
    w, h = img.size
    box = (0, max(0, h - crop_h), min(crop_w, w), h)
    crop = img.crop(box)
    crop = ImageOps.grayscale(crop)
    crop = ImageEnhance.Contrast(crop).enhance(1.20)  # imaging.AdjustContrast(20)
    crop = crop.filter(ImageFilter.SHARPEN)
    crop = ImageOps.invert(crop)
    return crop


def ocr_text(img: Image.Image) -> str:
    return pytesseract.image_to_string(img, config=TESS_CONFIG)


def extract_coords(text: str) -> str | None:
    """Strip whitespace, split on MPH, return the GPS half (or None)."""
    tidy = re.sub(r"\s+", "", text)
    parts = re.split("MPH", tidy)
    if len(parts) < 2:
        return None
    return parts[1] or None


def parse_latlng(s: str) -> tuple[float, float] | None:
    """Port of helpers.ParseLatLng:  W<lon>N<lat>  -> (lat, -lon), US-gated."""
    n = s.find("N")
    if n <= 0:
        return None
    try:
        lon = float(s[1:n])
        lat = float(s[n + 1 :])
    except ValueError:
        return None
    if lat == 0.0 or lon == 0.0:
        return None
    lon = -lon  # western hemisphere hardcoded, same as the Go impl
    if not (-90.0 <= lat <= 90.0 and -180.0 <= lon <= 180.0):
        return None
    if not (US_LAT_MIN <= lat <= US_LAT_MAX and US_LON_MIN <= lon <= US_LON_MAX):
        return None
    return lat, lon


@dataclass
class FrameRead:
    ts: float
    raw: str
    coord: tuple[float, float] | None


def read_clip(
    path: Path, interval_sec: float, dump_dir: Path | None
) -> list[FrameRead]:
    reads: list[FrameRead] = []
    for i, (ts, frame) in enumerate(sample_frames(path, interval_sec)):
        crop = crop_overlay(frame)
        if dump_dir is not None and i < 3:
            dump_dir.mkdir(parents=True, exist_ok=True)
            crop.save(dump_dir / f"{path.stem}_{i:03d}_{ts:06.1f}.png")
        raw = ocr_text(crop)
        gps = extract_coords(raw)
        coord = parse_latlng(gps) if gps else None
        reads.append(FrameRead(ts, raw.strip().replace("\n", " "), coord))
    return reads


# ----- reconciliation --------------------------------------------------------


@dataclass
class Reconciled:
    lat: float | None
    lon: float | None
    n_valid: int  # passed the per-frame US-bounds gate
    n_kept: int  # survived MAD outlier rejection
    n_total: int  # frames sampled
    spread_m: float  # max distance between kept reads (~ travel distance)


def _meters(a: tuple[float, float], b: tuple[float, float]) -> float:
    """Equirectangular approximation, good enough at clip scale."""
    import math

    dlat = (a[0] - b[0]) * 111_320.0
    dlon = (a[1] - b[1]) * 111_320.0 * math.cos(math.radians((a[0] + b[0]) / 2))
    return math.hypot(dlat, dlon)


def reconcile(
    reads: list[FrameRead], *, mad_k: float = 5.0, floor_m: float = 1500.0
) -> Reconciled:
    """Median of valid reads, then drop reads too far from it (MAD-based).

    A digit-flip misread that slips the US-bounds gate lands tens/hundreds of
    km from the real track; legitimate within-clip travel is a few km. So we
    reject any read whose distance from the median exceeds max(floor, k*MAD) —
    the floor keeps a tight cluster from rejecting normal travel.
    """
    valid = [r.coord for r in reads if r.coord is not None]
    n_total = len(reads)
    if not valid:
        return Reconciled(None, None, 0, 0, n_total, 0.0)

    med = (
        statistics.median([c[0] for c in valid]),
        statistics.median([c[1] for c in valid]),
    )
    dists = [_meters(c, med) for c in valid]
    mad = statistics.median(dists) if dists else 0.0
    cutoff = max(floor_m, mad_k * mad)
    kept = [c for c, d in zip(valid, dists) if d <= cutoff]
    if not kept:
        kept = valid

    lat = statistics.median([c[0] for c in kept])
    lon = statistics.median([c[1] for c in kept])
    spread = max((_meters(a, b) for a in kept for b in kept), default=0.0)
    return Reconciled(lat, lon, len(valid), len(kept), n_total, spread)


# ----- driver ----------------------------------------------------------------


def iter_clips(target: Path, sample: int | None) -> list[Path]:
    if target.is_dir():
        clips = sorted(p for p in target.iterdir() if p.suffix.upper() == ".MP4")
        if sample and len(clips) > sample:
            clips = random.sample(clips, sample)
        return clips
    return [target]


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("targets", nargs="+", type=Path, help="clip(s) or a directory")
    ap.add_argument("--interval", type=float, default=DEFAULT_INTERVAL_SEC)
    ap.add_argument("--sample", type=int, default=None, help="if dir, N random clips")
    ap.add_argument(
        "--dump-crops", type=Path, default=None, help="save first crops here"
    )
    ap.add_argument("--verbose", action="store_true", help="print every frame read")
    args = ap.parse_args()

    clips: list[Path] = []
    for t in args.targets:
        clips.extend(iter_clips(t, args.sample))

    print(f"# spike: {len(clips)} clip(s), interval={args.interval}s\n")
    for clip in clips:
        try:
            reads = read_clip(clip, args.interval, args.dump_crops)
        except Exception as e:  # noqa: BLE001 — spike, surface anything
            print(f"{clip.name}: ERROR {e}")
            continue
        rec = reconcile(reads)
        if args.verbose:
            for r in reads:
                tag = f"{r.coord[0]:.6f},{r.coord[1]:.6f}" if r.coord else "—"
                print(f"  {r.ts:7.1f}s  {tag:24}  raw={r.raw!r}")
        if rec.lat is None:
            print(f"{clip.name}: NO VALID READS ({rec.n_total} frames)\n")
            continue
        rejected = rec.n_valid - rec.n_kept
        print(
            f"{clip.name}: {rec.lat:.6f},{rec.lon:.6f}  "
            f"kept={rec.n_kept}/{rec.n_valid} valid ({rejected} rej) of {rec.n_total} frames  "
            f"spread=~{rec.spread_m / 1000:.1f}km\n"
        )
    return 0


if __name__ == "__main__":
    sys.exit(main())
