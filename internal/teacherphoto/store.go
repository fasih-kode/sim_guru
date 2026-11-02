// Package teacherphoto menyediakan penyimpanan privat untuk selfie kehadiran guru.
package teacherphoto

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// MaxPhotoBytes adalah batas byte JPEG final yang dapat disimpan.
	MaxPhotoBytes  = 100 * 1024
	maxInputBytes  = 5 * 1024 * 1024
	maxImageSide   = 4096
	maxImagePixels = int64(8_000_000)
)

var (
	ErrNotConfigured        = errors.New("penyimpanan selfie belum dikonfigurasi")
	ErrInvalidConfiguration = errors.New("konfigurasi penyimpanan selfie tidak aman")
	ErrUnsupportedMediaType = errors.New("format gambar tidak didukung")
	ErrInvalidImage         = errors.New("gambar tidak valid")
	ErrImageDimensions      = errors.New("dimensi gambar melewati batas")
	ErrInputTooLarge        = errors.New("ukuran gambar masukan melewati batas")
	ErrPhotoTooLarge        = errors.New("hasil JPEG melebihi 100 KB")
	ErrInvalidKey           = errors.New("kunci penyimpanan tidak valid")
	ErrNotFound             = errors.New("foto tidak ditemukan")
	ErrStorage              = errors.New("operasi penyimpanan selfie gagal")
	ErrClosed               = errors.New("penyimpanan selfie sudah ditutup")
)

// Config memerlukan lokasi privat eksplisit dan semua root yang menyajikan file.
// RepositoryRoot juga diblokir seluruhnya agar storage tidak masuk ke tree aplikasi.
type Config struct {
	Directory      string
	RepositoryRoot string
	PublicRoots    []string
}

type Metadata struct {
	StorageKey     string
	MIMEType       string
	SizeBytes      int
	ChecksumSHA256 string
}

// PreparedPhoto berisi JPEG yang sudah divalidasi dan dinormalisasi, tanpa file.
// Byte gambar tetap privat; metadata ini memungkinkan pemeriksaan retry sebelum menulis file.
type PreparedPhoto struct {
	data     []byte
	checksum string
}

func (p PreparedPhoto) MIMEType() string       { return "image/jpeg" }
func (p PreparedPhoto) SizeBytes() int         { return len(p.data) }
func (p PreparedPhoto) ChecksumSHA256() string { return p.checksum }

type Store struct {
	mu     sync.Mutex
	root   *os.Root
	random io.Reader
	closed bool
}

// NewStore membuka storage yang sudah ada atau membuat satu direktori privat.
// Parent direktori harus sudah ada; fungsi ini tidak membuat path bertingkat.
func NewStore(config Config) (*Store, error) {
	if strings.TrimSpace(config.Directory) == "" {
		return nil, ErrNotConfigured
	}
	if len(config.PublicRoots) == 0 {
		return nil, ErrInvalidConfiguration
	}
	repositoryRoot, err := canonicalDirectory(config.RepositoryRoot)
	if err != nil {
		return nil, ErrInvalidConfiguration
	}
	protected := []string{repositoryRoot}
	for _, path := range config.PublicRoots {
		root, err := canonicalDirectory(path)
		if err != nil {
			return nil, ErrInvalidConfiguration
		}
		protected = append(protected, root)
	}

	directory, err := cleanAbsolutePath(config.Directory)
	if err != nil {
		return nil, ErrInvalidConfiguration
	}
	parent := filepath.Dir(directory)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil || filepath.Clean(resolvedParent) != parent {
		return nil, ErrInvalidConfiguration
	}
	parentInfo, err := os.Stat(resolvedParent)
	if err != nil || !parentInfo.IsDir() || parentInfo.Mode().Perm()&0o022 != 0 {
		return nil, ErrInvalidConfiguration
	}
	directory = filepath.Join(resolvedParent, filepath.Base(directory))
	for _, root := range protected {
		if pathWithin(directory, root) {
			return nil, ErrInvalidConfiguration
		}
	}

	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		if err = os.Mkdir(directory, 0o700); err != nil {
			return nil, ErrStorage
		}
		info, err = os.Lstat(directory)
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm() != 0o700 {
		return nil, ErrInvalidConfiguration
	}

	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, ErrStorage
	}
	openedDirectory, err := root.Stat(".")
	if err != nil || !os.SameFile(info, openedDirectory) || openedDirectory.Mode().Perm() != 0o700 {
		root.Close()
		return nil, ErrInvalidConfiguration
	}
	store := &Store{root: root, random: rand.Reader}
	if err = store.validateExistingFiles(); err != nil {
		root.Close()
		return nil, err
	}
	return store, nil
}

// Close melepas handle direktori privat.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == nil || s.closed {
		return nil
	}
	s.closed = true
	if err := s.root.Close(); err != nil {
		return ErrStorage
	}
	return nil
}

