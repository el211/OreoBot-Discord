$env:GOOS = 'linux'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
Set-Location 'C:\Users\PC\GolandProjects\OreoBot'
go build -o bot.bin .
Write-Host "Exit code: $LASTEXITCODE"
