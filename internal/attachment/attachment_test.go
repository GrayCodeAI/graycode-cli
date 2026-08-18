package attachment

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// webpFixture is a static 4x3 webp raster (x/image/webp has no encoder; this
// was produced with Pillow and is verified by the decoder in the test).
const webpFixtureB64 = "UklGRjYAAABXRUJQVlA4ICoAAACQAQCdASoEAAMAAUAmJaACdLoAA5gA/vIiX/gN18W0y/+N462+tvZQAAA="

// rasterFixture encodes a 4x3 gradient raster in the requested format.
func rasterFixture(t *testing.T, mt ImageMediaType) []byte {
	t.Helper()
	if mt == MediaTypeWebP {
		data, err := base64.StdEncoding.DecodeString(webpFixtureB64)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	img := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(20 * x), G: uint8(40 * y), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	switch mt {
	case MediaTypePNG:
		if err := png.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
	case MediaTypeJPEG:
		if err := jpeg.Encode(&buf, img, nil); err != nil {
			t.Fatal(err)
		}
	case MediaTypeGIF:
		if err := gif.Encode(&buf, img, nil); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unsupported fixture type %q", mt)
	}
	return buf.Bytes()
}

func newTestStore(t *testing.T) *FSStore {
	t.Helper()
	return NewFSStore(t.TempDir())
}

func TestSaveReadRoundtripAllMediaTypes(t *testing.T) {
	ctx := context.Background()
	for _, mt := range AllMediaTypes {
		t.Run(string(mt), func(t *testing.T) {
			s := newTestStore(t)
			data := rasterFixture(t, mt)

			ref, err := s.SaveImage(ctx, SaveImage{Data: data, MediaType: mt, Name: "photo.png"})
			if err != nil {
				t.Fatal(err)
			}
			if ref.AttachmentID == "" {
				t.Fatal("empty attachment id")
			}
			if ref.MediaType != mt {
				t.Fatalf("ref media type = %q, want %q", ref.MediaType, mt)
			}
			if ref.Bytes != len(data) {
				t.Fatalf("ref bytes = %d, want %d", ref.Bytes, len(data))
			}
			if ref.Width != 4 || ref.Height != 3 {
				t.Fatalf("ref dims = %dx%d, want 4x3", ref.Width, ref.Height)
			}
			if ref.Name != "photo.png" {
				t.Fatalf("ref name = %q", ref.Name)
			}

			stored, err := s.ReadImage(ctx, ref)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(stored.Data, data) {
				t.Fatal("read data differs from saved data")
			}
		})
	}
}

func TestContentAddressedIdentity(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	data := rasterFixture(t, MediaTypePNG)
	r1, err := s.SaveImage(ctx, SaveImage{Data: data, MediaType: MediaTypePNG})
	if err != nil {
		t.Fatal(err)
	}
	// The id is a deterministic digest of the immutable object bytes.
	if r1.AttachmentID != contentID(data) {
		t.Fatalf("id = %q, want content digest %q", r1.AttachmentID, contentID(data))
	}
}

func TestMediaTypeMismatchRejected(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	jpegData := rasterFixture(t, MediaTypeJPEG)

	_, err := s.SaveImage(ctx, SaveImage{Data: jpegData, MediaType: MediaTypePNG})
	if !IsAdmission(err) {
		t.Fatalf("mismatch must be an admission error, got %v", err)
	}
	if CodeOf(err) != CodeImageTypeMismatch {
		t.Fatalf("code = %q, want IMAGE_TYPE_MISMATCH", CodeOf(err))
	}
}

func TestUnsupportedTypeRejected(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	data := rasterFixture(t, MediaTypePNG)
	_, err := s.SaveImage(ctx, SaveImage{Data: data, MediaType: "image/bmp"})
	if CodeOf(err) != CodeUnsupportedType {
		t.Fatalf("code = %q, want UNSUPPORTED_IMAGE_TYPE", CodeOf(err))
	}
}

func TestEmptyAndTruncatedRejected(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if _, err := s.SaveImage(ctx, SaveImage{Data: nil, MediaType: MediaTypePNG}); CodeOf(err) != CodeInvalidImage {
		t.Fatalf("empty data code = %q, want INVALID_IMAGE", CodeOf(err))
	}
	trunc := rasterFixture(t, MediaTypePNG)[:10]
	if _, err := s.SaveImage(ctx, SaveImage{Data: trunc, MediaType: MediaTypePNG}); CodeOf(err) != CodeInvalidImage {
		t.Fatalf("truncated data code = %q, want INVALID_IMAGE", CodeOf(err))
	}
}

func TestByteAndPixelLimits(t *testing.T) {
	ctx := context.Background()
	limits := DefaultLimits()
	limits.MaxImageBytes = 10 // fixture is larger than 10 bytes
	s := NewFSStoreWithLimits(t.TempDir(), limits)
	data := rasterFixture(t, MediaTypePNG)
	if _, err := s.SaveImage(ctx, SaveImage{Data: data, MediaType: MediaTypePNG}); CodeOf(err) != CodeImageTooLarge {
		t.Fatalf("byte-limit code = %q, want IMAGE_TOO_LARGE", CodeOf(err))
	}

	limits = DefaultLimits()
	limits.MaxImagePixels = 4 // fixture is 12 pixels
	s = NewFSStoreWithLimits(t.TempDir(), limits)
	if _, err := s.SaveImage(ctx, SaveImage{Data: data, MediaType: MediaTypePNG}); CodeOf(err) != CodeImageTooManyPixels {
		t.Fatalf("pixel-limit code = %q, want IMAGE_TOO_MANY_PIXELS", CodeOf(err))
	}
}

func TestSaveImagesBatchAdmission(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	// Too many images.
	limits := s.limits
	limits.MaxImagesPerMessage = 1
	limited := NewFSStoreWithLimits(t.TempDir(), limits)
	data := rasterFixture(t, MediaTypePNG)
	if _, err := limited.SaveImages(ctx, []SaveImage{
		{Data: data, MediaType: MediaTypePNG},
		{Data: data, MediaType: MediaTypePNG},
	}); CodeOf(err) != CodeTooManyImages {
		t.Fatalf("batch count code = %q, want TOO_MANY_IMAGES", CodeOf(err))
	}

	// Aggregate too large.
	limits = s.limits
	limits.MaxMessageImageBytes = 10
	limited = NewFSStoreWithLimits(t.TempDir(), limits)
	if _, err := limited.SaveImages(ctx, []SaveImage{{Data: data, MediaType: MediaTypePNG}}); CodeOf(err) != CodeImagesTooLarge {
		t.Fatalf("batch size code = %q, want IMAGES_TOO_LARGE", CodeOf(err))
	}

	// Validation failure starts no writes: a bad member before a good one.
	bad := []SaveImage{
		{Data: data[:10], MediaType: MediaTypePNG}, // truncated
		{Data: data, MediaType: MediaTypePNG},
	}
	if _, err := s.SaveImages(ctx, bad); !IsAdmission(err) {
		t.Fatalf("batch with invalid member = %v, want admission error", err)
	}
	if got := len(s.ListIDs()); got != 0 {
		t.Fatalf("batch validation failure wrote %d objects, want 0 (no partial writes)", got)
	}
}

func TestDuplicateWriteRejected(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	data := rasterFixture(t, MediaTypePNG)
	if _, err := s.SaveImage(ctx, SaveImage{Data: data, MediaType: MediaTypePNG}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveImage(ctx, SaveImage{Data: data, MediaType: MediaTypePNG}); CodeOf(err) != CodeWriteFailed {
		t.Fatalf("duplicate code = %q, want ATTACHMENT_WRITE_FAILED", CodeOf(err))
	}
}

func TestReadMissingAndCorrupt(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	data := rasterFixture(t, MediaTypePNG)
	ref, err := s.SaveImage(ctx, SaveImage{Data: data, MediaType: MediaTypePNG})
	if err != nil {
		t.Fatal(err)
	}

	// Missing object.
	missing := ref
	missing.AttachmentID = ID("deadbeef")
	if _, err := s.ReadImage(ctx, missing); CodeOf(err) != CodeNotFound {
		t.Fatalf("missing code = %q, want ATTACHMENT_NOT_FOUND", CodeOf(err))
	}

	// Invalid reference shape.
	if _, err := s.ReadImage(ctx, Ref{AttachmentID: "", MediaType: MediaTypePNG, Bytes: 1}); CodeOf(err) != CodeInvalidRef {
		t.Fatalf("invalid ref code = %q, want INVALID_ATTACHMENT_REF", CodeOf(err))
	}

	// Corrupt on disk: flip a byte.
	path := filepath.Join(s.root, string(ref.AttachmentID))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw[0] ^= 0xFF
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadImage(ctx, ref); CodeOf(err) != CodeCorrupt {
		t.Fatalf("corrupt code = %q, want ATTACHMENT_CORRUPT", CodeOf(err))
	}
}

func TestAdmissionClassification(t *testing.T) {
	if !IsAdmission(&Error{Code: CodeImageTooManyPixels, Msg: "x"}) {
		t.Fatal("IMAGE_TOO_MANY_PIXELS must be admission")
	}
	if IsAdmission(&Error{Code: CodeNotFound, Msg: "x"}) {
		t.Fatal("ATTACHMENT_NOT_FOUND must not be admission")
	}
	if IsAdmission(errors.New("plain")) {
		t.Fatal("plain errors must not be admission")
	}
	if CodeOf(errors.New("plain")) != "" {
		t.Fatal("plain errors must have empty code")
	}
}

// ListIDs is a test helper exposing the objects a store has persisted.
func (s *FSStore) ListIDs() []ID {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil
	}
	out := make([]ID, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, ID(e.Name()))
		}
	}
	return out
}

var _ image.Image = image.NewNRGBA(image.Rect(0, 0, 1, 1))
