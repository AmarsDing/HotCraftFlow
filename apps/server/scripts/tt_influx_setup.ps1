#requires -Version 7.0
<#
.SYNOPSIS
  ② TT InfluxDB 初始化（评审 A-17）：org=media，bucket 三层 mm_raw(90d)/mm_1h(1y)/mm_1d(3y)，
  降采样 Flux 任务 raw→1h→1d，tt-writer / tt-reader 最小权限 token。幂等，可重复执行。

.DESCRIPTION
  评审 A-17：tag 只允许 topic_id + platform（S-12/S-13 基数治理）；
  幂等由 (measurement + tag set + t_bucket) 覆盖写兜底，降采样本脚本只做 mean 收敛。
  M-16：2.9+ 对 token 落盘做散列，--json 之外的查询不再回显明文；本脚本创建的
  token 只在创建当下打印一次，务必立即写入环境变量 INFLUX_TOKEN（admin 服务 tt.yaml
  用 ${INFLUX_TOKEN} 引用，禁止明文入库）。

.EXAMPLE
  $env:INFLUX_ADMIN_TOKEN = "<首次 setup 的 operator token>"
  ./tt_influx_setup.ps1
  ./tt_influx_setup.ps1 -InfluxExe "D:/influxdb2-2.9.1-windows_amd64/influx.exe" -Url "http://127.0.0.1:8086"
#>
[CmdletBinding()]
param(
  [string]$InfluxExe = "influx",                          # 或 D:/influxdb2-2.9.1-windows_amd64/influx.exe
  [string]$Url      = "http://127.0.0.1:8086",
  [string]$Org      = "media",
  [string]$AdminToken = $env:INFLUX_ADMIN_TOKEN,
  [switch]$SkipTasks,                                     # 只建 bucket/token，不建降采样任务
  [switch]$SkipTokens
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($AdminToken)) {
  throw "需要 operator token：传 -AdminToken 或设环境变量 INFLUX_ADMIN_TOKEN（本机初始化见 docs 02-TT §1）"
}

function Invoke-Influx([string[]]$Args, [switch]$AsJson) {
  $full = @("--host", $Url, "--token", $AdminToken) + $Args
  $out = & $InfluxExe @full 2>&1
  if ($LASTEXITCODE -ne 0) { throw "influx $($Args -join ' ') 失败：$out" }
  if ($AsJson) { return ($out | ConvertFrom-Json) }
  return $out
}

# ---------------------------------------------------------------- org
$orgs = Invoke-Influx @("org", "list", "--json") -AsJson
$orgId = ($orgs | Where-Object { $_.name -eq $Org } | Select-Object -First 1).id
if (-not $orgId) {
  Write-Host "创建 org $Org"
  Invoke-Influx @("org", "create", "-n", $Org) | Out-Null
  $orgs = Invoke-Influx @("org", "list", "--json") -AsJson
  $orgId = ($orgs | Where-Object { $_.name -eq $Org } | Select-Object -First 1).id
}
Write-Host "org $Org id=$orgId"

# ---------------------------------------------------------------- buckets（A-17）
$bucketPlan = @(
  @{ Name = "mm_raw"; RetentionSeconds = 7776000;  Desc = "原始采样点，90d" },
  @{ Name = "mm_1h";  RetentionSeconds = 31536000; Desc = "1h 聚合，1y" },
  @{ Name = "mm_1d";  RetentionSeconds = 94608000; Desc = "1d 聚合，3y" }
)
$bucketIds = @{}
$existing = Invoke-Influx @("bucket", "list", "--org", $Org, "--json") -AsJson
foreach ($b in $bucketPlan) {
  $hit = $existing | Where-Object { $_.name -eq $b.Name } | Select-Object -First 1
  if ($hit) {
    Write-Host "bucket $($b.Name) 已存在（id=$($hit.id)，保留 $($hit.retentionSeconds)s），对齐保留期"
    Invoke-Influx @("bucket", "update", "-i", $hit.id, "-r", "$($b.RetentionSeconds)s") | Out-Null
    $bucketIds[$b.Name] = $hit.id
  } else {
    Write-Host "创建 bucket $($b.Name)（$($b.Desc)）"
    Invoke-Influx @("bucket", "create", "-n", $b.Name, "-o", $Org, "-r", "$($b.RetentionSeconds)s") | Out-Null
    $created = Invoke-Influx @("bucket", "list", "--org", $Org, "--json") -AsJson |
      Where-Object { $_.name -eq $b.Name } | Select-Object -First 1
    $bucketIds[$b.Name] = $created.id
  }
}

