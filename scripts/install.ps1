param(
  [string]$Version = "latest",
  [string]$InstallDir = "$env:LOCALAPPDATA\Podium\bin",
  [string]$PodiumHome = "",
  [ValidateSet("ask", "yes", "no")]
  [string]$Autostart = "ask",
  [switch]$NoOnboard,
  [switch]$SourceFallback,
  [switch]$DryRun
)

$ErrorActionPreference = "Stop"
$ReleaseBase = if ($env:PODIUM_RELEASE_BASE) { $env:PODIUM_RELEASE_BASE } else { "https://github.com/mar-schmidt/Podium/releases" }
$RepoUrl = if ($env:PODIUM_REPO_URL) { $env:PODIUM_REPO_URL } else { "https://github.com/mar-schmidt/Podium.git" }

function Say($Message) { Write-Host $Message }
function Invoke-Step([scriptblock]$Block, [string]$Description) {
  if ($DryRun) {
    Write-Host "[dry-run] $Description"
  } else {
    & $Block
  }
}

$arch = switch ((Get-CimInstance Win32_OperatingSystem).OSArchitecture) {
  { $_ -match "ARM64" } { "arm64"; break }
  default { "amd64" }
}

if ($Version -eq "latest") {
  $releaseUrl = "$ReleaseBase/latest/download"
  $archive = "podium_windows_$arch.zip"
} else {
  $releaseUrl = "$ReleaseBase/download/$Version"
  $archive = "podium_${Version}_windows_$arch.zip"
}
$tmp = Join-Path ([IO.Path]::GetTempPath()) ("podium-install-" + [guid]::NewGuid())
New-Item -ItemType Directory -Force -Path $tmp | Out-Null

try {
  $archivePath = Join-Path $tmp $archive
  $sumPath = Join-Path $tmp "SHA256SUMS"
  $url = "$releaseUrl/$archive"
  $sumUrl = "$releaseUrl/SHA256SUMS"

  $downloadOk = $false
  try {
    Say "Downloading $url"
    if ($DryRun) {
      Say "[dry-run] Invoke-WebRequest $url -OutFile $archivePath"
      Say "[dry-run] Invoke-WebRequest $sumUrl -OutFile $sumPath"
      Invoke-Step { New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null } "create $InstallDir"
      Say "[dry-run] verify checksum and unpack $archive"
      Say "[dry-run] install podium.exe and podiumd.exe into $InstallDir"
    } else {
      Invoke-WebRequest -Uri $url -OutFile $archivePath
      Invoke-WebRequest -Uri $sumUrl -OutFile $sumPath
      $line = Get-Content $sumPath | Where-Object { $_ -match [regex]::Escape($archive) } | Select-Object -First 1
      if (-not $line) { throw "checksum entry for $archive not found" }
      $expected = ($line -split "\s+")[0].ToLowerInvariant()
      $actual = (Get-FileHash -Algorithm SHA256 $archivePath).Hash.ToLowerInvariant()
      if ($expected -ne $actual) { throw "checksum mismatch: expected $expected, got $actual" }
      $unpack = Join-Path $tmp "unpack"
      Expand-Archive -Path $archivePath -DestinationPath $unpack -Force
      Invoke-Step { New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null } "create $InstallDir"
      Invoke-Step { Copy-Item (Join-Path $unpack "podium.exe") (Join-Path $InstallDir "podium.exe") -Force } "install podium.exe"
      Invoke-Step { Copy-Item (Join-Path $unpack "podiumd.exe") (Join-Path $InstallDir "podiumd.exe") -Force } "install podiumd.exe"
    }
    $downloadOk = $true
  } catch {
    if (-not $SourceFallback) {
      throw "Release download failed: $_. Re-run with -SourceFallback to build locally."
    }
  }

  if (-not $downloadOk) {
    Say "Building Podium from source fallback."
    $work = Join-Path $tmp "src"
    if ((Test-Path "go.mod") -and (Test-Path "cmd\podium")) {
      $work = (Get-Location).Path
    } else {
      git clone --depth 1 $RepoUrl $work
    }
    Push-Location $work
    try { make build } finally { Pop-Location }
    Invoke-Step { New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null } "create $InstallDir"
    Invoke-Step { Copy-Item (Join-Path $work "bin\podium.exe") (Join-Path $InstallDir "podium.exe") -Force } "install podium.exe"
    Invoke-Step { Copy-Item (Join-Path $work "bin\podiumd.exe") (Join-Path $InstallDir "podiumd.exe") -Force } "install podiumd.exe"
  }

  if ($PodiumHome) {
    $env:PODIUM_HOME = $PodiumHome
  }

  if ($Autostart -eq "ask") {
    $reply = Read-Host "Start Podium automatically when you log in? [Y/n]"
    if ($reply -and $reply.ToLowerInvariant().StartsWith("n")) { $Autostart = "no" } else { $Autostart = "yes" }
  }

  if ($Autostart -eq "yes") {
    $podiumd = Join-Path $InstallDir "podiumd.exe"
    Invoke-Step {
      if ($PodiumHome) {
        $quotedHome = $PodiumHome.Replace("'", "''")
        $quotedExe = $podiumd.Replace("'", "''")
        $action = New-ScheduledTaskAction -Execute "powershell.exe" -Argument "-NoProfile -WindowStyle Hidden -Command `$env:PODIUM_HOME='$quotedHome'; & '$quotedExe'"
      } else {
        $action = New-ScheduledTaskAction -Execute $podiumd
      }
      $trigger = New-ScheduledTaskTrigger -AtLogOn
      $principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive
      Register-ScheduledTask -TaskName "Podium" -Action $action -Trigger $trigger -Principal $principal -Description "Start Podium daemon at logon" -Force | Out-Null
    } "register current-user Scheduled Task: Podium"
  }

  Say "Podium installed to $InstallDir."
  if (-not ($env:Path.Split(';') -contains $InstallDir)) {
    Say "Add $InstallDir to PATH if PowerShell cannot find podium."
  }
  if (-not $NoOnboard) {
    $podium = Join-Path $InstallDir "podium.exe"
    Invoke-Step { & $podium onboard } "run podium onboard"
  }
} finally {
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
