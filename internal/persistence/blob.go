package persistence

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

const MaxBlobBytes int64 = 12 << 20

var safeDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

type BlobStore struct {
	directory string
}

func NewBlobStore(dataDirectory string) (*BlobStore, error) {
	directory := filepath.Join(dataDirectory, "blobs")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	return &BlobStore{directory: directory}, nil
}

func (b *BlobStore) Save(expectedDigest, mediaType string, source io.Reader) (string, int64, error) {
	if !safeDigest.MatchString(expectedDigest) {
		return "", 0, fmt.Errorf("影像摘要格式无效")
	}
	if mediaType != "image/jpeg" && mediaType != "image/png" && mediaType != "image/webp" {
		return "", 0, fmt.Errorf("媒体类型不受支持")
	}
	tmp, err := os.CreateTemp(b.directory, ".blob-*.tmp")
	if err != nil {
		return "", 0, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(source, MaxBlobBytes+1))
	if err != nil {
		tmp.Close()
		return "", 0, err
	}
	if written > MaxBlobBytes {
		tmp.Close()
		return "", 0, fmt.Errorf("影像载荷超过 %d 字节限制", MaxBlobBytes)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != expectedDigest {
		tmp.Close()
		return "", 0, fmt.Errorf("影像载荷 SHA-256 与登记摘要不一致")
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", 0, err
	}
	if err := tmp.Close(); err != nil {
		return "", 0, err
	}
	finalPath := filepath.Join(b.directory, expectedDigest)
	if _, err := os.Stat(finalPath); err == nil {
		return expectedDigest, written, nil
	}
	if err := os.Rename(tmpName, finalPath); err != nil {
		return "", 0, err
	}
	return expectedDigest, written, nil
}

func (b *BlobStore) Open(key string) (*os.File, error) {
	if !safeDigest.MatchString(key) {
		return nil, fmt.Errorf("载荷键无效")
	}
	file, err := os.Open(filepath.Join(b.directory, key))
	if err != nil {
		return nil, err
	}
	return file, nil
}
