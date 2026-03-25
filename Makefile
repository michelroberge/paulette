.PHONY: build clean dev docker-up dev-docker-build dev-docker-up

ifeq ($(OS),Windows_NT)
  BINARY := paulette.exe
else
  BINARY := paulette
endif

build:
	cd frontend && npm ci && npm run build
	rm -rf backend/static
	cp -r frontend/dist backend/static
	cd backend && go build -o ../$(BINARY) .

clean:
	rm -f paulette paulette.exe
	rm -rf backend/static

dev:
	cd frontend && npm run dev

# Build and start the container — everything builds inside Docker, no local Go/Node needed.
docker-up:
	docker compose up --build

# Dev: mount source from host, air hot-reloads Go, Vite HMR for frontend.
dev-docker-build:
	docker compose -f docker-compose.dev.yml build

dev-docker-up:
	docker compose -f docker-compose.dev.yml up
