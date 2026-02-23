#!/usr/bin/env python3
# -*- coding: utf-8 -*-

"""
Dataset cleaner & normalizer.

Step 1 (default):
- Reads raw CSV with columns:
  path,gender,category,style,color,season,material,description,error
- Validates required fields: gender, category, style
- Normalizes paths, checks file existence & readability
- Computes image size (w,h) and sha256 for deduplication
- Drops column `error`
- Writes:
  ./data/work/metadata_valid.csv
  ./data/work/metadata_rejected.csv
  ./data/work/path_map.csv
  ./data/work/report.json

Step 2 (optional via flags --flatten --assign-uuid):
- Copies valid unique images into ./data/clean/images/
- Renames as <uuid>.<ext>
- Writes ./data/clean/metadata_uuid.csv (path replaced by uuid/ext)
- Writes ./data/clean/map_uuid.csv (provisional_id -> uuid, sha256 -> uuid)

Usage examples:

  # Step 1 only (clean & validate)
  python cleanup_dataset.py \
    --images-root ./data/pics \
    --csv ./data/dataset_1k.csv

  # Step 1 + Step 2 (flatten + assign UUIDs)
  python cleanup_dataset.py \
    --images-root ./data/pics \
    --csv ./data/dataset_1k.csv \
    --flatten --assign-uuid

Requirements: pandas, pillow, tqdm
"""

import argparse
import csv
import hashlib
import io
import json
import os
import shutil
import sys
import uuid
from pathlib import Path
from typing import Dict, Tuple, Optional

import pandas as pd
from PIL import Image, UnidentifiedImageError
from tqdm import tqdm


REQUIRED_COLS = ["gender", "category", "style"]
ALL_INPUT_COLS = [
    "path", "gender", "category", "style", "color",
    "season", "material", "description", "error",
]

# Normalization maps (extend as needed)
GENDER_MAP = {
    "male": "male",
    "man": "male",
    "men": "male",
    "m": "male",
    "female": "female",
    "woman": "female",
    "women": "female",
    "f": "female",
    "unisex": "unisex",
    "uni": "unisex",
}
# For other columns we'll just trim + lower; you can add maps if needed.


def safe_lower(x: Optional[str]) -> str:
    if pd.isna(x):
        return ""
    return str(x).strip().lower()


def normalize_gender(val: str) -> str:
    v = safe_lower(val)
    return GENDER_MAP.get(v, v)  # keep as-is if unknown


def normalize_text(val: str) -> str:
    # generic normalizer for category/style/color/season/material
    return safe_lower(val)


def normalize_path(raw: str) -> str:
    # Normalize incoming "path" values (handle backslashes from CSV)
    p = str(raw).strip()
    # standardize slashes
    p = p.replace("\\", "/")
    # remove leading "./"
    if p.startswith("./"):
        p = p[2:]
    # collapse duplicate slashes
    while "//" in p:
        p = p.replace("//", "/")
    return p


def file_sha256(fp: Path, chunk_size: int = 1024 * 1024) -> str:
    h = hashlib.sha256()
    with fp.open("rb") as f:
        while True:
            b = f.read(chunk_size)
            if not b:
                break
            h.update(b)
    return h.hexdigest()


def check_image(fp: Path) -> Tuple[int, int]:
    """
    Returns (width, height) if image can be opened.
    Raises UnidentifiedImageError or OSError if unreadable.
    """
    with Image.open(fp) as im:
        im.verify()  # quick header check
    # re-open to get size (verify() invalidates the file pointer for some drivers)
    with Image.open(fp) as im2:
        w, h = im2.size
    return w, h


def ensure_dir(p: Path):
    p.mkdir(parents=True, exist_ok=True)


