GIT_COMMIT=$(shell git rev-list -1 HEAD)
CSI_IMAGE_VERSION=v2.0.1
CSI_IMAGE_NAME=192.168.20.240:29006/library/hostpathcsi
GO_PROJECT=ShixuanMiao/hostpathpv-plugin

# go build flags
LDFLAGS ?=
LDFLAGS += -X $(GO_PROJECT)/pkg/util.GitCommit=$(GIT_COMMIT)
# CSI_IMAGE_VERSION will be considered as the driver version
LDFLAGS += -X $(GO_PROJECT)/pkg/util.DriverVersion=$(CSI_IMAGE_VERSION)

# set GOARCH explicitly for cross building, default to native architecture
ifeq ($(origin GOARCH), undefined)
GOARCH := $(shell go env GOARCH)
endif

csi:
	if [ ! -d ./vendor ]; then (go mod tidy && go mod vendor); fi
	CGO_ENABLED=1 GOOS=linux go build -mod vendor -a -ldflags '$(LDFLAGS)' -o  _output/hostpathcsi ./cmd/csi-plugin

scheduler:
	if [ ! -d ./vendor ]; then (go mod tidy && go mod vendor); fi
    CGO_ENABLED=0 GOOS=linux go build -mod vendor -a -ldflags '$(LDFLAGS)' -o  _output/extender-scheduler ./cmd/extender-scheduler

ctl:
	if [ ! -d ./vendor ]; then (go mod tidy && go mod vendor); fi
    CGO_ENABLED=0 GOOS=linux go build -mod vendor -a -ldflags '$(LDFLAGS)' -o  _output/kubectl-hostpathpv ./cmd/kubectl-plugin

image: csi scheduler
	docker build -t $(CSI_IMAGE_NAME):$(CSI_IMAGE_VERSION) -f Dockerfile --build-arg GOLANG_VERSION=1.13.8 --build-arg CSI_IMAGE_NAME=$(CSI_IMAGE_NAME) --build-arg CSI_IMAGE_VERSION=$(CSI_IMAGE_VERSION) --build-arg GIT_COMMIT=$(GIT_COMMIT) --build-arg GO_ARCH=$(GOARCH) $(BASE_IMAGE_ARG) .

release: image
		docker push	$(CSI_IMAGE_NAME):$(CSI_IMAGE_VERSION)