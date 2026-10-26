package teacherphoto

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func testStore(t *testing.T) (*Store, string, string) {
	t.Helper()
	base := t.TempDir()
	repository := filepath.Join(base, "repo")
	public := filepath.Join(base, "public")
	private := filepath.Join(base, "private", "photos")
	for _, path := range []string{repository, public, filepath.Dir(private)} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	store, err := NewStore(Config{Directory: private, RepositoryRoot: repository, PublicRoots: []string{public}})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, private, public
}

func syntheticImage(t *testing.T, format string, width, height int, noisy bool) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	rng := rand.New(rand.NewSource(42))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if noisy {
				img.Set(x, y, color.RGBA{uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256)), 255})
			} else {
				img.Set(x, y, color.RGBA{uint8(x % 255), uint8(y % 255), 120, 255})
			}
		}
	}
	var out bytes.Buffer
	var err error
	switch format {
	case "jpeg":
		err = jpeg.Encode(&out, img, &jpeg.Options{Quality: 90})
	case "png":
		err = png.Encode(&out, img)
	default:
		t.Fatalf("unknown format %q", format)
	}
	if err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestSaveOpenDeleteAndMetadata(t *testing.T) {
	store, directory, _ := testStore(t)
	input := syntheticImage(t, "jpeg", 320, 240, false)
	meta, err := store.Save(bytes.NewReader(input), "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	if !validStorageKey(meta.StorageKey) || meta.MIMEType != "image/jpeg" || meta.SizeBytes <= 0 || meta.SizeBytes > MaxPhotoBytes {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
	data, err := os.ReadFile(filepath.Join(directory, meta.StorageKey))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if meta.ChecksumSHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("checksum mismatch: %s", meta.ChecksumSHA256)
	}
	file, err := store.Open(meta.StorageKey)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil || !bytes.Equal(data, opened) {
		t.Fatalf("Open content mismatch, err=%v", err)
	}
	info, err := os.Stat(filepath.Join(directory, meta.StorageKey))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("file permission = %v, err=%v", info.Mode().Perm(), err)
	}
	dirInfo, err := os.Stat(directory)
	if err != nil || dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("directory permission = %v, err=%v", dirInfo.Mode().Perm(), err)
	}
	second, err := store.Save(bytes.NewReader(input), "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	if second.StorageKey == meta.StorageKey {
		t.Fatal("storage key was reused")
	}
	if err = store.Delete(meta.StorageKey); err != nil {
		t.Fatal(err)
	}
	if err = store.Delete(meta.StorageKey); err != nil {
		t.Fatalf("second Delete must be idempotent: %v", err)
	}
	if _, err = store.Open(meta.StorageKey); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open deleted key error = %v", err)
	}
}

func TestSaveAcceptsPNGAndStoresJPEG(t *testing.T) {
	store, directory, _ := testStore(t)
	meta, err := store.Save(bytes.NewReader(syntheticImage(t, "png", 40, 30, false)), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if meta.MIMEType != "image/jpeg" {
		t.Fatalf("MIMEType = %q", meta.MIMEType)
	}
	file, err := os.Open(filepath.Join(directory, meta.StorageKey))
	if err != nil {
		t.Fatal(err)
	}
	_, format, decodeErr := image.Decode(file)
	_ = file.Close()
	if decodeErr != nil || format != "jpeg" {
		t.Fatalf("stored format=%q, err=%v", format, decodeErr)
	}
}

func TestSaveRejectsInvalidImagesAndMIME(t *testing.T) {
	store, _, _ := testStore(t)
	truncated := syntheticImage(t, "jpeg", 32, 32, false)
	truncated = truncated[:len(truncated)/2]
	cases := []struct {
		name string
		data []byte
		mime string
		want error
	}{
		{"empty", nil, "image/jpeg", ErrInvalidImage},
		{"not image", []byte("not an image"), "image/jpeg", ErrInvalidImage},
		{"truncated jpeg", truncated, "image/jpeg", ErrInvalidImage},
		{"fake mime", syntheticImage(t, "png", 20, 20, false), "image/jpeg", ErrUnsupportedMediaType},
		{"unsupported mime", syntheticImage(t, "jpeg", 20, 20, false), "image/webp", ErrUnsupportedMediaType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := store.Save(bytes.NewReader(tc.data), tc.mime); !errors.Is(err, tc.want) {
				t.Fatalf("Save error=%v, want %v", err, tc.want)
			}
		})
	}
}

