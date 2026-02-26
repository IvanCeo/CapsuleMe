.PHONY: proto
proto: proto-go proto-python

.PHONY: proto-go-clean
proto-go-clean:
	if exist services\app\internal\gen rmdir /s /q services\app\internal\gen

.PHONY: proto-python-clean
proto-python-clean:
	if exist services\capsule-gen\grpc_gen rmdir /s /q services\capsule-gen\grpc_gen
	if exist services\look-gen\grpc_gen rmdir /s /q services\look-gen\grpc_gen

.PHONY: proto-go
proto-go: proto-go-clean
	buf generate

.PHONY: proto-python
proto-python: proto-python-clean
	if not exist services\capsule-gen\grpc_gen mkdir services\capsule-gen\grpc_gen
	python -m grpc_tools.protoc -I api/proto \
		--python_out=services/capsule-gen/grpc_gen \
		--grpc_python_out=services/capsule-gen/grpc_gen \
		api/proto/capsule-gen/*.proto api/proto/common/*.proto

	if not exist services\look-gen\grpc_gen mkdir services\look-gen\grpc_gen
	python -m grpc_tools.protoc -I api/proto \
		--python_out=services/look-gen/grpc_gen \
		--grpc_python_out=services/look-gen/grpc_gen \
		api/proto/look-gen/*.proto api/proto/common/*.proto

.PHONY: proto-lint
proto-lint:
	buf lint

.PHONY: proto-breaking
proto-breaking:
	buf breaking --against '.git#branch=go-based'