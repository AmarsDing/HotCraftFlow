$root = Split-Path -Parent $PSScriptRoot
docker compose -f "$root\infra\docker-compose.yml" down
