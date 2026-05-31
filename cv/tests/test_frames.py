"""Frame sampling — needs the real LFS test clip (skipped in CI)."""

# pylint: disable=missing-function-docstring
from __future__ import annotations

from PIL import Image

from dashcam_cv.frames import sample_frames, video_duration_sec


def test_sample_frames_yields_increasing_timestamps(test_clip):
    samples = list(sample_frames(test_clip, interval_sec=5.0))
    assert samples, "expected at least one frame"
    timestamps = [ts for ts, _ in samples]
    assert timestamps == sorted(timestamps)
    # Roughly one sample per interval — consecutive samples ~>=5s apart.
    for earlier, later in zip(timestamps, timestamps[1:], strict=False):
        assert later - earlier >= 4.0
    # Frames decode to usable RGB images.
    _, first = samples[0]
    assert isinstance(first, Image.Image)
    assert first.width > 0 and first.height > 0


def test_video_duration_positive(test_clip):
    assert video_duration_sec(test_clip) > 0
