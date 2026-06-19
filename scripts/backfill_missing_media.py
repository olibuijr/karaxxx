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


def record_failure(conn: sqlite3.Connection, video_id: str, message: str) -> None:
    cur = conn.execute("SELECT COALESCE(retry_count, 0) FROM scrape_failures WHERE video_id = ?", (video_id,)).fetchone()
    retry_count = int(cur[0]) if cur else 0
    next_count = retry_count + 1
    delay = min(86400, 3600 * (2 ** min(next_count, 5)))
    next_retry = int(time.time()) + delay
    conn.execute(
        """
        INSERT INTO scrape_failures (video_id, retry_count, last_error, next_retry_at)
        VALUES (?, ?, ?, ?)
        ON CONFLICT(video_id) DO UPDATE SET
          retry_count = excluded.retry_count,
          last_error = excluded.last_error,
          next_retry_at = excluded.next_retry_at
        """,
        (video_id, next_count, message[:500], next_retry),
    )
    conn.commit()


def prune_dead_unplayable(conn: sqlite3.Connection, video_id: str) -> None:
    for table, col in (
        ("favorites", "video_id"),
        ("watch_history", "video_id"),
        ("playlist_videos", "video_id"),
        ("video_comments", "video_id"),
        ("video_reactions", "video_id"),
        ("video_watch_counts", "video_id"),
        ("scrape_failures", "video_id"),
        ("video_categories", "video_id"),
    ):
        conn.execute(f"DELETE FROM {table} WHERE {col} = ?", (video_id,))
    conn.execute("DELETE FROM videos WHERE id = ?", (video_id,))
    conn.commit()


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", default="/opt/karaxxx/karaxxx.db")
    ap.add_argument("--limit", type=int, default=250)
    ap.add_argument("--source", choices=["eporner", "tnaflix", "drtuber", "all"], default="all", help="all intentionally means eporner+tnaflix; drtuber yt-dlp extractor currently returns malformed data")
    ap.add_argument("--timeout", type=int, default=90)
    args = ap.parse_args()

    db_path = Path(args.db)
    if not db_path.exists():
        print(f"DB not found: {db_path}", file=sys.stderr)
        return 2

    conn = sqlite3.connect(str(db_path), timeout=30)
    conn.row_factory = sqlite3.Row
    source_filter = "AND source IN ('eporner','tnaflix')"
    params: list[Any] = []
    if args.source == "drtuber":
        print("DrTuber backfill is disabled: yt-dlp currently returns malformed extractor data for DrTuber.", flush=True)
        return 0
    if args.source != "all":
        source_filter = "AND source = ?"
        params.append(args.source)
    rows = conn.execute(
        f"""
        SELECT v.id, COALESCE(v.source,'') source, COALESCE(v.slug,'') slug, COALESCE(v.title,'') title
        FROM videos v
        LEFT JOIN scrape_failures f ON f.video_id = v.id
        WHERE COALESCE(v.url_360,'') = ''
          AND COALESCE(v.url_720,'') = ''
          AND COALESCE(v.url_1080,'') = ''
          AND COALESCE(v.hls_url,'') = ''
          AND (f.video_id IS NULL OR f.next_retry_at <= strftime('%s','now'))
          {source_filter}
        ORDER BY COALESCE(f.retry_count, 0) ASC, v.added_at DESC
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
            record_failure(conn, row.id, "no reconstructable URL for missing-media backfill")
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
            message = str(exc)
            if "HTTP Error 404" in message:
                prune_dead_unplayable(conn, row.id)
                skipped += 1
                print(f"PRUNE {row.source}:{row.id} dead detail page {url}: {message}", flush=True)
                time.sleep(2)
                continue
            fail += 1
            record_failure(conn, row.id, message)
            print(f"FAIL {row.source}:{row.id} {url}: {message}", flush=True)
            time.sleep(5)
    print(f"Done: ok={ok} fail={fail} skipped={skipped}")
    return 0 if fail == 0 else 1


if __name__ == "__main__":
    raise SystemExit(main())
