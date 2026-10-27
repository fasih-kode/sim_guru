package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/config"
)

func photoStorageConfig(t *testing.T) config.Config {
	t.Helper()
	base := t.TempDir()
	repository := filepath.Join(base, "repository")
	private := filepath.Join(base, "private")
	if err := os.Mkdir(private, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repository, "internal", "web", "static"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "go.mod"), []byte("module github.com/fasih-kode/sim_guru\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return config.Config{RepositoryRoot: repository, TeacherPhotoDirectory: filepath.Join(private, "photos")}
}

func TestOpenTeacherPhotoStorage(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		photos, err := openTeacherPhotoStorage(config.Config{})
		if err != nil || photos != nil {
			t.Fatalf("disabled storage=%v err=%v", photos, err)
		}
	})
	t.Run("explicit roots independent of working directory", func(t *testing.T) {
		cfg := photoStorageConfig(t)
		photos, err := openTeacherPhotoStorage(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer photos.Close()
		info, err := os.Stat(cfg.TeacherPhotoDirectory)
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatal("direktori privat tidak berizin 0700")
		}
		if _, err = photos.ListKeys(); err != nil {
			t.Fatal(err)
		}
	})
	for _, name := range []string{"inside repository", "inside public root", "unsafe permissions", "invalid repository", "wrong module", "missing static root", "symlink parent", "writable parent", "symlink photo directory", "missing parent"} {
		t.Run(name, func(t *testing.T) {
			cfg := photoStorageConfig(t)
			switch name {
			case "inside repository":
				cfg.TeacherPhotoDirectory = filepath.Join(cfg.RepositoryRoot, "photos")
			case "inside public root":
				cfg.TeacherPhotoDirectory = filepath.Join(cfg.RepositoryRoot, "internal", "web", "static", "photos")
			case "unsafe permissions":
				if err := os.Mkdir(cfg.TeacherPhotoDirectory, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(cfg.TeacherPhotoDirectory, 0o755); err != nil {
					t.Fatal(err)
				}
			case "invalid repository":
				cfg.RepositoryRoot = t.TempDir()
			case "wrong module":
				if err := os.WriteFile(filepath.Join(cfg.RepositoryRoot, "go.mod"), []byte("module example.com/other\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "missing static root":
				if err := os.Remove(filepath.Join(cfg.RepositoryRoot, "internal", "web", "static")); err != nil {
					t.Fatal(err)
				}
			case "symlink parent":
				parent := filepath.Dir(cfg.TeacherPhotoDirectory)
				link := filepath.Join(filepath.Dir(parent), "linked-private")
				if err := os.Symlink(parent, link); err != nil {
					t.Fatal(err)
				}
				cfg.TeacherPhotoDirectory = filepath.Join(link, "photos")
			case "writable parent":
				if err := os.Chmod(filepath.Dir(cfg.TeacherPhotoDirectory), 0o777); err != nil {
					t.Fatal(err)
				}
			case "symlink photo directory":
				if err := os.Mkdir(cfg.TeacherPhotoDirectory, 0o700); err != nil {
					t.Fatal(err)
				}
				link := filepath.Join(filepath.Dir(cfg.TeacherPhotoDirectory), "linked-photos")
				if err := os.Symlink(cfg.TeacherPhotoDirectory, link); err != nil {
					t.Fatal(err)
				}
				cfg.TeacherPhotoDirectory = link
			case "missing parent":
				cfg.TeacherPhotoDirectory = filepath.Join(t.TempDir(), "absent", "photos")
			}
			photos, err := openTeacherPhotoStorage(cfg)
			if photos != nil {
				photos.Close()
				t.Fatal("unsafe storage opened")
			}
			if err == nil {
				t.Fatal("unsafe storage accepted")
			}
			if strings.Contains(err.Error(), cfg.TeacherPhotoDirectory) || strings.Contains(err.Error(), cfg.RepositoryRoot) {
				t.Fatal("error exposed private path")
			}
		})
	}
}