# ---------------------------------------------------------------- downsample tasks
# 字段集与 data/tt/influx_client.go toPoint 保持一致；布尔/哨兵字段不进聚合。
$numericFields = "rank|rank_total|heat_score|views|likes|comments|eng_rate|velocity|accel|rank_pct|eng_pct|freshness|breadth|sample_count"

$task1h = @"
option task = {name: "tt_downsample_1h", every: 1h, offset: 5m}

// 窗口 2h（重叠 1h 覆盖迟到点，⚠️ late>24h 的数据不会进入聚合，属 A-14 预期）
from(bucket: "mm_raw")
  |> range(start: -2h)
  |> filter(fn: (r) => r._measurement == "topic_heat")
  |> filter(fn: (r) => r._field =~ /^($numericFields)`$/)
  |> aggregateWindow(every: 1h, fn: mean, createEmpty: false)
  |> to(bucket: "mm_1h", org: "$Org")
"@

$task1d = @"
option task = {name: "tt_downsample_1d", every: 1d, offset: 15m}

// 窗口 2d（重叠 1d；3y 长期趋势的幂等回填，同 (_measurement,tag,t) 覆盖写）
from(bucket: "mm_1h")
  |> range(start: -2d)
  |> filter(fn: (r) => r._measurement == "topic_heat")
  |> filter(fn: (r) => r._field =~ /^($numericFields)`$/)
  |> aggregateWindow(every: 1d, fn: mean, createEmpty: false)
  |> to(bucket: "mm_1d", org: "$Org")
"@

if (-not $SkipTasks) {
  $tasks = Invoke-Influx @("task", "list", "--org", $Org, "--json") -AsJson
  foreach ($pair in @(@{Name="tt_downsample_1h"; Flux=$task1h}, @{Name="tt_downsample_1d"; Flux=$task1d})) {
    $hit = $tasks | Where-Object { $_.name -eq $pair.Name } | Select-Object -First 1
    $fluxFile = New-TemporaryFile
    Set-Content -Path $fluxFile -Value $pair.Flux -Encoding UTF8
    if ($hit) {
      Write-Host "task $($pair.Name) 已存在（id=$($hit.id)），更新 Flux"
      Invoke-Influx @("task", "update", "-i", $hit.id, "--flux", "@$fluxFile") | Out-Null
    } else {
      Write-Host "创建 task $($pair.Name)"
      Invoke-Influx @("task", "create", "--org", $Org, "--flux", "@$fluxFile") | Out-Null
    }
    Remove-Item $fluxFile -Force
  }
}

# ---------------------------------------------------------------- tokens（最小权限）
$writerBuckets = @()
foreach ($n in @("mm_raw", "mm_1h", "mm_1d")) {
  $writerBuckets += @("--write-bucket", $bucketIds[$n])
}
$readerBuckets = @()
foreach ($n in @("mm_raw", "mm_1h", "mm_1d")) {
  $readerBuckets += @("--read-bucket", $bucketIds[$n])
}

function Ensure-Token([string]$Desc, [string[]]$Args) {
  $auths = Invoke-Influx @("auth", "list", "--org", $Org, "--json") -AsJson
  $hit = $auths | Where-Object { $_.description -eq $Desc } | Select-Object -First 1
  if ($hit) {
    Write-Warning "token '$Desc' 已存在（id=$($hit.id)）。2.9+ 散列存储无法回显明文；" +
      "如需轮换：influx auth inactive -i $($hit.id) 后重跑本脚本。"
    return $null
  }
  $auth = Invoke-Influx (@("auth", "create", "--org", $Org, "--description", $Desc, "--json") + $Args) -AsJson
  return $auth.token
}

if (-not $SkipTokens) {
  $writer = Ensure-Token "tt-writer (admin 服务快照写)" $writerBuckets
  $reader = Ensure-Token "tt-reader (看板/查询)" $readerBuckets

  Write-Host ""
  Write-Host "================ TT InfluxDB 初始化完成 ================" -ForegroundColor Green
  Write-Host "org=$Org  buckets: mm_raw(90d) mm_1h(1y) mm_1d(3y)  tasks: tt_downsample_1h / tt_downsample_1d"
  if ($writer) {
    Write-Host "[一次性显示，立即保存] tt-writer token:" -ForegroundColor Yellow
    Write-Host "  $writer"
    Write-Host "配置方式（禁明文入库）：[Environment]::SetEnvironmentVariable('INFLUX_TOKEN', '<token>', 'User')"
  }
  if ($reader) {
    Write-Host "[一次性显示，立即保存] tt-reader token:" -ForegroundColor Yellow
    Write-Host "  $reader"
  }
} else {
  Write-Host "已跳过 token 创建（-SkipTokens）。"
}
