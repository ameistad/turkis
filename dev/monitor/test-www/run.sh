# Delete the container if it exists
docker rm -f my-nginx-container
# Run the container
#docker run --name my-nginx-container --network turkis-public -p 8080:80 my-nginx

docker run --name my-nginx-container-$(date +%Y%m%d%H%M%S) \
  --network turkis-public \
  -l "turkis.app=my-nginx-container" \
  -l "turkis.deployment=$(date +%Y%m%d%H%M%S)" \
  -l "turkis.domain.0=domain.com" \
  -l "turkis.domain.0.canonical=domain.com" \
  -l "turkis.domain.0.alias.0=www.domain.com" \
  -l "turkis.domain.0.alias.1=m.domain.com" \
  -l "turkis.domain.1=api.domain.com" \
  -l "turkis.domains.all=domain.com,www.domain.com,m.domain.com,api.domain.com" \
  -e "NODE_ENV=production" \
  my-nginx
