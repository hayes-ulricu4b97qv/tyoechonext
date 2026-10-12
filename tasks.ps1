param(
    [ValidateSet('dev', 'web-build', 'api-run', 'build', 'clean')]
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

Push-Location $PSScriptRoot
try {
    switch ($Task) {
        'dev' {
            Push-Location (Join-Path $PSScriptRoot 'web')
            try { Invoke-CommandChecked 'npm.cmd' @('run', 'dev') } finally { Pop-Location }
        }
        'web-build' { Build-Web }
        'api-run' { Invoke-CommandChecked 'go' @('run', './cmd/server') }
        'build' {
            Build-Web
            New-Item -ItemType Directory -Force -Path (Join-Path $PSScriptRoot 'bin') | Out-Null
            Invoke-CommandChecked 'go' @('build', '-o', './bin/typecho-next.exe', './cmd/server')
        }
        'clean' {
            foreach ($relativePath in @('web/dist', 'bin')) {
                $targetPath = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot $relativePath))
                $workspacePrefix = [IO.Path]::GetFullPath($PSScriptRoot).TrimEnd('\') + '\'
                if (-not $targetPath.StartsWith($workspacePrefix, [StringComparison]::OrdinalIgnoreCase)) {
                    throw "Cleanup target is outside the project: $targetPath"
                }
                if (Test-Path -LiteralPath $targetPath) {
                    Remove-Item -LiteralPath $targetPath -Recurse -Force
                }
            }
        }
    }
} finally {
    Pop-Location
}
