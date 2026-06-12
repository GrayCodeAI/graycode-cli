package env

import "os"

// Getenv returns the value of an environment variable.
// This is the centralized access point for all env var reads in internal/
// packages. Packages that cannot import internal/config due to import cycles
// use this package instead. The config package also delegates to this function.
//
// Using this function instead of os.Getenv makes env access grep-able and
// allows future migration to a more sophisticated env management layer.
func Getenv(key string) string {
	return os.Getenv(key)
}
