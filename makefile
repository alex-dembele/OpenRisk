.PHONY: build up down logs clean test deploy

build:
	docker compose build

up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f

clean:
	docker compose down -v
	docker system prune -f

test:
	docker compose up -d
	# Ajoutez pytest ou tests frontend ici
	docker compose down

deploy: build up
	@echo "Deployment complete. Go to http://localhost:3000"