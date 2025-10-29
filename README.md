# OpenRisk - Unified Risk & Threat Intelligence Management Platform

## Overview
OpenRisk is an open-source platform that integrates OpenRMF, TheHive, Cortex, and OpenCTI to centralize risk management, incidents, threats, and corrective actions. It is infrastructure-agnostic, supporting Docker, Kubernetes, or bare Linux servers. The project is designed with production deployment in mind, using official production images where available, multi-stage Docker builds for custom components, secure configurations, and observability.

## Components
- **openrmf/**: Risk registry and compliance management (full multi-container stack using official images).
- **thehive/**: Incident and investigation management.
- **cortex/**: Automation and analysis execution.
- **opencti/**: Threat contextualization and global vulnerability sync.
- **sync-engine/**: Integration scripts for API syncing and cronjobs.
- **dashboard/**: React + FastAPI dashboard for visualization.
- **docs/**: Documentation files.
- **k8s/**: Kubernetes manifests.
- **helm/**: Optional Helm chart for production Kubernetes deployments.

## License
MIT License