"""dashcam-cv command line.

  dashcam-cv embed <slug> [...]   # embed specific videos
  dashcam-cv embed --all          # embed the whole corpus
  dashcam-cv find "sunset"        # search

Dry-run by default; pass --apply to write vectors (same discipline as
cmd/backfill-miles). Local dev needs no AWS/Secrets Manager — point DATABASE_*
at the docker-compose `db` service and DASHCAM_CV_CORPUS_DIR at your clips.
"""

from __future__ import annotations

import argparse
import sys
import time

from rich.console import Console
from rich.table import Table

from . import db
from .corpus import corpus_dir, find_unembedded, find_videos
from .frames import DEFAULT_INTERVAL_SEC

console = Console()


def _format_ts(seconds: float) -> str:
    """Seconds → m:ss (or h:mm:ss past an hour)."""
    s = int(seconds)
    return (
        f"{s // 3600:d}:{(s % 3600) // 60:02d}:{s % 60:02d}"
        if s >= 3600
        else f"{s // 60:d}:{s % 60:02d}"
    )


def _embed_videos(conn, embedder, videos, interval, apply):
    """Embed each video; return (frames, inserted, failed).

    One log line per video (no progress bar — a bar renders to nothing on a
    non-TTY, so a long batch looks hung; plain lines stream to kubectl logs with
    PYTHONUNBUFFERED set). A per-video error is rolled back, logged, and skipped
    so one bad video doesn't abort the batch; embed_video commits per video so
    that rollback (or an interrupt) leaves it cleanly un-embedded.
    """
    from .pipeline import embed_video

    frames = inserted = failed = 0
    total = len(videos)
    for i, v in enumerate(videos, 1):
        console.print(f"[{i}/{total}] embedding {v.slug} …")
        try:
            res = embed_video(conn, embedder, v, interval_sec=interval, apply=apply)
        except Exception as e:  # noqa: BLE001  # pylint: disable=broad-exception-caught
            conn.rollback()
            failed += 1
            console.print(f"      ✗ {v.slug}: {type(e).__name__}: {e} — rolled back, skipping")
            continue
        frames += res.frames
        inserted += res.inserted
        detail = f"{res.inserted} vectors written" if apply else "dry-run"
        console.print(f"      ✓ {v.slug}: {res.frames} frames, {detail}")
    return frames, inserted, failed


def cmd_embed(args: argparse.Namespace) -> int:
    """Embed frames for the selected videos into frame_embeddings."""
    from .embed import Embedder, model_id_for

    if not args.all and not args.slugs and not args.random:
        console.print("[red]give one or more slugs, or --all, or --random N[/red]")
        return 2

    conn = db.connect()
    orphans: list[str] = []
    if args.random:
        # Incremental mode: pick random videos with no rows yet for this model.
        videos = find_unembedded(conn, model_id_for(args.model), n=args.random)
        if not videos:
            console.print(
                "[green]nothing left to embed — corpus fully treated for this model[/green]"
            )
            conn.close()
            return 0
    else:
        videos, orphans = find_videos(conn, slugs=args.slugs or None, limit=args.limit)

    if orphans:
        console.print(
            f"[yellow]{len(orphans)} corpus file(s) with no videos row (skipped)[/yellow]"
        )
    if not videos:
        console.print(f"[red]no matching videos under {corpus_dir()}[/red]")
        conn.close()
        return 1

    mode = "[green]APPLY[/green]" if args.apply else "[cyan]dry-run[/cyan]"
    console.print(
        f"{mode}: embedding {len(videos)} video(s) at 1 frame / {args.interval}s "
        f"with [bold]{args.model}[/bold]"
    )

    console.print("loading model…")
    embedder = Embedder(model_name=args.model)
    embedder.check_dim()

    started = time.perf_counter()
    total_frames, total_inserted, failed = _embed_videos(
        conn, embedder, videos, args.interval, args.apply
    )
    elapsed = time.perf_counter() - started
    conn.close()

    table = Table(title="embed summary", show_header=False)
    table.add_row("videos", str(len(videos)))
    if failed:
        table.add_row("failed (rolled back)", str(failed))
    table.add_row("frames embedded", str(total_frames))
    table.add_row("vectors written", str(total_inserted) if args.apply else "0 (dry-run)")
    table.add_row("wall time", f"{elapsed:.1f}s")
    console.print(table)
    if not args.apply:
        console.print("[cyan]dry-run — re-run with --apply to persist[/cyan]")
    return 0


def cmd_find(args: argparse.Namespace) -> int:
    """Embed the query and print the nearest frames."""
    from .embed import Embedder
    from .search import search

    embedder = Embedder(model_name=args.model)
    conn = db.connect()
    try:
        hits = search(conn, embedder, args.query, k=args.k, state=args.state)
    finally:
        conn.close()

    if not hits:
        console.print("[yellow]no results — is frame_embeddings populated for this model?[/yellow]")
        return 1

    table = Table(title=f'find "{args.query}"')
    table.add_column("#", justify="right")
    table.add_column("similarity", justify="right")
    table.add_column("video (slug)")
    table.add_column("at")
    table.add_column("state")
    for i, h in enumerate(hits, 1):
        table.add_row(str(i), f"{h.similarity:.3f}", h.slug, _format_ts(h.ts_sec), h.state or "")
    console.print(table)
    return 0


