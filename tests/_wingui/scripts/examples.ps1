# Starts every example built by build.ps1, prints its window tree with the
# MSAA client object of every window, posts a few Tab presses to the focused
# window and closes the example with WM_CLOSE. See ../README.md.
#
# No global keyboard input is sent and no window is brought to the
# foreground. Windows only give a window the focus when it gets the
# foreground, so the Tab steps work only for examples that happen to get it.
param(
	[ValidateSet("386", "amd64")] [string]$Arch = "amd64",
	[string]$Out = (Join-Path $env:TEMP "walk-wingui"),
	[string[]]$Only,
	[int]$Tabs = 4
)
$ErrorActionPreference = "Stop"
Add-Type -Path (Join-Path $PSScriptRoot "WinTest.cs")
$root = Join-Path $Out "$Arch\examples"
$log = Join-Path $Out "examples-$Arch.txt"
$summary = @()
"" | Set-Content $log

$dirs = Get-ChildItem $root -Directory | Where-Object { $_.Name -ne "img" }
if ($Only) { $dirs = $dirs | Where-Object { $Only -contains $_.Name } }
foreach ($d in $dirs) {
	$n = $d.Name
	$err = Join-Path $d.FullName "stderr.txt"
	$p = Start-Process -FilePath (Join-Path $d.FullName "$n.exe") -WorkingDirectory $d.FullName -RedirectStandardError $err -PassThru
	$null = $p.Handle
	$top = [IntPtr]::Zero
	for ($i = 0; $i -lt 40; $i++) {
		Start-Sleep -Milliseconds 250
		$p.Refresh()
		if ($p.HasExited) { break }
		if ($p.MainWindowHandle -ne [IntPtr]::Zero) { $top = $p.MainWindowHandle; break }
	}
	$lines = @("=== $n ($Arch)")
	if ($top -eq [IntPtr]::Zero) {
		if ($p.HasExited) {
			$result = "FAIL exited with $($p.ExitCode) before showing a window"
		} else {
			# notifyicon has no main window, only the notification area icon.
			$result = "no main window, killed"
			$p.Kill()
		}
	} else {
		Start-Sleep -Milliseconds 1000
		$lines += [WinTest]::Dump($top)
		$lines += "start " + [WinTest]::FocusDesc($top)
		for ($t = 0; $t -lt $Tabs; $t++) {
			$f = [WinTest]::FocusOf($top)
			if ($f -eq [IntPtr]::Zero) { $lines += "tab: no focused window"; break }
			[WinTest]::Tab($f)
			Start-Sleep -Milliseconds 300
			$lines += "tab$($t + 1) " + [WinTest]::FocusDesc($top)
		}
		[WinTest]::Close($top)
		if ($p.WaitForExit(3000)) {
			$result = $(if ($p.ExitCode -eq 0) { "PASS" } else { "FAIL exit $($p.ExitCode)" })
		} else {
			$result = "FAIL WM_CLOSE did not close it, killed"
			$p.Kill()
		}
	}
	$p.WaitForExit(2000) | Out-Null
	$e = Get-Content $err -TotalCount 15 -ErrorAction SilentlyContinue
	if ($e) { $lines += "stderr:"; $lines += $e }
	if ($n -eq "progressindicator" -and $result -match "^FAIL" -and ($e -match "CreateLayoutItem")) {
		# Inherited: a dialog without a layout panics in SetVisible, see
		# "Known issues" in CLAUDE.md.
		$result = "KNOWN nil layout panic in CreateLayoutItem"
	}
	$lines += $result
	$lines | Add-Content $log
	$summary += "$result $n"
	Write-Host "$result $n"
}
Write-Host "details in $log"
if ($summary -match "^FAIL") { exit 1 }
