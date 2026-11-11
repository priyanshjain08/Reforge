# Creates .env with random secrets. Usage (PowerShell, from the repo root):  .\scripts\setup.ps1
if (Test-Path .env) { Write-Host ".env already exists; leaving it unchanged."; exit 0 }
function New-Secret { -join ((48..57) + (97..122) | Get-Random -Count 40 | ForEach-Object { [char]$_ }) }
$admin = New-Secret
@"
REFORGE_ADMIN_TOKEN=$admin
REFORGE_VIEWER_TOKEN=
REFORGE_AGENT_SECRET=$(New-Secret)
REFORGE_RECOVERY_MODE=approval
"@ | Set-Content -Encoding ascii .env
Write-Host "Created .env. Your dashboard login token is:"
Write-Host $admin
