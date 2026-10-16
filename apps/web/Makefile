# TwinWeb Makefile
# 在 TwinWeb/ 根目录执行 make <target>

NPM=npm
NPX=npx

.PHONY: install dev build preview lint lint-fix format stylelint stylelint-fix api typecheck help

help:
	@echo "TwinWeb 前端构建工具"
	@echo ""
	@echo "常用命令:"
	@echo "  make install        安装依赖"
	@echo "  make dev            启动开发服务器（端口 9001，代理 /admin 到 7788）"
	@echo "  make build          构建生产包到 dist/"
	@echo "  make preview        预览构建产物"
	@echo "  make typecheck      TypeScript 类型检查（vue-tsc --noEmit）"
	@echo "  make api            重新生成 TypeScript API 客户端（调用 TwinServer buf）"
	@echo "  make lint           ESLint 检查"
	@echo "  make lint-fix       ESLint 自动修复"
	@echo "  make format         Prettier 格式化"
	@echo "  make stylelint      Stylelint 检查"
	@echo "  make stylelint-fix  Stylelint 自动修复"
	@echo ""
	@echo "生成文件（已 gitignore，勿提交）:"
	@echo "  src/services/kratos/    protoc-gen-typescript-http 生成客户端"
	@echo "  dist/                   构建产物"
	@echo "  node_modules/           依赖"

# ===================== 依赖与运行 =====================

# 安装依赖
install:
	$(NPM) install

# 启动开发服务器
dev:
	$(NPX) quasar dev

# 构建生产包
build:
	$(NPX) quasar build

# 预览构建产物
preview:
	$(NPX) vite preview

# ===================== 代码生成 =====================

# 重新生成 TypeScript API 客户端
# buf.gen.yaml 在 ../TwinServer/api/，输出到 src/services/kratos/
api:
	cd ../TwinServer/api && buf generate

# ===================== 检查与格式化 =====================

# TypeScript 类型检查
typecheck:
	$(NPX) vue-tsc --noEmit

# ESLint 检查
lint:
	$(NPX) eslint "src/**/*.{ts,vue}"

# ESLint 自动修复
lint-fix:
	$(NPX) eslint "src/**/*.{ts,vue}" --fix

# Prettier 格式化
format:
	$(NPX) prettier --write "src/**/*.{ts,vue,css,scss}"

# Stylelint 检查
stylelint:
	$(NPX) stylelint "src/**/*.{vue,css,sass,scss}" --config .stylelintrc.json

# Stylelint 自动修复
stylelint-fix:
	$(NPX) stylelint "src/**/*.{vue,css,sass,scss}" --config .stylelintrc.json --fix
