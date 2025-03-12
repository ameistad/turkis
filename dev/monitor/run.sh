# Go to the project root directory
cd $(git rev-parse --show-toplevel)

docker run -it --rm \
  --name turkis-monitor-dev \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v $(pwd):/src \
  --network host \
  -e DRY_RUN=true \
  turkis-monitor-dev
