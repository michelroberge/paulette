.PHONY: build clean dev

build:
	cd frontend && npm ci && npm run build
	rm -rf backend/static
	cp -r frontend/dist backend/static
	cd backend && go build -o ai-app-factory .

clean:
	rm -rf backend/static backend/ai-app-factory

dev:
	cd frontend && npm run dev
