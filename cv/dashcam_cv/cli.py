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
from rich.progress import BarColumn, Progress, TextColumn, TimeElapsedColumn
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


def cmd_embed(args: argparse.Namespace) -> int:
    """Embed frames for the selected videos into frame_embeddings."""
    from .embed import Embedder, model_id_for
    from .pipeline import embed_video

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

    total_frames = 0
    total_inserted = 0
    started = time.perf_counter()
    with Progress(
        TextColumn("[progress.description]{task.description}"),
        BarColumn(),
        TextColumn("{task.completed}/{task.total}"),
        TimeElapsedColumn(),
        console=console,
    ) as progress:
        task = progress.add_task("embedding", total=len(videos))
        for v in videos:
            progress.update(task, description=v.slug)
            res = embed_video(conn, embedder, v, interval_sec=args.interval, apply=args.apply)
            total_frames += res.frames
            total_inserted += res.inserted
            progress.advance(task)
    elapsed = time.perf_counter() - started
    conn.close()

    table = Table(title="embed summary", show_header=False)
    table.add_row("videos", str(len(videos)))
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


def main(argv: list[str] | None = None) -> int:
    """Parse args and dispatch to the embed / find subcommand."""
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

    args = parser.parse_args(argv)
    return args.func(args)


if __name__ == "__main__":
    sys.exit(main())
