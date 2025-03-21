#!/bin/bash
# filepath: /Users/andreas/Projects/turkis/dev/manager/run.sh
# Go to the project root directory
cd $(git rev-parse --show-toplevel)

docker run -it --rm \
  --name turkis-manager-dev \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v $(pwd):/src \
  --network turkis-public \
  -e DRY_RUN=true \
  turkis-manager-dev
