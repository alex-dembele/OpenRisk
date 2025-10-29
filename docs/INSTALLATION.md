# Installation Guide

## Local Docker (Development)
...

## Production Deployment
- Use docker-compose with .env for secrets.
- Enable HTTPS: Mount certs, update ports to 8443.
- Scale services: Adjust replicas in K8s/Helm.
- Existing Infra: Deploy on Linux with systemd for services, or use K8s cluster. Secure DBs with auth, use managed services (e.g., AWS RDS for Postgres).
- Monitoring: Integrate with external Prometheus.
- Backup: Cron for volume backups.