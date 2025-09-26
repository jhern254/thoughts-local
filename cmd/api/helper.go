// helpers.go
package main

import (
//    "fmt"
//    "os"
    "net/http"
//    "errors"
//    "strings"
//    "io"
    "encoding/json"

//    "github.com/rs/zerolog" 
)

func (app *application) writeJSON(w http.ResponseWriter, status int, data interface{}, headers http.Header) error { // Encode data to json
    js, err := json.Marshal(data)
    if err != nil {
        return err
    }

    js = append(js, '\n')

    // add headers
    for key, value := range headers {
        w.Header()[key] = value
    }

    w.Header().Set("content-Type", jsonContentType)
    w.WriteHeader(status)
    w.Write(js)

    return nil
}
