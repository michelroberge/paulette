.PHONY: build clean dev

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
