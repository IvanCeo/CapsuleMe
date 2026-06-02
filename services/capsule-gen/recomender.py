import logging
import math
import os
import random
import uuid
from dataclasses import dataclass

import grpc
import pandas as pd
from PIL import Image


BASE_DIR = os.path.dirname(os.path.abspath(__file__))

DATA_PATH = os.getenv(
    "CAPSULE_METADATA_PATH",
    os.path.join(BASE_DIR, "metadata_uuid_prod.csv"),
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


@dataclass
class CapsuleResult:
    items: list[dict]
    image_path: str
    looks_count: int = 0


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


def generate_capsule(gender: str, style: str, season: str, palette: str) -> CapsuleResult:
    logging.info(
        "generate capsule: gender=%s style=%s season=%s palette=%s",
        gender,
        style,
        season,
        palette,
    )

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
    selected_paths: list[str] = []

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

    # ВАЖНО ДЛЯ ДЕПЛОЯ:
    # capsule-gen должен быстро вернуть капсулу и склеенную картинку.
    # Тяжёлый ML-скоринг образов через CLIP здесь намеренно отключён.
    looks_count = 0

    combined_path = combine_clothes(selected_paths)
    if not combined_path or not os.path.exists(combined_path):
        raise CapsuleRecommendationError(
            grpc.StatusCode.INTERNAL,
            "failed to combine capsule images",
        )

    logging.info(
        "capsule generated: items=%s image_paths=%s combined=%s",
        len(selected_group),
        len(selected_paths),
        combined_path,
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

    if color == "bright":
        return "bright"

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
