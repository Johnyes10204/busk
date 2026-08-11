package processor

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	sftpclient "github.com/buskseguros-design/services/api/internal/sftp"
)

// fileSource abstrae el origen de los archivos a procesar. La implementación real
// puede ser un SFTP remoto o una carpeta local (reingesta desde el archivo).
// El pipeline usa esta interfaz para no acoplar processOne al transporte.
type fileSource interface {
	Open(name string) (io.ReadCloser, error)
	MoveToFolder(name, folder string) (string, error)
	RenameInPlace(name, prefix string) (string, error)
	Delete(name string) error
	Close()
}

// sftpFileSource envuelve el cliente SFTP para satisfacer fileSource.
type sftpFileSource struct {
	c *sftpclient.Client
}

func (s sftpFileSource) Open(name string) (io.ReadCloser, error) {
	return s.c.OpenRelative(name)
}

func (s sftpFileSource) MoveToFolder(name, folder string) (string, error) {
	return s.c.MoveToFolder(name, folder)
}

func (s sftpFileSource) Delete(name string) error {
	return s.c.DeleteFile(name)
}

func (s sftpFileSource) RenameInPlace(name, prefix string) (string, error) {
	return s.c.RenameInPlace(name, prefix)
}

func (s sftpFileSource) Close() {
	if s.c != nil {
		s.c.Close()
	}
}

// localFileSource lee archivos de una carpeta local y los mueve a
// baseDir/PROCESSED|ERROR/ al terminar. Se usa desde POST /process/scan-local
// para reprocesar archivos ya archivados sin depender del SFTP.
type localFileSource struct {
	baseDir string
}

func newLocalFileSource(dir string) (localFileSource, error) {
	if dir == "" {
		return localFileSource{}, fmt.Errorf("directorio local vacío")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return localFileSource{}, fmt.Errorf("resolver ruta local: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return localFileSource{}, fmt.Errorf("acceder ruta local %s: %w", abs, err)
	}
	if !info.IsDir() {
		return localFileSource{}, fmt.Errorf("ruta local no es carpeta: %s", abs)
	}
	return localFileSource{baseDir: abs}, nil
}

func (l localFileSource) Open(name string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(l.baseDir, name))
}

func (l localFileSource) Delete(name string) error {
	return os.Remove(filepath.Join(l.baseDir, name))
}

func (l localFileSource) RenameInPlace(name, prefix string) (string, error) {
	ts := time.Now().UTC().Format("20060102150405")
	newName := fmt.Sprintf("%s-%s-%s", prefix, ts, name)
	src := filepath.Join(l.baseDir, name)
	dst := filepath.Join(l.baseDir, newName)
	if err := os.Rename(src, dst); err != nil {
		return "", fmt.Errorf("rename in place %s -> %s: %w", src, dst, err)
	}
	return newName, nil
}

func (l localFileSource) MoveToFolder(name, folder string) (string, error) {
	src := filepath.Join(l.baseDir, name)
	dstDir := filepath.Join(l.baseDir, folder)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dstDir, err)
	}
	dst := filepath.Join(dstDir, resolveLocalDestName(dstDir, name))
	if err := os.Rename(src, dst); err != nil {
		return "", fmt.Errorf("rename %s -> %s: %w", src, dst, err)
	}
	return dst, nil
}

// resolveLocalDestName evita colisiones agregando -YYYYMMDDHHMMSS antes de la extensión si el
// destino ya existe. Simetría con la implementación SFTP.
func resolveLocalDestName(dstDir, name string) string {
	if _, err := os.Stat(filepath.Join(dstDir, name)); err != nil {
		return name
	}
	ext := filepath.Ext(name)
	base := name[:len(name)-len(ext)]
	ts := time.Now().UTC().Format("20060102150405")
	candidate := fmt.Sprintf("%s-%s%s", base, ts, ext)
	if _, err := os.Stat(filepath.Join(dstDir, candidate)); err != nil {
		return candidate
	}
	for i := 1; i < 1000; i++ {
		alt := fmt.Sprintf("%s-%s-%d%s", base, ts, i, ext)
		if _, err := os.Stat(filepath.Join(dstDir, alt)); err != nil {
			return alt
		}
	}
	return fmt.Sprintf("%s-%s-%d%s", base, ts, time.Now().UnixNano(), ext)
}

func (l localFileSource) Close() {}
