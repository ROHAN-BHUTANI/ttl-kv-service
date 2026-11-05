IMAGE ?= ttl-kv:dev

.PHONY: test run docker kind-up kind-load deploy forward kind-down

test:
	go vet ./...
	go test -race -count=1 ./...

run:
	go run ./cmd/server

docker:
	docker build -t $(IMAGE) .

kind-up:
	kind create cluster --name ttl-kv --config deploy/k8s/kind-config.yaml

kind-load: docker
	kind load docker-image $(IMAGE) --name ttl-kv

deploy: kind-load
	kubectl apply -f deploy/k8s/
	kubectl rollout status deployment/ttl-kv

forward:
	kubectl port-forward svc/ttl-kv 8080:80

kind-down:
	kind delete cluster --name ttl-kv
