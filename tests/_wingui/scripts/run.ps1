# Runs the self-checking tests built by build.ps1 and prints their results.
# See ../README.md.
#
# No global keyboard input is sent and no window is brought to the
# foreground. Only -Drag (the olednd test) moves the real mouse pointer; it
# is restored afterwards.
param(
	[ValidateSet("386", "amd64")] [string]$Arch = "amd64",
	[string]$Out = (Join-Path $env:TEMP "walk-wingui"),
	[string[]]$Only,
	[switch]$Drag,
	[switch]$Full
)
$ErrorActionPreference = "Stop"
Add-Type -Path (Join-Path $PSScriptRoot "WinTest.cs")
$root = Join-Path $Out "$Arch\tests"
$results = [ordered]@{}

function Start-Test([string]$dir, [string]$name, [string[]]$arguments) {
	$o = Join-Path $dir "$name.out.txt"
	$e = Join-Path $dir "$name.err.txt"
	Remove-Item $o, $e -ErrorAction SilentlyContinue
	$params = @{
		FilePath               = Join-Path $dir "$name.exe"
		WorkingDirectory       = $dir
		RedirectStandardOutput = $o
		RedirectStandardError  = $e
		PassThru               = $true
	}
	if ($arguments) { $params.ArgumentList = $arguments }
	$p = Start-Process @params
	# Keep the handle so that ExitCode is available after the exit.
	$null = $p.Handle
	$p | Add-Member Out $o
	$p | Add-Member Err $e
	$p
}

function Read-Out($p) { Get-Content $p.Out -Raw -ErrorAction SilentlyContinue }

function Wait-Line($p, [string]$pattern, [int]$seconds) {
	for ($i = 0; $i -lt $seconds * 4; $i++) {
		if ((Read-Out $p) -match $pattern) { return $Matches }
		if ($p.HasExited) { return $null }
		Start-Sleep -Milliseconds 250
	}
	$null
}

function Stop-Test($p, [int]$seconds) {
	if (-not $p.WaitForExit($seconds * 1000)) {
		$p.Kill()
		$p.WaitForExit()
		return "timeout, killed"
	}
	"exit $($p.ExitCode)"
}

# Report prints the PASS, FAIL and KNOWN lines of a test (all of them with
# -Full) and records whether it passed: exit code 0, no FAIL line and no
# extra failure from the script.
function Report([string]$name, $p, [string]$stop, [string[]]$extra) {
	Write-Host "=== $name ($Arch): $stop, output in $($p.Out)"
	$text = Read-Out $p
	$lines = @($text -split "`r?`n") + $extra
	if (-not $Full) { $lines = $lines -match "^(PASS|FAIL|KNOWN) " }
	$lines | Write-Host
	$err = Get-Content $p.Err -TotalCount 20 -ErrorAction SilentlyContinue
	if ($err) { Write-Host "stderr:"; $err | Write-Host }
	$ok = $stop -eq "exit 0" -and $text -notmatch "(?m)^FAIL" -and -not ($extra -match "^FAIL")
	$known = @($text -split "`r?`n" -match "^KNOWN ").Count
	$script:results[$name] = $(if ($ok) { "PASS" } else { "FAIL" }) + $(if ($known) { " ($known known)" })
}

function Want([string]$name) { -not $Only -or $Only -contains $name }

if (Want "part3") {
	$p = Start-Test (Join-Path $root "part3") "part3" @("browse")
	$extra = @()
	$m = Wait-Line $p "(?s)BUTTON_HWND (\d+).*READY" 20
	if ($m) {
		$acc = [WinTest]::Acc([IntPtr][int64]$m[1], [WinTest]::OBJID_CLIENT)
		$ok = $acc -match "name='Annotated name'"
		$extra += "$(if ($ok) { 'PASS' } else { 'FAIL' }) MSAA name of the button: $acc"
		$p.Refresh()
		[WinTest]::Close($p.MainWindowHandle)
	} else {
		$extra += "FAIL READY not printed"
	}
	Report "part3" $p (Stop-Test $p 5) $extra
}

foreach ($n in "dragfinish", "hdn", "ods") {
	if (-not (Want $n)) { continue }
	$p = Start-Test (Join-Path $root $n) $n
	Report $n $p (Stop-Test $p 30)
}

if (Want "webview") {
	$p = Start-Test (Join-Path $root "webview") "webview"
	Report "webview" $p (Stop-Test $p 90)
}

if (Want "com") {
	# go test prints "--- PASS: TestX" lines, which only -Full shows; a
	# failure gives a FAIL line and exit code 1.
	$p = Start-Test (Join-Path $root "com") "com" @("-test.v")
	Report "com" $p (Stop-Test $p 30)
}

if ($Drag -and (Want "olednd")) {
	$dir = Join-Path $root "olednd"
	$t = Start-Test $dir "target"
	$extra = @()
	$m = Wait-Line $t "HWND (\d+)" 10
	if ($m) {
		$s = Start-Test $dir "source" @($m[1])
		$stop = Stop-Test $s 60
		$src = Read-Out $s
		$extra += "--- source: $stop"
		if ($src) { $extra += $src.TrimEnd() }
		$files = if ($src -match "FILES (.*)") { $Matches[1].Trim() }
		$drop = if ((Read-Out $t) -match "DROP (.*)") { $Matches[1].Trim() }
		$ok = $stop -eq "exit 0" -and $src -match "DoDragDrop hr=0x40100 " -and $files -and $files -eq $drop
		$extra += "$(if ($ok) { 'PASS' } else { 'FAIL' }) drop: dropped $drop, sent $files"
	} else {
		$extra += "FAIL target printed no HWND"
	}
	Report "olednd" $t (Stop-Test $t 10) $extra
}

Write-Host "=== summary ($Arch)"
foreach ($k in $results.Keys) { Write-Host "$($results[$k]) $k" }
if ($results.Values -match "^FAIL") { exit 1 }
