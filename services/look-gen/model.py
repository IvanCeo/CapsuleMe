# model.py
# Deploy-safe stub for look-gen.
# The real CLIP model is intentionally disabled in Docker/MVP mode.
# look_recommender.py uses heuristic scoring unless LOOK_GEN_USE_CLIP=1.

class OutfitEmbeddingModel:
    def __init__(self, *args, **kwargs):
        raise RuntimeError(
            "CLIP scoring is disabled in deploy-safe look-gen. "
            "Use the full model.py and ML requirements only for offline/experimental mode."
        )
