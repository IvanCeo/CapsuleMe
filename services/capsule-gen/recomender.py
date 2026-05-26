import itertools
import logging
import math
import os
import random
import threading
import uuid
from collections import Counter
from dataclasses import dataclass

import grpc
import pandas as pd
from PIL import Image

try:
    from model import OutfitEmbeddingModel
except Exception:
    OutfitEmbeddingModel = None


BASE_DIR = os.path.dirname(os.path.abspath(__file__))

DATA_PATH = os.getenv(
    "CAPSULE_METADATA_PATH",
    os.path.join(BASE_DIR, "metadata_uuid_prod.csv"),
)

OUTFITS_PATH = os.getenv(
    "CAPSULE_OUTFITS_PATH",
    os.path.join(BASE_DIR, "meta_outfits.csv"),
)

IMAGES_FOLDER = os.getenv(
    "CAPSULE_IMAGES_FOLDER",
    os.path.join(BASE_DIR, "images_new"),
)

COMBINED_FOLDER = os.getenv(
    "CAPSULE_COMBINED_FOLDER",
    os.path.join(BASE_DIR, "combined"),
)

os.makedirs(COMBINED_FOLDER, exist_ok=True)
logging.basicConfig(level=logging.INFO)

DATA = pd.read_csv(DATA_PATH)
DF_OUTFITS = pd.read_csv(OUTFITS_PATH) if os.path.exists(OUTFITS_PATH) else pd.DataFrame()

CATEGORY_ORDER = ["top", "bottom", "dress", "outerwear", "footwear", "accessory"]


@dataclass
class CapsuleResult:
    items: list[dict]
    image_path: str
    looks_count: int


class CapsuleRecommendationError(Exception):
    def __init__(self, code: grpc.StatusCode, message: str):
        super().__init__(message)
        self.code = code
        self.message = message

def str_value(value, default: str = "") -> str:
    if value is None:
        return default

    value = str(value).strip()
    if not value or value.lower() == "nan":
        return default

    return value


def normalize_text(value) -> str:
    return str_value(value).lower().strip()


_embedding_lock = threading.Lock()
_embedding_model = None
_reference_embeddings = None


def generate_capsule(gender: str, style: str, season: str, palette: str) -> CapsuleResult:
    filtered = recommend_outfit(
        DATA,
        gender=gender,
        style=style,
        season=season,
        palette=palette,
    )

    if filtered.empty and palette not in ("any", "bright"):
        filtered = recommend_outfit(
            DATA,
            gender=gender,
            style=style,
            season=season,
            palette="any",
        )

    if filtered.empty and palette == "bright":
        filtered = recommend_outfit(
            DATA,
            gender=gender,
            style=style,
            season=season,
            palette="any",
        )

    if filtered.empty:
        raise CapsuleRecommendationError(
            grpc.StatusCode.NOT_FOUND,
            "no items found for requested filters",
        )

    groups = make_capsule_groups(filtered, min_size=8, max_size=12)
    if not groups:
        raise CapsuleRecommendationError(
            grpc.StatusCode.NOT_FOUND,
            "no capsule groups generated",
        )

    selected_group = None
    selected_paths = []

    for group in groups:
        paths = image_paths_for_capsule(group)
        if paths:
            selected_group = group
            selected_paths = paths
            break

    if selected_group is None or not selected_paths:
        raise CapsuleRecommendationError(
            grpc.StatusCode.NOT_FOUND,
            "capsule generated, but no image files found",
        )

    looks_count = 0

    try:
        looks = generate_looks_from_capsule(
            capsule_df=selected_group,
            user_gender=gender,
            templates_by_gender=templates_by_gender,
            embedding_model=get_embedding_model(),
            reference_embeddings=get_reference_embeddings(),
            top_k=6,
        )
        looks_count = len(looks)
    except Exception:
        logging.exception("failed to generate looks")

    combined_path = combine_clothes(selected_paths)
    if not combined_path or not os.path.exists(combined_path):
        raise CapsuleRecommendationError(
            grpc.StatusCode.INTERNAL,
            "failed to combine capsule images",
        )

    return CapsuleResult(
        items=selected_group.to_dict("records"),
        image_path=combined_path,
        looks_count=looks_count,
    )


def recommend_outfit(df: pd.DataFrame, gender: str, style: str, season: str, palette: str) -> pd.DataFrame:
    df = df.copy()

    gender = normalize_text(gender)
    style = normalize_text(style)
    season = normalize_text(season)
    palette = normalize_text(palette) or "any"

    df["gender"] = df["gender"].fillna("").astype(str).str.lower().str.strip()
    df["style"] = df["style"].fillna("").astype(str).str.lower().str.strip()
    df["season"] = df["season"].fillna("all-seasons").astype(str).str.lower().str.strip()
    df["color"] = df["color"].fillna("").astype(str).str.lower().str.strip()

    df = df[(df["gender"] == gender) | (df["gender"] == "unisex")]
    df = df[df["style"] == style]

    season_map = {
        "winter": {"winter", "all-seasons"},
        "autumn": {"autumn", "demi-season", "demi-seasons", "all-seasons"},
        "spring": {"spring", "demi-season", "demi-seasons", "all-seasons"},
        "summer": {"summer", "all-seasons"},
    }

    allowed_seasons = season_map.get(season)
    if allowed_seasons:
        df = df[df["season"].isin(allowed_seasons)]

    if palette and palette != "any":
        df = df[df["color"].apply(detect_palette) == palette]

    return df.reset_index(drop=True)


