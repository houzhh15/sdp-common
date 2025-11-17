.PHONY: help test test-coverage test-integration test-benchmark \
        lint fmt vet clean build install deps \
        cert-gen example-controller example-ih example-ah \
        docker-build docker-push

# 默认目标
.DEFAULT_GOAL := help

# 项目信息
PROJECT_NAME := sdp-common
GO_VERSION := 1.21

# 目录
BIN_DIR := bin
CERT_DIR := certs

# Go 命令
GO := go
GOFMT := gofmt
GOVET := go vet
GOLINT := golangci-lint

# 构建标志
LDFLAGS := -s -w
BUILD_FLAGS := -ldflags "$(LDFLAGS)"

## help: 显示帮助信息
help:
	@echo "可用的 Make 目标："
	@echo ""
	@grep -E '^## .*:' $(MAKEFILE_LIST) | \
		sed 's/## \(.*\): \(.*\)/  \1|\2/' | \
		column -t -s '|'
	@echo ""

## test: 运行所有单元测试
test:
	@echo "运行单元测试..."
	$(GO) test ./... -v -race -cover

## test-coverage: 运行测试并生成覆盖率报告
test-coverage:
	@echo "生成测试覆盖率报告..."
	$(GO) test ./... -coverprofile=coverage.out -covermode=atomic
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "覆盖率报告已生成: coverage.html"

## test-integration: 运行集成测试
test-integration:
	@echo "运行集成测试..."
	$(GO) test ./test/integration -v -tags=integration

## test-benchmark: 运行性能基准测试
test-benchmark:
	@echo "运行性能基准测试..."
	$(GO) test ./test -bench=. -benchmem -run=^$$

## lint: 运行代码检查工具
lint:
	@echo "运行代码检查..."
	@if command -v $(GOLINT) > /dev/null; then \
		$(GOLINT) run ./...; \
	else \
		echo "golangci-lint 未安装，跳过检查"; \
		echo "安装命令: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

## fmt: 格式化代码
fmt:
	@echo "格式化代码..."
	$(GOFMT) -s -w .
	@echo "代码格式化完成"

## vet: 运行 go vet 检查
vet:
	@echo "运行 go vet..."
	$(GOVET) ./...

## clean: 清理生成的文件
clean:
	@echo "清理生成的文件..."
	rm -rf $(BIN_DIR)
	rm -f coverage.out coverage.html
	rm -f *.log
	$(GO) clean -cache -testcache
	@echo "清理完成"

## build: 构建示例程序
build: build-controller build-ih build-ah

## build-controller: 构建 Controller 示例
build-controller:
	@echo "构建 Controller 示例..."
	@mkdir -p $(BIN_DIR)
	cd examples/controller && $(GO) build $(BUILD_FLAGS) -o ../../$(BIN_DIR)/controller-example .

## build-ih: 构建 IH Client 示例
build-ih:
	@echo "构建 IH Client 示例..."
	@mkdir -p $(BIN_DIR)
	cd examples/ih-client && $(GO) build $(BUILD_FLAGS) -o ../../$(BIN_DIR)/ih-client-example .

## build-ah: 构建 AH Agent 示例
build-ah:
	@echo "构建 AH Agent 示例..."
	@mkdir -p $(BIN_DIR)
	cd examples/ah-agent && $(GO) build $(BUILD_FLAGS) -o ../../$(BIN_DIR)/ah-agent-example .

## deps: 下载依赖
deps:
	@echo "下载依赖..."
	$(GO) mod download
	$(GO) mod verify
	@echo "依赖下载完成"

## deps-update: 更新依赖
deps-update:
	@echo "更新依赖..."
	$(GO) get -u ./...
	$(GO) mod tidy
	@echo "依赖更新完成"

## cert-gen: 生成测试证书
cert-gen:
	@echo "生成测试证书..."
	@if [ -f "./scripts/generate-certs.sh" ]; then \
		chmod +x ./scripts/generate-certs.sh; \
		./scripts/generate-certs.sh; \
	else \
		echo "错误: scripts/generate-certs.sh 不存在"; \
		exit 1; \
	fi
	@echo "证书生成完成"

## example-controller: 运行 Controller 示例
example-controller: build-controller cert-gen
	@echo "启动 Controller 示例..."
	@if [ ! -f "examples/configs/controller.yaml" ]; then \
		echo "错误: examples/configs/controller.yaml 不存在"; \
		exit 1; \
	fi
	$(BIN_DIR)/controller-example

## example-ih: 运行 IH Client 示例
example-ih: build-ih cert-gen
	@echo "启动 IH Client 示例..."
	@if [ ! -f "examples/configs/ih-client.yaml" ]; then \
		echo "错误: examples/configs/ih-client.yaml 不存在"; \
		exit 1; \
	fi
	$(BIN_DIR)/ih-client-example

## example-ah: 运行 AH Agent 示例
example-ah: build-ah cert-gen
	@echo "启动 AH Agent 示例..."
	@if [ ! -f "examples/configs/ah-agent.yaml" ]; then \
		echo "错误: examples/configs/ah-agent.yaml 不存在"; \
		exit 1; \
	fi
	$(BIN_DIR)/ah-agent-example

## install-tools: 安装开发工具
install-tools:
	@echo "安装开发工具..."
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "工具安装完成"

## check: 运行所有检查（格式、vet、lint、测试）
check: fmt vet lint test
	@echo "所有检查通过！"

## ci: CI 环境下的完整检查
ci: deps check test-coverage
	@echo "CI 检查完成"

## mod-tidy: 整理 go.mod 和 go.sum
mod-tidy:
	@echo "整理 go.mod..."
	$(GO) mod tidy
	@echo "完成"

## doc: 生成并查看 godoc
doc:
	@echo "启动 godoc 服务器..."
	@echo "访问: http://localhost:6060/pkg/github.com/houzhh15/sdp-common/"
	godoc -http=:6060

## version: 显示版本信息
version:
	@echo "$(PROJECT_NAME)"
	@echo "Go version: $(GO_VERSION)"
	@$(GO) version
	@$(GO) env GOOS GOARCH

## info: 显示项目信息
info:
	@echo "项目名称: $(PROJECT_NAME)"
	@echo "Go 版本: $(GO_VERSION)"
	@echo "二进制目录: $(BIN_DIR)"
	@echo "证书目录: $(CERT_DIR)"
	@echo ""
	@echo "模块信息:"
	@$(GO) list -m
	@echo ""
	@echo "依赖数量:"
	@$(GO) list -m all | wc -l
