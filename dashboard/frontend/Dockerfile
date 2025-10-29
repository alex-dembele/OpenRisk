# Multi-stage for production
FROM node:18 AS builder
WORKDIR /app
COPY package.json yarn.lock ./
RUN yarn install --frozen-lockfile
COPY . .
RUN yarn build

FROM node:18-slim
WORKDIR /app
COPY --from=builder /app/build /app/build
RUN npm install -g serve
USER node
CMD ["serve", "-s", "build", "-l", "3000"]git 