func TestSaveRejectsOversizedInputAndDimensions(t *testing.T) {
	store, _, _ := testStore(t)
	if _, err := store.Save(bytes.NewReader(bytes.Repeat([]byte("x"), maxInputBytes+1)), "image/jpeg"); !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("oversized input error = %v", err)
	}
	for name, dimensions := range map[string][2]int{
		"side":   {maxImageSide + 1, 1},
		"pixels": {3000, 3000},
	} {
		t.Run(name, func(t *testing.T) {
			data := syntheticImage(t, "jpeg", dimensions[0], dimensions[1], false)
			if _, err := store.Save(bytes.NewReader(data), "image/jpeg"); !errors.Is(err, ErrImageDimensions) {
				t.Fatalf("dimensions error = %v", err)
			}
		})
	}
}

func TestSaveRejectsUncompressiblePhoto(t *testing.T) {
	store, _, _ := testStore(t)
	noisy := syntheticImage(t, "jpeg", 1024, 1024, true)
	if _, err := store.Save(bytes.NewReader(noisy), "image/jpeg"); !errors.Is(err, ErrPhotoTooLarge) {
		t.Fatalf("large noisy image error = %v", err)
	}
}

func TestPathTraversalAndSymlinksAreRejected(t *testing.T) {
	store, directory, public := testStore(t)
	for _, key := range []string{"../secret", filepath.Join("..", "secret"), strings.Repeat("a", 63), strings.Repeat("G", 64)} {
		if _, err := store.Open(key); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("Open(%q) error=%v", key, err)
		}
		if err := store.Delete(key); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("Delete(%q) error=%v", key, err)
		}
	}
	outside := filepath.Join(public, "outside")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkKey := strings.Repeat("a", 64)
	if err := os.Symlink(outside, filepath.Join(directory, linkKey)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(linkKey); !errors.Is(err, ErrStorage) {
		t.Fatalf("Open symlink error=%v", err)
	}
	if err := store.Delete(linkKey); !errors.Is(err, ErrStorage) {
		t.Fatalf("Delete symlink error=%v", err)
	}
}

func TestNewStoreRequiresSafeExplicitPrivateLocation(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	public := filepath.Join(base, "public")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(public, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(Config{RepositoryRoot: repo, PublicRoots: []string{public}}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unconfigured error=%v", err)
	}
	for name, path := range map[string]string{"repo": filepath.Join(repo, "photos"), "public": filepath.Join(public, "photos"), "noncanonical": base + "/repo/../photos"} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewStore(Config{Directory: path, RepositoryRoot: repo, PublicRoots: []string{public}}); err == nil {
				t.Fatal("expected unsafe path to fail")
			}
		})
	}
	parentFile := filepath.Join(base, "not-directory")
	if err := os.WriteFile(parentFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(Config{Directory: filepath.Join(parentFile, "photos"), RepositoryRoot: repo, PublicRoots: []string{public}}); err == nil {
		t.Fatal("expected non-directory parent to fail")
	}
}

func TestNewStoreRejectsSymlinkParentAndPermissiveExistingDirectory(t *testing.T) {
	base := t.TempDir()
	repo, public, realParent := filepath.Join(base, "repo"), filepath.Join(base, "public"), filepath.Join(base, "real")
	for _, path := range []string{repo, public, realParent} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	linkParent := filepath.Join(base, "linked")
	if err := os.Symlink(realParent, linkParent); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(Config{Directory: filepath.Join(linkParent, "photos"), RepositoryRoot: repo, PublicRoots: []string{public}}); err == nil {
		t.Fatal("expected symlink parent to fail")
	}
	permissive := filepath.Join(base, "permissive")
	if err := os.Mkdir(permissive, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(Config{Directory: permissive, RepositoryRoot: repo, PublicRoots: []string{public}}); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("permission error=%v", err)
	}
}

func TestSaveFailureLeavesNoPartialFiles(t *testing.T) {
	store, directory, _ := testStore(t)
	if _, err := store.Save(errorReader{}, "image/jpeg"); !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("invalid input Save error=%v", err)
	}
	store.random = bytes.NewReader(append(make([]byte, 32), []byte("short")...))
	if _, err := store.Save(bytes.NewReader(syntheticImage(t, "jpeg", 10, 10, false)), "image/jpeg"); !errors.Is(err, ErrStorage) {
		t.Fatalf("random failure Save error=%v", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("partial files remain: %v", entries)
	}
}

