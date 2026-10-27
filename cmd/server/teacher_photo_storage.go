package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/fasih-kode/sim_guru/internal/config"
	"github.com/fasih-kode/sim_guru/internal/teacherphoto"
)

func openTeacherPhotoStorage(cfg config.Config) (*teacherphoto.Store, error) {
	if cfg.TeacherPhotoDirectory == "" {
		return nil, nil
	}
	// Root eksplisit diverifikasi sebagai tree SIM-GURU, bukan ditebak dari cwd atau binary.
	module, err := os.ReadFile(filepath.Join(cfg.RepositoryRoot, "go.mod"))
	fields := strings.Fields(string(module))
	if err != nil || !filepath.IsAbs(cfg.RepositoryRoot) || len(fields) < 2 || fields[0] != "module" || fields[1] != "github.com/fasih-kode/sim_guru" {
		return nil, errors.New("APP_REPOSITORY_ROOT harus menunjuk root sumber SIM-GURU yang valid")
	}
	photos, err := teacherphoto.NewStore(teacherphoto.Config{
		Directory: cfg.TeacherPhotoDirectory, RepositoryRoot: cfg.RepositoryRoot,
		PublicRoots: []string{filepath.Join(cfg.RepositoryRoot, "internal", "web", "static")},
	})
	if err != nil {
		return nil, errors.New("storage foto guru tidak aman atau tidak dapat dibuka; periksa konfigurasi direktori privat dan permission")
	}
	return photos, nil
}
