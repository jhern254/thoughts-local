package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMoonshineDeveloperSetup(t *testing.T) {
	t.Run("ordinary execution needs no runtime and makes no downloads", func(t *testing.T) {
		fixture := newRuntimeScriptFixture(t)
		output, err := fixture.run("exec", "go", "probe")
		if err != nil {
			t.Fatal(err, output)
		}
		if !strings.Contains(output, "tags= flags= libraries=") {
			t.Fatalf("unexpected scoped environment: %s", output)
		}
		fixture.assertNoExtraction(t)
	})
	t.Run("checksum rejection happens before extraction", func(t *testing.T) {
		fixture := newRuntimeScriptFixture(t)
		fixture.environment = append(fixture.environment, "TEST_CHECKSUM=incorrect")
		output, err := fixture.run("setup")
		if err == nil || !strings.Contains(output, "checksum") {
			t.Fatalf("got %s, %v, want checksum rejection", output, err)
		}
		fixture.assertNoExtraction(t)
		fixture.assertNoPreparedRuntime(t)
	})
	t.Run("successful setup prepares scoped launch and is reused", func(t *testing.T) {
		fixture := newRuntimeScriptFixture(t)
		if output, err := fixture.run("setup"); err != nil {
			t.Fatal(err, output)
		}
		output, err := fixture.run("exec", "go", "probe")
		if err != nil {
			t.Fatal(err, output)
		}
		if !strings.Contains(output, "tags=-tags=moonshine") || !strings.Contains(output, "include") || !strings.Contains(output, "/lib") {
			t.Fatalf("runtime environment missing: %s", output)
		}
		if output, err := fixture.run("setup"); err != nil {
			t.Fatal(err, output)
		}
		downloads, err := os.ReadFile(filepath.Join(fixture.root, "downloads"))
		if err != nil {
			t.Fatal(err)
		}
		if string(downloads) != "download\n" {
			t.Fatalf("downloads = %q, want one download", downloads)
		}
	})
	t.Run("failed native check preserves prepared runtime and cleans staging", func(t *testing.T) {
		fixture := newRuntimeScriptFixture(t)
		if output, err := fixture.run("setup"); err != nil {
			t.Fatal(err, output)
		}
		fixture.environment = append(fixture.environment, "TEST_NATIVE_FAILURE=1")
		if output, err := fixture.run("setup"); err == nil {
			t.Fatalf("got %s, want failed native execution", output)
		}
		if _, err := os.Stat(filepath.Join(fixture.root, "bin/moonshine/v0.1.5/linux-amd64/.ready")); err != nil {
			t.Fatal("previous runtime lost", err)
		}
		staging, err := filepath.Glob(filepath.Join(fixture.root, "bin/moonshine/v0.1.5/.setup-*"))
		if err != nil {
			t.Fatal(err)
		}
		if len(staging) != 0 {
			t.Fatalf("staging remains: %v", staging)
		}
	})
	t.Run("incomplete runtime fails instead of silently disabling speech", func(t *testing.T) {
		fixture := newRuntimeScriptFixture(t)
		if err := os.MkdirAll(filepath.Join(fixture.root, "bin/moonshine/v0.1.5/linux-amd64"), 0700); err != nil {
			t.Fatal(err)
		}
		output, err := fixture.run("exec", "go", "probe")
		if err == nil || !strings.Contains(output, "incomplete") {
			t.Fatalf("got %s, %v, want incomplete runtime rejection", output, err)
		}
	})
	t.Run("missing prepared library fails before launching a command", func(t *testing.T) {
		fixture := newRuntimeScriptFixture(t)
		if output, err := fixture.run("setup"); err != nil {
			t.Fatal(err, output)
		}
		if err := os.Remove(filepath.Join(fixture.root, "bin/moonshine/v0.1.5/linux-amd64/moonshine-voice-linux-x86_64/lib/libmoonshine.so")); err != nil {
			t.Fatal(err)
		}
		output, err := fixture.run("exec", "go", "probe")
		if err == nil || !strings.Contains(output, "incomplete") {
			t.Fatalf("got %s, %v, want incomplete runtime rejection", output, err)
		}
	})
	t.Run("unsupported setup fails without downloading", func(t *testing.T) {
		fixture := newRuntimeScriptFixture(t)
		fixture.environment = append(fixture.environment, "TEST_HOST_ARCH=arm64")
		if output, err := fixture.run("setup"); err == nil {
			t.Fatalf("got %s, want unsupported target", output)
		}
		fixture.assertNoExtraction(t)
		if _, err := os.Stat(filepath.Join(fixture.root, "downloads")); !os.IsNotExist(err) {
			t.Fatalf("download occurred: %v", err)
		}
	})
	t.Run("macOS setup selects the complete desktop archive on both architectures", func(t *testing.T) {
		for _, architecture := range []string{"arm64", "amd64"} {
			t.Run(architecture, func(t *testing.T) {
				fixture := newRuntimeScriptFixture(t)
				fixture.environment = append(fixture.environment, "TEST_HOST_OS=darwin", "TEST_HOST_ARCH="+architecture)
				if output, err := fixture.run("setup"); err != nil {
					t.Fatal(err, output)
				}
				output, err := fixture.run("exec", "go", "probe")
				if err != nil || !strings.Contains(output, "/Headers") || !strings.Contains(output, "macos-arm64_x86_64") {
					t.Fatalf("got %s, %v, want desktop runtime settings", output, err)
				}
			})
		}
	})
	t.Run("explicit non-cgo launch is rejected for a prepared engine", func(t *testing.T) {
		fixture := newRuntimeScriptFixture(t)
		if output, err := fixture.run("setup"); err != nil {
			t.Fatal(err, output)
		}
		fixture.environment = append(fixture.environment, "CGO_ENABLED=0")
		if output, err := fixture.run("exec", "go", "probe"); err == nil || !strings.Contains(output, "cgo") {
			t.Fatalf("got %s, %v, want cgo prerequisite", output, err)
		}
	})
	t.Run("setup contention makes no downloads", func(t *testing.T) {
		fixture := newRuntimeScriptFixture(t)
		if err := os.MkdirAll(filepath.Join(fixture.root, "bin/moonshine/v0.1.5/.setup.lock"), 0700); err != nil {
			t.Fatal(err)
		}
		if output, err := fixture.run("setup"); err == nil || !strings.Contains(output, "already running") {
			t.Fatalf("got %s, %v, want busy setup", output, err)
		}
		if _, err := os.Stat(filepath.Join(fixture.root, "downloads")); !os.IsNotExist(err) {
			t.Fatalf("download occurred: %v", err)
		}
	})
}