def uuid5_from_text(text: str) -> str:
    # Deterministic UUID v5 based on text
    return str(uuid.uuid5(uuid.NAMESPACE_URL, text))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--images-root", required=True, help="Base folder with images (e.g., ./data/pics)")
    ap.add_argument("--csv", required=True, help="Path to raw CSV (e.g., ./data/dataset_1k.csv)")
    ap.add_argument("--work-dir", default="./data/work", help="Output work directory for step 1")
    ap.add_argument("--clean-dir", default="./data/clean", help="Output directory for step 2 (flattened)")
    ap.add_argument("--flatten", action="store_true", help="Copy valid images into a single folder (step 2)")
    ap.add_argument("--assign-uuid", action="store_true", help="Assign final UUIDs and write metadata_uuid.csv (step 2)")
    args = ap.parse_args()

    images_root = Path(args.images_root).resolve()
    csv_path = Path(args.csv).resolve()
    work_dir = Path(args.work_dir).resolve()
    clean_dir = Path(args.clean_dir).resolve()

    if not csv_path.exists():
        print(f"[ERROR] CSV not found: {csv_path}", file=sys.stderr)
        sys.exit(1)
    if not images_root.exists():
        print(f"[ERROR] Images root not found: {images_root}", file=sys.stderr)
        sys.exit(1)

    ensure_dir(work_dir)

    # --- Load CSV
    try:
        df = pd.read_csv(csv_path, encoding="utf-8-sig")
    except UnicodeDecodeError:
        # fallback
        df = pd.read_csv(csv_path, encoding="utf-8")

    # Validate columns presence
    missing_cols = [c for c in ["path"] + REQUIRED_COLS if c not in df.columns]
    if missing_cols:
        print(f"[ERROR] Missing required columns in CSV: {missing_cols}", file=sys.stderr)
        sys.exit(1)

    # Keep only known columns + pass-through extras (except 'error' to drop later)
    cols_to_keep = [c for c in df.columns if c in ALL_INPUT_COLS or c not in ["error"]]
    df = df[cols_to_keep].copy()

    # Normalize text columns
    df["path"] = df["path"].astype(str).map(normalize_path)
    for c in ["gender", "category", "style", "color", "season", "material", "description"]:
        if c in df.columns:
            if c == "gender":
                df[c] = df[c].map(normalize_gender)
            else:
                df[c] = df[c].map(normalize_text)

    # Drop 'error' column if present
    if "error" in df.columns:
        df = df.drop(columns=["error"])

    total_rows = len(df)

    # Build absolute file paths
    abs_paths = []
    for p in df["path"]:
        # If CSV paths are relative to a specific subfolder under images_root,
        # we try images_root / p; if already absolute, keep it.
        pp = Path(p)
        if not pp.is_absolute():
            pp = images_root / p
        abs_paths.append(str(pp.resolve()))
    df["_abs_path"] = abs_paths

    # Required fields non-empty check
    required_ok = (df["gender"].astype(str).str.len() > 0) & \
                  (df["category"].astype(str).str.len() > 0) & \
                  (df["style"].astype(str).str.len() > 0)

    # Existence + readability + metadata
    exists_ok = []
    widths = []
    heights = []
    shas = []
    reasons_missing = 0
    reasons_unread = 0

    print("[INFO] Checking files existence & readability, computing sha256 ...")
    for apath in tqdm(df["_abs_path"], total=len(df)):
        fp = Path(apath)
        if not fp.exists() or not fp.is_file():
            exists_ok.append(False)
            widths.append(None)
            heights.append(None)
            shas.append(None)
            reasons_missing += 1
            continue

        try:
            w, h = check_image(fp)
            sha = file_sha256(fp)
            exists_ok.append(True)
            widths.append(w)
            heights.append(h)
            shas.append(sha)
        except (UnidentifiedImageError, OSError):
            exists_ok.append(False)
            widths.append(None)
            heights.append(None)
            shas.append(None)
            reasons_unread += 1

    df["_exists_ok"] = exists_ok
    df["_width"] = widths
    df["_height"] = heights
    df["_sha256"] = shas

    prov_ids = []
    for row in df.itertuples(index=False):
        # pandas itertuples strips leading underscores from column names
        sha = getattr(row, "sha256", None)
        path_norm = getattr(row, "path")
        base_text = sha if pd.notna(sha) and sha else path_norm
        prov_ids.append(uuid5_from_text(base_text))
    df["provisional_id"] = prov_ids


    # Duplicate detection by content (sha256)
    df["_dup_sha"] = False
    if df["_sha256"].notna().any():
        df["_dup_sha"] = df.duplicated(subset=["_sha256"], keep="first")

    # Valid rows: required fields ok, file exists & readable, not a duplicate by sha
    valid_mask = required_ok & df["_exists_ok"] & (~df["_dup_sha"])
    df_valid = df[valid_mask].copy()
    df_rej = df[~valid_mask].copy()

        # Build rejection reasons (safe access via DataFrame indices)
    reasons = []
    for i in range(len(df_rej)):
        row = df_rej.iloc[i]
        r_reasons = []

        if not bool(row["_exists_ok"]):
            if pd.isna(row["_sha256"]) and pd.isna(row["_width"]):
                r_reasons.append("missing_or_unreadable_file")

        if not (isinstance(row["gender"], str) and len(row["gender"]) > 0):
            r_reasons.append("empty_required_field:gender")
        if not (isinstance(row["category"], str) and len(row["category"]) > 0):
            r_reasons.append("empty_required_field:category")
        if not (isinstance(row["style"], str) and len(row["style"]) > 0):
            r_reasons.append("empty_required_field:style")

        if "_dup_sha" in row and bool(row["_dup_sha"]):
            r_reasons.append("duplicate_content")
        if "_dup_path" in row and bool(row["_dup_path"]):
            r_reasons.append("duplicate_row_by_path")

        reasons.append(";".join(sorted(set(r_reasons))) or "unknown")

    df_rej["reason"] = reasons

    # Compose path_map.csv
    path_map_cols = ["path", "provisional_id", "_abs_path", "_sha256", "_width", "_height"]
    path_map = df_valid[path_map_cols].copy()
    # Extract extension for later use
    exts = []
    sizes = []
    for apath in path_map["_abs_path"]:
        fp = Path(apath)
        exts.append(fp.suffix.lower().lstrip("."))
        try:
            sizes.append(fp.stat().st_size)
        except OSError:
            sizes.append(None)
    path_map["ext"] = exts
    path_map["size_bytes"] = sizes

    # Prepare outputs
    ensure_dir(work_dir)
    valid_out = work_dir / "metadata_valid.csv"
    rej_out = work_dir / "metadata_rejected.csv"
    map_out = work_dir / "path_map.csv"
    report_out = work_dir / "report.json"

    # Drop internal columns in valid CSV
    drop_internal = ["_abs_path", "_exists_ok", "_width", "_height", "_sha256", "_dup_path", "_dup_sha"]
    df_valid_to_save = df_valid.drop(columns=[c for c in drop_internal if c in df_valid.columns])
    # Ensure column order (provisional_id near front)
    front_cols = ["provisional_id", "path", "gender", "category", "style"]
    other_cols = [c for c in df_valid_to_save.columns if c not in front_cols]
    df_valid_to_save = df_valid_to_save[front_cols + other_cols]

    # Save outputs
    df_valid_to_save.to_csv(valid_out, index=False)
    df_rej[["path", "reason"]].to_csv(rej_out, index=False)
    path_map.to_csv(map_out, index=False)

    # Report
    dup_path_count = int(df["_dup_path"].sum()) if "_dup_path" in df.columns else 0
    dup_sha_count = int(df["_dup_sha"].sum()) if "_dup_sha" in df.columns else 0

    report = {
        "rows_total": total_rows,
        "rows_valid": int(len(df_valid)),
        "rows_rejected": int(len(df_rej)),
        "reasons_missing_or_unreadable_files": int(reasons_missing + reasons_unread),
        "duplicate_by_path": dup_path_count,
        "duplicate_by_content": dup_sha_count,
        "images_root": str(images_root),
        "csv": str(csv_path),
        "work_dir": str(work_dir),
    }

    with open(report_out, "w", encoding="utf-8") as f:
        json.dump(report, f, ensure_ascii=False, indent=2)

    print(f"[OK] Step 1 done.")
    print(f"  Valid CSV:     {valid_out}")
    print(f"  Rejected CSV:  {rej_out}")
    print(f"  Path map:      {map_out}")
    print(f"  Report:        {report_out}")

    # --- Optional Step 2: flatten + assign UUIDs
    if args.flatten or args.assign_uuid:
        print("[INFO] Step 2 requested: flatten & assign UUIDs ...")
        images_out = clean_dir / "images"
        ensure_dir(images_out)
        ensure_dir(clean_dir)

        # Load from path_map & valid csv (ensure we use the same filtered set)
        df_valid = pd.read_csv(valid_out)
        path_map = pd.read_csv(map_out)

        # Join valid metadata with path_map (safe version)
        pm = path_map[["path", "_abs_path", "ext", "provisional_id", "_sha256"]].copy()
        pm.rename(
            columns={
                "_abs_path": "abs_path",
                "provisional_id": "provisional_id_pm",
                "_sha256": "sha256_pm",
                "ext": "ext_pm",
            },
            inplace=True,
        )

        merged = df_valid.merge(pm, on="path", how="left")

        # Assign final UUIDs (deterministic from sha256 if present)
        final_uuids = []
        for row in merged.itertuples(index=False):
            sha = getattr(row, "sha256_pm", None)
            pid = getattr(row, "provisional_id_pm", None)
            basis = sha if isinstance(sha, str) and len(sha) > 0 else pid
            final_uuids.append(str(uuid.uuid5(uuid.NAMESPACE_OID, basis)))
        merged["uuid"] = final_uuids

        # Copy files into clean/images as <uuid>.<ext>
        print("[INFO] Copying files into a flat folder ...")
        copied = 0
        for row in tqdm(merged.itertuples(index=False), total=len(merged)):
            src = Path(getattr(row, "abs_path"))
            ext = getattr(row, "ext_pm") or "jpg"
            uid = getattr(row, "uuid")
            dst = images_out / f"{uid}.{ext}"
            if not dst.exists():
                try:
                    shutil.copy2(src, dst)
                    copied += 1
                except Exception as e:
                    merged.loc[merged["uuid"] == uid, "uuid"] = ""
                    print(f"[WARN] Failed to copy {src} -> {dst}: {e}", file=sys.stderr)

        # Write clean metadata with uuid+ext (replace path)
        # Keep column order friendly
        meta_cols_front = ["uuid", "ext_pm", "gender", "category", "style"]
        meta_other = [c for c in ["color", "season", "material", "description"] if c in merged.columns]
        meta_df = merged[meta_cols_front + meta_other].copy()
        meta_df.rename(columns={"ext_pm": "ext"}, inplace=True)  # normalize column name
        # Filter rows that copied successfully (uuid not empty)
        meta_df = meta_df[meta_df["uuid"].astype(str).str.len() > 0]
        meta_out = clean_dir / "metadata_uuid.csv"
        meta_df.to_csv(meta_out, index=False)

        # Map files for reference
        map_uuid_cols = [
            "path",
            "uuid",
            "ext_pm",
            "provisional_id_pm",
            "sha256_pm",
        ]
        existing = [c for c in map_uuid_cols if c in merged.columns]
        map_uuid = merged[existing].copy()
        # привести имена к нормальному виду
        map_uuid.rename(
            columns={
                "ext_pm": "ext",
                "provisional_id_pm": "provisional_id",
                "sha256_pm": "_sha256",
            },
            inplace=True,
        )
        map_uuid_out = clean_dir / "map_uuid.csv"
        map_uuid.to_csv(map_uuid_out, index=False)


        print(f"[OK] Step 2 done.")
        print(f"  Images folder: {images_out}")
        print(f"  Clean CSV:     {meta_out}")
        print(f"  UUID map:      {map_uuid_out}")
        print(f"  Copied files:  {copied}/{len(merged)}")


if __name__ == "__main__":
    main()
