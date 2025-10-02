// helpers.go
package main

import (
    "fmt"
//    "os"
    "net/http"
    "errors"
    "strings"
    "io"
    "encoding/json"
    "strconv"

//    "github.com/rs/zerolog" 
    "github.com/julienschmidt/httprouter" 
)

type envelope map[string]any

// NOTE: handler helpers should be methods
// Encode data to json
func (a *application) writeJSON(w http.ResponseWriter, status int, data envelope, headers http.Header) error { 
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

func (a *application) readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
    // limit request body to 1MB
    maxBytes := 1_048_576
    r.Body = http.MaxBytesReader(w, r.Body, int64(maxBytes))

    // init decoder
    dec := json.NewDecoder(r.Body)
    dec.DisallowUnknownFields()
    // decode body
    err := dec.Decode(dst)
    if err != nil {
        var syntaxError *json.SyntaxError
        var unmarshalTypeError *json.UnmarshalTypeError
        var invalidUnmarshalError *json.InvalidUnmarshalError

        switch {
        // check err type
        case errors.As(err, &syntaxError):
            return fmt.Errorf("body contains badly-formed JSON (at character %d)", syntaxError.Offset)

        case errors.Is(err, io.ErrUnexpectedEOF):
            return errors.New("body contains badly-formed JSON")

        case errors.As(err, &unmarshalTypeError):
            if unmarshalTypeError.Field != "" {
                return fmt.Errorf("body contains incorrect JSON type for field %q", unmarshalTypeError.Field) 
            }
            return fmt.Errorf("body contains incorrect JSON type (at character %d)", unmarshalTypeError.Offset) 

        // return plain english err
        case errors.Is(err, io.EOF):
            return errors.New("body must not be empty")

        case strings.HasPrefix(err.Error(), "json: unknown field "):
            // NOTE: include the trailing space so TrimPrefix works
            fieldName := strings.TrimPrefix(err.Error(), "json: unknown field ")
            return fmt.Errorf("body contains unknown key %s", fieldName) 

        case err.Error() == "http: request body too large":
            return fmt.Errorf("body must not be larger than %d bytes", maxBytes)

        // pass non-nil pointer to Decode()
        case errors.As(err, &invalidUnmarshalError):
            panic(err) 
            
        default:
            return err
        }
    }

    err = dec.Decode(&struct{}{})
    if err != io.EOF {
        return errors.New("body must only contain a single JSON value")
    }

    return nil
}