type runtimeScriptFixture struct {
	root        string
	environment []string
	bashPath    string
}

func newRuntimeScriptFixture(t *testing.T) runtimeScriptFixture {
	t.Helper()
	bashPath, err := exec.LookPath("bash")
	if runtime.GOOS == "windows" {
		// System32's bash may be WSL; this helper targets the Git Bash toolchain.
		gitBashPath := filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin", "bash.exe")
		if _, gitBashErr := os.Stat(gitBashPath); gitBashErr == nil {
			bashPath, err = gitBashPath, nil
		}
	}
	if err != nil {
		t.Skip("developer helper requires Bash")
	}
	root := filepath.Join(t.TempDir(), "project with spaces")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{"scripts", "commands"} {
		if err := os.Mkdir(filepath.Join(root, directory), 0700); err != nil {
			t.Fatal(err)
		}
	}
	script, err := os.ReadFile("moonshine-dev.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts/moonshine-dev.sh"), script, 0700); err != nil {
		t.Fatal(err)
	}
	commands := map[string]string{
		"cc": "exit 0",
		"go": `case "$*" in
"env GOHOSTOS") echo "${TEST_HOST_OS:-linux}";;
"env GOHOSTARCH") echo "${TEST_HOST_ARCH:-amd64}";;
"env CC") echo cc;;
probe) echo "tags=${GOFLAGS:-} flags=${CGO_CFLAGS:-} libraries=${LD_LIBRARY_PATH:-}";;
*) test "${TEST_NATIVE_FAILURE:-0}" = 0;;
esac`,
		"curl":      `echo download >> "$TEST_ROOT/downloads"; while [ "$1" != -o ]; do shift; done; printf archive > "$2"`,
		"sha256sum": `echo "${TEST_CHECKSUM:-9c3a87fea93ff2ad957938868f95a0a366dce9ff8ad86bde6cdcf5a4cadb51df}  archive"`,
		"shasum":    `echo "${TEST_CHECKSUM:-6bc7fb4b6d3a470a2ae2d681299975f3ba9d710753786d1cd7e8beabaad066e8}  archive"`,
		"unzip":     `touch "$TEST_ROOT/extracted"; while [ "$1" != -d ]; do shift; done; mkdir -p "$2/Moonshine.xcframework/macos-arm64_x86_64/Headers"; touch "$2/Moonshine.xcframework/macos-arm64_x86_64/Headers/moonshine-c-api.h" "$2/Moonshine.xcframework/macos-arm64_x86_64/libmoonshine.a"`,
		"tar":       `touch "$TEST_ROOT/extracted"; while [ "$1" != -C ]; do shift; done; mkdir -p "$2/moonshine-voice-linux-x86_64/include" "$2/moonshine-voice-linux-x86_64/lib"; touch "$2/moonshine-voice-linux-x86_64/include/moonshine-c-api.h" "$2/moonshine-voice-linux-x86_64/lib/libmoonshine.so" "$2/moonshine-voice-linux-x86_64/lib/libonnxruntime.so.1"`,
	}
	for commandName, body := range commands {
		if err := os.WriteFile(filepath.Join(root, "commands", commandName), []byte("#!/bin/bash\nset -eu\n"+body+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	environment := []string{}
	for _, variable := range os.Environ() {
		if strings.HasPrefix(variable, "GO") || strings.HasPrefix(variable, "CGO_") || strings.HasPrefix(variable, "LD_LIBRARY_PATH=") || strings.HasPrefix(variable, "THOUGHTS_MOONSHINE_RUNTIME=") || strings.HasPrefix(variable, "PATH=") {
			continue
		}
		environment = append(environment, variable)
	}
	environment = append(environment,
		"PATH="+os.Getenv("PATH"),
		"TEST_ROOT="+filepath.ToSlash(root),
		"TEST_COMMAND_DIRECTORY="+filepath.ToSlash(filepath.Join(root, "commands")),
		// If fixture command selection breaks, real curl must not reach upstream.
		"HTTPS_PROXY=http://127.0.0.1:1",
		"https_proxy=http://127.0.0.1:1",
		"NO_PROXY=",
		"no_proxy=",
	)
	return runtimeScriptFixture{root: root, environment: environment, bashPath: bashPath}
}
func (fixture runtimeScriptFixture) run(arguments ...string) (string, error) {
	// Git Bash adjusts PATH on Windows startup. Set fixture precedence afterward.
	launchScript := `export PATH="$(cd "$TEST_COMMAND_DIRECTORY" && pwd):$PATH"; scriptPath=$1; shift; source "$scriptPath" "$@"`
	commandArguments := []string{"-c", launchScript, "moonshine-fixture", filepath.ToSlash(filepath.Join(fixture.root, "scripts/moonshine-dev.sh"))}
	command := exec.Command(fixture.bashPath, append(commandArguments, arguments...)...)
	command.Dir = fixture.root
	command.Env = fixture.environment
	output, err := command.CombinedOutput()
	return string(output), err
}
func (fixture runtimeScriptFixture) assertNoExtraction(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(fixture.root, "extracted")); !os.IsNotExist(err) {
		t.Fatalf("unexpected extraction: %v", err)
	}
}
func (fixture runtimeScriptFixture) assertNoPreparedRuntime(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(fixture.root, "bin/moonshine/v0.1.5/linux-amd64/.ready")); !os.IsNotExist(err) {
		t.Fatalf("unexpected readiness marker: %v", err)
	}
}
