"""SigLIP2 NaFlex image/text embedding (HuggingFace transformers).

The corpus is finite and immutable, so we embed every sampled frame once on the
batch run and never touch the image model in the hot path. Only query-time text
embedding (tiny, sub-second) happens live.

Model: SigLIP2 so400m **NaFlex** — the flagship checkpoint, non-gated on the HF
hub. NaFlex preserves the native aspect ratio (a 16:9 dashcam frame becomes a
landscape patch grid, ~21x12), so widescreen edge detail — road signs, scenery
at the sides — survives instead of being square-cropped away. That edge/sign
capability is the whole reason for SigLIP2 over a plain CLIP model.

The frame_embeddings.model column records the exact checkpoint so a future swap
can coexist with old vectors.
"""

from __future__ import annotations

import os

import numpy as np
import torch

# Production default — the non-gated flagship. Override with DASHCAM_CV_MODEL for
# experimentation (e.g. google/siglip2-base-patch16-naflex, 768-dim and much
# lighter); a model whose width != the frame_embeddings column is rejected before
# any DB write. The matching column width lives in db.EMBED_DIM.
DEFAULT_MODEL = os.environ.get("DASHCAM_CV_MODEL", "google/siglip2-so400m-patch16-naflex")

# SigLIP text towers use a fixed 64-token sequence (padding="max_length").
_TEXT_MAX_LEN = 64


def model_id_for(model_name: str) -> str:
    """Stable frame_embeddings.model value for a checkpoint name.

    Exposed module-level so the 'which videos still need embedding?' query can
    filter by model without paying to load the (heavy) model first.
    """
    return f"transformers:{model_name}"


# fp32 on the mini-PC (32 GB) for full accuracy; set DASHCAM_CV_DTYPE=bfloat16 to
# roughly halve memory on a constrained box (so400m is ~4.4 GB in fp32).
_DTYPE = {"float32": torch.float32, "bfloat16": torch.bfloat16, "float16": torch.float16}[
    os.environ.get("DASHCAM_CV_DTYPE", "float32")
]


class Embedder:
    """Loads a SigLIP2 model once and embeds images / text into one space.

    Vectors are L2-normalized, so pgvector cosine distance (`<=>`) ranks by
    cosine similarity.
    """

    def __init__(
        self,
        model_name: str = DEFAULT_MODEL,
        device: str = "cpu",
    ) -> None:
        from transformers import AutoModel, AutoProcessor

        self.model_name = model_name
        self.device = device
        self.dtype = _DTYPE
        self.model = (
            AutoModel.from_pretrained(model_name, dtype=_DTYPE, low_cpu_mem_usage=True)
            .to(device)
            .eval()
        )
        self.processor = AutoProcessor.from_pretrained(model_name)
        self._dim: int | None = None

    @property
    def model_id(self) -> str:
        """Stable identifier persisted in frame_embeddings.model."""
        return model_id_for(self.model_name)

    @property
    def dim(self) -> int:
        """Embedding width, discovered once via a throwaway text embed."""
        if self._dim is None:
            self._dim = int(self.embed_text("dimension probe").shape[0])
        return self._dim

    @staticmethod
    def _pooled(out):
        """The pooled embedding tensor.

        transformers 5.x get_*_features returns a ModelOutput (pooler_output +
        last_hidden_state); 4.x returned the pooled tensor directly. Normalize.
        """
        return getattr(out, "pooler_output", out)

    def check_dim(self) -> None:
        """Fail fast if the model's width doesn't match the DB column."""
        from .db import EMBED_DIM

        if self.dim != EMBED_DIM:
            raise ValueError(
                f"model {self.model_id} emits {self.dim}-dim vectors but the "
                f"frame_embeddings column is vector({EMBED_DIM}). Pick a matching "
                f"checkpoint or migrate the column to vector({self.dim})."
            )

    @torch.no_grad()
    def embed_images(self, images: list) -> np.ndarray:
        """Embed a batch of PIL images -> (N, dim) float32, L2-normalized.

        NaFlex preprocessing returns pixel_values + a per-image attention mask +
        spatial_shapes (the aspect-preserving patch grid); all are passed through.
        """
        inputs = self.processor(images=images, return_tensors="pt").to(self.device)
        if self.dtype != torch.float32:
            inputs["pixel_values"] = inputs["pixel_values"].to(self.dtype)
        feats = self._pooled(self.model.get_image_features(**inputs)).float()
        feats = feats / feats.norm(dim=-1, keepdim=True)
        return feats.cpu().numpy().astype(np.float32)

    @torch.no_grad()
    def embed_text(self, text: str) -> np.ndarray:
        """Embed a text query -> (dim,) float32, L2-normalized."""
        inputs = self.processor(
            text=[text],
            padding="max_length",
            max_length=_TEXT_MAX_LEN,
            return_tensors="pt",
        ).to(self.device)
        feats = self._pooled(self.model.get_text_features(**inputs)).float()
        feats = feats / feats.norm(dim=-1, keepdim=True)
        return feats.cpu().numpy().astype(np.float32)[0]
