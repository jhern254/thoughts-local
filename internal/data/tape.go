// tape.go
package data

import (
	//    "fmt"
	//    "io"
	"os"
	// "net/http"
	// "errors"
	// "strings"
	// "encoding/json"
)

type tape struct {
	file *os.File
}

func (t *tape) Write(p []byte) (n int, err error) {
	t.file.Truncate(0)
	t.file.Seek(0, 0)
	return t.file.Write(p)
}
