package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Config struct {
	Host           string
	Port           string
	User           string
	Password       string
	RemoteDir      string
	ProcessedDir   string
	PrivateKeyPath string
}

type Client struct {
	raw           *sftp.Client
	conn          *ssh.Client
	baseDir       string
	processedDir  string
	keepaliveStop chan struct{}
}

func main() {

	cfg := Config{
		Host:           getenv("SFTP_HOST", "192.168.46.101"),
		Port:           getenv("SFTP_PORT", "2222"),
		User:           getenv("SFTP_USER", "usuario_ftp"),
		Password:       getenv("SFTP_PASSWORD", "BuskS3g26*"),
		RemoteDir:      getenv("SFTP_REMOTE_DIR", "."),
		ProcessedDir:   getenv("SFTP_PROCESSED_DIR", "PROCESSED"),
		PrivateKeyPath: getenv("SFTP_PRIVATE_KEY_PATH", ""),
	}

	cfg.Host = strings.TrimPrefix(cfg.Host, "sftp://")
	cfg.Host = strings.TrimPrefix(cfg.Host, "http://")
	cfg.Host = strings.TrimPrefix(cfg.Host, "https://")

	client, err := Connect(cfg)
	if err != nil {
		log.Fatalf("Error conectando al SFTP: %v", err)
	}

	defer client.Close()

	fmt.Printf(
		"Conectado. Listando %q:\n",
		cfg.RemoteDir,
	)

	files, err := client.ListRootFiles()
	if err != nil {
		log.Fatalf(
			"Error listando archivos: %v",
			err,
		)
	}

	for _, file := range files {

		if file.IsDir() {
			fmt.Printf(
				"  drwx------ %10d  %s/\n",
				file.Size(),
				file.Name(),
			)
			continue
		}

		fmt.Printf(
			"  -rw------- %10d  %s\n",
			file.Size(),
			file.Name(),
		)
	}

	fmt.Println()

	// =========================================================
	// PROCESAR ARCHIVOS
	// =========================================================

	for _, file := range files {

		// Ignorar carpetas
		if file.IsDir() {
			continue
		}

		// Solo procesar archivos
		fileName := file.Name()

		localPath := filepath.Join(
			"downloads",
			fileName,
		)

		fmt.Printf(
			"Procesando: %s\n",
			fileName,
		)

		// =====================================================
		// DESCARGAR Y MOVER A PROCESSED
		// =====================================================

		err := client.DownloadAndMove(
			fileName,
			localPath,
		)

		if err != nil {

			log.Printf(
				"ERROR procesando %s: %v",
				fileName,
				err,
			)

			// Importante:
			// Si falla la descarga o el MOVE,
			// continuamos con el siguiente archivo.
			continue
		}

		fmt.Printf(
			"Descargado y movido a PROCESSED: %s -> %s\n",
			fileName,
			localPath,
		)
	}

	fmt.Println()
	fmt.Println("Proceso terminado.")
}

// =============================================================
// CONEXIÓN SFTP
// =============================================================

