$ErrorActionPreference = "Stop"

$oldCgoEnabled = [Environment]::GetEnvironmentVariable("CGO_ENABLED", "Process")
$oldGoos = [Environment]::GetEnvironmentVariable("GOOS", "Process")
$oldGoarch = [Environment]::GetEnvironmentVariable("GOARCH", "Process")

Push-Location $PSScriptRoot
try {
    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"

    go build -trimpath -ldflags="-s -w" -o logRead .
    if ($LASTEXITCODE -ne 0) {
        throw "go build failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
    [Environment]::SetEnvironmentVariable("CGO_ENABLED", $oldCgoEnabled, "Process")
    [Environment]::SetEnvironmentVariable("GOOS", $oldGoos, "Process")
    [Environment]::SetEnvironmentVariable("GOARCH", $oldGoarch, "Process")
}
