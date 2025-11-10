#!/bin/bash
set -e

echo "Vérification prerequisites..."
command -v docker > /dev/null || { echo "Docker non installé. Installez via https://docs.docker.com/install"; exit 1; }
command -v docker compose > /dev/null || { echo "Docker Compose non installé. Installez via https://docs.docker.com/compose/install"; exit 1; }

echo "Login Docker (optionnel pour éviter rate-limits)..."
docker login || echo "Login skipped – Si erreurs pull, créez compte hub.docker.com et retry."

echo "Configuration .env..."
if [ ! -f .env ]; then
  cp .env.example .env
  echo "Éditez .env avec vos secrets (ex. nano .env)"
  exit 0
fi

echo "Build et démarrage..."
docker compose build
docker compose up -d

echo "Vérification services..."
docker compose ps

echo "OpenRisk prêt ! Dashboard: http://localhost:3000"
echo "Si erreurs, voir logs: docker compose logs"
echo "Troubleshooting: Vérifiez ports libres (netstat -tuln), volumes (docker volume ls), ou login Docker."