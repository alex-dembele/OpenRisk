# Installation Guide for OpenRiskOps

## Overview
This guide covers deployment options for OpenRiskOps, including local development with Docker, production on bare Linux servers, and Kubernetes clusters. The platform is designed to be infrastructure-agnostic, but production deployments should prioritize security, scalability, and high availability.

## Prerequisites
- Docker 24+ and Docker Compose v2+ for containerized setups.
- Kubernetes 1.28+ with Helm 3+ for cluster deployments.
- Linux server (Ubuntu 22.04+ or equivalent) with at least 16GB RAM, 8 CPU cores, 100GB storage for production.
- Access to a container registry (e.g., Docker Hub, ECR) for pushing custom images.
- Secrets management (e.g., .env files, Kubernetes Secrets, HashiCorp Vault).
- HTTPS certificates for production (self-signed for testing, Let's Encrypt or CA-issued for prod).
- Backup tools (e.g., Velero for K8s, rsync for volumes).

## Local Development with Docker
1. Clone the repository: `git clone https://github.com/alex-dembele/OpenRisk.git`
2. Navigate to the root: `cd OpenRisk`
3. Copy `.env.example` to `.env` and fill in secrets (e.g., API keys, DB passwords).
4. Build and start: `docker compose up -d --build`
5. Access services:
   - Dashboard: http://localhost:3000
   - OpenRMF: http://localhost:8080
   - TheHive: http://localhost:9000
   - Cortex: http://localhost:9001
   - OpenCTI: http://localhost:8082
6. Stop: `docker compose down`

## Production Deployment with Docker on Linux Server
For production, use a dedicated server or VM. Enable HTTPS, persistent storage, and monitoring.

1. Install Docker and Compose: Follow official docs[](https://docs.docker.com/engine/install/).
2. Clone repo and configure `.env` with production values (e.g., strong passwords, external DB URIs if using managed services like AWS RDS).
3. Update `docker-compose.yml` for production:
   - Change ports to 443 for HTTPS.
   - Mount persistent volumes (e.g., /opt/openriskops/volumes).
   - Set `restart: always` for all services.
   - Use production images (e.g., tagged versions instead of :latest).
4. Build and start: `docker compose up -d --build --force-recreate`
5. Configure NGINX reverse proxy for HTTPS (example config in /openrmf/nginx.conf).
6. Set up systemd service for auto-start: Create /etc/systemd/system/openrisk.service
`
[Unit]
Description=OpenRiskOps
After=docker.service
[Service]
WorkingDirectory=/path/to/OpenRiskOps
ExecStart=/usr/bin/docker compose up -d
ExecStop=/usr/bin/docker compose down
Restart=always
[Install]
WantedBy=multi-user.target
`
   Enable: `systemctl enable openrisk`
7. Monitoring: Access Grafana at http://localhost:3001 (default admin/admin).
8. Backups: Schedule cronjobs for volume backups, e.g., `rsync -a /var/lib/docker/volumes /backup/`.

## Deployment on Kubernetes
1. Create namespace: `kubectl apply -f k8s/namespace.yaml`
2. Create secrets: `kubectl create secret generic openrisk-secrets --namespace=openrisk --from-env-file=.env`
3. Apply all manifests: `kubectl apply -f k8s/`
4. For Helm: `helm install openrisk ./helm --namespace openrisk --create-namespace`
5. Access via Ingress (configure domain and certs).
6. Scale: Edit replicas in deployments (e.g., `kubectl scale deployment openrmf-web --replicas=3`).
7. Monitoring: Deploy Prometheus Operator if not using built-in.

## Integration with Existing Infrastructure
If deploying into an existing setup (e.g., shared DB cluster):
1. Modify DB services in docker-compose.yml or k8s/statefulset-db.yaml to point to external hosts (e.g., RDS for Postgres, MongoDB Atlas).
2. Update env vars: e.g., `MONGO_URI=mongodb+srv://user:pass@cluster.mongodb.net/openrmf`
3. For sync-engine, ensure API endpoints are accessible (use service discovery in K8s).
4. Test integrations: Run manual syncs via `docker exec -it sync-engine python sync.py`
5. Security: Use network policies in K8s to isolate namespaces.
6. Logging: Integrate with ELK stack or Splunk by mounting /var/log.

## Troubleshooting
- Logs: `docker logs <container>` or `kubectl logs <pod>`
- Common issues: Port conflicts (change in compose), insufficient resources (increase limits), auth errors (check .env).
- Upgrades: Pull new images, `docker compose up -d --pull always`