// Save memvalidasi dan menormalkan gambar, lalu menerbitkan file secara atomik.
// contentType hanya petunjuk dari pemanggil dan harus cocok dengan format hasil decode.
func (s *Store) Save(source io.Reader, contentType string) (Metadata, error) {
	prepared, err := s.Prepare(source, contentType)
	if err != nil {
		return Metadata{}, err
	}
	return s.SavePrepared(prepared)
}

// Prepare memvalidasi, mendekode, menormalkan, dan menghitung hash gambar tanpa membuat file.
func (s *Store) Prepare(source io.Reader, contentType string) (PreparedPhoto, error) {
	if s == nil || s.root == nil {
		return PreparedPhoto{}, ErrNotConfigured
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return PreparedPhoto{}, ErrClosed
	}
	s.mu.Unlock()
	finalBytes, err := normalizeImage(source, contentType)
	if err != nil {
		return PreparedPhoto{}, err
	}
	checksum := sha256.Sum256(finalBytes)
	return PreparedPhoto{data: finalBytes, checksum: hex.EncodeToString(checksum[:])}, nil
}

// SavePrepared menyimpan JPEG yang sudah divalidasi dengan kunci yang dibuat oleh storage.
func (s *Store) SavePrepared(prepared PreparedPhoto) (Metadata, error) {
	if s == nil || s.root == nil {
		return Metadata{}, ErrNotConfigured
	}
	if len(prepared.data) == 0 || len(prepared.data) > MaxPhotoBytes || prepared.checksum == "" {
		return Metadata{}, ErrInvalidImage
	}
	digest := sha256.Sum256(prepared.data)
	if prepared.checksum != hex.EncodeToString(digest[:]) {
		return Metadata{}, ErrInvalidImage
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Metadata{}, ErrClosed
	}

	tempKey, err := s.randomKey()
	if err != nil {
		return Metadata{}, ErrStorage
	}
	tempName := ".tmp-" + tempKey
	temp, err := s.root.OpenFile(tempName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return Metadata{}, ErrStorage
	}
	tempExists := true
	defer func() {
		if tempExists {
			_ = s.root.Remove(tempName)
		}
	}()

	if err = writeAll(temp, prepared.data); err != nil {
		temp.Close()
		return Metadata{}, ErrStorage
	}
	if err = temp.Chmod(0o600); err != nil {
		temp.Close()
		return Metadata{}, ErrStorage
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return Metadata{}, ErrStorage
	}
	if err = temp.Close(); err != nil {
		return Metadata{}, ErrStorage
	}

	var storageKey string
	for attempt := 0; attempt < 3; attempt++ {
		storageKey, err = s.randomKey()
		if err != nil {
			return Metadata{}, ErrStorage
		}
		err = s.root.Link(tempName, storageKey)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return Metadata{}, ErrStorage
		}
	}
	if err != nil {
		return Metadata{}, ErrStorage
	}
	if err = s.root.Remove(tempName); err != nil {
		_ = s.root.Remove(storageKey)
		return Metadata{}, ErrStorage
	}
	tempExists = false

	return Metadata{
		StorageKey:     storageKey,
		MIMEType:       "image/jpeg",
		SizeBytes:      len(prepared.data),
		ChecksumSHA256: prepared.checksum,
	}, nil
}

// Open hanya membuka key acak terbitan storage dan memastikan file biasa privat.
func (s *Store) Open(storageKey string) (*os.File, error) {
	if s == nil || s.root == nil {
		return nil, ErrNotConfigured
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrClosed
	}
	if !validStorageKey(storageKey) {
		return nil, ErrInvalidKey
	}
	info, err := s.root.Lstat(storageKey)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > MaxPhotoBytes {
		return nil, ErrStorage
	}
	file, err := s.root.Open(storageKey)
	if err != nil {
		return nil, ErrStorage
	}
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		file.Close()
		return nil, ErrStorage
	}
	return file, nil
}

// Delete menghapus file privat; key yang sudah tidak ada dianggap berhasil.
func (s *Store) Delete(storageKey string) error {
	if s == nil || s.root == nil {
		return ErrNotConfigured
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	if !validStorageKey(storageKey) {
		return ErrInvalidKey
	}
	info, err := s.root.Lstat(storageKey)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return ErrStorage
	}
	if err = s.root.Remove(storageKey); err != nil && !errors.Is(err, os.ErrNotExist) {
		return ErrStorage
	}
	return nil
}

