$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Write-Host "Starting infra (postgres+redis)..."
docker compose -f "$root\infra\docker-compose.yml" up -d
Write-Host "Then start admin from apps\server\app\admin (make run / go run ./cmd)"
