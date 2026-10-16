#!/usr/bin/env bash
# Native libraries must be ready before Go starts; this developer wrapper keeps
# their build and loader settings local to the command it launches.
set -euo pipefail
repositoryDirectory=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repositoryDirectory"
operation=${1:-}
if [[ "$operation" != setup && "$operation" != ensure && "$operation" != exec ]]; then
    echo 'Usage: moonshine-dev.sh setup | ensure | exec COMMAND [ARGUMENTS...]' >&2
    exit 1
fi
shift
hostOS=$(go env GOHOSTOS)
hostArchitecture=$(go env GOHOSTARCH)
runtimeVersion=v0.1.5
runtimeParent="$repositoryDirectory/bin/moonshine/$runtimeVersion"
preparedDirectory="$runtimeParent/$hostOS-$hostArchitecture"
case "$hostOS-$hostArchitecture" in
    linux-amd64)
        runtimeURL=https://github.com/moonshine-ai/moonshine/releases/download/v0.1.5/moonshine-voice-linux-x86_64.tar.gz
        expectedSHA256=9c3a87fea93ff2ad957938868f95a0a366dce9ff8ad86bde6cdcf5a4cadb51df
        runtimeSubdirectory=moonshine-voice-linux-x86_64
        ;;
    darwin-arm64|darwin-amd64)
        runtimeURL=https://github.com/moonshine-ai/moonshine-swift/releases/download/v0.1.5/Moonshine.xcframework.zip
        expectedSHA256=6bc7fb4b6d3a470a2ae2d681299975f3ba9d710753786d1cd7e8beabaad066e8
        runtimeSubdirectory=Moonshine.xcframework/macos-arm64_x86_64
        ;;
    windows-amd64)
        runtimeURL=https://github.com/moonshine-ai/moonshine/releases/download/v0.1.5/moonshine-voice-windows-x86_64.tar.gz
        expectedSHA256=97c1987e8e1cd77bb5fe3b12ce5aad5172637107e1dfda112ea9b21bac8f4b65
        runtimeSubdirectory=moonshine-voice-windows-x86_64
        ;;
    *)
        if [[ "$operation" == exec && $# -gt 0 ]]; then exec "$@"; fi
        if [[ "$operation" == ensure ]]; then exit 0; fi
        echo 'Moonshine developer setup is unavailable for this platform.' >&2
        exit 1
        ;;
esac

configureRuntime() {
    local runtimeDirectory="$1/$runtimeSubdirectory"
    local requiredFiles=()
    case "$hostOS" in
        linux) requiredFiles=(include/moonshine-c-api.h lib/libmoonshine.so lib/libonnxruntime.so.1);;
        darwin) requiredFiles=(Headers/moonshine-c-api.h libmoonshine.a);;
        windows) requiredFiles=(include/moonshine-c-api.h bridge/libthoughts-moonshine.dll.a bridge/thoughts-moonshine.dll bridge/thoughts-moonshine-onnxruntime.dll);;
    esac
    for relativePath in "${requiredFiles[@]}"; do
        if [[ ! -f "$runtimeDirectory/$relativePath" ]]; then
            echo 'Moonshine setup is incomplete. Run make speech/setup.' >&2
            return 1
        fi
    done
    if [[ "${CGO_ENABLED:-1}" == 0 ]]; then
        echo 'Prepared Moonshine requires cgo; use an ordinary Go build for CGO_ENABLED=0.' >&2
        return 1
    fi
    if [[ "${GOOS:-$hostOS}" != "$hostOS" || "${GOARCH:-$hostArchitecture}" != "$hostArchitecture" ]]; then
        echo 'Prepared Moonshine requires a build for the current host platform.' >&2
        return 1
    fi
    export CGO_ENABLED=1
    export GOFLAGS="${GOFLAGS:+$GOFLAGS }-tags=moonshine"
    if [[ "$hostOS" == windows ]]; then
        runtimeDirectory=$(cygpath -m "$runtimeDirectory")
        export CC
        CC=$(cygpath -m "$(command -v gcc)")
        export CGO_CFLAGS="${CGO_CFLAGS:+$CGO_CFLAGS }\"-I$runtimeDirectory/include\""
        export CGO_LDFLAGS="${CGO_LDFLAGS:+$CGO_LDFLAGS }\"-L$runtimeDirectory/bridge\""
        export PATH="$(cygpath -u "$runtimeDirectory/bridge"):$PATH"
    elif [[ "$hostOS" == darwin ]]; then
        export CGO_CFLAGS="${CGO_CFLAGS:+$CGO_CFLAGS }\"-I$runtimeDirectory/Headers\""
        export CGO_LDFLAGS="${CGO_LDFLAGS:+$CGO_LDFLAGS }\"-L$runtimeDirectory\""
    else
        export CGO_CFLAGS="${CGO_CFLAGS:+$CGO_CFLAGS }\"-I$runtimeDirectory/include\""
        # Packaged binaries find their sibling libraries after being moved to
        # another machine. The upstream Moonshine library also uses $ORIGIN.
        export CGO_LDFLAGS="${CGO_LDFLAGS:+$CGO_LDFLAGS }\"-L$runtimeDirectory/lib\" -Wl,-rpath,\$ORIGIN/lib"
        export LD_LIBRARY_PATH="$runtimeDirectory/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
    fi
}