func TestSaveDoesNotOverwriteOnKeyCollision(t *testing.T) {
	store, directory, _ := testStore(t)
	key := strings.Repeat("0", 64)
	existing := []byte("existing file")
	if err := os.WriteFile(filepath.Join(directory, key), existing, 0o600); err != nil {
		t.Fatal(err)
	}
	store.random = zeroReader{}
	if _, err := store.Save(bytes.NewReader(syntheticImage(t, "jpeg", 10, 10, false)), "image/jpeg"); !errors.Is(err, ErrStorage) {
		t.Fatalf("collision Save error = %v", err)
	}
	after, err := os.ReadFile(filepath.Join(directory, key))
	if err != nil || !bytes.Equal(after, existing) {
		t.Fatalf("existing file changed: %q, err=%v", after, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("unexpected partial files after collisions: %v", entries)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("synthetic read failure") }

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestCleanupTempsRemovesOnlyStaleValidTemporaryFiles(t *testing.T) {
	store, directory, _ := testStore(t)
	oldName, recentName := ".tmp-"+strings.Repeat("a", 64), ".tmp-"+strings.Repeat("b", 64)
	finalName, foreignName := strings.Repeat("c", 64), "unrelated.txt"
	for _, name := range []string{oldName, recentName, finalName, foreignName} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("tmp"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	symlinkName := ".tmp-" + strings.Repeat("d", 64)
	if err := os.Symlink(filepath.Join(directory, finalName), filepath.Join(directory, symlinkName)); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	for _, name := range []string{oldName, finalName, foreignName} {
		if err := os.Chtimes(filepath.Join(directory, name), old, old); err != nil {
			t.Fatal(err)
		}
	}
	removed, err := store.CleanupTemps(time.Hour)
	if err != nil || removed != 1 {
		t.Fatalf("CleanupTemps=(%d,%v)", removed, err)
	}
	if _, err := os.Stat(filepath.Join(directory, oldName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale temp remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, recentName)); err != nil {
		t.Fatalf("recent temp removed: %v", err)
	}
	for _, name := range []string{finalName, foreignName, symlinkName} {
		if _, err := os.Lstat(filepath.Join(directory, name)); err != nil {
			t.Fatalf("cleanup touched non-owned file %q: %v", name, err)
		}
	}
	removed, err = store.CleanupTemps(time.Hour)
	if err != nil || removed != 0 {
		t.Fatalf("repeated CleanupTemps=(%d,%v)", removed, err)
	}
}

func TestCleanupWaitsForActiveSaveOnSameStore(t *testing.T) {
	store, directory, _ := testStore(t)
	blocking := &blockingRandom{entered: make(chan struct{}), release: make(chan struct{})}
	store.random = blocking
	input := syntheticImage(t, "jpeg", 64, 64, false)
	saved := make(chan error, 1)
	go func() {
		_, err := store.Save(bytes.NewReader(input), "image/jpeg")
		saved <- err
	}()
	select {
	case <-blocking.entered: // Save sudah menulis temp dan sedang membuat key final.
	case <-time.After(5 * time.Second):
		t.Fatal("Save tidak mencapai tahap publikasi")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !validTempName(entries[0].Name()) {
		t.Fatalf("file final tampak sebelum publikasi atomik: %v", entries)
	}
	cleaned := make(chan struct {
		removed int
		err     error
	}, 1)
	go func() {
		removed, err := store.CleanupTemps(time.Nanosecond)
		cleaned <- struct {
			removed int
			err     error
		}{removed, err}
	}()
	runtime.Gosched()
	select {
	case result := <-cleaned:
		t.Fatalf("cleanup raced with active save: %+v", result)
	case <-time.After(20 * time.Millisecond):
	}
	close(blocking.release)
	if err := <-saved; err != nil {
		t.Fatalf("Save: %v", err)
	}
	result := <-cleaned
	if result.err != nil || result.removed != 0 {
		t.Fatalf("CleanupTemps after Save=(%d,%v)", result.removed, result.err)
	}
	entries, err = os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !validStorageKey(entries[0].Name()) {
		t.Fatalf("hasil publikasi tidak utuh: %v", entries)
	}
}

func TestOperationsAfterCloseFailSafelyAndCloseIsIdempotent(t *testing.T) {
	store, _, _ := testStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := store.Save(bytes.NewReader(nil), "image/jpeg"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Save after Close error=%v", err)
	}
	if _, err := store.Open(strings.Repeat("a", 64)); !errors.Is(err, ErrClosed) {
		t.Fatalf("Open after Close error=%v", err)
	}
	if err := store.Delete(strings.Repeat("a", 64)); !errors.Is(err, ErrClosed) {
		t.Fatalf("Delete after Close error=%v", err)
	}
	if _, err := store.CleanupTemps(time.Hour); !errors.Is(err, ErrClosed) {
		t.Fatalf("CleanupTemps after Close error=%v", err)
	}
}

type blockingRandom struct {
	calls   int
	entered chan struct{}
	release chan struct{}
}

func (r *blockingRandom) Read(p []byte) (int, error) {
	r.calls++
	if r.calls == 2 {
		close(r.entered)
		<-r.release
	}
	clear(p)
	return len(p), nil
}
