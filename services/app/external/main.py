import os
import io
import math
import uuid
import random
import logging
from pathlib import Path
from typing import Optional, Literal

import pandas as pd
import httpx
from PIL import Image
from rembg import remove

from fastapi import FastAPI, HTTPException, Request as StarletteRequest
from pydantic import BaseModel, Field


# ============================================================
# CONFIG / PATHS
# ============================================================

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("capsule-service")

BASE_DIR = Path(__file__).resolve().parent

DATA_PATH = BASE_DIR / "metadata_uuid.csv"

# Images folder: prefer ./images, but if ./images/images exists, use it (compat with old bot)
IMAGES_FOLDER = BASE_DIR / "images"
if (IMAGES_FOLDER / "images").exists():
    IMAGES_FOLDER = IMAGES_FOLDER / "images"

COMBINED_FOLDER = BASE_DIR / "combined_images"
COMBINED_FOLDER.mkdir(exist_ok=True)

BOT_TOKEN = os.getenv("TG_TOKEN")
TELEGRAM_API_BASE = f"https://api.telegram.org/bot{BOT_TOKEN}"

# How many outfits to send
DEFAULT_OUTFITS = 3


# ============================================================
# DATASET (loaded on startup)
# ============================================================

DATA: Optional[pd.DataFrame] = None


# ============================================================
# API MODEL
# (User requested model name: Request)
# ============================================================

class Request(BaseModel):
    gender: Literal["male", "female"] = Field(...)
    style: Literal["casual", "classic", "sport"] = Field(...)
    season: Literal["winter", "autumn", "spring", "summer"] = Field(...)
    palette: Optional[Literal["dark", "light", "bright"]] = Field(default=None)  # None = no constraints
    chat_id: int = Field(..., gt=0)


# ============================================================
# COLOR KEYWORDS
# ============================================================

COLOR_KEYWORDS = {
    "dark": ["black", "dark", "navy", "brown", "grey", "gray"],
    "light": ["white", "beige", "cream", "ivory", "light"],
    "bright": ["red", "blue", "green", "yellow", "pink", "orange"],
}


def color_matches_palette(color_text: str, palette: str) -> bool:
    if not palette or not isinstance(color_text, str):
        return True
    color_text = color_text.lower()
    for kw in COLOR_KEYWORDS.get(palette, []):
        if kw in color_text:
            return True
    return False


# ============================================================
# FILTER (same logic as old bot)
# ============================================================

def recommend_outfit(df: pd.DataFrame, gender: str, style: str, season: str, palette: Optional[str]) -> pd.DataFrame:
    df = df.copy()

    # gender match or unisex
    df = df[
        (df["gender"].str.lower() == gender)
        | (df["gender"].str.lower() == "unisex")
    ]

    # style match
    df = df[df["style"].str.lower() == style]

    season_map = {
        "winter": ["winter", "autumn-winter", "all-seasons"],
        "autumn": ["autumn", "autumn-winter", "all-seasons"],
        "spring": ["spring", "spring-summer", "all-seasons"],
        "summer": ["summer", "spring-summer", "all-seasons"],
    }

    df["season"] = df["season"].fillna("all-seasons").astype(str).str.lower()
    df = df[df["season"].isin(season_map.get(season, ["all-seasons"]))]

    if palette:
        df = df[df["color"].apply(lambda c: color_matches_palette(c, palette))]

    return df.reset_index(drop=True)


# ============================================================
# OUTFIT GENERATION (same idea as old bot, safer)
# ============================================================

