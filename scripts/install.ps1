# Install the ondx CLI from GitHub Releases on Windows.
#
#   irm https://raw.githubusercontent.com/openndx/openndx-core/main/scripts/install.ps1 | iex
#
# Environment:
#   ONDX_VERSION      Release tag to install (e.g. v0.3.0). Default: latest.
#   ONDX_REPO         GitHub repo to download releases from, for forks/mirrors.
#                     Default: openndx/openndx-core.
#   ONDX_INSTALL_DIR  Where to put ondx.exe. Default: %LOCALAPPDATA%\Programs\ondx.
#                     Added to the user PATH if it isn't there already.
#
# Errors are thrown rather than calling `exit`, since under `irm | iex` exit
# would close the user's shell.

& {
    $ErrorActionPreference = 'Stop'
    # Invoke-WebRequest is much slower with the progress bar on Windows PowerShell 5.1.
    $ProgressPreference = 'SilentlyContinue'

    $repo = if ($env:ONDX_REPO) { $env:ONDX_REPO } else { 'openndx/openndx-core' }
    $version = if ($env:ONDX_VERSION) { $env:ONDX_VERSION } else { 'latest' }

    if ($PSVersionTable.PSEdition -eq 'Core' -and -not $IsWindows) {
        throw 'install.ps1 is for Windows; on macOS/Linux use: curl -fsSL https://raw.githubusercontent.com/openndx/openndx-core/main/scripts/install.sh | sh'
    }

    # Windows PowerShell 5.1 may default to TLS 1.0, which GitHub rejects.
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    # OSArchitecture reports the real CPU even when this shell runs under x64
    # emulation on ARM64; fall back to the env vars on older .NET.
    $osArch = try { [Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString() } catch { $null }
    if (-not $osArch) {
        $osArch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    }
    $arch = switch ($osArch.ToUpperInvariant()) {
        { $_ -in 'X64', 'AMD64' } { 'amd64'; break }
        'ARM64' { 'arm64'; break }
        default { throw "unsupported architecture: $osArch" }
    }

    $base = if ($version -eq 'latest') {
        "https://github.com/$repo/releases/latest/download"
    } else {
        "https://github.com/$repo/releases/download/$version"
    }

    $archive = "ondx_windows_$arch.zip"
    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("ondx-install-" + [Guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Write-Host "Downloading $archive ($version)..."
        $archivePath = Join-Path $tmp $archive
        $checksumsPath = Join-Path $tmp 'checksums.txt'
        Invoke-WebRequest -UseBasicParsing -Uri "$base/$archive" -OutFile $archivePath
        Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile $checksumsPath

        $expected = Get-Content $checksumsPath |
            ForEach-Object { $hash, $file = -split $_; if ($file -eq $archive) { $hash } } |
            Select-Object -First 1
        if (-not $expected) { throw "$archive not listed in checksums.txt" }
        $actual = (Get-FileHash -Algorithm SHA256 -Path $archivePath).Hash
        # -ne is case-insensitive, so the lowercase checksums.txt hash compares fine.
        if ($expected -ne $actual) {
            throw "checksum mismatch for $archive (expected $expected, got $($actual.ToLowerInvariant()))"
        }

        Expand-Archive -Path $archivePath -DestinationPath $tmp -Force

        $dir = if ($env:ONDX_INSTALL_DIR) { $env:ONDX_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\ondx' }
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
        $exe = Join-Path $dir 'ondx.exe'
        Copy-Item -Path (Join-Path $tmp 'ondx.exe') -Destination $exe -Force
    } finally {
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }

    Write-Host "Installed ondx to $exe"

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $entries = if ($userPath) { $userPath -split ';' | Where-Object { $_ } } else { @() }
    if ($entries -notcontains $dir) {
        [Environment]::SetEnvironmentVariable('Path', (@($entries) + $dir) -join ';', 'User')
        $env:Path = "$env:Path;$dir"
        Write-Host "Added $dir to your user PATH (open a new terminal for other shells to pick it up)."
    }

    & $exe version
}
