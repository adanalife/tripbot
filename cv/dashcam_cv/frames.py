"""Sample frames from a video at a fixed time interval.

Slow-TV dashcam footage changes slowly, so ~1 frame / 1-2 s captures the scene
without embedding every frame. Frames are yielded in-memory as PIL images and
discarded by the caller after embedding — nothing is written to disk (the
"don't persist frames" principle; see the plan's "Parked: frame persistence").
"""

from __future__ import annotations

from collections.abc import Iterator
from pathlib import Path

import av
from PIL import Image, ImageStat

DEFAULT_INTERVAL_SEC = 2.0


def mean_luminance(img: Image.Image) -> float:
    """Average pixel luminance (0-255). Cheap near-black detector."""
    return ImageStat.Stat(img.convert("L")).mean[0]


def sample_frames(
    path: str | Path, interval_sec: float = DEFAULT_INTERVAL_SEC
) -> Iterator[tuple[float, Image.Image]]:
    """Yield (ts_sec, frame) pairs roughly every `interval_sec` seconds.

    ts_sec is the frame's presentation timestamp in seconds from the start of
    the file — the value stored in frame_embeddings.ts_sec and used to build
    deep links back into the video.
    """
    with av.open(str(path)) as container:
        stream = container.streams.video[0]
        # Frame-accurate decode is fine for ~3-minute clips; sparse seeking
        # trades accuracy for speed we don't need here.
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


def video_duration_sec(path: str | Path) -> float:
    """Container duration in seconds (0.0 if unknown). Used for size estimates."""
    with av.open(str(path)) as container:
        if container.duration is not None:
            return float(container.duration) / av.time_base
        stream = container.streams.video[0]
        if stream.duration is not None and stream.time_base is not None:
            return float(stream.duration * stream.time_base)
    return 0.0
