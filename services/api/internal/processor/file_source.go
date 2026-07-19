package processor

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	sftpclient "github.com/buskseguros-design/services/api/internal/sftp"
)

// fileSource abstrae el origen de los archivos a procesar. La implementación real
// puede ser un SFTP remoto o una carpeta local (reingesta desde el archivo).
// El pipeline usa esta interfaz para no acoplar processOne al transporte.
type fileSource interface {
	Open(name string) (io.ReadCloser, error)
	MoveToFolder(name, folder string) (string, error)
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

func (l localFileSource) MoveToFolder(name, folder string) (string, error) {
	src := filepath.Join(l.baseDir, name)
	dstDir := filepath.Join(l.baseDir, folder)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dstDir, err)
	}
	dst := filepath.Join(dstDir, name)
	if err := os.Rename(src, dst); err != nil {
		return "", fmt.Errorf("rename %s -> %s: %w", src, dst, err)
	}
	return dst, nil
}

func (l localFileSource) Close() {}