// ListKeys mengembalikan nama file final saja untuk rekonsiliasi internal.
// Berkas sementara tidak pernah disajikan sebagai foto.
func (s *Store) ListKeys() ([]string, error) {
	if s == nil || s.root == nil {
		return nil, ErrNotConfigured
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrClosed
	}
	entries, err := readRootEntries(s.root)
	if err != nil {
		return nil, ErrStorage
	}
	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if validTempName(name) {
			continue
		}
		if !validStorageKey(name) {
			return nil, ErrStorage
		}
		info, statErr := s.root.Lstat(name)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() > MaxPhotoBytes {
			return nil, ErrStorage
		}
		keys = append(keys, name)
	}
	return keys, nil
}

// CleanupTemps membuang file temporer valid yang lebih lama dari umur yang ditentukan.
// Save dan cleanup diserialkan pada instance Store yang sama; umur positif
// membatasi cleanup pada berkas sementara yang sudah lama tertinggal.
func (s *Store) CleanupTemps(olderThan time.Duration) (int, error) {
	if s == nil || s.root == nil {
		return 0, ErrNotConfigured
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, ErrClosed
	}
	if olderThan <= 0 {
		return 0, ErrInvalidConfiguration
	}
	entries, err := readRootEntries(s.root)
	if err != nil {
		return 0, ErrStorage
	}
	cutoff := time.Now().Add(-olderThan)
	removed := 0
	for _, entry := range entries {
		name := entry.Name()
		if !validTempName(name) {
			continue
		}
		info, statErr := s.root.Lstat(name)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				continue
			}
			return removed, ErrStorage
		}
		if !info.Mode().IsRegular() || info.ModTime().After(cutoff) {
			continue
		}
		if err = s.root.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return removed, ErrStorage
		}
		removed++
	}
	return removed, nil
}

func normalizeImage(source io.Reader, contentType string) ([]byte, error) {
	if source == nil {
		return nil, ErrInvalidImage
	}
	declaredType, _, err := mime.ParseMediaType(contentType)
	if err != nil || (declaredType != "image/jpeg" && declaredType != "image/png") {
		return nil, ErrUnsupportedMediaType
	}
	input, err := io.ReadAll(io.LimitReader(source, maxInputBytes+1))
	if err != nil || len(input) == 0 {
		return nil, ErrInvalidImage
	}
	if len(input) > maxInputBytes {
		return nil, ErrInputTooLarge
	}

	config, format, err := image.DecodeConfig(bytes.NewReader(input))
	if err != nil {
		return nil, ErrInvalidImage
	}
	actualType := ""
	switch format {
	case "jpeg":
		actualType = "image/jpeg"
	case "png":
		actualType = "image/png"
	default:
		return nil, ErrUnsupportedMediaType
	}
	if actualType != declaredType {
		return nil, ErrUnsupportedMediaType
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxImageSide || config.Height > maxImageSide || int64(config.Width)*int64(config.Height) > maxImagePixels {
		return nil, ErrImageDimensions
	}

	decoded, actualFormat, err := image.Decode(bytes.NewReader(input))
	if err != nil || actualFormat != format {
		return nil, ErrInvalidImage
	}
	for _, quality := range [...]int{85, 75, 65, 55, 45, 35, 25} {
		var output bytes.Buffer
		if err = jpeg.Encode(&output, decoded, &jpeg.Options{Quality: quality}); err != nil {
			return nil, ErrInvalidImage
		}
		if output.Len() <= MaxPhotoBytes {
			return output.Bytes(), nil
		}
	}
	return nil, ErrPhotoTooLarge
}

func writeAll(file *os.File, data []byte) error {
	for len(data) > 0 {
		n, err := file.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func (s *Store) randomKey() (string, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(s.random, key); err != nil {
		return "", err
	}
	return hex.EncodeToString(key), nil
}

func (s *Store) validateExistingFiles() error {
	entries, err := readRootEntries(s.root)
	if err != nil {
		return ErrStorage
	}
	for _, entry := range entries {
		name := entry.Name()
		if !validStorageKey(name) && !validTempName(name) {
			return ErrInvalidConfiguration
		}
		info, err := s.root.Lstat(name)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() > MaxPhotoBytes {
			return ErrInvalidConfiguration
		}
	}
	return nil
}

func readRootEntries(root *os.Root) ([]os.DirEntry, error) {
	directory, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	return directory.ReadDir(-1)
}

func validStorageKey(key string) bool {
	if len(key) != 64 {
		return false
	}
	for _, char := range key {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func validTempName(name string) bool {
	return strings.HasPrefix(name, ".tmp-") && validStorageKey(strings.TrimPrefix(name, ".tmp-"))
}

func cleanAbsolutePath(path string) (string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", ErrInvalidConfiguration
	}
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if part == ".." {
			return "", ErrInvalidConfiguration
		}
	}
	return path, nil
}

func canonicalDirectory(path string) (string, error) {
	path, err := cleanAbsolutePath(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", ErrInvalidConfiguration
	}
	return filepath.Clean(resolved), nil
}

func pathWithin(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && (relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))))
}
