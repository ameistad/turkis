# Delete the container if it exists
docker rm -f my-nginx-container
# Run the container
#docker run --name my-nginx-container --network turkis-public -p 8080:80 my-nginx

# Generate a deployment ID to use in multiple places
DEPLOYMENT_ID=$(date +%Y%m%d%H%M%S)

docker run --name my-nginx-container-${DEPLOYMENT_ID} \
  --network turkis-public \
  -l "turkis.app=my-nginx-container" \
  -l "turkis.deployment=${DEPLOYMENT_ID}" \
  -l "turkis.tls.email=test@turkis.dev" \
  -l "turkis.domain.0=domain.com" \
  -l "turkis.domain.0.canonical=domain.com" \
  -l "turkis.domain.0.alias.0=www.domain.com" \
  -l "turkis.domain.0.alias.1=m.domain.com" \
  -l "turkis.domain.1=api.domain.com" \
  -l "turkis.domains.all=domain.com,www.domain.com,m.domain.com,api.domain.com" \
  -l "turkis.health-check-path=/health" \
  -l "turkis.drain-time=15s" \
  -e "NODE_ENV=production" \
  my-nginx
