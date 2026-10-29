# Builds the GUI tests on the host and runs them in Windows Sandbox, so their
# windows do not take the focus of the user's desktop. See ../README.md.
#
# The executables are built with build.ps1 into $Work\out, which the sandbox
# sees as C:\walk-wingui; run.ps1 and examples.ps1 run there through
# inside.ps1 with PowerShell 7 mapped read-only from the host. The output
# goes to $Work\results; the lines that are not PASS are printed when the
# sandbox has shut down.
param(
	[ValidateSet("386", "amd64")] [string[]]$Arch = @("amd64", "386"),
	[string[]]$Only,
	[switch]$Full,
	# Run examples.ps1 too (-NoRun runs only examples.ps1).
	[switch]$Examples,
	[switch]$NoRun,
	# Use the executables of the previous build.
	[switch]$NoBuild,
	# Leave the sandbox running after the tests, for a look inside.
	[switch]$Keep,
	# Minimize the window of the sandbox as soon as it appears (experimental:
	# not known yet whether every test passes in a minimized sandbox).
	[switch]$Minimize,
	# Run only this program (with -ExecArgs) instead of the tests, for a
	# diagnostic; its directory is mapped read-only and its output goes to
	# exec-out.txt. Nothing is built.
	[string]$Exec,
	[string[]]$ExecArgs,
	[string]$Work = (Join-Path $env:LOCALAPPDATA "walk-sandbox"),
	[int]$TimeoutMinutes = 30
)
$ErrorActionPreference = "Stop"
function Mapped([string]$hostDir, [string]$sandboxDir, [bool]$readOnly) {
	"<MappedFolder><HostFolder>$([Security.SecurityElement]::Escape($hostDir))</HostFolder>" +
	"<SandboxFolder>$sandboxDir</SandboxFolder><ReadOnly>$($readOnly.ToString().ToLower())</ReadOnly></MappedFolder>"
}
$wingui = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$sandboxExe = Join-Path $env:windir "System32\WindowsSandbox.exe"
if (-not (Test-Path $sandboxExe)) {
	throw "Windows Sandbox is not installed; enable it with (elevated): Enable-WindowsOptionalFeature -Online -FeatureName Containers-DisposableClientVM"
}
if (Get-Process -Name WindowsSandbox* -ErrorAction SilentlyContinue) {
	throw "a Windows Sandbox is already running; only one can run at a time"
}
$pwshDir = Split-Path -Parent (Get-Process -Id $PID).Path
$out = Join-Path $Work "out"
$results = Join-Path $Work "results"
$execMapping = ""
if ($Exec) {
	$Exec = (Resolve-Path $Exec).Path
	$execMapping = Mapped (Split-Path -Parent $Exec) "C:\walk-exec" $true
	$NoBuild = $true
}

if (-not $NoBuild) {
	# Build only what runs: the tests of -Only (all without it) unless
	# -NoRun, and the examples only with -Examples or -NoRun.
	$b = @{ Out = $out }
	if ($Only) { $b.Only = $Only }
	if ($NoRun) { $b.NoTests = $true }
	if (-not ($Examples -or $NoRun)) { $b.NoExamples = $true }
	foreach ($a in $Arch) {
		& (Join-Path $wingui "scripts\build.ps1") -Arch $a @b
		if ($LASTEXITCODE -ne 0) { throw "build.ps1 -Arch $a failed" }
	}
}

if (Test-Path $results) { Remove-Item -Recurse -Force $results }
New-Item -ItemType Directory -Force $results | Out-Null
@{
	Arch     = $Arch
	Only     = $Only
	Full     = [bool]$Full
	Run      = -not $NoRun -and -not $Exec
	Examples = [bool]($Examples -or $NoRun)
	Keep     = [bool]$Keep
	Exec     = $(if ($Exec) { Split-Path -Leaf $Exec } else { "" })
	ExecArgs = $ExecArgs
} | ConvertTo-Json | Set-Content -Encoding utf8 (Join-Path $results "job.json")
# No network and no clipboard sharing: the clipboard example must not touch
# the host clipboard.
$wsb = @"
<Configuration>
<Networking>Disable</Networking>
<ClipboardRedirection>Disable</ClipboardRedirection>
<AudioInput>Disable</AudioInput>
<VideoInput>Disable</VideoInput>
<PrinterRedirection>Disable</PrinterRedirection>
<MemoryInMB>4096</MemoryInMB>
<MappedFolders>
$(Mapped $pwshDir "C:\pwsh" $true)
$(Mapped $wingui "C:\walk-tests" $true)
$(Mapped $out "C:\walk-wingui" $false)
$(Mapped $results "C:\walk-results" $false)
$execMapping
</MappedFolders>
<LogonCommand><Command>C:\pwsh\pwsh.exe -NoProfile -ExecutionPolicy Bypass -File C:\walk-tests\sandbox\inside.ps1</Command></LogonCommand>
</Configuration>
"@
$wsbPath = Join-Path $Work "walk.wsb"
Set-Content -Encoding utf8 $wsbPath $wsb

Write-Host "starting Windows Sandbox; results in $results"
Start-Process $sandboxExe -ArgumentList "`"$wsbPath`""
if ($Minimize) {
	# The window of the sandbox captures the keyboard while it has the
	# focus; minimizing it gives the focus back to the previous window.
	Add-Type -Namespace WalkSandbox -Name User32 -MemberDefinition '[DllImport("user32.dll")] public static extern bool ShowWindowAsync(IntPtr hWnd, int nCmdShow);'
	$deadline = (Get-Date).AddMinutes(2)
	do {
		Start-Sleep -Milliseconds 200
		$w = Get-Process -Name WindowsSandboxRemoteSession -ErrorAction SilentlyContinue |
			Where-Object { $_.MainWindowHandle -ne 0 } | Select-Object -First 1
	} while (-not $w -and (Get-Date) -lt $deadline)
	if ($w) { [WalkSandbox.User32]::ShowWindowAsync($w.MainWindowHandle, 6) | Out-Null } # SW_MINIMIZE
}
$done = Join-Path $results "done.txt"
$deadline = (Get-Date).AddMinutes($TimeoutMinutes)
while (-not (Test-Path $done)) {
	if ((Get-Date) -gt $deadline) { throw "no results after $TimeoutMinutes minutes; the sandbox is still running" }
	Start-Sleep -Seconds 5
}
if (-not $Keep) {
	# The shutdown in inside.ps1 does not close the window of the sandbox,
	# so give it a moment and then close the window, which ends the sandbox.
	$deadline = (Get-Date).AddSeconds(15)
	while ((Get-Process -Name WindowsSandbox* -ErrorAction SilentlyContinue) -and (Get-Date) -lt $deadline) {
		Start-Sleep -Seconds 1
	}
	Get-Process -Name WindowsSandbox* -ErrorAction SilentlyContinue | Stop-Process -Force
}

$failed = $false
$errFile = Join-Path $results "error.txt"
if (Test-Path $errFile) { Write-Host "ERROR $(Get-Content -Raw $errFile)"; $failed = $true }
foreach ($f in Get-ChildItem $results -Filter "*-*.txt" | Sort-Object Name) {
	Write-Host "=== $($f.BaseName)"
	foreach ($line in Get-Content $f.FullName) {
		if ($Full -or $line -notmatch "^PASS") { Write-Host $line }
		if ($line -match "^FAIL") { $failed = $true }
	}
}
if ($failed) { exit 1 }
