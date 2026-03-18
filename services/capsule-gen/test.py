import sys
import os

# Добавляем путь к папке, где лежат сгенерированные пакеты
sys.path.insert(0, os.path.join(os.path.dirname(__file__), 'grpc_gen'))

# Теперь можно импортировать
from capsule_gen import service_pb2, service_pb2_grpc
from common import common_pb2



print(service_pb2.Capsule)                 # должно напечатать <class '...Capsule'>
print(dir(service_pb2))                     # должен быть 'Capsule' в списке