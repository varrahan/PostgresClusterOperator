#!/bin/bash

PROJECT_ROOT=$(dirname $(dirname $(realpath $0)))

ENV_FILE=${PROJECT_ROOT}/.env
source "$ENV_FILE"


mkdir -p ${PROJECT_ROOT}/local-storage/postgres-{data,backups}
chmod 777 ${PROJECT_ROOT}/local-storage/*

cat <<EOF > ${PROJECT_ROOT}/config/samples/local-secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: pg-user-secret
type: Opaque
stringData:
  password: "$PG_USER_PASSWORD"
EOF

echo "Password set to: $PG_USER_PASSWORD"
echo "Local storage setup complete!"