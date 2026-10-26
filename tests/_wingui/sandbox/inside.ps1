# Runs in Windows Sandbox as its logon command (see sandbox.ps1): runs
# run.ps1 and examples.ps1 for the architectures in C:\walk-results\job.json
# against the executables built on the host, writes their output to
# C:\walk-results and shuts the sandbox down.
$ErrorActionPreference = "Continue"
$results = "C:\walk-results"
$scripts = "C:\walk-tests\scripts"
$out = "C:\walk-wingui"
$job = Get-Content (Join-Path $results "job.json") -Raw | ConvertFrom-Json

try {
	if ($job.Exec) {
		$exe = Join-Path "C:\walk-exec" $job.Exec
		$execArgs = @($job.ExecArgs | Where-Object { $_ })
		& $exe @execArgs *>&1 | ForEach-Object { "$_" } |
			Set-Content -Encoding utf8 (Join-Path $results "exec-out.txt")
		Add-Content -Encoding utf8 (Join-Path $results "exec-out.txt") "exit code $LASTEXITCODE"
	}
	foreach ($arch in $job.Arch) {
		if ($job.Run) {
			$a = @{ Arch = $arch; Out = $out }
			if ($job.Only) { $a.Only = $job.Only }
			if ($job.Full) { $a.Full = $true }
			& "$scripts\run.ps1" @a *>&1 | ForEach-Object { "$_" } |
				Set-Content -Encoding utf8 (Join-Path $results "run-$arch.txt")
		}
		if ($job.Examples) {
			& "$scripts\examples.ps1" -Arch $arch -Out $out *>&1 | ForEach-Object { "$_" } |
				Set-Content -Encoding utf8 (Join-Path $results "examples-$arch.txt")
		}
	}
} catch {
	"$_" | Set-Content -Encoding utf8 (Join-Path $results "error.txt")
} finally {
	Set-Content (Join-Path $results "done.txt") (Get-Date -Format o)
	if (-not $job.Keep) { & shutdown.exe /s /t 0 }
}