func Connect(cfg Config) (*Client, error) {

	log.Printf(
		"[sftp] conectando host=%s port=%s user=%s remote_dir=%s processed_dir=%s",
		cfg.Host,
		cfg.Port,
		cfg.User,
		cfg.RemoteDir,
		cfg.ProcessedDir,
	)

	authMethods, err := buildAuthMethods(cfg)
	if err != nil {
		return nil, err
	}

	sshCfg := &ssh.ClientConfig{
		User: cfg.User,
		Auth: authMethods,

		// Para producción sería mejor validar
		// el host key real del servidor.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),

		Timeout: 30 * time.Second,
	}

	addr := net.JoinHostPort(
		cfg.Host,
		cfg.Port,
	)

	// =========================================================
	// TCP
	// =========================================================

	netConn, err := net.DialTimeout(
		"tcp",
		addr,
		30*time.Second,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"tcp dial %s: %w",
			addr,
			err,
		)
	}

	if tcp, ok := netConn.(*net.TCPConn); ok {

		_ = tcp.SetKeepAlive(true)

		_ = tcp.SetKeepAlivePeriod(
			30 * time.Second,
		)
	}

	// =========================================================
	// SSH
	// =========================================================

	sshClientConn, chans, reqs, err := ssh.NewClientConn(
		netConn,
		addr,
		sshCfg,
	)

	if err != nil {

		_ = netConn.Close()

		return nil, fmt.Errorf(
			"ssh handshake %s: %w",
			addr,
			err,
		)
	}

	conn := ssh.NewClient(
		sshClientConn,
		chans,
		reqs,
	)

	log.Printf(
		"[sftp] SSH conectado: %s",
		addr,
	)

	// =========================================================
	// SFTP
	// =========================================================

	raw, err := sftp.NewClient(conn)

	if err != nil {

		_ = conn.Close()

		return nil, fmt.Errorf(
			"sftp new client: %w",
			err,
		)
	}

	log.Printf(
		"[sftp] SFTP conectado: %s",
		addr,
	)

	stop := make(chan struct{})

	go sshKeepalive(
		conn,
		addr,
		stop,
	)

	client := &Client{
		raw:           raw,
		conn:          conn,
		baseDir:       cfg.RemoteDir,
		processedDir:  cfg.ProcessedDir,
		keepaliveStop: stop,
	}

	// =========================================================
	// VALIDAR DIRECTORIO PRINCIPAL
	// =========================================================

	info, err := client.raw.Stat(
		client.baseDir,
	)

	if err != nil {

		client.Close()

		return nil, fmt.Errorf(
			"stat remote directory %s: %w",
			client.baseDir,
			err,
		)
	}

	if !info.IsDir() {

		client.Close()

		return nil, fmt.Errorf(
			"remote path is not a directory: %s",
			client.baseDir,
		)
	}

	log.Printf(
		"[sftp] Stat OK: %s isDir=%v",
		client.baseDir,
		info.IsDir(),
	)

	// =========================================================
	// VALIDAR PROCESSED
	// =========================================================

	processedPath := pathJoin(
		client.baseDir,
		client.processedDir,
	)

	processedInfo, err := client.raw.Stat(
		processedPath,
	)

	if err != nil {

		client.Close()

		return nil, fmt.Errorf(
			"processed directory does not exist %s: %w",
			processedPath,
			err,
		)
	}

	if !processedInfo.IsDir() {

		client.Close()

		return nil, fmt.Errorf(
			"processed path is not a directory: %s",
			processedPath,
		)
	}

	log.Printf(
		"[sftp] PROCESSED OK: %s",
		processedPath,
	)

	return client, nil
}

// =============================================================
// AUTENTICACIÓN
// =============================================================

func buildAuthMethods(
	cfg Config,
) ([]ssh.AuthMethod, error) {

	var methods []ssh.AuthMethod

	if cfg.PrivateKeyPath != "" {

		keyBytes, err := os.ReadFile(
			cfg.PrivateKeyPath,
		)

		if err != nil {
			return nil, fmt.Errorf(
				"leer llave privada %s: %w",
				cfg.PrivateKeyPath,
				err,
			)
		}

		signer, err := ssh.ParsePrivateKey(
			keyBytes,
		)

		if err != nil {
			return nil, fmt.Errorf(
				"parsear llave privada %s: %w",
				cfg.PrivateKeyPath,
				err,
			)
		}

		methods = append(
			methods,
			ssh.PublicKeys(signer),
		)
	}

	if cfg.Password != "" {

		methods = append(
			methods,
			ssh.Password(cfg.Password),
		)
	}

	if len(methods) == 0 {

		return nil, fmt.Errorf(
			"sin métodos de autenticación SFTP configurados",
		)
	}

	return methods, nil
}

// =============================================================
// KEEPALIVE
// =============================================================

func sshKeepalive(
	conn *ssh.Client,
	addr string,
	stop <-chan struct{},
) {

	ticker := time.NewTicker(
		30 * time.Second,
	)

	defer ticker.Stop()

	misses := 0

	for {

		select {

		case <-stop:
			return

		case <-ticker.C:

			_, _, err := conn.SendRequest(
				"keepalive@openssh.com",
				true,
				nil,
			)

			if err == nil {

				misses = 0

				continue
			}

			misses++

			log.Printf(
				"[sftp] keepalive fallo addr=%s misses=%d err=%v",
				addr,
				misses,
				err,
			)

			if misses >= 2 {

				log.Printf(
					"[sftp] cerrando conexión por timeout",
				)

				_ = conn.Close()

				return
			}
		}
	}
}

// =============================================================
// CLOSE
// =============================================================

func (c *Client) Close() {

	if c == nil {
		return
	}

	if c.keepaliveStop != nil {

		select {

		case <-c.keepaliveStop:

		default:
			close(c.keepaliveStop)
		}
	}

	if c.raw != nil {
		_ = c.raw.Close()
	}

	if c.conn != nil {
		_ = c.conn.Close()
	}
}

// =============================================================
// LISTAR ARCHIVOS
// =============================================================

