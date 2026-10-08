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
# Build in an x64 Visual Studio developer shell with MinGW tools on PATH.
Get-Command cl.exe, lib.exe, dumpbin.exe, dlltool.exe -ErrorAction Stop | Out-Null
New-Item -ItemType Directory -Force $OutputDirectory | Out-Null
$OutputDirectory = (Resolve-Path $OutputDirectory).Path
& tar -xzf $RuntimeArchivePath -C $OutputDirectory
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
Write-Output "Moonshine C API bridge built from verified v0.1.5 runtime"
