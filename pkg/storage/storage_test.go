package storage

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
)

func jpegBytes(t *testing.T) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 8, 8))
	canvas.Set(1, 1, color.RGBA{R: 200, G: 40, B: 40, A: 255})
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, canvas, nil); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 8, 8))
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// gpsExifJPEG splices an APP1/Exif segment carrying a GPS tag into a real JPEG, the way a phone
// camera does.
func gpsExifJPEG(t *testing.T) ([]byte, []byte) {
	t.Helper()
	base := jpegBytes(t)
	marker := []byte("GPSLatitude 14.5995 GPSLongitude 120.9842")
	payload := append([]byte("Exif\x00\x00"), marker...)
	segment := make([]byte, 0, len(payload)+4)
	segment = append(segment, 0xFF, 0xE1)
	segment = binary.BigEndian.AppendUint16(segment, uint16(len(payload)+2))
	segment = append(segment, payload...)
	// After the SOI marker is where a camera puts it.
	out := append([]byte{}, base[:2]...)
	out = append(out, segment...)
	out = append(out, base[2:]...)
	return out, marker
}

// Test-plan item 2: what the bytes are is what counts, never the extension or declared type.
func TestDetectImageTypeAcceptsImagesAndRejectsEverythingElse(t *testing.T) {
	webp := append([]byte("RIFF"), 0x1A, 0x00, 0x00, 0x00)
	webp = append(webp, []byte("WEBPVP8 ")...)
	webp = append(webp, make([]byte, 16)...)

	for _, test := range []struct {
		name string
		data []byte
		want string
	}{
		{name: "jpeg", data: jpegBytes(t), want: "image/jpeg"},
		{name: "png", data: pngBytes(t), want: "image/png"},
		{name: "webp", data: webp, want: "image/webp"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := DetectImageType(test.data)
			if err != nil || got != test.want {
				t.Fatalf("DetectImageType() = %q, %v, want %q", got, err, test.want)
			}
		})
	}

	// An acceptance criterion: a .exe renamed .jpg must not get through.
	for _, test := range []struct {
		name string
		data []byte
	}{
		{name: "windows executable renamed to jpg", data: append([]byte("MZ\x90\x00\x03"), make([]byte, 64)...)},
		{name: "elf binary", data: append([]byte("\x7fELF\x02\x01\x01"), make([]byte, 64)...)},
		{name: "pdf", data: []byte("%PDF-1.7\n1 0 obj\n")},
		{name: "gif is an image but not one we take", data: []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00,")},
		{name: "html", data: []byte("<!DOCTYPE html><html><body>hi</body></html>")},
		{name: "empty", data: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DetectImageType(test.data); !apperror.Is(err, "UNSUPPORTED_MEDIA_TYPE") {
				t.Fatalf("DetectImageType(%s) error = %v, want UNSUPPORTED_MEDIA_TYPE", test.name, err)
			}
		})
	}
}

func TestAssertSizeEnforcesTheTenMegabyteCeiling(t *testing.T) {
	if err := AssertSize(MaxImageBytes); err != nil {
		t.Fatalf("a file exactly at the limit must be accepted: %v", err)
	}
	err := AssertSize(MaxImageBytes + 1)
	if !apperror.Is(err, "FILE_TOO_LARGE") {
		t.Fatalf("AssertSize(over) = %v, want FILE_TOO_LARGE", err)
	}
	// The message has to name the limit, or the user cannot act on it.
	if message := err.(*apperror.Error).Message; !strings.Contains(message, "10.0 MB") {
		t.Fatalf("message = %q, want it to state the limit", message)
	}
	if err = AssertSize(0); !apperror.Is(err, "VALIDATION_FAILED") {
		t.Fatalf("AssertSize(0) = %v, want VALIDATION_FAILED", err)
	}
}

// Test-plan item 3: the GPS coordinates a phone embeds must not reach storage.
func TestStripMetadataRemovesGPSAndKeepsTheImageDecodable(t *testing.T) {
	withGPS, marker := gpsExifJPEG(t)
	if !bytes.Contains(withGPS, marker) {
		t.Fatal("the fixture does not actually carry the GPS tag")
	}
	stripped := StripMetadata(withGPS)
	if bytes.Contains(stripped, marker) {
		t.Fatal("GPS coordinates survived stripping")
	}
	if bytes.Contains(stripped, []byte("Exif\x00\x00")) {
		t.Fatal("the EXIF header survived stripping")
	}
	if _, err := jpeg.Decode(bytes.NewReader(stripped)); err != nil {
		t.Fatalf("the stripped JPEG no longer decodes: %v", err)
	}
	if _, err := DetectImageType(stripped); err != nil {
		t.Fatalf("the stripped JPEG is no longer recognised: %v", err)
	}
}

func TestStripMetadataRemovesPNGTextChunksAndKeepsThePicture(t *testing.T) {
	base := pngBytes(t)
	marker := []byte("GPSLatitude")
	chunk := make([]byte, 0, len(marker)+12)
	chunk = binary.BigEndian.AppendUint32(chunk, uint32(len(marker)))
	chunk = append(chunk, []byte("eXIf")...)
	chunk = append(chunk, marker...)
	chunk = binary.BigEndian.AppendUint32(chunk, 0) // CRC is not validated by the stripper.
	// Splice it in after the 8-byte signature and the IHDR chunk.
	const signature = 8
	ihdrLen := int(binary.BigEndian.Uint32(base[signature:signature+4])) + 12
	withExif := append([]byte{}, base[:signature+ihdrLen]...)
	withExif = append(withExif, chunk...)
	withExif = append(withExif, base[signature+ihdrLen:]...)

	stripped := StripMetadata(withExif)
	if bytes.Contains(stripped, marker) {
		t.Fatal("the eXIf chunk survived stripping")
	}
	if _, err := png.Decode(bytes.NewReader(stripped)); err != nil {
		t.Fatalf("the stripped PNG no longer decodes: %v", err)
	}
}

