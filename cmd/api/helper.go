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
    "strconv"
    "errors"

//    "github.com/rs/zerolog" 
    "github.com/julienschmidt/httprouter" 
)

type envelope map[string]any

// NOTE: handler helpers should be methods
func (a *application) writeJSON(w http.ResponseWriter, status int, data envelope, headers http.Header) error { // Encode data to json
    // pass map to fn, returns []byte slice of encoded json
    js, err := json.MarshalIndent(data, "", "\t")
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

func (a *application) readIDParam(r *http.Request) (int64, error) {
    // get url params in slice
    params := httprouter.ParamsFromContext(r.Context())

    // convert string id to base 64 int
    id, err := strconv.ParseInt(params.ByName("id"), 10, 64)
    if err != nil || id < 1 {
        return 0, errors.New("invalid id parameter")
    }

    return id, nil
}
