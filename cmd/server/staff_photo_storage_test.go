package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/config"
)

func TestOpenStaffPhotoStorage(t *testing.T) {
	t.Run("disabled without fallback", func(t *testing.T) {
		cfg := photoStorageConfig(t)
		photos, err := openStaffPhotoStorage(cfg)
		if err != nil || photos != nil {
			t.Fatalf("staff storage=%v err=%v", photos, err)
		}
	})
	t.Run("isolated private directory", func(t *testing.T) {
		cfg := photoStorageConfig(t)
		cfg.StaffPhotoDirectory = filepath.Join(filepath.Dir(cfg.TeacherPhotoDirectory), "staff-selfies")
		photos, err := openStaffPhotoStorage(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer photos.Close()
		info, err := os.Stat(cfg.StaffPhotoDirectory)
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatal("direktori foto pegawai tidak privat 0700")
		}
		if _, err = photos.ListKeys(); err != nil {
			t.Fatal(err)
		}
	})
	for _, directory := range []string{"same", "nested"} {
		t.Run(directory+" teacher storage rejected", func(t *testing.T) {
			cfg := photoStorageConfig(t)
			if directory == "same" {
				cfg.StaffPhotoDirectory = cfg.TeacherPhotoDirectory
			} else {
				cfg.StaffPhotoDirectory = filepath.Join(cfg.TeacherPhotoDirectory, "staff")
			}
			photos, err := openStaffPhotoStorage(cfg)
			if photos != nil {
				_ = photos.Close()
				t.Fatal("direktori foto guru digunakan bersama")
			}
			if err == nil || strings.Contains(err.Error(), cfg.StaffPhotoDirectory) {
				t.Fatal("konflik storage diterima atau path privat terungkap")
			}
		})
	}
	t.Run("invalid config", func(t *testing.T) {
		photos, err := openStaffPhotoStorage(config.Config{StaffPhotoDirectory: "relative", RepositoryRoot: "/missing"})
		if photos != nil || err == nil || strings.Contains(err.Error(), "relative") {
			t.Fatal("konfigurasi tidak aman diterima")
		}
	})
}
