$base = "http://localhost:8080/mcp"
$token = "test-secret"

$headers = @{
    "Authorization" = "Bearer $token"
    "Accept"        = "application/json, text/event-stream"
    "Content-Type"  = "application/json"
}

# 1) initialize
Write-Host "=== initialize ===" -ForegroundColor Cyan
$initBody = @{
    jsonrpc = "2.0"
    id      = 1
    method  = "initialize"
    params  = @{
        protocolVersion = "2024-11-05"
        capabilities    = @{}
        clientInfo      = @{ name = "ps-test"; version = "1" }
    }
} | ConvertTo-Json -Depth 10

$resp = Invoke-WebRequest -Uri $base -Method Post -Headers $headers -Body $initBody
$sid = $resp.Headers["Mcp-Session-Id"]
Write-Host "session: $sid" -ForegroundColor Green
Write-Host $resp.Content

# 2) 通知 initialized（无响应）
Write-Host "=== notifications/initialized ===" -ForegroundColor Cyan
$notifyBody = @{
    jsonrpc = "2.0"
    method  = "notifications/initialized"
} | ConvertTo-Json

$headers2 = $headers.Clone()
$headers2["Mcp-Session-Id"] = $sid
try {
    Invoke-WebRequest -Uri $base -Method Post -Headers $headers2 -Body $notifyBody | Out-Null
} catch {}

# 3) tools/list
Write-Host "=== tools/list ===" -ForegroundColor Cyan
$listBody = @{
    jsonrpc = "2.0"
    id      = 2
    method  = "tools/list"
} | ConvertTo-Json

$resp = Invoke-WebRequest -Uri $base -Method Post -Headers $headers2 -Body $listBody
Write-Host $resp.Content

# 4) tools/call list_boards
Write-Host "=== tools/call list_boards ===" -ForegroundColor Cyan
$callBody = @{
    jsonrpc = "2.0"
    id      = 3
    method  = "tools/call"
    params  = @{
        name      = "list_boards"
        arguments = @{}
    }
} | ConvertTo-Json -Depth 10

$resp = Invoke-WebRequest -Uri $base -Method Post -Headers $headers2 -Body $callBody
Write-Host $resp.Content