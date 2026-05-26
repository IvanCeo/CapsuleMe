# model.py
#BOT_TOKEN = "8246889384:AAFYmgP-LyXbwQTdUC2rFMqLUivbpB1sgxg"

import os
import torch
import numpy as np
from PIL import Image
from transformers import CLIPProcessor, CLIPModel
from sklearn.metrics.pairwise import cosine_similarity


class OutfitEmbeddingModel:
    def __init__(self, device=None):
        self.device = device or ("cuda" if torch.cuda.is_available() else "cpu")

        self.model = CLIPModel.from_pretrained(
            "openai/clip-vit-base-patch32"
        ).to(self.device)

        self.processor = CLIPProcessor.from_pretrained(
            "openai/clip-vit-base-patch32"
        )

        self.model.eval()

        # 🧠 in-memory cache: path -> np.ndarray
        self.image_cache: dict[str, np.ndarray] = {}

    @torch.no_grad()
    def image_embedding(self, image_path: str) -> np.ndarray:
        if image_path in self.image_cache:
            return self.image_cache[image_path]

        if not os.path.exists(image_path):
            raise FileNotFoundError(image_path)

        image = Image.open(image_path).convert("RGB")

        inputs = self.processor(
            images=image,
            return_tensors="pt"
        ).to(self.device)

        # 🔑 СТАБИЛЬНЫЙ СПОСОБ (НЕ get_image_features)
        vision_out = self.model.vision_model(
            pixel_values=inputs["pixel_values"]
        )

        pooled = vision_out.pooler_output
        emb = self.model.visual_projection(pooled)

        emb = emb / emb.norm(dim=-1, keepdim=True)
        emb_np = emb.cpu().numpy()[0]

        self.image_cache[image_path] = emb_np
        return emb_np

    def outfit_embedding(self, outfit_df) -> np.ndarray:
        embeddings = []

        for _, row in outfit_df.iterrows():
            emb = self.image_embedding(row["path"])
            embeddings.append(emb)

        return np.mean(embeddings, axis=0)

    def score_outfit(self, outfit_df, reference_embeddings) -> float:
        emb = self.outfit_embedding(outfit_df).reshape(1, -1)

        sims = [
            cosine_similarity(
                emb,
                ref["embedding"].reshape(1, -1)
            )[0][0]
            for ref in reference_embeddings
        ]

        return float(max(sims))

    def clear_cache(self):
        self.image_cache.clear()