func TestStripMetadataRemovesWebPExifAndFixesTheRIFFLength(t *testing.T) {
	payload := []byte("GPSLatitude 14.5995")
	body := []byte("VP8 ")
	body = binary.LittleEndian.AppendUint32(body, 8)
	body = append(body, make([]byte, 8)...)
	exif := []byte("EXIF")
	exif = binary.LittleEndian.AppendUint32(exif, uint32(len(payload)))
	exif = append(exif, payload...)
	if len(payload)%2 == 1 {
		exif = append(exif, 0)
	}
	file := append([]byte("RIFF"), 0, 0, 0, 0)
	file = append(file, []byte("WEBP")...)
	file = append(file, body...)
	file = append(file, exif...)
	binary.LittleEndian.PutUint32(file[4:8], uint32(len(file)-8))

	stripped := StripMetadata(file)
	if bytes.Contains(stripped, payload) {
		t.Fatal("the EXIF chunk survived stripping")
	}
	if declared := binary.LittleEndian.Uint32(stripped[4:8]); int(declared) != len(stripped)-8 {
		t.Fatalf("RIFF length = %d, want %d — a stale length corrupts the file", declared, len(stripped)-8)
	}
	if _, err := DetectImageType(stripped); err != nil {
		t.Fatalf("the stripped WebP is no longer recognised: %v", err)
	}
}

func TestKeyGroupsImagesByTripAndUsesTheSniffedExtension(t *testing.T) {
	tripID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	receiptID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	got := Key(tripID, receiptID, "image/png")
	want := "trips/11111111-1111-1111-1111-111111111111/receipts/22222222-2222-2222-2222-222222222222.png"
	if got != want {
		t.Fatalf("Key() = %q, want %q", got, want)
	}
}

func TestFilesystemRoundTripsAndExpiresItsLinks(t *testing.T) {
	store := NewFilesystem(t.TempDir(), "http://localhost:8080/dev-storage", []byte("test-secret"))
	ctx := context.Background()
	data := jpegBytes(t)
	key := Key(uuid.New(), uuid.New(), "image/jpeg")
	if err := store.Put(ctx, key, bytes.NewReader(data), int64(len(data)), "image/jpeg"); err != nil {
		t.Fatal(err)
	}

	link, err := store.PresignedGet(ctx, key, PresignTTL)
	if err != nil {
		t.Fatal(err)
	}
	signature, expires := parseLink(t, link)
	reader, err := store.Open(key, signature, expires)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || !bytes.Equal(stored, data) {
		t.Fatalf("stored bytes differ from what was written (err %v)", err)
	}

	// A tampered signature is refused...
	if _, err = store.Open(key, "not-the-signature", expires); !apperror.Is(err, "FORBIDDEN") {
		t.Fatalf("Open(bad signature) = %v, want FORBIDDEN", err)
	}
	// ...and so is a link whose clock has run out.
	store.now = func() time.Time { return time.Now().Add(2 * PresignTTL) }
	if _, err = store.Open(key, signature, expires); !apperror.Is(err, "FORBIDDEN") {
		t.Fatalf("Open(expired) = %v, want FORBIDDEN", err)
	}
	store.now = time.Now

	if err = store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Open(key, signature, expires); !apperror.Is(err, "RECEIPT_NOT_FOUND") {
		t.Fatalf("Open(deleted) = %v, want RECEIPT_NOT_FOUND", err)
	}
	// Deleting something already gone is not an error — the caller's intent is satisfied.
	if err = store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete(missing) = %v", err)
	}
}

// A key that tries to climb out of the storage root must stay inside it.
func TestFilesystemKeepsCraftedKeysInsideTheRoot(t *testing.T) {
	root := t.TempDir()
	store := NewFilesystem(root, "http://localhost", []byte("secret"))
	if err := store.Put(context.Background(), "../../escaped.txt", strings.NewReader("nope"), 4, "text/plain"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open("../../escaped.txt", store.sign("../../escaped.txt", time.Now().Add(time.Minute).Unix()), time.Now().Add(time.Minute).Unix()); err != nil {
		t.Fatalf("the file should be readable from inside the root: %v", err)
	}
	if !strings.HasPrefix(store.path("../../escaped.txt"), root) {
		t.Fatalf("path() escaped the root: %s", store.path("../../escaped.txt"))
	}
}

func parseLink(t *testing.T, link string) (string, int64) {
	t.Helper()
	_, query, ok := strings.Cut(link, "?")
	if !ok {
		t.Fatalf("link has no query: %s", link)
	}
	var signature string
	var expires int64
	for _, pair := range strings.Split(query, "&") {
		key, value, _ := strings.Cut(pair, "=")
		switch key {
		case "signature":
			signature = value
		case "expires":
			var parsed int64
			for _, digit := range value {
				parsed = parsed*10 + int64(digit-'0')
			}
			expires = parsed
		}
	}
	return signature, expires
}
