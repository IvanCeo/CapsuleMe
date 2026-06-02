import itertools
import logging
import math
import os
import random
import uuid
from collections import Counter
from dataclasses import dataclass

import grpc
import pandas as pd
from PIL import Image

BASE_DIR = os.path.dirname(os.path.abspath(__file__))

OUTFITS_PATH = os.getenv(
    "LOOK_GEN_OUTFITS_PATH",
    os.path.join(BASE_DIR, "meta_outfits.csv"),
)

IMAGES_FOLDER = os.getenv(
    "LOOK_GEN_IMAGES_FOLDER",
    os.path.join(BASE_DIR, "images_new"),
)

COMBINED_FOLDER = os.getenv(
    "LOOK_GEN_COMBINED_FOLDER",
    os.path.join(BASE_DIR, "combined"),
)

# Для Docker/MVP по умолчанию НЕ грузим CLIP/torch/transformers.
# Включать только явно: LOOK_GEN_USE_CLIP=1.
USE_CLIP = os.getenv("LOOK_GEN_USE_CLIP", "0") == "1"
MAX_CANDIDATES = int(os.getenv("LOOK_GEN_MAX_CANDIDATES", "200"))

os.makedirs(COMBINED_FOLDER, exist_ok=True)

logging.basicConfig(level=logging.INFO)

DF_OUTFITS = pd.read_csv(OUTFITS_PATH) if os.path.exists(OUTFITS_PATH) else pd.DataFrame()

_embedding_model = None
_reference_embeddings = None


@dataclass
class LookGenItem:
    proto: object
    id: str
    gender: str
    category_group: str
    category: str
    style: str
    color: str
    season: str
    material: str
    description: str
    ext: str
    path: str


@dataclass
class GeneratedLook:
    id: str
    items: list[LookGenItem]
    score: float
    template: str


class LookGenError(Exception):
    def __init__(self, code: grpc.StatusCode, message: str):
        super().__init__(message)
        self.code = code
        self.message = message


def generate_looks(request) -> list[GeneratedLook]:
    capsule_items = capsule_to_items(request.capsule)

    if not capsule_items:
        raise LookGenError(
            grpc.StatusCode.INVALID_ARGUMENT,
            "capsule has no items",
        )

    options = request.options

    max_looks = request.max_looks if request.max_looks > 0 else 6

    min_items = 2
    max_items = 4
    use_ml_scoring = True
    require_diverse_items = True

    if options is not None:
        if options.min_items_per_look > 0:
            min_items = options.min_items_per_look

        if options.max_items_per_look > 0:
            max_items = options.max_items_per_look

        use_ml_scoring = options.use_ml_scoring
        require_diverse_items = options.require_diverse_items

    # Даже если Go передал use_ml_scoring=true, в deploy-режиме CLIP выключен
    # и скоринг будет эвристическим.
    if not USE_CLIP:
        use_ml_scoring = False

    logging.info(
        "generate looks: capsule_items=%s max_looks=%s min_items=%s max_items=%s use_clip=%s",
        len(capsule_items),
        max_looks,
        min_items,
        max_items,
        USE_CLIP,
    )

    user_gender = detect_capsule_gender(capsule_items)

    templates = get_templates_for_gender(user_gender)
    candidates = build_candidates_from_templates(
        capsule_items=capsule_items,
        templates=templates,
        min_items=min_items,
        max_items=max_items,
    )

    if not candidates:
        candidates = build_fallback_candidates(
            capsule_items=capsule_items,
            min_items=min_items,
            max_items=max_items,
        )

    if not candidates:
        raise LookGenError(
            grpc.StatusCode.NOT_FOUND,
            "no looks generated from capsule",
        )

    if len(candidates) > MAX_CANDIDATES:
        random.shuffle(candidates)
        candidates = candidates[:MAX_CANDIDATES]

    logging.info("look candidates built: %s", len(candidates))

    scored = []

    for look in candidates:
        score = score_look(look, use_ml_scoring=use_ml_scoring)
        scored.append(
            GeneratedLook(
                id=look.id,
                items=look.items,
                score=score,
                template=look.template,
            )
        )

    scored.sort(key=lambda x: x.score, reverse=True)

    if require_diverse_items:
        selected = select_diverse_looks(scored, max_looks=max_looks)
    else:
        selected = scored[:max_looks]

    logging.info("looks generated: %s", len(selected))
    return selected


