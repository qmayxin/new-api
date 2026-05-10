FRONTEND_DIR = ./web/default
FRONTEND_CLASSIC_DIR = ./web/classic
BACKEND_DIR = .

.PHONY: all dev dev-frontend dev-backend build-frontend build-frontend-classic build-all-frontends start-backend dev-api dev-web dev-web-classic

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

all: build-all-frontends start-backend

build-frontend:
	@echo "Building default frontend..."
	@cd $(FRONTEND_DIR) && bun install && DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$(shell cat VERSION) bun run build

build-frontend-classic:
	@echo "Building classic frontend..."
	@cd $(FRONTEND_CLASSIC_DIR) && bun install && VITE_REACT_APP_VERSION=$(shell cat VERSION) bun run build

build-all-frontends: build-frontend build-frontend-classic

start-backend:
	@echo "Starting backend..."
	@cd $(BACKEND_DIR) && go run main.go &

dev-api:
	@echo "Starting backend services (docker)..."
	@docker compose -f docker-compose.dev.yml up -d

dev-web:
	@echo "Starting frontend dev server..."
	@cd $(FRONTEND_DIR) && bun install && bun run dev

dev-web-classic:
	@echo "Starting classic frontend dev server..."
	@cd $(FRONTEND_CLASSIC_DIR) && bun install && bun run dev
