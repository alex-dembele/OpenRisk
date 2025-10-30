#!/bin/bash
set -e

echo "Cloning the repo..."
git clone https://github.com/alex-dembele/OpenRiskOps.git || cd OpenRisk
cd OpenRisk

echo "Configuring .env..."
cp .env.example .env
# Edit .env manually or automatically with read -s for passwords

echo "Build and start..."
make build
make up

echo "Verifying..."
docker compose ps

echo "Dashboard ready! http://localhost:3000"
echo "Configure API keys in .env and reboot if needed."