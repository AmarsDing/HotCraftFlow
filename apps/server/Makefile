# TwinServer 系统 Makefile
# 在 TwinServer/ 根目录执行 make <target>；go.mod 位于本目录，所有 go 命令在此运行。
#
# 两层产物：
#   bin/    各 app 调试/运行二进制（make build / make build-<app>）
#   ../build/  完整系统部署包（make package：二进制 + 全部配置 + 前端 + 内置 NATS + 启动脚本）

GOCMD=go
VERSION?=1.0.0

# 全部后端 app（gateway 统一入口，打包脚本中排最后启动）
APPS=admin media collector-snmp-ssh monitoralarm moniternotice monitorquery report analysis visualization linemonitor aiops remediation virtman opstools voice radar windfield metar ops gateway
# 部署工具（nats_setup: JetStream 初始化；webserve: 前端托管；license-tool: 许可证工具）
TOOLS=nats_setup webserve license-tool

PACKAGE_ARGS?=

# 注意：build-<app> 不得加入 .PHONY —— phony 目标会跳过模式规则搜索，导致 build-% 失效
.PHONY: all build tools web webserve-single package clean help \
        gen api ent wire init tidy docker run clean-gen

help:
	@echo "TwinServer 系统构建工具"
	@echo ""
	@echo "== 构建 =="
	@echo "  make build          构建全部 18 个后端服务到 bin/<name>.exe"
	@echo "  make build-<app>    构建单个服务（如 make build-admin）"
	@echo "  make tools          构建部署工具（nats_setup / webserve / license-tool）"
	@echo "  make web            构建 TwinWeb 前端生产包（../TwinWeb/dist）"
	@echo "  make webserve-single  单文件 webserve：前端内嵌进二进制（Windows+Linux，../build/single）"
	@echo ""
	@echo "== 部署包 =="
	@echo "  make package        组装完整部署包到 ../build/（build + tools + web + 配置 + 启动脚本）"
	@echo "  make package PACKAGE_ARGS=-SkipWeb   跳过前端，仅后端部署包"
	@echo "  部署包入口: start-all.ps1（分项: start-env / start-backend / start-web / start-ue）"
	@echo ""
	@echo "== 代码生成（admin 域；其余 app 用各自目录下的 Makefile） =="
	@echo "  make gen            生成全部代码（api + ent + wire）"
	@echo "  make api            buf 生成 Go API + 前端 TypeScript 客户端"
	@echo "  make ent            生成 admin ent ORM 代码"
	@echo "  make wire           生成 admin wire 依赖注入代码"
	@echo "  make init           安装 protoc 插件"
	@echo ""
	@echo "== 其他 =="
	@echo "  make run            本地运行 admin（加载 conf + app/admin/config）"
	@echo "  make docker         构建 admin docker 镜像"
	@echo "  make tidy           go mod tidy"
	@echo "  make clean          清理 bin/ 构建产物"
	@echo "  make clean-gen      清理全部生成代码（再运行 make gen 恢复）"

# ===================== 构建 =====================

all: build

# 全量构建：18 个后端服务 + 部署工具，统一输出 bin/<name>.exe（必须带 .exe，避免 PATHEXT 误解析旧二进制）
build: $(addprefix build-,$(APPS)) tools
	@echo "==> 全部服务构建完成（bin/）"

# 单服务构建：make build-<app>
build-%:
	$(GOCMD) build -o bin/$*.exe ./app/$*/cmd

# 部署工具
tools:
	$(GOCMD) build -o bin/nats_setup.exe ./cmd/nats_setup
	$(GOCMD) build -o bin/webserve.exe ./cmd/webserve
	$(GOCMD) build -o bin/license-tool.exe ./tools/license-tool

# TwinWeb 前端生产包
web:
	cd ../TwinWeb && npx quasar build

# ===================== 单文件前端（前端内嵌进 webserve） =====================

# 把 TwinWeb/dist 编进 webserve 二进制：部署时一个可执行文件就是全部前端，不再需要 web/ 目录。
# 默认产物 ../build/single/{webserve-windows-amd64.exe,webserve-linux-amd64}。
# 覆盖路径：make webserve-single WEB_DIST=../TwinWeb/dist SINGLE_OUT=../build/single
WEB_DIST?=../TwinWeb/dist
SINGLE_OUT?=../build/single
webserve-single:
	bash scripts/build-webserve-single.sh "$(WEB_DIST)" "$(SINGLE_OUT)"

# ===================== 部署包 =====================

# 组装完整部署包到 ../build/（二进制 + 配置 + 前端 + 内置 NATS + 启动脚本）
package: build
	powershell -ExecutionPolicy Bypass -File scripts/deploy/package.ps1 $(PACKAGE_ARGS)

# ===================== 代码生成（admin 域） =====================

gen: api ent wire

init:
	$(GOCMD) install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	$(GOCMD) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	$(GOCMD) install github.com/go-kratos/kratos/cmd/protoc-gen-go-http/v2@latest
	$(GOCMD) install github.com/go-kratos/kratos/cmd/protoc-gen-go-errors/v2@latest
	$(GOCMD) install github.com/envoyproxy/protoc-gen-validate@latest
	$(GOCMD) install go.einride.tech/protoc-gen-typescript-http@latest
	@echo "==> 插件安装完成，请确保 \$$(go env GOPATH)/bin 在 PATH 中"

api:
	cd api && buf generate

ent:
	cd app/admin/internal/data/ent && $(GOCMD) run -mod=mod ./entc.go

wire:
	$(GOCMD) run github.com/google/wire/cmd/wire gen ./app/admin/cmd/

# ===================== 运行与其他 =====================

run:
	$(GOCMD) run ./app/admin/cmd -conf-common ./conf -conf ./app/admin/config

docker:
	docker build -f app/admin/Dockerfile -t framework-admin:$(VERSION) .

tidy:
	$(GOCMD) mod tidy

clean:
	$(GOCMD) clean
	rm -rf bin/

clean-gen: clean
	rm -rf api/gen/go/
	rm -f app/admin/cmd/wire_gen.go
	rm -f app/visualization/cmd/wire_gen.go
	rm -f conf/pb/*.pb.go
	rm -f app/admin/internal/conf/*.pb.go
	rm -rf ../TwinWeb/src/services/kratos/
	@echo "==> 生成代码已清理，ent 生成文件需手动删除或运行 make ent 覆盖"

# ---------- Docker 部署（2026-08-13） ----------
.PHONY: docker-config docker-runtime docker-up docker-voice docker-vlm docker-release docker-migrate docker-models docker-wgrib2
docker-config:    ; powershell -ExecutionPolicy Bypass -File docker/gen-config.ps1
docker-runtime:   ; powershell -ExecutionPolicy Bypass -File docker/build-runtime.ps1
docker-up:        ; powershell -ExecutionPolicy Bypass -File docker/dev-up.ps1
docker-voice:     ; docker compose build voice
docker-vlm:       ; powershell -ExecutionPolicy Bypass -File docker/build-vlm.ps1
docker-release:   ; powershell -ExecutionPolicy Bypass -File docker/build-release.ps1
docker-migrate:   ; powershell -ExecutionPolicy Bypass -File docker/migrate-data.ps1
docker-models:    ; powershell -ExecutionPolicy Bypass -File docker/fetch-voice-models.ps1
docker-wgrib2:    ; powershell -ExecutionPolicy Bypass -File docker/build-wgrib2.ps1
