
DEPLOYMENT_ID=$(date +%Y%m%d%H%M%S)
DEPLOYMENT_ID_STATIC=20250318152205

docker run --name my-nginx-container-two-${DEPLOYMENT_ID} \
  --network turkis-public \
  -l "turkis.appName=my-nginx-container" \
  -l "turkis.deployment-id=${DEPLOYMENT_ID}" \
  -l "turkis.acme.email=test@turkis.dev" \
  -l "turkis.domain.0=two.domain.com" \
  -l "turkis.health-check-path=/health" \
  -l "turkis.port=80" \
  -l "turkis.drain-time=15s" \
  -e "NODE_ENV=production" \
  my-nginx


  # -l "turkis.domain.0.alias.0=www.domain.com" \
  # -l "turkis.domain.0.alias.1=m.domain.com" \
  # -l "turkis.domain.1=api.domain.com" \
