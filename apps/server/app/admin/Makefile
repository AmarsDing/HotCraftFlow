# TwinServer admin 服务 Makefile（HTTP :7788）
# 在 app/admin/ 目录执行 make <target>；go 命令统一在 TwinServer 根目录（go.mod 所在）运行。

GOCMD=go
PROJECT_ROOT=../..
BINARY=admin.exe
VERSION?=1.0.0

# 配置目录（相对 TwinServer 根目录）
CONF_COMMON=./conf
CONF_PRIVATE=./app/admin/config

.PHONY: all build run wire ent api docker tidy clean help

help:
	@echo "TwinServer admin 服务构建工具（HTTP :7788）"
	@echo ""
	@echo "  make build   构建二进制到 bin/admin.exe"
	@echo "  make run     本地运行（-conf-common ./conf -conf ./app/admin/config）"
	@echo "  make wire    生成 wire_gen.go"
	@echo "  make ent     生成 ent ORM 代码"
	@echo "  make api     buf 生成 API 代码（Go + 前端 TypeScript，全量）"
	@echo "  make docker  构建 docker 镜像 framework-admin"
	@echo "  make tidy    go mod tidy"
	@echo "  make clean   删除 bin/admin.exe"

all: build

build:
	cd $(PROJECT_ROOT) && $(GOCMD) build -o bin/$(BINARY) ./app/admin/cmd

run:
	cd $(PROJECT_ROOT) && $(GOCMD) run ./app/admin/cmd -conf-common $(CONF_COMMON) -conf $(CONF_PRIVATE)

wire:
	cd $(PROJECT_ROOT) && $(GOCMD) run github.com/google/wire/cmd/wire gen ./app/admin/cmd/

ent:
	cd $(PROJECT_ROOT)/app/admin/internal/data/ent && $(GOCMD) run -mod=mod ./entc.go

api:
	cd $(PROJECT_ROOT)/api && buf generate

docker:
	cd $(PROJECT_ROOT) && docker build -f app/admin/Dockerfile -t framework-admin:$(VERSION) .

tidy:
	cd $(PROJECT_ROOT) && $(GOCMD) mod tidy

clean:
	rm -f $(PROJECT_ROOT)/bin/$(BINARY)
