# Latest stable production tag
# Run as non-root
FROM opencti/platform:6.3.3  
ENV REDIS_URL=redis://redis:6379
COPY config/production.yml /etc/opencti/config.yml
USER opencti  