def capsule_to_items(capsule) -> list[LookGenItem]:
    result = []

    for item in capsule.item:
        item_id = str_value(item.id) or str_value(item.object_id)
        ext = normalize_ext(str_value(item.ext))

        path = resolve_image_path(item_id=item_id, ext=ext)

        result.append(
            LookGenItem(
                proto=item,
                id=item_id,
                gender=gender_to_str(item.gender),
                category_group=normalize_text(item.category_group),
                category=normalize_text(item.category),
                style=style_to_str(item.style),
                color=normalize_text(item.color),
                season=season_to_str(item.season),
                material=normalize_text(item.material),
                description=str_value(item.description),
                ext=ext,
                path=path,
            )
        )

    return result


def detect_capsule_gender(items: list[LookGenItem]) -> str:
    genders = [item.gender for item in items if item.gender]

    if "female" in genders:
        return "female"

    if "male" in genders:
        return "male"

    return "female"


def build_candidates_from_templates(
    capsule_items: list[LookGenItem],
    templates: list[tuple[str, ...]],
    min_items: int,
    max_items: int,
) -> list[GeneratedLook]:
    result = []

    by_group = {}

    for item in capsule_items:
        if not item.category_group:
            continue

        by_group.setdefault(item.category_group, []).append(item)

    for template in templates:
        template = tuple(group for group in template if group)

        if len(template) < min_items:
            continue

        if len(template) > max_items:
            continue

        pools = []

        for group in template:
            pool = by_group.get(group, [])

            if not pool:
                break

            pools.append(pool)
        else:
            for combo in itertools.product(*pools):
                unique_items = unique_by_id(list(combo))

                if len(unique_items) < min_items:
                    continue

                if len(unique_items) > max_items:
                    continue

                result.append(
                    GeneratedLook(
                        id=str(uuid.uuid4()),
                        items=unique_items,
                        score=0.0,
                        template="+".join(template),
                    )
                )

                if len(result) >= MAX_CANDIDATES:
                    return deduplicate_looks(result)

    return deduplicate_looks(result)


def build_fallback_candidates(
    capsule_items: list[LookGenItem],
    min_items: int,
    max_items: int,
) -> list[GeneratedLook]:
    result = []

    priority = [
        "top",
        "bottom",
        "dress",
        "outerwear",
        "footwear",
        "accessory",
    ]

    by_group = {}

    for item in capsule_items:
        by_group.setdefault(item.category_group, []).append(item)

    fallback_templates = [
        ("top", "bottom", "footwear"),
        ("top", "bottom", "outerwear", "footwear"),
        ("dress", "footwear"),
        ("dress", "outerwear", "footwear"),
        ("top", "bottom", "footwear", "accessory"),
    ]

    for template in fallback_templates:
        if len(template) < min_items or len(template) > max_items:
            continue

        pools = []

        for group in template:
            pool = by_group.get(group, [])

            if not pool:
                break

            pools.append(pool)
        else:
            for combo in itertools.product(*pools):
                unique_items = unique_by_id(list(combo))

                if len(unique_items) < min_items:
                    continue

                if len(unique_items) > max_items:
                    continue

                result.append(
                    GeneratedLook(
                        id=str(uuid.uuid4()),
                        items=unique_items,
                        score=0.0,
                        template="+".join(template),
                    )
                )

                if len(result) >= MAX_CANDIDATES:
                    return deduplicate_looks(result)

    if result:
        return deduplicate_looks(result)

    shuffled = capsule_items[:]
    random.shuffle(shuffled)

    for size in range(max_items, min_items - 1, -1):
        for combo in itertools.combinations(shuffled, size):
            unique_items = unique_by_id(list(combo))

            groups = [item.category_group for item in unique_items]
            groups_score = sum(1 for group in priority if group in groups)

            if groups_score <= 1:
                continue

            result.append(
                GeneratedLook(
                    id=str(uuid.uuid4()),
                    items=unique_items,
                    score=0.0,
                    template="+".join(groups),
                )
            )

            if len(result) >= MAX_CANDIDATES:
                return deduplicate_looks(result)

    return deduplicate_looks(result)


