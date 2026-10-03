param(
  [string]$SkillsDir = (Join-Path $env:USERPROFILE '.agents/skills'),
  [string]$BinDir = (Join-Path $env:USERPROFILE '.local/bin'),
  [string]$Ref = 'main',
  [string]$SourceDir = '',
  [string]$Python = '',
  [switch]$NoPathUpdate
)
$ErrorActionPreference = 'Stop'
$prefix = @()
if (-not $Python) {
  foreach ($candidate in @('python', 'python3', 'py')) {
    $found = Get-Command $candidate -ErrorAction SilentlyContinue
    if (-not $found) { continue }
    $probePrefix = @()
    if ($candidate -eq 'py') { $probePrefix = @('-3') }
    $previousPreference = $ErrorActionPreference
    $validPython = $false
    try {
      # Windows PowerShell 5.1 turns native stderr into errors under Stop.
      $ErrorActionPreference = 'Continue'
      & $found.Source @probePrefix -c 'import sys; sys.exit(sys.version_info < (3, 10))' *> $null
      $validPython = $LASTEXITCODE -eq 0
    } catch { $validPython = $false }
    finally { $ErrorActionPreference = $previousPreference }
    if ($validPython) { $Python = $found.Source; $prefix = $probePrefix; break }
  }
}
if (-not $Python) { throw 'Please install Python 3.10+ and rerun.' }
$temporary = ''
try {
  $installer = ''
  if ($PSScriptRoot) { $installer = Join-Path $PSScriptRoot 'install-client.py' }
  if (-not $installer -or -not (Test-Path -LiteralPath $installer -PathType Leaf)) {
    $temporary = Join-Path ([IO.Path]::GetTempPath()) ('shiji-install-' + [Guid]::NewGuid().ToString('N') + '.py')
    Invoke-WebRequest -UseBasicParsing 'https://raw.githubusercontent.com/art-shier/notes/main/install-client.py' -OutFile $temporary
    $installer = $temporary
  }
  $installArgs = @($installer, '--skills-dir', $SkillsDir, '--bin-dir', $BinDir, '--ref', $Ref)
  if ($SourceDir) { $installArgs += @('--source-dir', $SourceDir) }
  & $Python @prefix @installArgs
  if ($LASTEXITCODE -ne 0) { throw 'Shiji client installation failed.' }
  if (-not $NoPathUpdate) {
    $resolvedBin = [IO.Path]::GetFullPath($BinDir)
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (@($userPath -split ';') -notcontains $resolvedBin) {
      $newUserPath = $resolvedBin
      if ($userPath) { $newUserPath += ';' + $userPath }
      [Environment]::SetEnvironmentVariable('Path', $newUserPath, 'User')
    }
    if (@($env:Path -split ';') -notcontains $resolvedBin) { $env:Path = $resolvedBin + ';' + $env:Path }
    Write-Host 'PATH updated for this PowerShell session and future user sessions.'
  }
} finally {
  if ($temporary -and (Test-Path -LiteralPath $temporary)) { Remove-Item -LiteralPath $temporary }
}
