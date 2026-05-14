# ──────────────────────────────────────────────────────────────────────────────
# weather-app — eBPF talk demo
# ──────────────────────────────────────────────────────────────────────────────

APP      := weather-app
IMAGE    := $(APP)
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
REGISTRY ?=
FULL_IMG := $(if $(REGISTRY),$(REGISTRY)/$(IMAGE):$(VERSION),$(IMAGE):$(VERSION))
K8S_NS   := weather-demo
KUBECONFIG ?=

ifeq ($(KUBECONFIG),)
  KUBECTL := kubectl
else
  KUBECTL := kubectl --kubeconfig=$(KUBECONFIG)
endif

.DEFAULT_GOAL := help

# ── Local build ────────────────────────────────────────────────────────────────

.PHONY: build
build: ## Build the binary locally (output: ./weather-app)
	go build -ldflags="-X main.version=$(VERSION)" -o ./$(APP) ./cmd/weather-app

.PHONY: test
test: ## Run all unit tests
	go test -race -count=1 ./...

.PHONY: test-cover
test-cover: ## Run tests with HTML coverage report
	go test -race -count=1 -coverprofile=coverage.out ./... && \
	go tool cover -html=coverage.out -o coverage.html
	@echo "Open coverage.html in your browser"

.PHONY: vet
vet: ## Run go vet
	go vet ./...

# ── Container image ────────────────────────────────────────────────────────────

.PHONY: image
image: ## Build the Docker image  (weather-app:<VERSION>)
	docker build --build-arg VERSION=$(VERSION) -t $(FULL_IMG) .

.PHONY: load
load: ## Load the Docker image into the kind cluster
	kind load docker-image $(FULL_IMG)

# ── Kubernetes ─────────────────────────────────────────────────────────────────

.PHONY: deploy
deploy: ## Apply all manifests via kustomize
	$(KUBECTL) apply -k manifests/

.PHONY: undeploy
undeploy: ## Delete all manifests (keeps the namespace)
	$(KUBECTL) delete -k manifests/ --ignore-not-found

.PHONY: wait
wait: ## Wait for both Deployments to reach the Available condition
	$(KUBECTL) -n $(K8S_NS) wait deployment/redis       --for=condition=Available --timeout=120s
	$(KUBECTL) -n $(K8S_NS) wait deployment/weather-app --for=condition=Available --timeout=120s
	@echo "✓ Both deployments are ready"

.PHONY: lb-ip
lb-ip: ## Print the MetalLB LoadBalancer IP for weather-app
	@$(KUBECTL) -n $(K8S_NS) get svc weather-app \
	  -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null && echo

# ── Demo helpers ───────────────────────────────────────────────────────────────

.PHONY: reset
reset: ## Restart Redis to wipe the cache (forces all cities to re-fetch)
	$(KUBECTL) -n $(K8S_NS) rollout restart deployment/redis
	$(KUBECTL) -n $(K8S_NS) rollout status  deployment/redis

.PHONY: loadgen
loadgen: ## Run loadgen.sh locally against the MetalLB IP (requires APP_IP or lb-ip)
	@IP=$${APP_IP:-$$($(MAKE) -s lb-ip)}; \
	if [ -z "$$IP" ]; then echo "ERROR: APP_IP not set and lb-ip returned empty" >&2; exit 1; fi; \
	bash loadgen/loadgen.sh "http://$$IP"

.PHONY: loadgen-job
loadgen-job: ## Apply loadgen Job inside the cluster and tail its logs
	$(KUBECTL) apply -f loadgen/loadgen.yaml
	@echo "Waiting for loadgen pod to start..."
	$(KUBECTL) -n $(K8S_NS) wait job/loadgen --for=condition=ready --timeout=30s 2>/dev/null || true
	$(KUBECTL) -n $(K8S_NS) logs -f job/loadgen

.PHONY: loadgen-clean
loadgen-clean: ## Delete the loadgen Job and its ConfigMap
	$(KUBECTL) -n $(K8S_NS) delete job/loadgen --ignore-not-found
	$(KUBECTL) -n $(K8S_NS) delete configmap/loadgen-script --ignore-not-found

.PHONY: logs
logs: ## Tail weather-app logs (JSON structured)
	$(KUBECTL) -n $(K8S_NS) logs -l app=weather-app -f --tail=50

# ── Helpers ────────────────────────────────────────────────────────────────────

.PHONY: clean
clean: ## Remove local build artefacts
	rm -f ./$(APP) coverage.out coverage.html

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} \
	  /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
