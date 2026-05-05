$ErrorActionPreference = 'Stop'

$RootDir = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$OutDir = Join-Path $RootDir '.build\bin'
$BinDir = Join-Path $HOME '.local\bin'

New-Item -ItemType Directory -Force -Path $OutDir, $BinDir | Out-Null

$results = [System.Collections.Generic.List[object]]::new()

function Build-One {
    param(
        [Parameter(Mandatory = $true)][string]$Package,
        [Parameter(Mandatory = $true)][string]$OutputName
    )

    $OutputPath = Join-Path $OutDir "$OutputName.exe"
    $InstallPath = Join-Path $BinDir "$OutputName.exe"

    Write-Host "Building $OutputName..."
    try {
        go build -o $OutputPath $Package
        if ($LASTEXITCODE -ne 0) {
            throw "go build exited with code $LASTEXITCODE"
        }
        Copy-Item -Force $OutputPath $InstallPath
        $results.Add([pscustomobject]@{
            Name = "$OutputName.exe"
            Status = 'built+installed'
            Path = $InstallPath
            Error = ''
        }) | Out-Null
    }
    catch {
        $results.Add([pscustomobject]@{
            Name = "$OutputName.exe"
            Status = 'failed'
            Path = $OutputPath
            Error = $_.Exception.Message
        }) | Out-Null
        Write-Warning "Failed to build ${OutputName}: $($_.Exception.Message)"
    }
}

Build-One "$RootDir" 'omnillm'
Build-One (Join-Path $RootDir 'cmd\omniproxy') 'omniproxy'
Build-One (Join-Path $RootDir 'cmd\omnicode') 'omnicode'

Write-Host "Results:"
$results | Select-Object Name, Status, Path, Error | Format-Table -AutoSize
