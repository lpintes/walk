# Builds the walk examples and the tests in tests/_wingui for one
# architecture into $Out\$Arch\examples and $Out\$Arch\tests. See
# ../README.md.
#
# hdn and ods are built with their trace.patch applied to the working tree;
# the patch is reverted right after the build.
param(
	[ValidateSet("386", "amd64")] [string]$Arch = "amd64",
	[string]$Out = (Join-Path $env:TEMP "walk-wingui")
)
$ErrorActionPreference = "Stop"
$repo = (Resolve-Path (Join-Path $PSScriptRoot "..\..\..")).Path
$dest = Join-Path $Out $Arch
# All examples have the same manifest; the tests get a copy of it too.
$manifest = Join-Path $repo "examples\actions\actions.exe.manifest"
$failed = @()

$oldGOOS, $oldGOARCH = $env:GOOS, $env:GOARCH
$env:GOOS = "windows"
$env:GOARCH = $Arch

function Build([string]$pkg, [string]$dir, [string]$name) {
	New-Item -ItemType Directory -Force $dir | Out-Null
	$exe = Join-Path $dir "$name.exe"
	Write-Host "build $Arch $pkg"
	& go build -o $exe $pkg
	if ($LASTEXITCODE -ne 0) {
		$script:failed += $pkg
		return
	}
	# Windows caches the activation context, so a manifest added after
	# the first start is ignored until the exe changes.
	Copy-Item $manifest "$exe.manifest"
}

# Builds several packages with one go build, which compiles and links them
# in parallel. $progs holds @(pkg, dir) pairs; the exe is named after the
# last element of pkg, so the names in one group must differ. go build
# keeps building the other packages when one fails, so a failed package is
# the one whose exe is missing.
function BuildGroup([object[]]$progs) {
	$tmp = Join-Path $dest ".group"
	if (Test-Path $tmp) { Remove-Item -Recurse -Force $tmp }
	New-Item -ItemType Directory -Force $tmp | Out-Null
	$pkgs = $progs | ForEach-Object { $_[0] }
	Write-Host "build $Arch $($pkgs -join ' ')"
	# The trailing slash makes -o a directory; a backslash before the
	# closing quote would escape it.
	& go build -o "$tmp/" @pkgs
	foreach ($p in $progs) {
		$pkg, $dir = $p
		$name = Split-Path -Leaf $pkg
		$built = Join-Path $tmp "$name.exe"
		if (-not (Test-Path $built)) {
			$script:failed += $pkg
			continue
		}
		New-Item -ItemType Directory -Force $dir | Out-Null
		$exe = Join-Path $dir "$name.exe"
		Move-Item -Force $built $exe
		Copy-Item $manifest "$exe.manifest"
	}
	Remove-Item -Recurse -Force $tmp
}

function BuildPatched([string]$name) {
	$patch = Join-Path $repo "tests\_wingui\$name\trace.patch"
	& git -C $repo apply --check $patch
	if ($LASTEXITCODE -ne 0) {
		Write-Host "cannot apply $patch; are the files it touches modified?"
		$script:failed += $name
		return
	}
	& git -C $repo apply $patch
	try {
		Build "./tests/_wingui/$name" (Join-Path $dest "tests\$name") $name
	} finally {
		& git -C $repo apply -R $patch
		if ($LASTEXITCODE -ne 0) { throw "could not revert $patch, check git status" }
	}
}

Push-Location $repo
try {
	# Examples and tests are separate groups, because examples/webview and
	# tests/_wingui/webview both give webview.exe.
	$examples = @()
	foreach ($d in Get-ChildItem (Join-Path $repo "examples") -Directory) {
		if (-not (Get-ChildItem $d.FullName -Filter *.go)) { continue }
		$examples += , @("./examples/$($d.Name)", (Join-Path $dest "examples\$($d.Name)"))
	}
	BuildGroup $examples
	# The examples load images from ../img.
	Copy-Item -Recurse -Force (Join-Path $repo "examples\img") (Join-Path $dest "examples")

	$tests = @()
	foreach ($n in "part3", "dragfinish", "webview", "webview2", "upstream", "shell", "uia", "richedit", "dialogs") {
		$tests += , @("./tests/_wingui/$n", (Join-Path $dest "tests\$n"))
	}
	foreach ($n in "source", "target") {
		$tests += , @("./tests/_wingui/olednd/$n", (Join-Path $dest "tests\olednd"))
	}
	foreach ($n in "app", "client") {
		$tests += , @("./tests/_wingui/liveregion/$n", (Join-Path $dest "tests\liveregion"))
	}
	BuildGroup $tests
	# The unit tests of internal/com need Windows; build them as an exe.
	$comDir = Join-Path $dest "tests\com"
	New-Item -ItemType Directory -Force $comDir | Out-Null
	Write-Host "build $Arch test ./internal/com"
	& go test -c -o (Join-Path $comDir "com.exe") ./internal/com
	if ($LASTEXITCODE -ne 0) { $failed += "./internal/com" }
	foreach ($n in "hdn", "ods") {
		BuildPatched $n
	}
} finally {
	Pop-Location
	$env:GOOS, $env:GOARCH = $oldGOOS, $oldGOARCH
}

if ($failed) {
	Write-Host "FAILED: $($failed -join ', ')"
	exit 1
}
Write-Host "built into $dest"
