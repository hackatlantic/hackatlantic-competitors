$ErrorActionPreference = "Stop"

$avastRoot = Get-ChildItem Cert:\CurrentUser\Root |
    Where-Object { $_.Subject -like "*Avast Web/Mail Shield Root*" } |
    Select-Object -First 1

if ($null -ne $avastRoot) {
    $certificatePath = Join-Path $env:TEMP "hackatlantic-avast-web-shield-root.pem"
    $base64 = [Convert]::ToBase64String($avastRoot.RawData)
    $certificateLines = [regex]::Matches($base64, ".{1,64}") |
        ForEach-Object { $_.Value }
    $pem = @("-----BEGIN CERTIFICATE-----") +
        $certificateLines +
        @("-----END CERTIFICATE-----")
    Set-Content -LiteralPath $certificatePath -Value $pem -Encoding ascii
    $env:NODE_EXTRA_CA_CERTS = $certificatePath
    Write-Host "Using the installed Avast HTTPS inspection certificate for Node development traffic."
}

& (Join-Path $PSScriptRoot "..\node_modules\.bin\next.cmd") dev
exit $LASTEXITCODE