def generate_outfits_variable(df: pd.DataFrame, season: str, n_outfits: int = DEFAULT_OUTFITS) -> list[pd.DataFrame]:
    df = df.copy()
    if df.empty:
        return []

    groups = {c: df[df["category_group"] == c] for c in df["category_group"].unique()}

    outfits: list[pd.DataFrame] = []

    for _ in range(n_outfits):
        outfit_rows = []

        # either dress or top+bottom
        if "dress" in groups and not groups["dress"].empty and random.random() < 0.4:
            outfit_rows.append(groups["dress"].sample(1).iloc[0])
        else:
            if "top" in groups and not groups["top"].empty:
                outfit_rows.append(groups["top"].sample(1).iloc[0])
            if "bottom" in groups and not groups["bottom"].empty:
                outfit_rows.append(groups["bottom"].sample(1).iloc[0])

        if len(outfit_rows) < 2:
            continue

        # footwear
        if "footwear" in groups and not groups["footwear"].empty and random.random() < 0.8:
            outfit_rows.append(groups["footwear"].sample(1).iloc[0])

        # outerwear: always in winter, otherwise sometimes
        if "outerwear" in groups and not groups["outerwear"].empty:
            if season == "winter" or random.random() < 0.5:
                outfit_rows.append(groups["outerwear"].sample(1).iloc[0])

        # accessory sometimes
        if "accessory" in groups and not groups["accessory"].empty and random.random() < 0.4:
            outfit_rows.append(groups["accessory"].sample(1).iloc[0])

        outfits.append(pd.DataFrame(outfit_rows))

    return outfits


# ============================================================
# LINKS FROM DATASET
# ============================================================

CATEGORY_NAMES = {
    "top": "Верх",
    "bottom": "Низ",
    "dress": "Платье",
    "footwear": "Обувь",
    "outerwear": "Верхняя одежда",
    "accessory": "Аксессуары",
}


def get_wb_links_from_metadata(outfit_df: pd.DataFrame) -> dict[str, list[str]]:
    result: dict[str, list[str]] = {}
    for _, item in outfit_df.iterrows():
        url = str(getattr(item, "wb_url", "") or "").strip()
        if url:
            result.setdefault(str(item.category_group), []).append(url)
    return result


def build_caption(outfit_index: int, item_links: dict[str, list[str]]) -> str:
    caption = f"👗 *Образ {outfit_index}*\n\n"

    if not item_links:
        caption += "_Ссылки на товары не найдены в датасете._"
        return caption

    caption += "🛍 *Товары на Wildberries:*\n"
    for cat, links in item_links.items():
        caption += f"\n• *{CATEGORY_NAMES.get(cat, cat)}:*\n"
        for l in links:
            caption += f"{l}\n"

    return caption


# ============================================================
# IMAGE COMBINE (same as old bot + safety)
# ============================================================