def _human_bytes(n: float) -> str:
    for unit in ("B", "KB", "MB", "GB", "TB"):
        if n < 1024:
            return f"{n:.1f} {unit}"
        n /= 1024
    return f"{n:.1f} PB"


def cmd_stats(args: argparse.Namespace) -> int:
    """Show embedding coverage, DB size, and (optionally) a concept scan."""
    from .embed import model_id_for
    from .stats import coverage, db_size

    conn = db.connect()
    mid = model_id_for(args.model)
    cov = coverage(conn, mid)
    size = db_size(conn)

    ct = Table(title="corpus coverage", show_header=False)
    ct.add_row("model", args.model)
    ct.add_row("videos embedded", f"{cov.embedded_videos} / {cov.total_videos}  ({cov.pct:.1f}%)")
    ct.add_row("videos remaining", str(cov.remaining))
    ct.add_row("frames (vectors)", f"{cov.frames:,}")
    ct.add_row("avg frames/video", f"{cov.frames_per_video:.0f}")
    console.print(ct)

    st = Table(title="vector storage", show_header=False)
    st.add_row("frame_embeddings", size["total_pretty"])
    if cov.embedded_videos and cov.total_videos:
        st.add_row(
            "projected (full corpus)",
            _human_bytes(size["total_bytes"] / cov.embedded_videos * cov.total_videos),
        )
        st.add_row("projected frames", f"~{int(cov.frames_per_video * cov.total_videos):,}")
    console.print(st)

    if args.concepts:
        from .embed import Embedder
        from .stats import concept_scan

        console.print("loading model for concept scan…")
        embedder = Embedder(model_name=args.model)
        hits = concept_scan(conn, embedder, mid, threshold=args.threshold)
        peak = max((h.matches for h in hits), default=0) or 1
        tbl = Table(title=f"concept scan (sim ≥ {args.threshold}, over {cov.frames:,} frames)")
        tbl.add_column("concept")
        tbl.add_column("matches", justify="right")
        tbl.add_column("")
        tbl.add_column("best", justify="right")
        for h in hits:
            meter = "█" * int(28 * h.matches / peak)
            tbl.add_row(h.concept, f"{h.matches:,}", meter, f"{h.best_sim:.3f}")
        console.print(tbl)

    conn.close()
    return 0


def main(argv: list[str] | None = None) -> int:
    """Parse args and dispatch to the embed / find / stats subcommand."""
    parser = argparse.ArgumentParser(prog="dashcam-cv", description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)

    from .embed import DEFAULT_MODEL

    def add_model_args(p: argparse.ArgumentParser) -> None:
        p.add_argument("--model", default=DEFAULT_MODEL, help="HuggingFace SigLIP2 checkpoint id")

    p_embed = sub.add_parser("embed", help="embed video frames into frame_embeddings")
    p_embed.add_argument("slugs", nargs="*", help="video slugs to embed (omit with --all)")
    p_embed.add_argument("--all", action="store_true", help="embed every video in the corpus")
    p_embed.add_argument(
        "--random",
        type=int,
        default=None,
        metavar="N",
        help="embed N random videos not yet embedded for this model (incremental fill)",
    )
    p_embed.add_argument(
        "--interval",
        type=float,
        default=DEFAULT_INTERVAL_SEC,
        help="seconds between sampled frames",
    )
    p_embed.add_argument(
        "--limit", type=int, default=None, help="cap number of videos (subset prototyping)"
    )
    p_embed.add_argument("--apply", action="store_true", help="persist vectors (default: dry-run)")
    add_model_args(p_embed)
    p_embed.set_defaults(func=cmd_embed)

    p_find = sub.add_parser("find", help="search the corpus by text")
    p_find.add_argument("query", help="text query, e.g. 'sunset over water'")
    p_find.add_argument("-k", type=int, default=10, help="number of results")
    p_find.add_argument("--state", default=None, help="restrict to a US state (e.g. Nevada)")
    add_model_args(p_find)
    p_find.set_defaults(func=cmd_find)

    p_stats = sub.add_parser("stats", help="embedding coverage, DB size, and concept scan")
    p_stats.add_argument(
        "--concepts", action="store_true", help="also run the concept scan (loads the model)"
    )
    p_stats.add_argument(
        "--threshold", type=float, default=0.1, help="cosine similarity cutoff for concept matches"
    )
    add_model_args(p_stats)
    p_stats.set_defaults(func=cmd_stats)

    args = parser.parse_args(argv)
    return args.func(args)


if __name__ == "__main__":
    sys.exit(main())