def detect_palette(color: str) -> str:
    color = normalize_text(color)

    if color == "dark":
        return "dark"

    if color == "light":
        return "light"

    return "neutral"


def make_capsule_groups(df_filtered: pd.DataFrame, min_size: int = 8, max_size: int = 12) -> list[pd.DataFrame]:
    groups = []

    items = df_filtered.to_dict("records")
    random.shuffle(items)

    category_map = {}
    for item in items:
        category = str(item.get("category") or item.get("category_group") or "unknown")
        category_map.setdefault(category, []).append(item)

    used_ids = set()

    while True:
        capsule = []
        category_count = {}

        for category, category_items in category_map.items():
            if len(capsule) >= max_size:
                break

            for item in category_items:
                item_id = item_key(item)
                if item_id not in used_ids:
                    capsule.append(item)
                    used_ids.add(item_id)
                    category_count[category] = 1
                    break

        if len(capsule) < min_size:
            for category, category_items in category_map.items():
                if len(capsule) >= min_size or len(capsule) >= max_size:
                    break

                if category_count.get(category, 0) >= 2:
                    continue

                for item in category_items:
                    item_id = item_key(item)
                    if item_id not in used_ids:
                        capsule.append(item)
                        used_ids.add(item_id)
                        category_count[category] = category_count.get(category, 0) + 1
                        break

        if len(capsule) < min_size:
            for item in items:
                if len(capsule) >= min_size or len(capsule) >= max_size:
                    break

                item_id = item_key(item)
                if item_id not in used_ids:
                    capsule.append(item)
                    used_ids.add(item_id)

        if not capsule:
            break

        groups.append(pd.DataFrame(capsule[:max_size]))

        if len(used_ids) >= len(items):
            break

    return groups


def item_key(item: dict) -> str:
    return str(item.get("uuid") or item.get("id") or item.get("article") or uuid.uuid4())


def extract_templates_by_gender(df: pd.DataFrame) -> dict[str, Counter]:
    templates = {"male": Counter(), "female": Counter()}

    if df.empty:
        return templates

    for (_, gender), group in df.groupby(["outfit", "gender"]):
        gender = normalize_text(gender)
        if gender not in templates:
            continue

        template = tuple(
            sorted(
                str(v).strip().lower()
                for v in group["category_group"].dropna().unique()
                if str(v).strip()
            )
        )

        if template:
            templates[gender][template] += 1

    return templates


def filter_templates(templates_counter: Counter, min_count: int) -> list[tuple[str, ...]]:
    return [template for template, count in templates_counter.items() if count >= min_count]


def build_templates_by_gender() -> dict[str, list[tuple[str, ...]]]:
    counters = extract_templates_by_gender(DF_OUTFITS)

    male = filter_templates(counters["male"], min_count=6)
    female = filter_templates(counters["female"], min_count=4)

    if not male:
        male = [
            ("top", "bottom", "footwear"),
            ("top", "bottom", "outerwear", "footwear"),
        ]

    if not female:
        female = [
            ("top", "bottom", "footwear"),
            ("top", "bottom", "outerwear", "footwear"),
            ("dress", "footwear"),
        ]

    return {"male": male, "female": female}


templates_by_gender = build_templates_by_gender()


def get_embedding_model():
    global _embedding_model

    if OutfitEmbeddingModel is None:
        return None

    if _embedding_model is not None:
        return _embedding_model

    with _embedding_lock:
        if _embedding_model is None:
            _embedding_model = OutfitEmbeddingModel()

    return _embedding_model


def get_reference_embeddings() -> list[dict]:
    global _reference_embeddings

    if _reference_embeddings is not None:
        return _reference_embeddings

    model = get_embedding_model()
    if model is None or DF_OUTFITS.empty:
        _reference_embeddings = []
        return _reference_embeddings

    refs = []

    for outfit_id, group in DF_OUTFITS.groupby("outfit"):
        group = group.copy()
        group["path"] = group.apply(resolve_reference_image_path, axis=1)
        group = group[group["path"].apply(lambda p: bool(p) and os.path.exists(p))]

        if group.empty:
            continue

        try:
            emb = model.outfit_embedding(group)
            refs.append({"outfit": outfit_id, "embedding": emb})
        except Exception:
            logging.exception("failed to build reference embedding for outfit=%s", outfit_id)

    _reference_embeddings = refs
    return _reference_embeddings


