#!/usr/bin/env bash
# Engine preparation belongs to the builder. A user bundle contains the already
# linked app and its private libraries; only model weights are installed in-app.
set -euo pipefail
repositoryDirectory=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repositoryDirectory"
if [[ $# != 0 ]]; then echo 'Package accepts no arguments.' >&2; exit 1; fi
hostOS=$(go env GOHOSTOS)
hostArchitecture=$(go env GOHOSTARCH)
case "$hostOS-$hostArchitecture" in
    linux-amd64|darwin-arm64|darwin-amd64|windows-amd64) ;;
    *) echo 'Speech-enabled packages are unavailable for this platform.' >&2; exit 1;;
esac
if [[ "${CGO_ENABLED:-1}" == 0 || "${GOOS:-$hostOS}" != "$hostOS" || "${GOARCH:-$hostArchitecture}" != "$hostArchitecture" ]]; then
    echo 'Packaging requires a native build for the current host.' >&2; exit 1
fi
bash scripts/moonshine-dev.sh ensure
packageParent="$repositoryDirectory/bin/packages"
bundleDirectory="$packageParent/thoughts-$hostOS-$hostArchitecture"
mkdir -p "$packageParent"
stagingDirectory=$(mktemp -d "$packageParent/.package-XXXXXXXX")
trap 'rm -rf -- "$stagingDirectory"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
preparedDirectory="$repositoryDirectory/bin/moonshine/v0.1.5/$hostOS-$hostArchitecture"
executableName=thoughts-tui
case "$hostOS" in
    linux)
        nativeLibraryDirectory="$preparedDirectory/moonshine-voice-linux-x86_64/lib"
        mkdir "$stagingDirectory/lib"
        cp "$nativeLibraryDirectory/libmoonshine.so" "$nativeLibraryDirectory/libonnxruntime.so.1" "$stagingDirectory/lib/"
        ;;
    windows)
        executableName=thoughts-tui.exe
        bridgeDirectory="$preparedDirectory/moonshine-voice-windows-x86_64/bridge"
        if [[ ! -f "$bridgeDirectory/vcruntime140.dll" || ! -f "$bridgeDirectory/msvcp140.dll" ]]; then
            bash scripts/moonshine-dev.sh setup
        fi
        cp "$preparedDirectory/moonshine-voice-windows-x86_64/bridge/"*.dll "$stagingDirectory/"
        ;;
    # macOS links the official desktop static archive into the executable.
esac
bash scripts/moonshine-dev.sh exec go build -o "$stagingDirectory/$executableName" ./cmd/thoughts-tui

mkdir -p "$stagingDirectory/licenses"
cp internal/speech/moonshine/LICENSE-* "$stagingDirectory/licenses/"
goRootDirectory=$(go env GOROOT)
if [[ "$hostOS" == windows ]]; then goRootDirectory=$(cygpath -u "$goRootDirectory"); fi
cp "$goRootDirectory/LICENSE" "$stagingDirectory/licenses/LICENSE-GO"
# Retain notices from the pinned Go dependencies without fetching new versions.
go list -m -f '{{.Path}}|{{.Dir}}' all > "$stagingDirectory/modules.txt"
while IFS='|' read -r modulePath moduleDirectory; do
    if [[ -z "$moduleDirectory" ]]; then continue; fi
    if [[ "$hostOS" == windows ]]; then moduleDirectory=$(cygpath -u "$moduleDirectory"); fi
    if [[ "$moduleDirectory" == "$repositoryDirectory" ]]; then continue; fi
    mkdir -p "$stagingDirectory/licenses/go/$modulePath"
    find "$moduleDirectory" -maxdepth 1 -type f \( -iname '*license*' -o -iname 'copying*' -o -iname 'notice*' \) -exec cp '{}' "$stagingDirectory/licenses/go/$modulePath/" \;
done < "$stagingDirectory/modules.txt"
rm "$stagingDirectory/modules.txt"
cp docs/desktop-bundle.md "$stagingDirectory/README.md"

# Loader checks must use the bundle, not the developer library search path.
unset LD_LIBRARY_PATH DYLD_LIBRARY_PATH
"$stagingDirectory/$executableName" --help >/dev/null
rm -rf -- "$bundleDirectory"
mv "$stagingDirectory" "$bundleDirectory"
bundleName="thoughts-$hostOS-$hostArchitecture"
if [[ "$hostOS" == windows ]]; then
    archiveFilename="$bundleName.zip"
    export THOUGHTS_PACKAGE_FOLDER
    THOUGHTS_PACKAGE_FOLDER=$(cygpath -m "$bundleDirectory")
    export THOUGHTS_PACKAGE_ARCHIVE
    THOUGHTS_PACKAGE_ARCHIVE=$(cygpath -m "$packageParent/$archiveFilename")
    pwsh -NoProfile -Command 'Compress-Archive -Path $env:THOUGHTS_PACKAGE_FOLDER -DestinationPath $env:THOUGHTS_PACKAGE_ARCHIVE -Force'
else
    archiveFilename="$bundleName.tar.gz"
    # tar preserves executable permissions through CI artifact upload/download.
    tar -czf "$packageParent/$archiveFilename" -C "$packageParent" "$bundleName"
fi
(
    cd "$packageParent"
    if [[ "$hostOS" == darwin ]]; then
        shasum -a 256 "$archiveFilename" > "$archiveFilename.sha256"
    else
        sha256sum "$archiveFilename" > "$archiveFilename.sha256"
    fi
)
echo "Speech-enabled app release artifact: bin/packages/$archiveFilename."
