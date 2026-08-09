// Package storage keeps receipt images out of the database and behind short-lived links.
//
// The bucket is private: nothing here ever returns a durable public URL, only presigned reads that
// expire (ST-2). Callers hand over bytes and get back a key; the key is the only thing worth
// persisting.
package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
)

// ObjectStorage is the port the receipt domain depends on. Both the S3/MinIO adapter and the
// filesystem adapter used in dev and tests satisfy it.
type ObjectStorage interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	// Get reads an object back. Extraction needs the bytes server-side; presigned links are for
	// browsers, not for the service talking to itself.
	Get(ctx context.Context, key string) ([]byte, error)
	PresignedGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	Delete(ctx context.Context, key string) error
}

const (
	// MaxImageBytes is ST-4. The reference checked this in the browser only; it is enforced here.
	MaxImageBytes int64 = 10 << 20
	// PresignTTL is ST-2: a read link lives five minutes, then the client asks for another.
	PresignTTL = 5 * time.Minute
	// RetentionDays is ST-6. The extracted line items outlive the photograph.
	RetentionDays = 90
)

// allowedTypes is ST-3. The value is what the bytes actually are, never what the upload claimed.
var allowedTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// Key is ST-1. Grouping by trip keeps a trip's images deletable as a prefix.
func Key(tripID, receiptID uuid.UUID, contentType string) string {
	return path.Join("trips", tripID.String(), "receipts", receiptID.String()+extensionFor(contentType))
}

func extensionFor(contentType string) string {
	if ext, ok := allowedTypes[contentType]; ok {
		return ext
	}
	return ".bin"
}

// DetectImageType sniffs the real media type from the leading bytes and rejects anything that is
// not an image we accept — a renamed executable never gets past this, whatever its extension or
// declared Content-Type said (ST-3).
func DetectImageType(data []byte) (string, error) {
	detected := http.DetectContentType(data)
	if _, ok := allowedTypes[detected]; !ok {
		return "", apperror.Newf("UNSUPPORTED_MEDIA_TYPE", "%s is not an accepted image type; upload a JPEG, PNG, or WebP", detected)
	}
	return detected, nil
}

// AssertSize is ST-4, phrased so the client can show the limit it broke.
func AssertSize(size int64) error {
	if size <= 0 {
		return apperror.Newf("VALIDATION_FAILED", "the uploaded file is empty")
	}
	if size > MaxImageBytes {
		return apperror.Newf("FILE_TOO_LARGE", "the image is %s; the limit is %s", humanBytes(size), humanBytes(MaxImageBytes))
	}
	return nil
}

func humanBytes(value int64) string {
	const unit = 1 << 20
	if value < unit {
		return fmt.Sprintf("%d KB", value>>10)
	}
	return fmt.Sprintf("%.1f MB", float64(value)/float64(unit))
}
