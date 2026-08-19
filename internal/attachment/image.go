package attachment

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"  // register gif for image.DecodeConfig / image.Decode
	_ "image/jpeg" // register jpeg
	_ "image/png"  // register png

	"golang.org/x/image/webp"
)

func init() {
	// The stdlib registers png/jpeg/gif via blank imports; webp is not
	// self-registering, so register it here so image.DecodeConfig and
	// image.Decode recognise it.
	image.RegisterFormat("webp", "RIFF????WEBP", webp.Decode, webp.DecodeConfig)
}

// formatFor maps a media type to the decoder format name returned by
// image.DecodeConfig, or "" for unsupported types.
func formatFor(mt ImageMediaType) string {
	switch mt {
	case MediaTypePNG:
		return "png"
	case MediaTypeJPEG:
		return "jpeg"
	case MediaTypeWebP:
		return "webp"
	case MediaTypeGIF:
		return "gif"
	default:
		return ""
	}
}

// decodeInfo fully decodes one image and returns its intrinsic dimensions and
// decoder format name.
func decodeInfo(data []byte) (width, height int, format string, err error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, "", err
	}
	// Full decode verifies the raster is not just header-parseable.
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return 0, 0, "", err
	}
	return cfg.Width, cfg.Height, format, nil
}

// validateImage applies the deployment policy to one image without persisting
// it: supported media type, byte cap, decodable raster, pixel cap, and
// declared-vs-decoded media type agreement.
func validateImage(limits Limits, input SaveImage) error {
	declared := formatFor(input.MediaType)
	if declared == "" {
		return &Error{Code: CodeUnsupportedType, Msg: fmt.Sprintf("image type %q is not accepted by this deployment", input.MediaType)}
	}
	if len(input.Data) == 0 {
		return &Error{Code: CodeInvalidImage, Msg: "image data is empty"}
	}
	if len(input.Data) > limits.MaxImageBytes {
		return &Error{Code: CodeImageTooLarge, Msg: fmt.Sprintf("image exceeds the configured byte limit (%d > %d)", len(input.Data), limits.MaxImageBytes)}
	}

	width, height, format, err := decodeInfo(input.Data)
	if err != nil {
		return &Error{Code: CodeInvalidImage, Msg: "image does not decode as a raster", Err: err}
	}
	if format != declared {
		return &Error{Code: CodeImageTypeMismatch, Msg: fmt.Sprintf("declared %s but decoded as %s", input.MediaType, format)}
	}
	if width*height > limits.MaxImagePixels {
		return &Error{Code: CodeImageTooManyPixels, Msg: fmt.Sprintf("image has %d pixels, exceeding the limit of %d", width*height, limits.MaxImagePixels)}
	}
	return nil
}
