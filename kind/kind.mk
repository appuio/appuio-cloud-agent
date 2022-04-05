kind_dir ?= .kind

.PHONY: kind
kind: export KUBECONFIG = $(KIND_KUBECONFIG)
kind: kind-load-image ## All-in-one kind target

.PHONY: kind-setup
kind-setup: export KUBECONFIG = $(KIND_KUBECONFIG)
kind-setup: $(KIND_KUBECONFIG) ## Creates the kind cluster

.PHONY: kind-load-image
kind-load-image: kind-setup build-docker ## Load the container image onto kind cluster
	@$(KIND) load docker-image --name $(KIND_CLUSTER) $(CONTAINER_IMG)

.PHONY: kind-clean
kind-clean: export KUBECONFIG = $(KIND_KUBECONFIG)
kind-clean: ## Removes the kind Cluster
	@$(KIND) delete cluster --name $(KIND_CLUSTER) || true
	@rm -rf $(kind_dir)

webhook-certs/tls.key:
	mkdir -p webhook-certs
	openssl req -x509 -newkey rsa:4096 -nodes -keyout webhook-certs/tls.key -out webhook-certs/tls.crt -days 3650 -subj "/CN=webhook-service.default.svc" -addext "subjectAltName = DNS:webhook-service.default.svc"


$(KIND_KUBECONFIG): export KUBECONFIG = $(KIND_KUBECONFIG)
$(KIND_KUBECONFIG): webhook-certs/tls.key
	$(KIND) create cluster \
		--name $(KIND_CLUSTER) \
		--image $(KIND_IMAGE) \
		--config kind/config.yaml
	@kubectl version
	@kubectl cluster-info
	@kubectl config use-context kind-$(KIND_CLUSTER)
	@echo =======
	@echo "Setup finished. To interact with the local dev cluster, set the KUBECONFIG environment variable as follows:"
	@echo "export KUBECONFIG=$$(realpath "$(KIND_KUBECONFIG)")"
	@echo =======
