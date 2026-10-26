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
	if (-not $Full) { $lines = $lines -match "^(PASS|FAIL|KNOWN|SKIP) " }
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
		# SetRole(AccRoleLink) annotated ROLE_SYSTEM_LINK (30).
		$ok = $acc -match "\(30\) name='Annotated name'"
		$extra += "$(if ($ok) { 'PASS' } else { 'FAIL' }) MSAA name and role of the button: $acc"
		$p.Refresh()
		[WinTest]::Close($p.MainWindowHandle)
	} else {
		$extra += "FAIL READY not printed"
	}
	Report "part3" $p (Stop-Test $p 5) $extra
}

foreach ($n in "dragfinish", "hdn", "ods", "upstream", "shell", "uia", "richedit") {
	if (-not (Want $n)) { continue }
	$p = Start-Test (Join-Path $root $n) $n
	Report $n $p (Stop-Test $p 30)
}

if (Want "dialogs") {
	# The test prints the handle of an open walk dialog and waits for the
	# named event Local\walk-dialogs-msaa while the script reads its MSAA
	# objects.
	$p = Start-Test (Join-Path $root "dialogs") "dialogs"
	$extra = @()
	$m = Wait-Line $p "(?s)DIALOG_HWND (\d+).*MSAA_READY" 40
	if ($m) {
		$h = [IntPtr][int64]$m[1]
		$client = [WinTest]::Acc($h, [WinTest]::OBJID_CLIENT)
		$window = [WinTest]::Acc($h, [WinTest]::OBJID_WINDOW)
		# As for #32770 dialogs: ROLE_SYSTEM_DIALOG (18) on the client
		# object, ROLE_SYSTEM_WINDOW (9) on the window object.
		$extra += "$(if ($client -match '\(18\) name=') { 'PASS' } else { 'FAIL' }) MSAA client object of the walk dialog: $client"
		$extra += "$(if ($window -match '\(9\) name=') { 'PASS' } else { 'FAIL' }) MSAA window object of the walk dialog: $window"
		$ev = [System.Threading.EventWaitHandle]::OpenExisting("Local\walk-dialogs-msaa")
		$null = $ev.Set()
		$ev.Dispose()
	} else {
		$extra += "FAIL MSAA_READY not printed"
	}
	Report "dialogs" $p (Stop-Test $p 90) $extra
}

if (Want "webview") {
	$p = Start-Test (Join-Path $root "webview") "webview"
	Report "webview" $p (Stop-Test $p 90)
}

if (Want "webview2") {
	# Prints a SKIP line and exits with 0 without a WebView2 runtime.
	$p = Start-Test (Join-Path $root "webview2") "webview2"
	Report "webview2" $p (Stop-Test $p 120)
}

if (Want "com") {
	# go test prints "--- PASS: TestX" lines, which only -Full shows; a
	# failure gives a FAIL line and exit code 1.
	$p = Start-Test (Join-Path $root "com") "com" @("-test.v")
	Report "com" $p (Stop-Test $p 30)
}

if (Want "liveregion") {
	# client plays the screen reader: it reads LiveSetting through UI
	# Automation, subscribes to the events and then lets app run its checks.
	$dir = Join-Path $root "liveregion"
	$a = Start-Test $dir "app"
	$extra = @()
	$m = Wait-Line $a "HANDLES (\d+) (\d+) (\d+) (\d+) (\d+)" 20
	if ($m) {
		$c = Start-Test $dir "client" @($m[1], $m[2], $m[3], $m[4], $m[5])
		$stop = Stop-Test $c 60
		$extra += "--- client: $stop"
		$out = Read-Out $c
		if ($out) { $extra += $out.TrimEnd() -split "`r?`n" }
		if ($stop -ne "exit 0") { $extra += "FAIL client: $stop" }
		if (-not (Wait-Line $a "CHECKS DONE" 30)) { $extra += "FAIL app printed no CHECKS DONE" }
		# The main window handle, not MainWindowHandle: the 386 build is a
		# console program.
		[WinTest]::Close([IntPtr][int64]$m[1])
	} else {
		$extra += "FAIL app printed no HANDLES"
	}
	Report "liveregion" $a (Stop-Test $a 10) $extra
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
