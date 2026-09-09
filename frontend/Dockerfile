# Frontend Dockerfile
# Multi-stage build for optimized production image

# Stage 1: Build
FROM node:20-alpine AS builder
WORKDIR /app

# Install dependencies
COPY package*.json ./
RUN npm ci

# Copy source code
COPY . .

# Build the application
RUN npm run build

# Stage 2: Runtime
FROM node:20-alpine

WORKDIR /app

# Install serve to serve static files
RUN npm install -g serve

# Copy built application from builder, owned by the base image's built-in
# non-root `node` user (UID/GID 1000). We reuse `node` rather than creating a new
# UID/GID 1000 user — the alpine base already occupies 1000, so `addgroup -g 1000`
# would fail (this broke the container build on a clean runner).
COPY --from=builder --chown=node:node /app/dist ./dist
USER node

EXPOSE 5173

# For development with hot-reload, use: npm run dev -- --host 0.0.0.0
# For production, serve the static files
CMD ["serve", "-s", "dist", "-l", "5173"]
