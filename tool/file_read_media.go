package tool

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"path/filepath"
	"strings"
)



var imageExtensions = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
}

// isImageFile checks if a file path has an image extension.
func isImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	_, ok := imageExtensions[ext]
	return ok
}

// getImageDimensions returns width and height of an image.
func getImageDimensions(data []byte) (int, int) {
	cfg, _, err := image.DecodeConfig(strings.NewReader(string(data)))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}


