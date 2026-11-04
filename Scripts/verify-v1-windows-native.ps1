param(
    [Parameter(Mandatory=$true)][string]$Stage,
    [Parameter(Mandatory=$true)][ValidateSet('amd64','arm64')][string]$Arch
)
$ErrorActionPreference = 'Stop'
# NSIS writes per-user shortcuts and registry entries; run only on disposable CI VMs.
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_OS -ne 'Windows') { throw 'Native installer verification requires a GitHub Windows runner' }
$nativeArch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLower()
if (($Arch -eq 'amd64' -and $nativeArch -ne 'x64') -or ($Arch -eq 'arm64' -and $nativeArch -ne 'arm64')) { throw 'Runner architecture does not match package' }
$Stage = (Resolve-Path $Stage).Path
$installers = @(Get-ChildItem $Stage -Filter "quosparr-setup-*-$Arch.exe")
if ($installers.Count -ne 1) { throw 'Expected exactly one NSIS installer' }
$temporary = Join-Path $env:RUNNER_TEMP ('quosparr-native-' + [guid]::NewGuid())
$installed = Join-Path $temporary 'installed'
$testHome = Join-Path $temporary 'home'
$uninstallKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\com.quosparr.app'
if (Test-Path $uninstallKey) { throw 'Runner already has a Quosparr installation' }
$saved = @{}
foreach ($key in @('HOME','USERPROFILE','CODEX_HOME','GROK_HOME','MYGO_SQLITE_LIBRARY','QUOSPARR_PREFS_DOMAIN','QUOSPARR_DEV_VERIFY_SQLITE')) { $saved[$key] = [Environment]::GetEnvironmentVariable($key) }
try {
    New-Item -ItemType Directory -Path $testHome -Force | Out-Null
    $process = Start-Process -FilePath $installers[0].FullName -ArgumentList @('/S', "/D=$installed") -PassThru -Wait
    if ($process.ExitCode -ne 0) { throw "NSIS installation failed: $($process.ExitCode)" }
    foreach ($source in (Get-ChildItem $Stage -File)) {
        if ($source.Name -like 'quosparr-setup-*' -or $source.Name -like '*.tar.gz' -or $source.Name -like 'update-*') { continue }
        $destination = Join-Path $installed $source.Name
        if (!(Test-Path $destination) -or (Get-FileHash $source.FullName).Hash -ne (Get-FileHash $destination).Hash) { throw "Installed payload mismatch: $($source.Name)" }
    }
    if (!(Test-Path $uninstallKey)) { throw 'Installer did not register its uninstaller' }
    if (Test-Path (Join-Path $installed 'mygo-sqlite3.dll')) { throw 'Installer still shipped an external SQLite DLL' }
    $env:HOME = $testHome
    $env:USERPROFILE = $testHome
    $env:CODEX_HOME = Join-Path $testHome 'codex'
    $env:GROK_HOME = Join-Path $testHome 'grok'
    $env:QUOSPARR_PREFS_DOMAIN = 'com.quosparr.ci.package'
    $env:MYGO_SQLITE_LIBRARY = $null
    New-Item -ItemType Directory -Path $env:CODEX_HOME,$env:GROK_HOME -Force | Out-Null
    # Copy only the EXE to an empty directory and exercise its real SQLite ABI.
    $portable = Join-Path $temporary 'portable'
    New-Item -ItemType Directory -Path $portable | Out-Null
    Copy-Item (Join-Path $installed 'Quosparr.exe') $portable
    $env:QUOSPARR_DEV_VERIFY_SQLITE = '1'
    foreach ($attempt in 1,2) {
        $sqliteLog = Join-Path $temporary "sqlite-$attempt.log"
        $sqliteError = Join-Path $temporary "sqlite-$attempt-error.log"
        $process = Start-Process -FilePath (Join-Path $portable 'Quosparr.exe') -RedirectStandardOutput $sqliteLog -RedirectStandardError $sqliteError -Wait -PassThru
        if ($process.ExitCode -ne 0) { throw "Embedded SQLite verification failed: $(Get-Content $sqliteError -Raw)" }
        if (!(Get-Content $sqliteLog -Raw).Contains('SQLite runtime: load and parameterized query passed')) { throw 'Embedded SQLite query did not complete' }
        $cached = @(Get-ChildItem (Join-Path $testHome '.quosparr/runtime/sqlite') -Recurse -Filter 'mygo-sqlite3.dll')
        if ($cached.Count -ne 1 -or (Get-ChildItem $portable).Count -ne 1) { throw 'Embedded SQLite was not extracted into its private runtime directory' }
        if ($attempt -eq 1) { Remove-Item $cached[0].FullName }
    }
    $env:QUOSPARR_DEV_VERIFY_SQLITE = $null
    $log = Join-Path $temporary 'self-check.log'
    $stderr = Join-Path $temporary 'self-check-error.log'
    $process = Start-Process -FilePath (Join-Path $installed 'Quosparr.exe') -ArgumentList '--self-check' -RedirectStandardOutput $log -RedirectStandardError $stderr -Wait -PassThru
    if ($process.ExitCode -ne 0) { throw "Packaged self-check failed: $($process.ExitCode)" }
    $text = Get-Content $log -Raw
    Write-Output $text
    if (!$text.Contains("(windows/$Arch)")) { throw 'Packaged self-check did not run on the expected architecture' }
    if ((Test-Path (Join-Path $env:CODEX_HOME 'auth.json')) -or (Test-Path (Join-Path $env:GROK_HOME 'auth.json'))) { throw 'Self-check created official credentials' }
} finally {
    foreach ($key in $saved.Keys) { [Environment]::SetEnvironmentVariable($key, $saved[$key]) }
    $uninstaller = Join-Path $installed 'Uninstall.exe'
    if (Test-Path $uninstaller) {
        $process = Start-Process -FilePath $uninstaller -ArgumentList '/S' -Wait -PassThru
        if ($process.ExitCode -ne 0) { throw 'NSIS uninstallation failed' }
        $deadline = (Get-Date).AddSeconds(20)
        while (((Test-Path $uninstallKey) -or (Test-Path (Join-Path $installed 'Quosparr.exe'))) -and (Get-Date) -lt $deadline) { Start-Sleep -Milliseconds 200 }
        if ((Test-Path $uninstallKey) -or (Test-Path (Join-Path $installed 'Quosparr.exe'))) { throw 'Uninstall left application files or registry entries' }
    }
    if (Test-Path $temporary) { Remove-Item $temporary -Recurse -Force }
}
Write-Output "Native Windows installer, uninstaller and embedded SQLite from standalone EXE verified: $Arch"