def generate_looks_from_capsule(
    capsule_df: pd.DataFrame,
    user_gender: str,
    templates_by_gender: dict[str, list[tuple[str, ...]]],
    embedding_model,
    reference_embeddings: list[dict],
    top_k: int = 6,
) -> list[pd.DataFrame]:
    templates = templates_by_gender.get(user_gender) or []
    candidates = []

    for template in templates:
        pools = []

        for group_name in template:
            pool = capsule_df[capsule_df["category_group"].astype(str).str.lower() == group_name]

            if pool.empty:
                break

            pools.append(pool.to_dict("records"))
        else:
            for combo in itertools.product(*pools):
                df_combo = pd.DataFrame(combo)
                df_combo["path"] = df_combo.apply(resolve_image_path, axis=1)
                df_combo = df_combo[df_combo["path"].apply(lambda p: bool(p) and os.path.exists(p))]

                if not df_combo.empty:
                    candidates.append(df_combo)

    if not candidates:
        return []

    scored = []

    if embedding_model is not None and reference_embeddings:
        for outfit_df in candidates:
            try:
                score = embedding_model.score_outfit(outfit_df, reference_embeddings)
                scored.append((score, outfit_df))
            except Exception:
                logging.exception("failed to score outfit")
    else:
        scored = [(0.0, outfit_df) for outfit_df in candidates]

    scored.sort(key=lambda x: x[0], reverse=True)

    return select_diverse_looks(scored, capsule_df, top_k=top_k)


def select_diverse_looks(
    scored_outfits: list[tuple[float, pd.DataFrame]],
    capsule_df: pd.DataFrame,
    top_k: int = 6,
) -> list[pd.DataFrame]:
    capsule_items = set(capsule_df["uuid"].astype(str)) if "uuid" in capsule_df.columns else set()
    target_looks = max(4, math.ceil(len(capsule_items) / 2)) if capsule_items else top_k
    target_looks = min(target_looks, top_k)

    selected = []
    selected_keys = set()
    covered_items = set()

    for _, outfit_df in scored_outfits:
        outfit_items = set(outfit_df["uuid"].astype(str)) if "uuid" in outfit_df.columns else set()
        key = tuple(sorted(outfit_items))

        if key in selected_keys:
            continue

        if not outfit_items or not outfit_items.issubset(covered_items):
            selected.append(outfit_df)
            selected_keys.add(key)
            covered_items.update(outfit_items)

        if len(selected) >= target_looks and (not capsule_items or covered_items >= capsule_items):
            break

    if len(selected) < target_looks:
        for _, outfit_df in scored_outfits:
            outfit_items = set(outfit_df["uuid"].astype(str)) if "uuid" in outfit_df.columns else set()
            key = tuple(sorted(outfit_items))

            if key in selected_keys:
                continue

            selected.append(outfit_df)
            selected_keys.add(key)

            if len(selected) >= target_looks:
                break

    return selected


def image_paths_for_capsule(capsule_df: pd.DataFrame) -> list[str]:
    paths = []

    for _, row in capsule_df.iterrows():
        path = resolve_image_path(row)

        if path and os.path.exists(path):
            paths.append(path)

    return paths


def resolve_image_path(row) -> str:
    item_uuid = str_value(row.get("uuid"))
    ext = str_value(row.get("ext"), "webp").lstrip(".")

    if item_uuid:
        path = os.path.join(IMAGES_FOLDER, f"{item_uuid}.{ext}")
        if os.path.exists(path):
            return path

        for fallback_ext in ("webp", "jpg", "jpeg", "png"):
            fallback = os.path.join(IMAGES_FOLDER, f"{item_uuid}.{fallback_ext}")
            if os.path.exists(fallback):
                return fallback

    csv_path = str_value(row.get("path"))
    if csv_path:
        if os.path.isabs(csv_path) and os.path.exists(csv_path):
            return csv_path

        path = os.path.join(BASE_DIR, csv_path)
        if os.path.exists(path):
            return path

    return ""


def resolve_reference_image_path(row) -> str:
    csv_path = str_value(row.get("path"))

    if csv_path:
        if os.path.isabs(csv_path) and os.path.exists(csv_path):
            return csv_path

        path = os.path.join(BASE_DIR, csv_path)
        if os.path.exists(path):
            return path

    return resolve_image_path(row)


def combine_clothes(image_paths: list[str]) -> str:
    images = []

    for path in image_paths:
        try:
            img = Image.open(path).convert("RGB")
            img = img.resize((300, 300))
            images.append(img)
        except Exception as e:
            logging.warning("skip image %s: %s", path, e)

    if not images:
        return ""

    cols = min(3, len(images))
    rows = math.ceil(len(images) / cols)
    cell_w = 300
    cell_h = 300

    canvas = Image.new("RGB", (cols * cell_w, rows * cell_h), "white")

    for i, img in enumerate(images):
        x = (i % cols) * cell_w
        y = (i // cols) * cell_h
        canvas.paste(img, (x, y))

    out_path = os.path.join(COMBINED_FOLDER, f"{uuid.uuid4().hex}.jpg")
    canvas.save(out_path, format="JPEG", quality=95)

    return out_path
