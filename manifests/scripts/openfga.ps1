[CmdletBinding()]
param(
    [Parameter(Mandatory = $true, Position = 0)]
    [ValidateSet('init', 'apply')]
    [string]$Command,
    [string]$ApiUrl,
    [string]$Model = 'manifests/openfga/fga.mod',
    [string]$StoreName,
    [string]$StoreId,
    [string]$EnvFile = '.env'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if (-not (Get-Command fga -ErrorAction SilentlyContinue)) {
    throw 'required command not found: fga'
}
if (-not (Test-Path -LiteralPath $Model -PathType Leaf)) {
    throw "model file not found: $Model"
}

function Get-DotEnvValue([string]$Key) {
    if (-not (Test-Path -LiteralPath $EnvFile -PathType Leaf)) { return $null }
    foreach ($line in (Get-Content -LiteralPath $EnvFile)) {
        if ($line -match '^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)=(.*)$' -and $Matches[1] -eq $Key) {
            $value = $Matches[2].Trim()
            $value = $value -replace '\s+#.*$', ''
            if (($value.StartsWith("'") -and $value.EndsWith("'")) -or ($value.StartsWith('"') -and $value.EndsWith('"'))) {
                $value = $value.Substring(1, $value.Length - 2)
            }
            return $value
        }
    }
    return $null
}

function Get-ConfigValue([string]$Key) {
    $value = [Environment]::GetEnvironmentVariable($Key)
    if (-not [string]::IsNullOrWhiteSpace($value)) { return $value }
    return Get-DotEnvValue $Key
}

if ([string]::IsNullOrWhiteSpace($ApiUrl)) {
    $ApiUrl = Get-ConfigValue 'FGA_API_URL'
    if ([string]::IsNullOrWhiteSpace($ApiUrl)) { $ApiUrl = 'http://localhost:18080' }
}
if ([string]::IsNullOrWhiteSpace($StoreName)) {
    $StoreName = if ($env:OPENFGA_STORE_NAME) { $env:OPENFGA_STORE_NAME } else { 'plateau' }
}

$storeIdExplicit = -not [string]::IsNullOrWhiteSpace($StoreId)
function Set-DotEnvValues([hashtable]$Values) {
    $filePath = if ([IO.Path]::IsPathRooted($EnvFile)) { $EnvFile } else { Join-Path (Get-Location) $EnvFile }
    $filePath = [IO.Path]::GetFullPath($filePath)
    $parent = Split-Path -Parent $filePath
    if ($parent) { New-Item -ItemType Directory -Force -Path $parent | Out-Null }
    $lines = if (Test-Path -LiteralPath $filePath -PathType Leaf) { @(Get-Content -LiteralPath $filePath) } else { @() }
    $updated = @{}
    $result = @()
    foreach ($line in $lines) {
        if ($line -match '^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)=' -and $Values.ContainsKey($Matches[1])) {
            $key = $Matches[1]
            if (-not $updated.ContainsKey($key)) {
                $updated[$key] = $true
                $result += "$key=$([string]$Values[$key])"
            }
        } else {
            $result += $line
        }
    }
    foreach ($key in $Values.Keys) {
        if (-not $updated.ContainsKey($key)) { $result += "$key=$([string]$Values[$key])" }
    }

    $tempPath = "$filePath.tmp.$([Guid]::NewGuid().ToString('N'))"
    try {
        [System.IO.File]::WriteAllLines($tempPath, [string[]]$result)
        if (Test-Path -LiteralPath $filePath -PathType Leaf) {
            [System.IO.File]::Replace($tempPath, $filePath, $null)
        } else {
            [System.IO.File]::Move($tempPath, $filePath)
        }
    } finally {
        if (Test-Path -LiteralPath $tempPath -PathType Leaf) { Remove-Item -LiteralPath $tempPath -Force }
    }
}

function Invoke-Fga([string[]]$CommandArgs) {
    $args = @('--api-url', $ApiUrl)
    $output = & fga @args @CommandArgs 2>&1
    if ($LASTEXITCODE -ne 0) { throw "fga command failed: $($CommandArgs -join ' ')" }
    return ($output -join "`n")
}

function Get-JsonId([string]$Json, [string]$Kind) {
    try { $parsed = $Json | ConvertFrom-Json } catch { throw "OpenFGA $Kind response was not valid JSON" }
    $id = if ($Kind -eq 'store') {
        if ($parsed.store) { $parsed.store.id } else { $parsed.id }
    } else {
        if ($parsed.authorization_model) { $parsed.authorization_model.id } elseif ($parsed.authorization_model_id) { $parsed.authorization_model_id } else { $parsed.id }
    }
    if ([string]::IsNullOrWhiteSpace([string]$id)) { throw "OpenFGA $Kind response returned no ID" }
    return [string]$id
}

function Find-StoreIdByName {
    $response = Invoke-Fga @('store', 'list')
    try { $parsed = $response | ConvertFrom-Json } catch { throw 'OpenFGA store list response was not valid JSON' }
    $matches = @($parsed.stores | Where-Object { $_.name -eq $StoreName })
    if ($matches.Count -gt 1) { throw "multiple OpenFGA stores found with exact name: $StoreName" }
    if ($matches.Count -eq 0) { return $null }
    $id = [string]$matches[0].id
    if ([string]::IsNullOrWhiteSpace($id)) { throw 'matching OpenFGA store returned no store ID' }
    return $id
}

function Write-Model {
    $response = Invoke-Fga @('model', 'write', '--store-id', $StoreId, '--file', $Model)
    return Get-JsonId $response 'model'
}

function Write-InitEnvironment([string]$ModelId) {
    Set-DotEnvValues @{
        FGA_API_URL = $ApiUrl
        FGA_STORE_ID = $StoreId
        FGA_MODEL_ID = $ModelId
    }
}

if ($Command -eq 'init') {
    if (-not $storeIdExplicit) { $StoreId = Find-StoreIdByName }
    if ([string]::IsNullOrWhiteSpace($StoreId)) {
        $StoreId = Get-JsonId (Invoke-Fga @('store', 'create', '--name', $StoreName)) 'store'
    }
    $modelId = Write-Model
    Write-InitEnvironment $modelId
    Write-Output "OpenFGA store ready: $StoreId"
    Write-Output "OpenFGA model ready: $modelId"
} else {
    if ([string]::IsNullOrWhiteSpace($StoreId)) { $StoreId = Get-ConfigValue 'FGA_STORE_ID' }
    if ([string]::IsNullOrWhiteSpace($StoreId)) { throw 'store ID required: use -StoreId or FGA_STORE_ID' }
    $modelId = Write-Model
    Set-DotEnvValues @{ FGA_MODEL_ID = $modelId }
    Write-Output "OpenFGA model applied: $modelId"
}
