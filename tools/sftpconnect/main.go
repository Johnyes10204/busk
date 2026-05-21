// Comando de ejemplo: conexión SFTP, listado y descarga de archivos remotos.
//
// Variables de entorno (no guardar secretos en el repo):
//
//	SFTP_HOST      host o IP (por defecto 192.168.46.101)
//	SFTP_PORT      puerto (por defecto 2222)
//	SFTP_USER      usuario
//	SFTP_PASSWORD      contraseña (obligatoria)
//	SFTP_REMOTE_DIR    ruta remota a listar (por defecto ".")
//	SFTP_DOWNLOAD      si es "true", descarga archivos (por defecto "false")
//	SFTP_DOWNLOAD_DIR  carpeta local destino (por defecto "./downloads")
//	SFTP_FILTER        filtros separados por coma; coincide por subcadena en nombre
//	SFTP_MAX_FILES     máximo de archivos a descargar (0 = sin límite)
//	SFTP_SKIP_EXISTING si es "true", omite archivos ya existentes (por defecto "true")
//
// Ejemplo:
//
//	SFTP_PASSWORD='...' go run .
package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvBool(key string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes" || v == "y"
}

func getenvInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func dialSFTP() (*sftp.Client, func(), error) {
	host := strings.TrimPrefix(strings.TrimPrefix(getenv("SFTP_HOST", "192.168.46.101"), "sftp://"), "http://")
	port := getenv("SFTP_PORT", "2222")
	user := getenv("SFTP_USER", "usuario_ftp")
	pass := getenv("SFTP_PASSWORD", "BuskS3g26*")
	if pass == "" {
		return nil, nil, fmt.Errorf("defina SFTP_PASSWORD (no se acepta contraseña en código fuente)")
	}

	cfg := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
			ssh.KeyboardInteractive(func(_ string, _ string, questions []string, _ []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range questions {
					answers[i] = pass
				}
				return answers, nil
			}),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: usar known_hosts en producción
		Timeout:         30 * time.Second,
	}

	addr := net.JoinHostPort(host, port)
	conn, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}

	client, err := sftp.NewClient(conn)
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("sftp client: %w", err)
	}

	cleanup := func() {
		_ = client.Close()
		_ = conn.Close()
	}
	return client, cleanup, nil
}

func main() {
	log.SetFlags(0)

	client, cleanup, err := dialSFTP()
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	remoteDir := getenv("SFTP_REMOTE_DIR", ".")
	w := os.Stdout
	fmt.Fprintf(w, "Conectado. Listando %q:\n", remoteDir)

	entries, err := client.ReadDir(remoteDir)
	if err != nil {
		log.Fatalf("ReadDir: %v", err)
	}

	for _, e := range entries {
		mode := e.Mode().String()
		size := e.Size()
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		fmt.Fprintf(w, "  %s  %8d  %s\n", mode, size, name)
	}

	fi, err := client.Stat(remoteDir)
	if err != nil {
		log.Fatalf("Stat: %v", err)
	}
	fmt.Fprintf(w, "Stat OK: %s isDir=%v\n", fi.Name(), fi.IsDir())

	if !getenvBool("SFTP_DOWNLOAD", true) {
		return
	}

	downloadDir := getenv("SFTP_DOWNLOAD_DIR", "./downloads")
	skipExisting := getenvBool("SFTP_SKIP_EXISTING", true)
	maxFiles := getenvInt("SFTP_MAX_FILES", 0)
	rawFilters := strings.TrimSpace(os.Getenv("SFTP_FILTER"))
	var filters []string
	if rawFilters != "" {
		for _, f := range strings.Split(rawFilters, ",") {
			f = strings.TrimSpace(f)
			if f != "" {
				filters = append(filters, strings.ToLower(f))
			}
		}
	}

	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		log.Fatalf("mkdir destino: %v", err)
	}

	downloaded := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if len(filters) > 0 {
			nameLower := strings.ToLower(e.Name())
			matched := false
			for _, f := range filters {
				if strings.Contains(nameLower, f) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if maxFiles > 0 && downloaded >= maxFiles {
			break
		}

		remotePath := path.Join(remoteDir, e.Name())
		localPath := filepath.Join(downloadDir, e.Name())
		if err := downloadOne(client, remotePath, localPath, skipExisting); err != nil {
			log.Printf("descarga fallida %q: %v", e.Name(), err)
			continue
		}
		downloaded++
		fmt.Fprintf(w, "Descargado: %s -> %s\n", remotePath, localPath)
	}
	fmt.Fprintf(w, "Descarga completada. Archivos descargados: %d\n", downloaded)
}

func downloadOne(client *sftp.Client, remotePath, localPath string, skipExisting bool) error {
	if skipExisting {
		if _, err := os.Stat(localPath); err == nil {
			return nil
		}
	}

	src, err := client.Open(remotePath)
	if err != nil {
		return fmt.Errorf("open remoto: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create local: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	return nil
}
