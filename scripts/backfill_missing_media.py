#!/usr/bin/env python3
"""Backfill missing media URLs in KaraXXX using yt-dlp.

Intended for one-off/admin use on the production host:
  python3 scripts/backfill_missing_media.py --db /opt/karaxxx/karaxxx.db --limit 500
"""
from __future__ import annotations

import argparse
import json
import sqlite3
import subprocess
import sys
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any


@dataclass
class VideoRow:
    id: str
    source: str
    slug: str
    title: str


def build_url(row: VideoRow) -> str | None:
    slug = (row.slug or "").strip().lstrip("/")
    if row.source == "eporner":
        if slug:
            return f"https://www.eporner.com/video-{row.id}/{slug}/"
        return f"https://www.eporner.com/video-{row.id}/"
    if row.source == "tnaflix":
        if slug.startswith(("http://", "https://")):
            return slug
        if slug:
            return f"https://www.tnaflix.com/{slug}"
        return None
    if row.source == "drtuber":
        if slug:
            return f"https://www.drtuber.com/video/{row.id}/{slug}"
        return None
    return None


def quality_from_format(fmt: dict[str, Any]) -> int:
    height = fmt.get("height")
    if isinstance(height, int):
        return height
    text = " ".join(str(fmt.get(k, "")) for k in ("format_id", "format", "url"))
    for q in (2160, 1440, 1080, 720, 480, 360, 240, 144):
        if f"{q}p" in text:
            return q
    return 0


def extract_media(info: dict[str, Any]) -> dict[str, Any]:
    out: dict[str, Any] = {
        "title": info.get("title") or "",
        "thumb_uuid": info.get("thumbnail") or "",
        "duration": int(info.get("duration") or 0),
        "url_360": "",
        "url_720": "",
        "url_1080": "",
        "hls_url": "",
    }
    for fmt in info.get("formats") or []:
        url = (fmt.get("url") or "").strip()
        if not url.startswith(("http://", "https://")):
            continue
        ext = (fmt.get("ext") or "").lower()
        if ".m3u8" in url or ext == "m3u8":
            out["hls_url"] = out["hls_url"] or url
            continue
        if ext and ext != "mp4" and ".mp4" not in url:
            continue
        q = quality_from_format(fmt)
        if q >= 1080:
            out["url_1080"] = url
        elif q >= 720:
            out["url_720"] = url
        elif q > 0:
            out["url_360"] = url
        elif not out["url_360"]:
            out["url_360"] = url
    return out


def run_ytdlp(url: str, timeout: int) -> dict[str, Any]:
    proc = subprocess.run(
        ["yt-dlp", "-J", "--no-playlist", "--no-warnings", url],
        text=True,
        capture_output=True,
        timeout=timeout,
    )
    if proc.returncode != 0:
        raise RuntimeError(proc.stderr.strip() or proc.stdout.strip() or f"exit {proc.returncode}")
    return json.loads(proc.stdout)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", default="/opt/karaxxx/karaxxx.db")
    ap.add_argument("--limit", type=int, default=250)
    ap.add_argument("--source", choices=["eporner", "tnaflix", "drtuber", "all"], default="all")
    ap.add_argument("--timeout", type=int, default=90)
    args = ap.parse_args()

    db_path = Path(args.db)
    if not db_path.exists():
        print(f"DB not found: {db_path}", file=sys.stderr)
        return 2

    conn = sqlite3.connect(str(db_path), timeout=30)
    conn.row_factory = sqlite3.Row
    source_filter = "AND source IN ('eporner','tnaflix','drtuber')"
    params: list[Any] = []
    if args.source != "all":
        source_filter = "AND source = ?"
        params.append(args.source)
    rows = conn.execute(
        f"""
        SELECT id, COALESCE(source,'') source, COALESCE(slug,'') slug, COALESCE(title,'') title
        FROM videos
        WHERE COALESCE(url_360,'') = ''
          AND COALESCE(url_720,'') = ''
          AND COALESCE(url_1080,'') = ''
          AND COALESCE(hls_url,'') = ''
          {source_filter}
        ORDER BY added_at DESC
        LIMIT ?
        """,
        (*params, args.limit),
    ).fetchall()

    ok = fail = skipped = 0
    print(f"Backfill candidates: {len(rows)}")
    for r in rows:
        row = VideoRow(r["id"], r["source"], r["slug"], r["title"])
        url = build_url(row)
        if not url:
            skipped += 1
            print(f"SKIP {row.source}:{row.id} no reconstructable URL", flush=True)
            continue
        try:
            info = run_ytdlp(url, args.timeout)
            media = extract_media(info)
            if not any(media[k] for k in ("url_360", "url_720", "url_1080", "hls_url")):
                raise RuntimeError("yt-dlp returned no playable media")
            conn.execute(
                """
                UPDATE videos
                SET title = CASE WHEN ? != '' THEN ? ELSE title END,
                    thumb_uuid = CASE WHEN ? != '' THEN ? ELSE thumb_uuid END,
                    duration = CASE WHEN ? > 0 THEN ? ELSE duration END,
                    url_360 = CASE WHEN ? != '' THEN ? ELSE url_360 END,
                    url_720 = CASE WHEN ? != '' THEN ? ELSE url_720 END,
                    url_1080 = CASE WHEN ? != '' THEN ? ELSE url_1080 END,
                    hls_url = CASE WHEN ? != '' THEN ? ELSE hls_url END,
                    expires_at = CASE WHEN expires_at IS NULL OR expires_at = 0 THEN ? ELSE expires_at END
                WHERE id = ?
                """,
                (
                    media["title"], media["title"],
                    media["thumb_uuid"], media["thumb_uuid"],
                    media["duration"], media["duration"],
                    media["url_360"], media["url_360"],
                    media["url_720"], media["url_720"],
                    media["url_1080"], media["url_1080"],
                    media["hls_url"], media["hls_url"],
                    4102444800,
                    row.id,
                ),
            )
            conn.commit()
            ok += 1
            print(f"OK {row.source}:{row.id} {url}", flush=True)
            time.sleep(3)
        except Exception as exc:  # noqa: BLE001 - admin script prints and continues
            fail += 1
            print(f"FAIL {row.source}:{row.id} {url}: {exc}", flush=True)
            time.sleep(5)
    print(f"Done: ok={ok} fail={fail} skipped={skipped}")
    return 0 if fail == 0 else 1


if __name__ == "__main__":
    raise SystemExit(main())
