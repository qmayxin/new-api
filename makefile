FRONTEND_DIR = ./web
BACKEND_DIR = .

.PHONY: all dev dev-frontend dev-backend build-frontend start-backend

# 开发模式：前后端热更新同时启动（需要两个终端或后台运行）
dev: dev-frontend dev-backend

# 前端热更新（Vite）
dev-frontend:
	@echo "Starting frontend dev server (hot reload enabled)..."
	@cd $(FRONTEND_DIR) && bun run dev

# 后端热更新（air）
dev-backend:
	@echo "Starting backend dev server (hot reload enabled)..."
	@cd $(BACKEND_DIR) && air

all: build-frontend start-backend

build-frontend:
	@echo "Building frontend..."
	@cd $(FRONTEND_DIR) && bun install && DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$(shell cat VERSION) bun run build

start-backend:
	@echo "Starting backend..."
	@cd $(BACKEND_DIR) && go run main.go &
