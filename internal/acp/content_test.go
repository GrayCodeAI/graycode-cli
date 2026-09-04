package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/attachment"
)

// pngFixture encodes a 4x3 NRGBA raster as PNG bytes.
func pngFixture(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(20 * x), G: uint8(40 * y), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func canonicalB64(t *testing.T, data []byte) string {
	t.Helper()
	return base64.StdEncoding.EncodeToString(data)
}

func newTestStore(t *testing.T) *attachment.FSStore {
	t.Helper()
	return attachment.NewFSStore(t.TempDir())
}

func textBlock(s string) AcpContentBlock {
	return AcpContentBlock{Type: "text", Text: s}
}

func imageBlock(t *testing.T, mt attachment.ImageMediaType) AcpContentBlock {
	t.Helper()
	return AcpContentBlock{Type: "image", MimeType: string(mt), Data: canonicalB64(t, pngFixture(t))}
}

func TestAdmitAcpPrompt_TextOnly(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	content, err := AdmitAcpPrompt(ctx, store, []AcpContentBlock{
		textBlock("hello "),
		textBlock("world"),
	}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []ContentBlock{{Type: "text", Text: "hello world"}}
	if !reflect.DeepEqual(content, want) {
		t.Fatalf("content = %#v, want %#v", content, want)
	}
}

func TestAdmitAcpPrompt_ResourceLink(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	content, err := AdmitAcpPrompt(ctx, store, []AcpContentBlock{
		textBlock("see: "),
		{Type: "resource_link", Name: "doc", URI: "file:///tmp/a.md"},
	}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) != 1 || content[0].Type != "text" {
		t.Fatalf("content = %#v", content)
	}
	if content[0].Text != "see: \n[resource_link name=\"doc\" uri=\"file:///tmp/a.md\"]\n" {
		t.Fatalf("text = %q", content[0].Text)
	}
}

func TestAdmitAcpPrompt_ImageEnabled(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	content, err := AdmitAcpPrompt(ctx, store, []AcpContentBlock{
		textBlock("pic: "),
		imageBlock(t, attachment.MediaTypePNG),
		textBlock(" done"),
	}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) != 3 {
		t.Fatalf("content = %#v", content)
	}
	if content[0].Type != "text" || content[0].Text != "pic: " {
		t.Fatalf("first block = %#v", content[0])
	}
	if content[1].Type != "image" || content[1].Attachment == nil {
		t.Fatalf("image block = %#v", content[1])
	}
	if content[2].Type != "text" || content[2].Text != " done" {
		t.Fatalf("last block = %#v", content[2])
	}
	// Verify the attachment is durable and readable.
	stored, err := store.ReadImage(ctx, *content[1].Attachment)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored.Data, pngFixture(t)) {
		t.Fatalf("round-tripped bytes differ")
	}
}

func TestAdmitAcpPrompt_ImageDisabledRejected(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	_, err := AdmitAcpPrompt(ctx, store, []AcpContentBlock{
		imageBlock(t, attachment.MediaTypePNG),
	}, false, nil)
	ce, ok := asContentError(err)
	if !ok || ce.Kind != FailureInvalid {
		t.Fatalf("err = %v, want invalid ContentError", err)
	}
}

func TestAdmitAcpPrompt_AudioAndResourceRejected(t *testing.T) {
	ctx := context.Background()
	for _, typ := range []string{"audio", "resource"} {
		_, err := AdmitAcpPrompt(ctx, nil, []AcpContentBlock{{Type: typ}}, true, nil)
		ce, ok := asContentError(err)
		if !ok || ce.Kind != FailureInvalid {
			t.Fatalf("type %q: err = %v, want invalid ContentError", typ, err)
		}
	}
}

func TestAdmitAcpPrompt_EmptyPromptRejected(t *testing.T) {
	_, err := AdmitAcpPrompt(context.Background(), newTestStore(t), []AcpContentBlock{
		textBlock("   "),
	}, false, nil)
	ce, ok := asContentError(err)
	if !ok || ce.Kind != FailureInvalid {
		t.Fatalf("err = %v, want invalid ContentError", err)
	}
}

func TestAdmitAcpPrompt_UnsupportedType(t *testing.T) {
	_, err := AdmitAcpPrompt(context.Background(), nil, []AcpContentBlock{{Type: "embedding"}}, false, nil)
	ce, ok := asContentError(err)
	if !ok || ce.Kind != FailureInvalid {
		t.Fatalf("err = %v, want invalid ContentError", err)
	}
}

func TestAdmitAcpPrompt_ImageMimeTypeRejected(t *testing.T) {
	_, err := AdmitAcpPrompt(context.Background(), newTestStore(t), []AcpContentBlock{
		{Type: "image", MimeType: "image/bmp", Data: canonicalB64(t, pngFixture(t))},
	}, true, nil)
	ce, ok := asContentError(err)
	if !ok || ce.Kind != FailureInvalid {
		t.Fatalf("err = %v, want invalid ContentError", err)
	}
}

func TestAdmitAcpPrompt_NonCanonicalBase64Rejected(t *testing.T) {
	// URL-safe base64 is not canonical: substitute '-' for '+' so the
	// regexp rejects it.
	data := pngFixture(t)
	urlSafe := base64.RawURLEncoding.EncodeToString(data)
	_, err := AdmitAcpPrompt(context.Background(), newTestStore(t), []AcpContentBlock{
		{Type: "image", MimeType: "image/png", Data: urlSafe},
	}, true, nil)
	_ = data
	ce, ok := asContentError(err)
	if !ok || ce.Kind != FailureInvalid {
		t.Fatalf("err = %v, want invalid ContentError", err)
	}
}

func TestAdmitAcpPrompt_NoStoreForImageRejected(t *testing.T) {
	_, err := AdmitAcpPrompt(context.Background(), nil, []AcpContentBlock{
		imageBlock(t, attachment.MediaTypePNG),
	}, true, nil)
	ce, ok := asContentError(err)
	if !ok || ce.Kind != FailureInvalid {
		t.Fatalf("err = %v, want invalid ContentError", err)
	}
}

func TestAssistantBlockToAcp_Text(t *testing.T) {
	out, err := AssistantBlockToAcp(context.Background(), newTestStore(t), ContentBlock{Type: "text", Text: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || out.Type != "text" || out.Text != "hi" {
		t.Fatalf("out = %#v", out)
	}
	empty, err := AssistantBlockToAcp(context.Background(), nil, ContentBlock{Type: "text", Text: ""})
	if err != nil {
		t.Fatal(err)
	}
	if empty != nil {
		t.Fatalf("empty text should project to nil, got %#v", empty)
	}
}

func TestAssistantBlockToAcp_Image(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	content, err := AdmitAcpPrompt(ctx, store, []AcpContentBlock{
		imageBlock(t, attachment.MediaTypePNG),
	}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := AssistantBlockToAcp(ctx, store, content[0])
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || out.Type != "image" {
		t.Fatalf("out = %#v", out)
	}
	if out.MimeType != "image/png" {
		t.Fatalf("mimeType = %q", out.MimeType)
	}
	decoded, err := base64.StdEncoding.DecodeString(out.Data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, pngFixture(t)) {
		t.Fatalf("re-projected bytes differ")
	}
}

func TestAssistantBlockToAcp_UnsupportedBlock(t *testing.T) {
	// Non-output blocks (and image without a ref) project to nil.
	if out, err := AssistantBlockToAcp(context.Background(), newTestStore(t), ContentBlock{Type: "tool_use"}); err != nil || out != nil {
		t.Fatalf("tool_use -> %#v, %v", out, err)
	}
	if out, err := AssistantBlockToAcp(context.Background(), nil, ContentBlock{Type: "image"}); err != nil || out != nil {
		t.Fatalf("image without ref -> %#v, %v", out, err)
	}
}

func TestSupportsAcpImagePrompts(t *testing.T) {
	store := newTestStore(t)
	if !SupportsAcpImagePrompts(store, true) {
		t.Fatalf("expected true for mounted store with model support")
	}
	if SupportsAcpImagePrompts(store, false) {
		t.Fatalf("expected false when model lacks image support")
	}
	if SupportsAcpImagePrompts(nil, true) {
		t.Fatalf("expected false for nil store")
	}
	// A store whose limits admit only non-raster media types reports false.
	stores := readOnlyStoreLackingRaster(t)
	if SupportsAcpImagePrompts(stores, true) {
		t.Fatalf("expected false when store admits no raster media types")
	}
}

// readOnlyStoreLackingRaster returns a Store with media types excluding the
// ACP raster vocabulary, for capability gate testing.
func readOnlyStoreLackingRaster(t *testing.T) *attachment.FSStore {
	t.Helper()
	root := t.TempDir()
	// Persist one file so the store has content; limits override media types.
	s := attachment.NewFSStore(root)
	_, err := s.SaveImage(context.Background(), attachment.SaveImage{Data: pngFixture(t), MediaType: attachment.MediaTypePNG})
	if err != nil {
		t.Fatal(err)
	}
	return attachment.NewFSStoreWithLimits(root, attachment.Limits{
		MaxImageBytes:        10 << 20,
		MaxImagesPerMessage:  4,
		MaxMessageImageBytes: 20 << 20,
		MaxImagePixels:       16_000_000,
		MediaTypes:           []attachment.ImageMediaType{"image/tiff"},
	})
}

// runServerWith drives Serve for a caller-configured server (with a mounted
// store) over canned input lines and returns every output message.
func runServerWith(t *testing.T, srv *Server, inputLines []string) []rpcMessage {
	t.Helper()
	in := strings.NewReader(strings.Join(inputLines, "\n") + "\n")
	pr, pw := io.Pipe()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = srv.Serve(ctx, in, pw)
		_ = pw.Close()
	}()

	var msgs []rpcMessage
	scanner := bufio.NewScanner(pr)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var m rpcMessage
		if err := json.Unmarshal(line, &m); err == nil {
			msgs = append(msgs, m)
		}
	}
	wg.Wait()
	return msgs
}

// TestServer_ImageCapabilityAdvertisement verifies initialize advertises the
// runtime image capability derived from the mounted store + model gate, and
// that the initialize response carries the truth.
func TestServer_ImageCapabilityAdvertisement(t *testing.T) {
	store := newTestStore(t)
	srv := NewServer(testFactory)
	srv.SetAttachmentStore(store, true)
	if !srv.imageCapable {
		t.Fatalf("expected imageCapable true with mounted store + model support")
	}
	srv.SetAttachmentStore(store, false)
	if srv.imageCapable {
		t.Fatalf("expected imageCapable false when model lacks image support")
	}
	srv.SetAttachmentStore(nil, true)
	if srv.imageCapable {
		t.Fatalf("expected imageCapable false when store is nil")
	}

	// Truthful advertisement in the initialize response.
	srv2 := NewServer(testFactory)
	srv2.SetAttachmentStore(newTestStore(t), true)
	msgs := runServerWith(t, srv2, []string{`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}`})
	if len(msgs) != 1 {
		t.Fatalf("expected 1 initialize response, got %d", len(msgs))
	}
	var initResp struct {
		AgentCapabilities struct {
			PromptCapabilities struct {
				Image bool `json:"image"`
			} `json:"promptCapabilities"`
		} `json:"agentCapabilities"`
	}
	if err := json.Unmarshal(msgs[0].Result, &initResp); err != nil {
		t.Fatal(err)
	}
	if !initResp.AgentCapabilities.PromptCapabilities.Image {
		t.Fatalf("expected image capability advertised true, got %+v", initResp)
	}
}

// TestServer_AdmitInlineImage verifies a mounted server admits an inline image
// prompt end-to-end: the prompt streams and completes, and the image is
// durably committed by admission.
func TestServer_AdmitInlineImage(t *testing.T) {
	store := newTestStore(t)
	srv := NewServer(testFactory)
	srv.SetAttachmentStore(store, true)

	img := imageBlock(t, attachment.MediaTypePNG)
	imgRaw, _ := json.Marshal(img)
	lines := []string{
		`{"jsonrpc":"2.0","id":1,"method":"session/new","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":{"sessionId":"sess_1","prompt":[{"type":"text","text":"describe: "},` + string(imgRaw) + `]}}`,
	}
	msgs := runServerWith(t, srv, lines)

	var gotUpdate, gotPromptResult bool
	for _, m := range msgs {
		switch {
		case m.Method == "session/update":
			gotUpdate = true
		case hasID(m, 2):
			gotPromptResult = true
			if m.Error != nil {
				t.Fatalf("prompt errored: %+v", m.Error)
			}
		}
	}
	if !gotUpdate || !gotPromptResult {
		t.Fatalf("missing responses: update=%v promptResult=%v (msgs=%d)", gotUpdate, gotPromptResult, len(msgs))
	}
}

// TestServer_RejectInlineImageWhenNotAdvertised verifies a server with no
// mounted store refuses image prompts as invalid params (they were never
// advertised).
func TestServer_RejectInlineImageWhenNotAdvertised(t *testing.T) {
	srv := NewServer(testFactory) // no store, imageCapable false
	img := imageBlock(t, attachment.MediaTypePNG)
	imgRaw, _ := json.Marshal(img)
	msgs := runServerWith(t, srv, []string{
		`{"jsonrpc":"2.0","id":1,"method":"session/new","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":{"sessionId":"sess_1","prompt":[` + string(imgRaw) + `]}}`,
	})
	found := false
	for _, m := range msgs {
		if hasID(m, 2) {
			found = true
			if m.Error == nil || m.Error.Code != errCodeInvalidParams {
				t.Fatalf("expected invalid-params error for unadvertised image, got %+v", m)
			}
		}
	}
	if !found {
		t.Fatalf("no prompt response; msgs=%+v", msgs)
	}
}
