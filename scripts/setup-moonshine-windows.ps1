param(
    [Parameter(Mandatory = $true)][string]$RuntimeArchivePath,
    [Parameter(Mandatory = $true)][string]$OutputDirectory
)
$ErrorActionPreference = 'Stop'

$expectedSHA256 = '97c1987e8e1cd77bb5fe3b12ce5aad5172637107e1dfda112ea9b21bac8f4b65'
$archiveSHA256 = (Get-FileHash -Algorithm SHA256 $RuntimeArchivePath).Hash.ToLowerInvariant()
if ($archiveSHA256 -ne $expectedSHA256) { throw 'Moonshine runtime archive checksum differs' }

# The release contains MSVC C++ static archives, not a Moonshine DLL. Keep those
# objects behind an MSVC-built DLL so cgo's MinGW compiler only sees the C ABI.
# Activate MSVC only inside this helper; the caller's environment is unchanged.
if (-not (Get-Command cl.exe -ErrorAction SilentlyContinue) -or -not $env:VCToolsRedistDir) {
    $vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio/Installer/vswhere.exe'
    if (-not (Test-Path $vswhere)) { throw 'Moonshine setup requires Visual Studio C++ build tools' }
    $visualStudioDirectory = & $vswhere -latest -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
    if (-not $visualStudioDirectory) { throw 'Moonshine setup requires Visual Studio C++ build tools' }
    $developerCommand = "`"$visualStudioDirectory\Common7\Tools\VsDevCmd.bat`" -arch=x64 -host_arch=x64 >nul && set"
    $developerEnvironment = & $env:ComSpec /d /s /c $developerCommand
    if ($LASTEXITCODE -ne 0) { throw 'Moonshine MSVC environment setup failed' }
    foreach ($environmentLine in $developerEnvironment) {
        if ($environmentLine -match '^([^=]+)=(.*)$') {
            [Environment]::SetEnvironmentVariable($Matches[1], $Matches[2], 'Process')
        }
    }
}
Get-Command cl.exe, lib.exe, dumpbin.exe, dlltool.exe -ErrorAction Stop | Out-Null
New-Item -ItemType Directory -Force $OutputDirectory | Out-Null
$OutputDirectory = (Resolve-Path $OutputDirectory).Path
# Git Bash's GNU tar interprets a drive-letter archive path as a remote host.
# Use Windows' bsdtar so this helper works from either PowerShell or Git Bash.
$windowsTar = Join-Path $env:SystemRoot 'System32/tar.exe'
& $windowsTar -xzf $RuntimeArchivePath -C $OutputDirectory
if ($LASTEXITCODE -ne 0) { throw 'Moonshine runtime extraction failed' }
$runtimeDirectory = Join-Path $OutputDirectory 'moonshine-voice-windows-x86_64'
$nativeLibraryDirectory = Join-Path $runtimeDirectory 'lib'
$bridgeDirectory = Join-Path $runtimeDirectory 'bridge'
New-Item -ItemType Directory -Force $bridgeDirectory | Out-Null

# Windows can select its System32 onnxruntime.dll before directories on PATH.
# Keep the verified CPU runtime under a private name and regenerate its import
# library so the bridge cannot silently bind to a different ONNX API version.
$onnxExportDefinitions = Join-Path $bridgeDirectory 'onnx-runtime.def'
@'
LIBRARY thoughts-moonshine-onnxruntime
EXPORTS
    OrtGetApiBase
    OrtSessionOptionsAppendExecutionProvider_CPU
'@ | Set-Content -Encoding ASCII $onnxExportDefinitions
$onnxImportLibrary = Join-Path $bridgeDirectory 'thoughts-moonshine-onnxruntime.lib'
& lib.exe /nologo /MACHINE:X64 "/DEF:$onnxExportDefinitions" "/OUT:$onnxImportLibrary"
if ($LASTEXITCODE -ne 0) { throw 'Private ONNX import library build failed' }
Copy-Item (Join-Path $nativeLibraryDirectory 'onnxruntime.dll') (Join-Path $bridgeDirectory 'thoughts-moonshine-onnxruntime.dll') -Force

$bridgeSource = Join-Path $bridgeDirectory 'runtime-bridge.cpp'
@'
#include <moonshine-c-api.h>
static_assert(MOONSHINE_HEADER_VERSION == 30000, "Thoughts requires C API 3.0.0");
'@ | Set-Content -Encoding ASCII $bridgeSource
$exportDefinitions = Join-Path $bridgeDirectory 'runtime-bridge.def'
@'
LIBRARY thoughts-moonshine
EXPORTS
    moonshine_get_version
    moonshine_load_transcriber_from_memory_files
    moonshine_transcribe_without_streaming
    moonshine_create_stream
    moonshine_start_stream
    moonshine_transcribe_add_audio_to_stream
    moonshine_transcribe_stream
    moonshine_stop_stream
    moonshine_free_stream
    moonshine_free_transcriber
'@ | Set-Content -Encoding ASCII $exportDefinitions

$bridgeDLL = Join-Path $bridgeDirectory 'thoughts-moonshine.dll'
$bridgeObject = Join-Path $bridgeDirectory 'runtime-bridge.obj'
$msvcImportLibrary = Join-Path $bridgeDirectory 'thoughts-moonshine.lib'
$nativeLibraries = @('moonshine.lib', 'bin-tokenizer.lib', 'ort-utils.lib', 'moonshine-utils.lib') |
    ForEach-Object { Join-Path $nativeLibraryDirectory $_ }
$nativeLibraries += $onnxImportLibrary
& cl.exe /nologo /LD /O2 /MD "/I$runtimeDirectory/include" "/Fo$bridgeObject" $bridgeSource /link "/DEF:$exportDefinitions" "/OUT:$bridgeDLL" "/IMPLIB:$msvcImportLibrary" @nativeLibraries ole32.lib mmdevapi.lib
if ($LASTEXITCODE -ne 0) { throw 'Moonshine MSVC bridge build failed' }

$mingwImportLibrary = Join-Path $bridgeDirectory 'libthoughts-moonshine.dll.a'
& dlltool.exe --machine i386:x86-64 --input-def $exportDefinitions --dllname thoughts-moonshine.dll --output-lib $mingwImportLibrary
if ($LASTEXITCODE -ne 0) { throw 'Moonshine MinGW import library build failed' }
$bridgeDependencies = & dumpbin.exe /DEPENDENTS $bridgeDLL
if ($LASTEXITCODE -ne 0) { throw 'Moonshine bridge dependency inspection failed' }
if ($bridgeDependencies -match '^\s*onnxruntime\.dll\s*$') { throw 'Moonshine bridge imports ambient ONNX Runtime' }
if (-not ($bridgeDependencies -match 'thoughts-moonshine-onnxruntime\.dll')) { throw 'Moonshine bridge lacks its private ONNX dependency' }
# The user bundle must not depend on Visual Studio being installed. Copy only
# the toolchain's designated app-local redistributables, never System32 DLLs.
$redistributableDirectories = @(Get-ChildItem (Join-Path $env:VCToolsRedistDir 'x64') -Directory |
    Where-Object { $_.Name -match '^Microsoft\.VC\d+\.CRT$' })
if ($redistributableDirectories.Count -ne 1) { throw 'Moonshine setup requires the x64 Visual C++ redistributable directory' }
Copy-Item (Join-Path $redistributableDirectories[0].FullName '*.dll') $bridgeDirectory -Force
Write-Output "Moonshine C API bridge built from verified v0.1.5 runtime"
