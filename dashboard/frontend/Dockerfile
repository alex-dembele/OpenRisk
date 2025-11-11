# Multi-stage for production
FROM node:18 AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci  
COPY . .
RUN npm run build

FROM node:18-slim
WORKDIR /app
COPY --from=builder /app/build /app/build
RUN npm install -g serve
USER node
CMD ["serve", "-s", "build", "-l", "3000"]