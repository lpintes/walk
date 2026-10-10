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
$flags = @()
if ($Arch -ne "386") {
	# The rsrc.syso files of the examples are 386 COFF objects and do not
	# link on other architectures. An overlay hides them; the manifest
	# next to the exe replaces the embedded one.
	New-Item -ItemType Directory -Force $Out | Out-Null
	$replace = @{}
	foreach ($s in Get-ChildItem (Join-Path $repo "examples\*\rsrc.syso")) {
		$replace[$s.FullName -replace "\\", "/"] = ""
	}
	$overlay = Join-Path $Out "overlay.json"
	@{ Replace = $replace } | ConvertTo-Json | Set-Content $overlay
	$flags += "-overlay=$overlay"
}

function Build([string]$pkg, [string]$dir, [string]$name) {
	New-Item -ItemType Directory -Force $dir | Out-Null
	$exe = Join-Path $dir "$name.exe"
	Write-Host "build $Arch $pkg"
	& go build @flags -o $exe $pkg
	if ($LASTEXITCODE -ne 0) {
		$script:failed += $pkg
		return
	}
	# Windows caches the activation context, so a manifest added after
	# the first start is ignored until the exe changes.
	Copy-Item $manifest "$exe.manifest"
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
	foreach ($d in Get-ChildItem (Join-Path $repo "examples") -Directory) {
		if (-not (Get-ChildItem $d.FullName -Filter *.go)) { continue }
		Build "./examples/$($d.Name)" (Join-Path $dest "examples\$($d.Name)") $d.Name
	}
	# The examples load images from ../img.
	Copy-Item -Recurse -Force (Join-Path $repo "examples\img") (Join-Path $dest "examples")

	foreach ($n in "part3", "dragfinish") {
		Build "./tests/_wingui/$n" (Join-Path $dest "tests\$n") $n
	}
	foreach ($n in "source", "target") {
		Build "./tests/_wingui/olednd/$n" (Join-Path $dest "tests\olednd") $n
	}
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
