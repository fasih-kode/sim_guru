package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/fasih-kode/sim_guru/internal/config"
	"github.com/fasih-kode/sim_guru/internal/teacherphoto"
)

func openStaffPhotoStorage(cfg config.Config) (*teacherphoto.Store, error) {
	if cfg.StaffPhotoDirectory == "" {
		return nil, nil
	}
	if photoDirectoriesOverlap(cfg.StaffPhotoDirectory, cfg.TeacherPhotoDirectory) {
		return nil, errors.New("storage foto guru dan pegawai harus memakai direktori privat terpisah")
	}
	module, err := os.ReadFile(filepath.Join(cfg.RepositoryRoot, "go.mod"))
	fields := strings.Fields(string(module))
	if err != nil || !filepath.IsAbs(cfg.RepositoryRoot) || len(fields) < 2 || fields[0] != "module" || fields[1] != "github.com/fasih-kode/sim_guru" {
		return nil, errors.New("APP_REPOSITORY_ROOT harus menunjuk root sumber SIM-GURU yang valid")
	}
	photos, err := teacherphoto.NewStore(teacherphoto.Config{
		Directory: cfg.StaffPhotoDirectory, RepositoryRoot: cfg.RepositoryRoot,
		PublicRoots: []string{filepath.Join(cfg.RepositoryRoot, "internal", "web", "static")},
	})
	if err != nil {
		return nil, errors.New("storage foto pegawai tidak aman atau tidak dapat dibuka; periksa konfigurasi direktori privat dan permission")
	}
	return photos, nil
}
