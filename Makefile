GREEN  := \033[32m
CYAN   := \033[36m
YELLOW := \033[33m
RESET  := \033[0m

COLLECTOR_DIR := ./prioqueue-collector
BIN_NAME      := prioqueue-collector
BIN_OUT       := $(COLLECTOR_DIR)/build/$(BIN_NAME)

IMAGE_NAME    := prioqueue-collector
REGISTRY      := ghcr.io/observability-system/otel-prioqueue-collector
TAG           := v0.2.0

.DEFAULT_GOAL := help

.PHONY: help
help:
	@echo ""
	@echo "$(CYAN)Available targets:$(RESET)"
	@echo "  $(GREEN)build-bin$(RESET)        Build collector binary using OCB"
	@echo "  $(GREEN)checksum$(RESET)         Create SHA-256 checksum for the binary"
	@echo "  $(GREEN)release$(RESET)          Build + checksum"
	@echo "  $(GREEN)docker-build$(RESET)     Build docker image locally"
	@echo "  $(GREEN)docker-buildx$(RESET)    Build multi-arch image (amd64, arm64)"
	@echo "  $(GREEN)docker-push$(RESET)      Push local image to registry"
	@echo "  $(GREEN)docker-pushx$(RESET)     Build and push multi-arch image"
	@echo ""

.PHONY: build-bin
build-bin:
	@cd $(COLLECTOR_DIR) && builder --config manifest.yaml
	@echo "$(GREEN)Binary built at $(YELLOW)$(BIN_OUT)$(RESET)"

.PHONY: checksum
checksum: build-bin
	sha256sum $(BIN_OUT) > $(COLLECTOR_DIR)/build/checksums.txt
	@echo "$(GREEN)Checksum written to $(YELLOW)prioqueue-collector/build/checksums.txt$(RESET)"
	@cat $(COLLECTOR_DIR)/build/checksums.txt

.PHONY: release
release: checksum
	@echo "$(GREEN)Release artifacts ready:$(RESET)"
	@echo " - $(YELLOW)$(BIN_OUT)$(RESET)"
	@echo " - $(YELLOW)$(COLLECTOR_DIR)/build/checksums.txt$(RESET)"

.PHONY: docker-build
docker-build:
	docker build -t $(IMAGE_NAME):$(TAG) -f Dockerfile .

.PHONY: docker-push 
docker-push: 
	docker push $(REGISTRY):$(TAG)

.PHONY: docker-buildx
docker-buildx:
	docker buildx build --platform linux/amd64,linux/arm64 \
		-t $(IMAGE_NAME):$(TAG) -f Dockerfile .

.PHONY: docker-pushx
docker-pushx:
    docker buildx build --platform linux/amd64,linux/arm64 \
		-t $(REGISTRY):latest \
        -t $(REGISTRY):$(TAG) \
        -f Dockerfile . --push