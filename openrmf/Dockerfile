# Use official production image for OpenRMF web (part of the multi-container stack; full stack handled in compose)
# Use latest stable version for production
# For HTTPS in production
# Adjust if needed based on entrypoint

FROM cingulara/openrmf-web:1.13.00  
COPY config/nginx.conf /etc/nginx/nginx.conf
COPY config/ssl/ /etc/ssl/   
ENV ASPNETCORE_ENVIRONMENT=Production
CMD ["dotnet", "OpenRMF.Web.dll"]  