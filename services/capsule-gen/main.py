import os
import sys
import traceback
from concurrent import futures

import grpc

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "grpc_gen"))

from capsule_gen import service_pb2, service_pb2_grpc
from common import common_pb2

from recomender import CapsuleRecommendationError, generate_capsule


class CapsuleGenServicer(service_pb2_grpc.CapsuleGenServiceServicer):
    def CapsuleGenerate(self, request, context):
        print(
            f"Request: gender={request.gender}, "
            f"style={request.style}, "
            f"season={request.season}, "
            f"palette={request.palette}"
        )

        try:
            gender = _gender_to_str(request.gender)
            style = _style_to_str(request.style)
            season = _season_to_str(request.season)
            palette = _palette_to_str(_request_palette(request))

            result = generate_capsule(
                gender=gender,
                style=style,
                season=season,
                palette=palette,
            )

            capsule = service_pb2.Capsule()

            for row in result.items:
                item = capsule.item.add()
                item.id = _str_value(row.get("uuid"))
                item.gender = request.gender
                item.category_group = _str_value(row.get("category_group"))
                item.category = _str_value(row.get("category"))
                item.style = request.style
                item.color = _str_value(row.get("color_category")) or _str_value(row.get("color"))
                item.season = request.season
                item.material = _str_value(row.get("material"))
                item.description = _build_description(row)
                item.ext = _normalize_ext(_str_value(row.get("ext")))

            yield service_pb2.CapsuleGenerateResponse(capsule=capsule)

            chunk_size = 64 * 1024
            with open(result.image_path, "rb") as f:
                while True:
                    chunk = f.read(chunk_size)
                    if not chunk:
                        break

                    yield service_pb2.CapsuleGenerateResponse(image_chunk=chunk)

            print(
                f"capsule sent: items={len(result.items)}, "
                f"looks={result.looks_count}, "
                f"image_path={result.image_path}"
            )

        except CapsuleRecommendationError as e:
            print(f"CapsuleGenerate failed: {e.message}")
            context.abort(e.code, e.message)

        except Exception as e:
            print("!!! Ошибка в CapsuleGenerate:")
            traceback.print_exc()
            context.abort(grpc.StatusCode.INTERNAL, str(e))


def _gender_to_str(value: int) -> str:
    mapping = {
        common_pb2.GENDER_MALE: "male",
        common_pb2.GENDER_FEMALE: "female",
    }

    result = mapping.get(value)
    if result is None:
        raise CapsuleRecommendationError(
            grpc.StatusCode.INVALID_ARGUMENT,
            f"unsupported gender enum: {value}",
        )

    return result


def _style_to_str(value: int) -> str:
    mapping = {
        common_pb2.STYLE_CASUAL: "casual",
        common_pb2.STYLE_CLASSIC: "classic",
        common_pb2.STYLE_SPORT: "sport",
    }

    result = mapping.get(value)
    if result is None:
        raise CapsuleRecommendationError(
            grpc.StatusCode.INVALID_ARGUMENT,
            f"unsupported style enum: {value}",
        )

    return result


def _season_to_str(value: int) -> str:
    mapping = {
        common_pb2.SEASON_WINTER: "winter",
        common_pb2.SEASON_AUTUMN: "autumn",
        common_pb2.SEASON_SPRING: "spring",
        common_pb2.SEASON_SUMMER: "summer",
    }

    result = mapping.get(value)
    if result is None:
        raise CapsuleRecommendationError(
            grpc.StatusCode.INVALID_ARGUMENT,
            f"unsupported season enum: {value}",
        )

    return result


def _request_palette(request) -> int:
    try:
        if request.HasField("palette"):
            return request.palette
    except ValueError:
        pass

    return common_pb2.PALETTE_UNSPECIFIED


def _palette_to_str(value: int) -> str:
    mapping = {
        common_pb2.PALETTE_UNSPECIFIED: "any",
        common_pb2.PALETTE_DARK: "dark",
        common_pb2.PALETTE_LIGHT: "light",
        common_pb2.PALETTE_BRIGHT: "bright",
    }

    return mapping.get(value, "any")


def _str_value(value, default: str = "") -> str:
    if value is None:
        return default

    value = str(value).strip()
    if not value or value.lower() == "nan":
        return default

    return value


def _normalize_ext(ext: str) -> str:
    ext = _str_value(ext)
    if not ext:
        return ""

    if not ext.startswith("."):
        return "." + ext

    return ext


def _build_description(row: dict) -> str:
    name = (
        _str_value(row.get("name"))
        or _str_value(row.get("name_en"))
        or _str_value(row.get("description"))
        or _str_value(row.get("category"))
        or "Товар"
    )

    wb_url = _str_value(row.get("wb_url"))

    return f"{name}|{wb_url}"


def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))

    service_pb2_grpc.add_CapsuleGenServiceServicer_to_server(
        CapsuleGenServicer(),
        server,
    )

    PORT = os.getenv('CAPSULE_GEN_PORT', "50052")
    server.add_insecure_port(f"[::]:{PORT}")
    print(f'Capsule-gen gRPC server starting on port {PORT}...')
    server.start()

    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        print("Shutting down...")
        server.stop(0)


if __name__ == "__main__":
    serve()