func (c *Client) ListRootFiles() (
	[]os.FileInfo,
	error,
) {

	if c.raw == nil {

		return nil, fmt.Errorf(
			"SFTP client is not connected",
		)
	}

	files, err := c.raw.ReadDir(
		c.baseDir,
	)

	if err != nil {

		return nil, fmt.Errorf(
			"read directory %s: %w",
			c.baseDir,
			err,
		)
	}

	return files, nil
}

// =============================================================
// DESCARGAR + MOVER
// =============================================================

func (c *Client) DownloadAndMove(
	name string,
	localPath string,
) error {

	if c.raw == nil {

		return fmt.Errorf(
			"SFTP client is not connected",
		)
	}

	remotePath := pathJoin(
		c.baseDir,
		name,
	)

	log.Printf(
		"[sftp] ========================================",
	)

	log.Printf(
		"[sftp] PROCESANDO: %s",
		name,
	)

	log.Printf(
		"[sftp] REMOTO: %s",
		remotePath,
	)

	log.Printf(
		"[sftp] LOCAL: %s",
		localPath,
	)

	// =========================================================
	// VERIFICAR ARCHIVO
	// =========================================================

	info, err := c.raw.Stat(
		remotePath,
	)

	if err != nil {

		return fmt.Errorf(
			"stat remote file %s: %w",
			remotePath,
			err,
		)
	}

	if info.IsDir() {

		return fmt.Errorf(
			"%s es un directorio",
			remotePath,
		)
	}

	expectedSize := info.Size()

	log.Printf(
		"[sftp] tamaño esperado: %d bytes",
		expectedSize,
	)

	// =========================================================
	// CREAR DIRECTORIO LOCAL
	// =========================================================

	localDir := filepath.Dir(
		localPath,
	)

	if err := os.MkdirAll(
		localDir,
		0755,
	); err != nil {

		return fmt.Errorf(
			"create local directory %s: %w",
			localDir,
			err,
		)
	}

	// =========================================================
	// ARCHIVO TEMPORAL
	// =========================================================

	tempPath := localPath + ".part"

	_ = os.Remove(tempPath)

	localFile, err := os.Create(
		tempPath,
	)

	if err != nil {

		return fmt.Errorf(
			"create temporary file %s: %w",
			tempPath,
			err,
		)
	}

	downloadOK := false

	defer func() {

		_ = localFile.Close()

		if !downloadOK {
			_ = os.Remove(tempPath)
		}

	}()

	// =========================================================
	// ABRIR REMOTO
	// =========================================================

	remoteFile, err := c.raw.Open(
		remotePath,
	)

	if err != nil {

		return fmt.Errorf(
			"open remote file %s: %w",
			remotePath,
			err,
		)
	}

	defer remoteFile.Close()

	// =========================================================
	// DESCARGAR
	// =========================================================

	log.Printf(
		"[sftp] descargando %s...",
		name,
	)

	written, err := io.Copy(
		localFile,
		remoteFile,
	)

	if err != nil {

		return fmt.Errorf(
			"download %s: %w",
			remotePath,
			err,
		)
	}

	// =========================================================
	// CERRAR LOCAL
	// =========================================================

	if err := localFile.Close(); err != nil {

		return fmt.Errorf(
			"close temporary file %s: %w",
			tempPath,
			err,
		)
	}

	// =========================================================
	// VALIDAR TAMAÑO
	// =========================================================

	if written != expectedSize {

		return fmt.Errorf(
			"incomplete download: expected=%d actual=%d",
			expectedSize,
			written,
		)
	}

	// =========================================================
	// RENOMBRAR ARCHIVO LOCAL
	// =========================================================

	if err := os.Rename(
		tempPath,
		localPath,
	); err != nil {

		return fmt.Errorf(
			"rename %s -> %s: %w",
			tempPath,
			localPath,
			err,
		)
	}

	downloadOK = true

	log.Printf(
		"[sftp] DESCARGA OK: %s -> %s",
		name,
		localPath,
	)

	// =========================================================
	// AHORA MOVER A PROCESSED
	// =========================================================

	log.Printf(
		"[sftp] MOVIENDO A PROCESSED: %s",
		name,
	)

	processedPath, err := c.MoveToProcessed(
		name,
	)

	if err != nil {

		return fmt.Errorf(
			"download succeeded but move to PROCESSED failed: %w",
			err,
		)
	}

	log.Printf(
		"[sftp] MOVE OK: %s -> %s",
		name,
		processedPath,
	)

	log.Printf(
		"[sftp] PROCESAMIENTO COMPLETO: %s",
		name,
	)

	log.Printf(
		"[sftp] ========================================",
	)

	return nil
}

// =============================================================
// MOVER A PROCESSED
// =============================================================

