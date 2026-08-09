package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jblabs/tripmate-be/pkg/apperror"
)

// Filesystem stores objects under a directory. It exists so the whole receipt flow runs on a
// laptop and in tests with no bucket, no credentials, and no network.
//
// It still honours ST-2: reads go through a link that carries an expiry and an HMAC, so a leaked
// URL stops working, exactly as a real presigned URL does.
type Filesystem struct {
	root    string
	baseURL string
	secret  []byte
	now     func() time.Time
}

func NewFilesystem(root, baseURL string, secret []byte) *Filesystem {
	return &Filesystem{root: root, baseURL: strings.TrimSuffix(baseURL, "/"), secret: secret, now: time.Now}
}

func (f *Filesystem) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	target := f.path(key)
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	defer file.Close()
	if _, err = io.Copy(file, r); err != nil {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return nil
}

func (f *Filesystem) Get(_ context.Context, key string) ([]byte, error) {
	data, err := os.ReadFile(f.path(key))
	if err != nil {
		return nil, apperror.New("RECEIPT_NOT_FOUND")
	}
	return data, nil
}

func (f *Filesystem) PresignedGet(_ context.Context, key string, ttl time.Duration) (string, error) {
	expires := f.now().Add(ttl).Unix()
	return fmt.Sprintf("%s/%s?expires=%d&signature=%s", f.baseURL, key, expires, f.sign(key, expires)), nil
}

func (f *Filesystem) Delete(_ context.Context, key string) error {
	if err := os.Remove(f.path(key)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return nil
}

// Open serves a previously presigned link, refusing anything expired or tampered with. The dev
// image route uses it; the S3 adapter has no equivalent because the bucket answers directly.
func (f *Filesystem) Open(key, signature string, expires int64) (io.ReadCloser, error) {
	if f.now().Unix() > expires {
		return nil, apperror.Newf("FORBIDDEN", "this image link has expired")
	}
	if !hmac.Equal([]byte(signature), []byte(f.sign(key, expires))) {
		return nil, apperror.New("FORBIDDEN")
	}
	file, err := os.Open(f.path(key))
	if err != nil {
		return nil, apperror.New("RECEIPT_NOT_FOUND")
	}
	return file, nil
}

func (f *Filesystem) sign(key string, expires int64) string {
	mac := hmac.New(sha256.New, f.secret)
	mac.Write([]byte(key + ":" + strconv.FormatInt(expires, 10)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// path resolves a key inside the root. Cleaning the key as if it were rooted collapses any ".."
// before it is joined, so a crafted key cannot climb out of the storage directory.
func (f *Filesystem) path(key string) string {
	return filepath.Join(f.root, filepath.Clean(filepath.FromSlash("/"+key)))
}
