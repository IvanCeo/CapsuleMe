import os
import sys
import traceback
from concurrent import futures

import grpc

BASE_DIR = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(BASE_DIR, "grpc_gen"))

from look_gen import service_pb2 as look_pb2
from look_gen import service_pb2_grpc as look_pb2_grpc

from look_recommender import LookGenError, generate_looks, combine_look_images


class LookGenService(look_pb2_grpc.LookGenServiceServicer):
    def GenerateLooks(self, request, context):
        try:
            max_looks = request.max_looks if request.max_looks > 0 else 6

            print(
                f"GenerateLooks request: "
                f"capsule_items={len(request.capsule.item)}, "
                f"max_looks={max_looks}, "
                f"include_images={request.include_images}"
            )

            looks = generate_looks(request)

            look_pack = look_pb2.LookPack()

            for image_index, look in enumerate(looks):
                look_dto = look_pack.looks.add()

                look_dto.id = look.id
                look_dto.score = look.score
                look_dto.template = look.template
                look_dto.image_index = image_index

                for item in look.items:
                    look_item = look_dto.items.add()
                    look_item.CopyFrom(item.proto)

            yield look_pb2.GenerateLooksResponse(look_pack=look_pack)

            if request.include_images:
                for image_index, look in enumerate(looks):
                    image_path = combine_look_images(look)

                    if not image_path:
                        continue

                    chunk_size = 64 * 1024

                    with open(image_path, "rb") as f:
                        while True:
                            chunk = f.read(chunk_size)
                            if not chunk:
                                break

                            yield look_pb2.GenerateLooksResponse(
                                image_chunk=look_pb2.LookImageChunk(
                                    look_id=look.id,
                                    image_index=image_index,
                                    chunk=chunk,
                                )
                            )

            print(f"GenerateLooks response sent: looks={len(looks)}")

        except LookGenError as e:
            print(f"LookGen error: {e.message}")
            context.abort(e.code, e.message)

        except Exception as e:
            print("!!! Ошибка в GenerateLooks:")
            traceback.print_exc()
            context.abort(grpc.StatusCode.INTERNAL, str(e))


def serve():
    port = os.getenv("LOOK_GEN_PORT", "50053")

    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))

    look_pb2_grpc.add_LookGenServiceServicer_to_server(
        LookGenService(),
        server,
    )

    server.add_insecure_port(f"[::]:{port}")

    print(f"LookGen gRPC server starting on port {port}...")

    server.start()

    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        print("Shutting down...")
        server.stop(0)


if __name__ == "__main__":
    serve()