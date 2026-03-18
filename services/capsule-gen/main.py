import sys
import os
import grpc
from concurrent import futures
import time
import io
from PIL import Image
import uuid
import traceback

sys.path.insert(0, os.path.join(os.path.dirname(__file__), 'grpc_gen'))

from capsule_gen import service_pb2, service_pb2_grpc
from common import common_pb2

class CapsuleGenServicer(service_pb2_grpc.CapsuleGenServiceServicer):
    """
    мок мл сервера
    """
    def CapsuleGenerate(self, request, context):
        """
        генерирует поток
        """

        print(f"Request: gender={request.gender}, style={request.style}, season={request.season}, palette={request.palette}")

        try:
            capsule = service_pb2.Capsule()
            item = capsule.item.add()
            item.id = str(uuid.uuid4())
            item.gender = request.gender
            item.category_group = "top"
            item.category = "t-shirt"
            item.style = request.style
            item.color = "blue"
            item.season = request.season
            item.material = "cotton"
            item.description = "A nice blue t-shirt"
            item.ext = ".jpg"

            yield service_pb2.CapsuleGenerateResponse(capsule=capsule)

            img = Image.open("combined_images/img.jpg")
            img_bytes = io.BytesIO()
            img.save(img_bytes, format='JPEG')
            img_data = img_bytes.getvalue()

            chunk_size = 64 * 1024
            for i in range(0, len(img_data), chunk_size):
                chunk = img_data[i:i+chunk_size]
                yield service_pb2.CapsuleGenerateResponse(image_chunk=chunk)

            print(f"capsule {item.id} sent, image size: {len(img_data)} bytes")
        except Exception as e:
            print("!!! Ошибка в CapsuleGenerate:")
            traceback.print_exc()
            context.abort(grpc.StatusCode.INTERNAL, str(e))

def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    service_pb2_grpc.add_CapsuleGenServiceServicer_to_server(
        CapsuleGenServicer(), server
    )
    server.add_insecure_port('[::]:50052')
    print("Python gRPC server starting on port 50052...")
    server.start()
    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        print("Shutting down...")
        server.stop(0)

if __name__ == '__main__':
    serve()