def combine_clothes(image_paths: list[str], out_path: str) -> str:
    images: list[Image.Image] = []

    for p in image_paths:
        img = Image.open(p)
        b = io.BytesIO()
        img.save(b, format="PNG")
        img = Image.open(io.BytesIO(remove(b.getvalue()))).convert("RGBA")

        bbox = img.getbbox()
        if bbox:
            img = img.crop(bbox)

        img.thumbnail((300, 300))
        images.append(img)

    if not images:
        raise ValueError("No images found to combine")

    cols = min(3, len(images))
    rows = math.ceil(len(images) / cols)
    w = max(i.width for i in images)
    h = max(i.height for i in images)

    canvas = Image.new("RGB", (cols * w + 80, rows * h + 80), "white")

    for i, img in enumerate(images):
        x = 40 + (i % cols) * w
        y = 40 + (i // cols) * h
        canvas.paste(img, (x, y), img)

    canvas.save(out_path)
    return out_path


# ============================================================
# TELEGRAM (Bot API via HTTP)
# ============================================================

async def telegram_send_message(chat_id: int, text: str) -> None:
    if not BOT_TOKEN:
        raise RuntimeError("BOT_TOKEN is not set")

    url = f"{TELEGRAM_API_BASE}/sendMessage"
    payload = {"chat_id": str(chat_id), "text": text, "parse_mode": "Markdown"}

    async with httpx.AsyncClient(timeout=30) as client:
        r = await client.post(url, data=payload)

    if r.status_code != 200:
        raise RuntimeError(f"Telegram sendMessage failed: {r.status_code} {r.text}")


async def telegram_send_photo(chat_id: int, photo_path: str, caption: str) -> None:
    if not BOT_TOKEN:
        raise RuntimeError("BOT_TOKEN is not set")

    url = f"{TELEGRAM_API_BASE}/sendPhoto"
    data = {"chat_id": str(chat_id), "caption": caption, "parse_mode": "Markdown"}

    async with httpx.AsyncClient(timeout=60) as client:
        with open(photo_path, "rb") as f:
            files = {"photo": f}
            r = await client.post(url, data=data, files=files)

    if r.status_code != 200:
        raise RuntimeError(f"Telegram sendPhoto failed: {r.status_code} {r.text}")


# ============================================================
# PIPELINE
# ============================================================

async def run_pipeline(payload: Request) -> dict:
    global DATA
    if DATA is None or DATA.empty:
        raise RuntimeError("Dataset is not loaded")

    filtered = recommend_outfit(DATA, payload.gender, payload.style, payload.season, payload.palette)
    outfits = generate_outfits_variable(filtered, payload.season, n_outfits=DEFAULT_OUTFITS)

    if not outfits:
        await telegram_send_message(
            payload.chat_id,
            "😕 Не удалось собрать образы по твоим параметрам.\n"
            "Попробуй изменить *стиль*, *сезон* или *палитру*.",
        )
        return {"sent_outfits": 0, "note": "no outfits"}

    sent = 0
    for i, outfit in enumerate(outfits, 1):
        item_links = get_wb_links_from_metadata(outfit)

        # resolve image paths
        image_paths: list[str] = []
        used_items: list[str] = []

        for _, item in outfit.iterrows():
            img_path = IMAGES_FOLDER / f"{item.uuid}.{item.ext}"
            if img_path.exists():
                image_paths.append(str(img_path))
                used_items.append(str(item.uuid))

        if not image_paths:
            logger.warning("Outfit %s has no existing images; used=%s", i, used_items)
            continue

        combined_path = COMBINED_FOLDER / f"{uuid.uuid4().hex}.jpg"
        combine_clothes(image_paths, str(combined_path))

        caption = build_caption(i, item_links)
        await telegram_send_photo(payload.chat_id, str(combined_path), caption)
        sent += 1

    if sent == 0:
        await telegram_send_message(
            payload.chat_id,
            "😕 Я нашёл подходящие вещи, но не смог найти изображения на диске.\n"
            "Проверь, что файлы лежат в папке *images* и названы как *uuid.ext*.",
        )
        return {"sent_outfits": 0, "note": "no images"}
    
    await telegram_send_message(
        payload.chat_id,
        "Хочешь еще? Жми -> /start"
    )

    return {"sent_outfits": sent}


# ============================================================
# FASTAPI APP
# ============================================================

app = FastAPI(title="CapsuleMe Outfit Service")


@app.on_event("startup")
def startup():
    global DATA

    logger.info("BASE_DIR=%s", BASE_DIR)
    logger.info("DATA_PATH=%s exists=%s", DATA_PATH, DATA_PATH.exists())
    logger.info("IMAGES_FOLDER=%s exists=%s", IMAGES_FOLDER, IMAGES_FOLDER.exists())
    logger.info("COMBINED_FOLDER=%s", COMBINED_FOLDER)

    if not DATA_PATH.exists():
        raise RuntimeError(f"metadata_uuid.csv not found рядом с main.py: {DATA_PATH}")

    df = pd.read_csv(DATA_PATH)
    df["wb_url"] = df.get("wb_url", "").fillna("")

    # normalize key columns to lower-case strings where present
    for col in ["gender", "style", "category_group", "season", "color"]:
        if col in df.columns:
            df[col] = df[col].astype(str).str.lower()

    DATA = df
    logger.info("Loaded dataset: rows=%s", len(DATA))


@app.post("/v1/outfits/recommend")
async def recommend(req: Request, request: StarletteRequest):
    # raw+parsed logs (and fixes your earlier 'await' underline issue: endpoint is async)
    raw = await request.body()
    logger.info("RAW REQUEST BODY: %s", raw.decode("utf-8", errors="replace"))
    logger.info("PARSED REQUEST: %s", req.model_dump())

    try:
        result = await run_pipeline(req)
        return {"ok": True, "result": result}
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception:
        logger.exception("Pipeline failed")
        raise HTTPException(status_code=500, detail="Internal error")