func (c *Client) MoveToProcessed(
	name string,
) (string, error) {

	return c.MoveToFolder(
		name,
		c.processedDir,
	)
}

// =============================================================
// MOVER A CARPETA
// =============================================================

func (c *Client) MoveToFolder(
	name string,
	folder string,
) (string, error) {

	src := pathJoin(
		c.baseDir,
		name,
	)

	dstDir := pathJoin(
		c.baseDir,
		folder,
	)

	log.Printf(
		"[sftp] MOVE FROM: %s",
		src,
	)

	log.Printf(
		"[sftp] MOVE TO:   %s",
		dstDir,
	)

	// =========================================================
	// VERIFICAR ORIGEN
	// =========================================================

	srcInfo, err := c.raw.Stat(
		src,
	)

	if err != nil {

		return "", fmt.Errorf(
			"source file does not exist %s: %w",
			src,
			err,
		)
	}

	if srcInfo.IsDir() {

		return "", fmt.Errorf(
			"source is a directory: %s",
			src,
		)
	}

	// =========================================================
	// VERIFICAR DESTINO
	// =========================================================

	dstInfo, err := c.raw.Stat(
		dstDir,
	)

	if err != nil {

		return "", fmt.Errorf(
			"destination directory does not exist %s: %w",
			dstDir,
			err,
		)
	}

	if !dstInfo.IsDir() {

		return "", fmt.Errorf(
			"destination is not a directory: %s",
			dstDir,
		)
	}

	// =========================================================
	// RESOLVER NOMBRE
	// =========================================================

	dstName, err := resolveDestName(
		c.raw,
		dstDir,
		name,
	)

	if err != nil {

		return "", fmt.Errorf(
			"resolve destination name: %w",
			err,
		)
	}

	dst := pathJoin(
		dstDir,
		dstName,
	)

	log.Printf(
		"[sftp] RENAME: %s -> %s",
		src,
		dst,
	)

	// =========================================================
	// SFTP RENAME = MOVE
	// =========================================================

	err = c.raw.Rename(
		src,
		dst,
	)

	if err != nil {

		return "", fmt.Errorf(
			"rename/move %s -> %s: %w",
			src,
			dst,
			err,
		)
	}

	// =========================================================
	// VERIFICAR QUE EXISTE EN DESTINO
	// =========================================================

	_, err = c.raw.Stat(
		dst,
	)

	if err != nil {

		return "", fmt.Errorf(
			"move completed but destination cannot be verified: %s: %w",
			dst,
			err,
		)
	}

	log.Printf(
		"[sftp] MOVE SUCCESS: %s -> %s",
		src,
		dst,
	)

	return dst, nil
}

// =============================================================
// RESOLVER COLISIONES
// =============================================================

func resolveDestName(
	client *sftp.Client,
	dstDir string,
	name string,
) (string, error) {

	originalPath := pathJoin(
		dstDir,
		name,
	)

	_, err := client.Stat(
		originalPath,
	)

	if err != nil {

		if os.IsNotExist(err) {
			return name, nil
		}

		return "", err
	}

	ext := filepath.Ext(
		name,
	)

	base := strings.TrimSuffix(
		name,
		ext,
	)

	ts := time.Now().
		UTC().
		Format("20060102150405")

	// =========================================================
	// nombre-20260811173000.xlsx
	// =========================================================

	candidate := fmt.Sprintf(
		"%s-%s%s",
		base,
		ts,
		ext,
	)

	_, err = client.Stat(
		pathJoin(dstDir, candidate),
	)

	if err != nil {

		if os.IsNotExist(err) {
			return candidate, nil
		}

		return "", err
	}

	// =========================================================
	// nombre-20260811173000-1.xlsx
	// =========================================================

	for i := 1; i < 1000; i++ {

		alternative := fmt.Sprintf(
			"%s-%s-%d%s",
			base,
			ts,
			i,
			ext,
		)

		_, err := client.Stat(
			pathJoin(
				dstDir,
				alternative,
			),
		)

		if err != nil {

			if os.IsNotExist(err) {
				return alternative, nil
			}

			return "", err
		}
	}

	return fmt.Sprintf(
		"%s-%s-%d%s",
		base,
		ts,
		time.Now().UnixNano(),
		ext,
	), nil
}

// =============================================================
// PATH JOIN
// =============================================================

func pathJoin(
	parts ...string,
) string {

	return filepath.ToSlash(
		filepath.Join(parts...),
	)
}

// =============================================================
// ENV
// =============================================================

func getenv(
	key string,
	def string,
) string {

	if value := os.Getenv(key); value != "" {
		return value
	}

	return def
}