if [[ "$operation" == ensure ]]; then
    if [[ $# != 0 ]]; then echo 'Ensure accepts no arguments.' >&2; exit 1; fi
    if [[ "${CGO_ENABLED:-1}" == 0 || "${GOOS:-$hostOS}" != "$hostOS" || "${GOARCH:-$hostArchitecture}" != "$hostArchitecture" ]]; then
        exit 0
    fi
    if [[ ! -e "$preparedDirectory" ]]; then
        exec bash "$repositoryDirectory/scripts/moonshine-dev.sh" setup
    fi
    if [[ ! -f "$preparedDirectory/.ready" ]] || [[ "$(cat "$preparedDirectory/.ready")" != "$expectedSHA256" ]]; then
        echo 'Moonshine setup is incomplete. Run make speech/setup.' >&2
        exit 1
    fi
    configureRuntime "$preparedDirectory"
    exit 0
fi

if [[ "$operation" == exec ]]; then
    if [[ $# == 0 ]]; then echo 'A developer command is required.' >&2; exit 1; fi
    if [[ -e "$preparedDirectory" ]]; then
        if [[ ! -f "$preparedDirectory/.ready" ]] || [[ "$(cat "$preparedDirectory/.ready")" != "$expectedSHA256" ]]; then
            echo 'Moonshine setup is incomplete. Run make speech/setup.' >&2
            exit 1
        fi
        configureRuntime "$preparedDirectory"
    fi
    exec "$@"
fi

if [[ $# != 0 ]]; then echo 'Setup accepts no arguments.' >&2; exit 1; fi
for requiredCommand in curl tar mktemp; do
    if ! command -v "$requiredCommand" >/dev/null; then
        echo "Moonshine setup requires $requiredCommand." >&2; exit 1
    fi
done
if [[ "$hostOS" == darwin ]]; then
    command -v unzip >/dev/null && command -v shasum >/dev/null || { echo 'Moonshine setup requires unzip and shasum.' >&2; exit 1; }
else
    command -v sha256sum >/dev/null || { echo 'Moonshine setup requires sha256sum.' >&2; exit 1; }
fi
if [[ "$hostOS" == windows ]]; then
    for requiredCommand in pwsh gcc dlltool cygpath; do
        command -v "$requiredCommand" >/dev/null || { echo "Moonshine setup requires $requiredCommand." >&2; exit 1; }
    done
else
    command -v "$(go env CC)" >/dev/null || { echo 'Moonshine setup requires the Go C compiler.' >&2; exit 1; }
fi
mkdir -p "$runtimeParent"
if ! mkdir "$runtimeParent/.setup.lock" 2>/dev/null; then
    echo 'Moonshine setup is already running, or its lock needs manual cleanup.' >&2
    exit 1
fi
stagingDirectory=''
trap 'if [[ -n "$stagingDirectory" ]]; then rm -rf -- "$stagingDirectory"; fi; rmdir "$runtimeParent/.setup.lock"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
stagingDirectory=$(mktemp -d "$runtimeParent/.setup-XXXXXXXX")
archivePath="$stagingDirectory/runtime.archive"
if [[ -f "$preparedDirectory/runtime.archive" ]]; then
    cp "$preparedDirectory/runtime.archive" "$archivePath"
else
    echo "Downloading Moonshine Voice $runtimeVersion for $hostOS/$hostArchitecture."
    curl --fail --location --proto '=https' --proto-redir '=https' --connect-timeout 10 --max-time 600 "$runtimeURL" -o "$archivePath"
fi
if [[ "$hostOS" == darwin ]]; then
    actualSHA256=$(shasum -a 256 "$archivePath")
else
    actualSHA256=$(sha256sum "$archivePath")
fi
if [[ "${actualSHA256%% *}" != "$expectedSHA256" ]]; then
    echo 'Moonshine runtime archive checksum differs; nothing was extracted.' >&2
    exit 1
fi
if [[ "$hostOS" == darwin ]]; then
    unzip -q "$archivePath" 'Moonshine.xcframework/macos-arm64_x86_64/*' -d "$stagingDirectory"
elif [[ "$hostOS" == windows ]]; then
    pwsh -NoProfile -File scripts/setup-moonshine-windows.ps1 -RuntimeArchivePath "$archivePath" -OutputDirectory "$stagingDirectory"
else
    tar -xzf "$archivePath" -C "$stagingDirectory"
fi
# Executing a linked test catches loader problems, not merely compile failures.
# This check needs no model weights and downloads nothing.
(
    configureRuntime "$stagingDirectory"
    go test -count=1 ./internal/speech/moonshine
)
printf '%s\n' "$expectedSHA256" > "$stagingDirectory/.ready"
# Leave a working bundle untouched until its replacement has passed the check.
previousDirectory="$runtimeParent/.previous-$hostOS-$hostArchitecture"
if [[ -e "$previousDirectory" ]]; then
    echo 'A previous setup backup remains; inspect it before retrying setup.' >&2
    exit 1
fi
if [[ -e "$preparedDirectory" ]]; then mv "$preparedDirectory" "$previousDirectory"; fi
if ! mv "$stagingDirectory" "$preparedDirectory"; then
    if [[ -e "$previousDirectory" ]]; then mv "$previousDirectory" "$preparedDirectory"; fi
    exit 1
fi
rm -rf -- "$previousDirectory"
echo 'Moonshine engine ready. make tui/browser will use it; model installation remains separate.'