def score_look(look: GeneratedLook, use_ml_scoring: bool) -> float:
    if not use_ml_scoring:
        return heuristic_score(look)

    if not USE_CLIP:
        return heuristic_score(look)

    if not all(item.path and os.path.exists(item.path) for item in look.items):
        return heuristic_score(look)

    refs = get_reference_embeddings()

    if not refs:
        return heuristic_score(look)

    model = get_embedding_model()

    rows = []

    for item in look.items:
        rows.append(
            {
                "uuid": item.id,
                "category_group": item.category_group,
                "category": item.category,
                "gender": item.gender,
                "style": item.style,
                "color": item.color,
                "season": item.season,
                "material": item.material,
                "description": item.description,
                "ext": item.ext,
                "path": item.path,
            }
        )

    try:
        df = pd.DataFrame(rows)
        return float(model.score_outfit(df, refs))
    except Exception:
        logging.exception("failed to score look")
        return heuristic_score(look)


def heuristic_score(look: GeneratedLook) -> float:
    groups = {item.category_group for item in look.items if item.category_group}

    score = 0.0

    if "top" in groups:
        score += 0.2

    if "bottom" in groups:
        score += 0.2

    if "dress" in groups:
        score += 0.25

    if "footwear" in groups:
        score += 0.25

    if "outerwear" in groups:
        score += 0.15

    if "accessory" in groups:
        score += 0.05

    score += min(len(groups), 4) * 0.05

    return score


def select_diverse_looks(looks: list[GeneratedLook], max_looks: int) -> list[GeneratedLook]:
    selected = []
    used_keys = set()
    covered_items = set()

    for look in looks:
        key = tuple(sorted(item.id for item in look.items))

        if key in used_keys:
            continue

        look_item_ids = {item.id for item in look.items}

        if not look_item_ids.issubset(covered_items):
            selected.append(look)
            used_keys.add(key)
            covered_items.update(look_item_ids)

        if len(selected) >= max_looks:
            break

    if len(selected) < max_looks:
        for look in looks:
            key = tuple(sorted(item.id for item in look.items))

            if key in used_keys:
                continue

            selected.append(look)
            used_keys.add(key)

            if len(selected) >= max_looks:
                break

    return selected


def combine_look_images(look: GeneratedLook) -> str:
    image_paths = [
        item.path
        for item in look.items
        if item.path and os.path.exists(item.path)
    ]

    if not image_paths:
        return ""

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

    out_path = os.path.join(COMBINED_FOLDER, f"{look.id}.jpg")
    canvas.save(out_path, format="JPEG", quality=95)

    return out_path


def get_templates_for_gender(gender: str) -> list[tuple[str, ...]]:
    templates_by_gender = build_templates_by_gender()

    templates = templates_by_gender.get(gender, [])

    if templates:
        return templates

    return [
        ("top", "bottom", "footwear"),
        ("top", "bottom", "outerwear", "footwear"),
        ("dress", "footwear"),
        ("dress", "outerwear", "footwear"),
    ]


def build_templates_by_gender() -> dict[str, list[tuple[str, ...]]]:
    counters = extract_templates_by_gender(DF_OUTFITS)

    male = filter_templates(counters["male"], min_count=3)
    female = filter_templates(counters["female"], min_count=3)

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
            ("dress", "outerwear", "footwear"),
        ]

    return {
        "male": male,
        "female": female,
    }


def extract_templates_by_gender(df: pd.DataFrame) -> dict[str, Counter]:
    result = {
        "male": Counter(),
        "female": Counter(),
    }

    if df.empty:
        return result

    required = {"outfit", "gender", "category_group"}
    if not required.issubset(set(df.columns)):
        return result

    for (_, gender), group in df.groupby(["outfit", "gender"]):
        gender = normalize_text(gender)

        if gender not in result:
            continue

        template = tuple(
            sorted(
                normalize_text(value)
                for value in group["category_group"].dropna().unique()
                if normalize_text(value)
            )
        )

        if template:
            result[gender][template] += 1

    return result


