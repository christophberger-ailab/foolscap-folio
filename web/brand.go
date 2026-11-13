package web

import (
	"path/filepath"

	"github.com/christophberger-ailab/foolscap-folio/internal/build"
)

// UserBrand is the path of the user's brand file: _brand.yml beside the
// settings file. It styles the projects that have no brand file of their
// own. "" when there is no config directory.
func UserBrand() string { return userBrandIn(defaultConfigFile()) }

func userBrandIn(configFile string) string {
	if configFile == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(configFile), build.BrandFile)
}
