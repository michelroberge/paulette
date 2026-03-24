.PHONY: build clean dev

build:
	cd frontend && npm ci && npm run build
	rm -rf backend/static
	cp -r frontend/dist backend/static
	cd backend && go build -o ../claudine .

clean:
	rm -f claudine
	rm -rf backend/static

dev:
	cd frontend && npm run dev
