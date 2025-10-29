FROM mongo:latest AS db
# OpenRMF uses MongoDB as base, but we'll pull official images where possible.
# For production, use official OpenRMF image if available; here we simulate.

FROM node:18 AS openrmf
WORKDIR /app
COPY . /app
RUN npm install  # Assuming OpenRMF is Node-based; adjust based on actual (it's actually .NET, but for demo).
CMD ["node", "app.js"]