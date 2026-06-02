class OutfitEmbeddingModel:
    def __init__(self, *args, **kwargs):
        raise RuntimeError(
            "OutfitEmbeddingModel is disabled in capsule-gen deploy build. "
            "Move CLIP scoring to look-gen or enable ML dependencies explicitly."
        )
