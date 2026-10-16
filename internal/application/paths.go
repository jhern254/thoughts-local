package application

import "path/filepath"

// DataDirectory keeps personal assets independent of source checkouts and app
// bundles. Models and imported backgrounds share this visible application folder.
func DataDirectory(homeDirectory string) string {
	return filepath.Join(homeDirectory, "Documents", "Thoughts")
}
