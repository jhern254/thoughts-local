// tape_test.go
package data

import (
	//    "fmt"
	"io"
	"testing"
	// "os"
	// "net/http"
	// "errors"
	// "strings"
	// "encoding/json"
)

func TestTape_Write(t *testing.T) {
	file, clean := CreateTempFile(t, "12345")
	defer clean()

	tape := &tape{file}

	tape.Write([]byte("abc"))

	file.Seek(0, 0)
	newFileContents, _ := io.ReadAll(file)

	got := string(newFileContents)
	want := "abc"

	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}
