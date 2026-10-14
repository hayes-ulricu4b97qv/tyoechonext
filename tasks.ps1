param(
    [ValidateSet('dev', 'web-build', 'api-run', 'build', 'fmt-check', 'clean')]
    [string]$Task = 'build'
)

$ErrorActionPreference = 'Stop'
if ($env:OS -ne 'Windows_NT') {
    throw 'Typecho Next supports Windows only.'
}

function Invoke-CommandChecked {
    param([string]$Executable, [string[]]$Arguments)
    & $Executable @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$Executable exited with code $LASTEXITCODE"
    }
}

function Build-Web {
    Push-Location (Join-Path $PSScriptRoot 'web')
    try {
        Invoke-CommandChecked 'npm.cmd' @('ci')
        Invoke-CommandChecked 'npm.cmd' @('run', 'build')
    } finally {
        Pop-Location
    }
}

function Test-GoFormat {
    $goFiles = @(git ls-files --cached --others --exclude-standard -- '*.go')
    if ($LASTEXITCODE -ne 0) {
        throw "git ls-files exited with code $LASTEXITCODE"
    }

    $unformatted = @()
    foreach ($goFile in $goFiles) {
        $result = & gofmt -l $goFile
        if ($LASTEXITCODE -ne 0) {
            throw "gofmt failed for $goFile with code $LASTEXITCODE"
        }
        if ($result) {
            $unformatted += $result
        }
    }
    if ($unformatted.Count -gt 0) {
        throw "Run gofmt on:`n$($unformatted -join "`n")"
    }
    Write-Output 'All Go files are gofmt-formatted.'
}

Push-Location $PSScriptRoot
try {
    switch ($Task) {
        'dev' {
            Push-Location (Join-Path $PSScriptRoot 'web')
            try { Invoke-CommandChecked 'npm.cmd' @('run', 'dev') } finally { Pop-Location }
        }
        'web-build' { Build-Web }
        'api-run' { Invoke-CommandChecked 'go' @('run', './cmd/server') }
        'fmt-check' { Test-GoFormat }
        'build' {
            Build-Web
            New-Item -ItemType Directory -Force -Path (Join-Path $PSScriptRoot 'bin') | Out-Null
            Invoke-CommandChecked 'go' @('build', '-o', './bin/typecho-next.exe', './cmd/server')
        }
        'clean' {
            $cleanupTargets = @()
            foreach ($relativePath in @('web/dist', 'bin')) {
                $targetPath = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot $relativePath))
                $workspacePrefix = [IO.Path]::GetFullPath($PSScriptRoot).TrimEnd('\') + '\'
                if (-not $targetPath.StartsWith($workspacePrefix, [StringComparison]::OrdinalIgnoreCase)) {
                    throw "Cleanup target is outside the project: $targetPath"
                }
                if (Test-Path -LiteralPath $targetPath) {
                    $targetItem = Get-Item -LiteralPath $targetPath -Force
                    if (($targetItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
                        throw "Cleanup target is a link and will not be followed: $targetPath"
                    }
                    if ($targetItem.PSIsContainer) {
                        $nestedLink = Get-ChildItem -LiteralPath $targetPath -Force -Recurse |
                            Where-Object { ($_.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 } |
                            Select-Object -First 1
                        if ($null -ne $nestedLink) {
                            throw "Cleanup target contains a link and will not be followed: $($nestedLink.FullName)"
                        }
                    }
                }
                $cleanupTargets += $targetPath
            }
            foreach ($targetPath in $cleanupTargets) {
                if (Test-Path -LiteralPath $targetPath) {
                    Remove-Item -LiteralPath $targetPath -Recurse -Force
                }
            }
        }
    }
} finally {
    Pop-Location
}
