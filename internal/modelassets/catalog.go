// Package modelassets installs application-approved, pinned model files. The
// production catalog is intentionally empty pending separately reviewed manifests.
// Catalog maintenance must pin immutable upstream revisions, exact byte sizes and
// SHA-256 digests; callers cannot supply manifests, URLs or install destinations.
//
// The caller owns the root and must keep it open until all operations return.
// Only Lookup establishes installation validity; directory existence does not.
// After a process interruption, remove stale .install.lock, .stage-* directories
// and incomplete revisions manually, only while no installer is running. There
// is no automatic repair or lock takeover. Earlier revisions are retained.
package modelassets

import (
	"encoding/hex"
	"errors"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ModelID selects an entry from the application-approved catalog.
type ModelID string

// Installation describes a verified revision. Directory is relative to the root.
type Installation struct {
	ModelID   ModelID
	Revision  string
	Directory string
}

var (
	ErrUnknownModel   = errors.New("unknown model")
	ErrInvalidCatalog = errors.New("invalid model catalog")
	ErrBusy           = errors.New("model installation busy")
	ErrNotInstalled   = errors.New("model installation missing or incomplete")
	ErrDownload       = errors.New("model download failed")
	ErrIntegrity      = errors.New("model integrity verification failed")
	ErrFilesystem     = errors.New("model filesystem operation failed")
)

// operationError prints only a fixed category while retaining causes for errors.Is.
type operationError struct{ category, cause error }

func (e *operationError) Error() string   { return e.category.Error() }
func (e *operationError) Unwrap() []error { return []error{e.category, e.cause} }
func failure(category, cause error) error {
	if cause == nil {
		return category
	}
	return &operationError{category: category, cause: cause}
}

type asset struct {
	URL    string
	Path   string
	Size   int64
	SHA256 string
}
type entry struct {
	ID          ModelID
	Kind        string
	Runtime     string
	Revision    string
	Files       []asset
	Attribution string
}

// Installer shares no mutable operation state; its methods may be called concurrently.
type Installer struct {
	root        *os.Root
	catalog     map[ModelID]entry
	client      *http.Client
	idleTimeout time.Duration
	fileTimeout time.Duration
	// Narrow fault-injection seams for resource closure and publication tests.
	closeFile func(*os.File) error
	rename    func(string, string) error
	complete  func(string) error
}

// NewInstaller uses a dedicated credential-free client and the pinned catalog.
func NewInstaller(root *os.Root) (*Installer, error) {
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		DisableCompression:    true,
		// No idle pooled connections outlive individual operations.
		DisableKeepAlives: true,
	}
	client := &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return newInstaller(root, nil, client)
}

func newInstaller(root *os.Root, entries []entry, client *http.Client) (*Installer, error) {
	if root == nil {
		return nil, ErrFilesystem
	}
	catalog := make(map[ModelID]entry, len(entries))
	for _, e := range entries {
		if !identifier(string(e.ID)) || !identifier(e.Revision) || !identifier(e.Kind) || !identifier(e.Runtime) || len(e.Files) == 0 {
			return nil, ErrInvalidCatalog
		}
		if _, exists := catalog[e.ID]; exists {
			return nil, ErrInvalidCatalog
		}
		for n, f := range e.Files {
			u, err := url.Parse(f.URL)
			digest, digestErr := hex.DecodeString(f.SHA256)
			if err != nil {
				return nil, ErrInvalidCatalog
			}
			if u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
				return nil, ErrInvalidCatalog
			}
			if !relativePath(f.Path) || f.Size <= 0 || f.Size == math.MaxInt64 || digestErr != nil || len(digest) != 32 {
				return nil, ErrInvalidCatalog
			}
			for _, prior := range e.Files[:n] {
				a, b := strings.ToLower(f.Path), strings.ToLower(prior.Path)
				if a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/") {
					return nil, ErrInvalidCatalog
				}
			}
		}
		e.Files = append([]asset(nil), e.Files...)
		for n := range e.Files {
			e.Files[n].SHA256 = strings.ToLower(e.Files[n].SHA256)
		}
		catalog[e.ID] = e
	}
	return &Installer{
		root:        root,
		catalog:     catalog,
		client:      client,
		idleTimeout: 30 * time.Second,
		fileTimeout: 30 * time.Minute,
		closeFile:   func(f *os.File) error { return f.Close() },
		rename:      root.Rename,
		complete:    func(name string) error { return root.Mkdir(name, 0700) },
	}, nil
}

// Portable names avoid case aliases, Windows device names and special syntax.
func identifier(s string) bool {
	if s == "" || !(s[0] >= 'a' && s[0] <= 'z' || s[0] >= '0' && s[0] <= '9') || strings.HasSuffix(s, ".") {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	base := strings.ToUpper(strings.SplitN(s, ".", 2)[0])
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return false
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9' {
		return false
	}
	return true
}
func relativePath(s string) bool {
	for _, part := range strings.Split(s, "/") {
		if !identifier(part) || part == ".complete" {
			return false
		}
	}
	return true
}