def filter_templates(counter: Counter, min_count: int) -> list[tuple[str, ...]]:
    return [
        template
        for template, count in counter.items()
        if count >= min_count
    ]


def get_embedding_model():
    global _embedding_model

    if not USE_CLIP:
        return None

    if _embedding_model is not None:
        return _embedding_model

    try:
        # Ленивый импорт: torch/transformers не грузятся на старте сервера.
        from model import OutfitEmbeddingModel

        _embedding_model = OutfitEmbeddingModel()
        return _embedding_model
    except Exception:
        logging.exception("failed to initialize CLIP model, fallback to heuristic scoring")
        _embedding_model = None
        return None


def get_reference_embeddings() -> list[dict]:
    global _reference_embeddings

    if _reference_embeddings is not None:
        return _reference_embeddings

    if not USE_CLIP or DF_OUTFITS.empty:
        _reference_embeddings = []
        return _reference_embeddings

    model = get_embedding_model()

    if model is None:
        _reference_embeddings = []
        return _reference_embeddings

    refs = []

    for outfit_id, group in DF_OUTFITS.groupby("outfit"):
        group = group.copy()

        group["path"] = group.apply(resolve_reference_image_path, axis=1)
        group = group[group["path"].apply(lambda value: bool(value) and os.path.exists(value))]

        if group.empty:
            continue

        try:
            emb = model.outfit_embedding(group)
            refs.append(
                {
                    "outfit": outfit_id,
                    "embedding": emb,
                }
            )
        except Exception:
            logging.exception("failed to build reference embedding for outfit=%s", outfit_id)

    _reference_embeddings = refs

    return _reference_embeddings


def resolve_reference_image_path(row) -> str:
    path = str_value(row.get("path"))

    if path:
        if os.path.isabs(path) and os.path.exists(path):
            return path

        local_path = os.path.join(BASE_DIR, path)
        if os.path.exists(local_path):
            return local_path

    item_id = str_value(row.get("uuid")) or str_value(row.get("id"))
    ext = normalize_ext(str_value(row.get("ext"), "webp"))

    return resolve_image_path(item_id=item_id, ext=ext)


def resolve_image_path(item_id: str, ext: str) -> str:
    item_id = str_value(item_id)
    ext = normalize_ext(ext)

    if not item_id:
        return ""

    candidates = []

    if ext:
        candidates.append(os.path.join(IMAGES_FOLDER, f"{item_id}.{ext}"))

    for fallback_ext in ("webp", "jpg", "jpeg", "png"):
        candidates.append(os.path.join(IMAGES_FOLDER, f"{item_id}.{fallback_ext}"))

    for path in candidates:
        if os.path.exists(path):
            return path

    return candidates[0] if candidates else ""


def unique_by_id(items: list[LookGenItem]) -> list[LookGenItem]:
    seen = set()
    result = []

    for item in items:
        if item.id in seen:
            continue

        seen.add(item.id)
        result.append(item)

    return result


def deduplicate_looks(looks: list[GeneratedLook]) -> list[GeneratedLook]:
    result = []
    seen = set()

    for look in looks:
        key = tuple(sorted(item.id for item in look.items))

        if key in seen:
            continue

        seen.add(key)
        result.append(look)

    return result


def gender_to_str(value: int) -> str:
    mapping = {
        1: "male",
        2: "female",
    }
    return mapping.get(int(value), "")


def style_to_str(value: int) -> str:
    mapping = {
        1: "casual",
        2: "classic",
        3: "sport",
    }
    return mapping.get(int(value), "")


def season_to_str(value: int) -> str:
    mapping = {
        1: "winter",
        2: "autumn",
        3: "spring",
        4: "summer",
    }
    return mapping.get(int(value), "")


def normalize_ext(ext: str) -> str:
    ext = str_value(ext).lower().strip()

    if ext.startswith("."):
        ext = ext[1:]

    return ext


def normalize_text(value) -> str:
    return str_value(value).lower().strip()


def str_value(value, default: str = "") -> str:
    if value is None:
        return default

    value = str(value).strip()

    if not value or value.lower() == "nan":
        return default

    return value