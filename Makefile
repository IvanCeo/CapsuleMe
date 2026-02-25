.PHONY: proto
proto: proto-go proto-python

.PHONY: proto-lint
proto-lint:
	buf lint

.PHONY: proto-go
proto-go:
	rmdir /s /q services\app\internal\gen
	buf generate

# проверка на излом совместимости
.PHONY: proto-breaking
proto-breaking:
	buf breaking --against '.git#branch=main'

.PHONY: proto-python
proto-python:
	python -m grpc_tools.protoc -I api/proto \
		--python_out=services/capsule-gen/grpc_gen \
		--grpc_python_out=services/capsule-gen/grpc_gen \
		api/proto/capsule-gen/*.proto api/proto/common/*.proto
	python -m grpc_tools.protoc -I api/proto \
		--python_out=services/look-gen/grpc_gen \
		--grpc_python_out=services/look-gen/grpc_gen \
		api/proto/look-gen/*.proto api/proto/common/*.proto

.PHONY: ex
ex:
	type nul > temp.txt

.PHONY: de
de:
	rmdir /s /q services\app\internal\gen