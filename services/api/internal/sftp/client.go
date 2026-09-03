package sftpclient

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path"
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
	PrivateKeyPath string
	ProcessedDir   string
}

type Client struct {
	raw           *sftp.Client
	conn          *ssh.Client
	baseDir       string
	processedDir  string
	keepaliveStop chan struct{}
}

func NewFromEnv() (Config, error) {
	cfg := Config{
		Host:           getenv("SFTP_HOST", "192.168.46.101"),
		Port:           getenv("SFTP_PORT", "2222"),
		User:           getenv("SFTP_USER", "usuario_ftp"),
		Password:       getenv("SFTP_PASSWORD", ""),
		RemoteDir:      getenv("SFTP_REMOTE_DIR", "."),
		PrivateKeyPath: getenv("SFTP_PRIVATE_KEY_PATH", ""),
		ProcessedDir:   getenv("SFTP_PROCESSED_DIR", "PROCESSED"),
	}

	cfg.Host = strings.TrimPrefix(cfg.Host, "sftp://")
	cfg.Host = strings.TrimPrefix(cfg.Host, "http://")
	cfg.Host = strings.TrimPrefix(cfg.Host, "https://")

	if cfg.Password == "" && cfg.PrivateKeyPath == "" {
		return Config{}, fmt.Errorf(
			"missing SFTP_PASSWORD or SFTP_PRIVATE_KEY_PATH",
		)
	}

	return cfg, nil
}

func Connect(cfg Config) (*Client, error) {
	log.Printf(
		"[sftp] iniciando conexión host=%s port=%s user=%s remote_dir=%s processed_dir=%s pubkey=%v pass_len=%d",
		cfg.Host,
		cfg.Port,
		cfg.User,
		cfg.RemoteDir,
		cfg.ProcessedDir,
		cfg.PrivateKeyPath != "",
		len(cfg.Password),
	)

	authMethods, err := buildAuthMethods(cfg)
	if err != nil {
		return nil, err
	}

	sshCfg := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}

	addr := net.JoinHostPort(cfg.Host, cfg.Port)

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
		_ = tcp.SetKeepAlivePeriod(30 * time.Second)
	}

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

	raw, err := sftp.NewClient(conn)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf(
			"sftp new client: %w",
			err,
		)
	}

	stop := make(chan struct{})

	go sshKeepalive(
		conn,
		addr,
		stop,
	)

	return &Client{
		raw:           raw,
		conn:          conn,
		baseDir:       cfg.RemoteDir,
		processedDir:  cfg.ProcessedDir,
		keepaliveStop: stop,
	}, nil
}

func buildAuthMethods(cfg Config) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	if cfg.PrivateKeyPath != "" {
		keyBytes, err := os.ReadFile(cfg.PrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf(
				"leer llave privada %s: %w",
				cfg.PrivateKeyPath,
				err,
			)
		}

		signer, err := ssh.ParsePrivateKey(keyBytes)
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
			"sin métodos de auth SFTP configurados",
		)
	}

	return methods, nil
}

func sshKeepalive(
	conn *ssh.Client,
	addr string,
	stop <-chan struct{},
) {
	ticker := time.NewTicker(30 * time.Second)
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
				_ = conn.Close()
				return
			}
		}
	}
}

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

func (c *Client) ListRootFiles() ([]os.FileInfo, error) {
	if c.raw == nil {
		return nil, fmt.Errorf(
			"SFTP client is not connected",
		)
	}

	return c.raw.ReadDir(c.baseDir)
}

func (c *Client) OpenRelative(name string) (io.ReadCloser, error) {
	if c.raw == nil {
		return nil, fmt.Errorf(
			"SFTP client is not connected",
		)
	}

	filePath := path.Join(
		c.baseDir,
		name,
	)

	return c.raw.Open(filePath)
}

func (c *Client) DeleteFile(name string) error {
	if c.raw == nil {
		return fmt.Errorf(
			"SFTP client is not connected",
		)
	}

	filePath := path.Join(
		c.baseDir,
		name,
	)

	return c.raw.Remove(filePath)
}

func (c *Client) RenameInPlace(
	name string,
	prefix string,
) (string, error) {

	if c.raw == nil {
		return "", fmt.Errorf(
			"SFTP client is not connected",
		)
	}

	ts := time.Now().
		UTC().
		Format("20060102150405")

	newName := fmt.Sprintf(
		"%s-%s-%s",
		prefix,
		ts,
		name,
	)

	src := path.Join(
		c.baseDir,
		name,
	)

	dst := path.Join(
		c.baseDir,
		newName,
	)

	if err := c.raw.Rename(src, dst); err != nil {
		return "", fmt.Errorf(
			"rename in place %s -> %s: %w",
			src,
			dst,
			err,
		)
	}

	return newName, nil
}

func (c *Client) DownloadAndMove(
	name string,
	localPath string,
) error {

	if c.raw == nil {
		return fmt.Errorf(
			"SFTP client is not connected",
		)
	}

	if strings.TrimSpace(name) == "" {
		return fmt.Errorf(
			"file name cannot be empty",
		)
	}

	if strings.TrimSpace(localPath) == "" {
		return fmt.Errorf(
			"local path cannot be empty",
		)
	}

	src := path.Join(
		c.baseDir,
		name,
	)

	log.Printf(
		"[sftp] descargando remote=%q local=%q",
		src,
		localPath,
	)

	info, err := c.raw.Stat(src)
	if err != nil {
		return fmt.Errorf(
			"source file does not exist %s: %w",
			src,
			err,
		)
	}

	if info.IsDir() {
		return fmt.Errorf(
			"source %s is a directory",
			src,
		)
	}

	localDir := path.Dir(localPath)

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

	tempPath := localPath + ".part"

	localFile, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf(
			"create local file %s: %w",
			tempPath,
			err,
		)
	}

	success := false

	defer func() {
		_ = localFile.Close()

		if !success {
			_ = os.Remove(tempPath)
		}
	}()

	remoteFile, err := c.raw.Open(src)
	if err != nil {
		return fmt.Errorf(
			"open remote file %s: %w",
			src,
			err,
		)
	}

	defer remoteFile.Close()

	written, err := io.Copy(
		localFile,
		remoteFile,
	)

	if err != nil {
		return fmt.Errorf(
			"download %s: %w",
			src,
			err,
		)
	}

	if err := localFile.Close(); err != nil {
		return fmt.Errorf(
			"close local file %s: %w",
			tempPath,
			err,
		)
	}

	if err := os.Rename(
		tempPath,
		localPath,
	); err != nil {
		return fmt.Errorf(
			"rename downloaded file %s -> %s: %w",
			tempPath,
			localPath,
			err,
		)
	}

	log.Printf(
		"[sftp] descarga completada remote=%q local=%q bytes=%d",
		src,
		localPath,
		written,
	)

	_, err = c.MoveToFolder(
		name,
		c.processedDir,
	)

	if err != nil {
		return fmt.Errorf(
			"download succeeded but move to processed failed for %s: %w",
			name,
			err,
		)
	}

	success = true

	log.Printf(
		"[sftp] archivo movido a PROCESSED file=%q",
		name,
	)

	return nil
}

func (c *Client) MoveToFolder(
	name string,
	folder string,
) (string, error) {

	if c.raw == nil {
		return "", fmt.Errorf(
			"SFTP client is not connected",
		)
	}

	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf(
			"file name cannot be empty",
		)
	}

	if strings.TrimSpace(folder) == "" {
		return "", fmt.Errorf(
			"destination folder cannot be empty",
		)
	}

	src := path.Join(
		c.baseDir,
		name,
	)

	dstDir := path.Join(
		c.baseDir,
		folder,
	)

	log.Printf(
		"[sftp] MOVE src=%q dstDir=%q",
		src,
		dstDir,
	)

	srcInfo, err := c.raw.Stat(src)
	if err != nil {
		return "", fmt.Errorf(
			"source file does not exist %s: %w",
			src,
			err,
		)
	}

	if srcInfo.IsDir() {
		return "", fmt.Errorf(
			"source %s is a directory",
			src,
		)
	}

	// MkdirAll idempotente: si ERROR/ o PROCESSED/ no existen, crearlos antes del Stat.
	// Sin esto el MOVE fallaba con "destination folder does not exist" y caía al fallback
	// RenameInPlace, que solo etiqueta el archivo pero lo deja en la raíz del SFTP.
	if err := c.raw.MkdirAll(dstDir); err != nil {
		log.Printf(
			"[sftp] MkdirAll dstDir=%q err=%v — se intenta Stat igualmente",
			dstDir,
			err,
		)
	}

	dstInfo, err := c.raw.Stat(dstDir)
	if err != nil {
		return "", fmt.Errorf(
			"destination folder does not exist %s: %w",
			dstDir,
			err,
		)
	}

	if !dstInfo.IsDir() {
		return "", fmt.Errorf(
			"destination %s is not a directory",
			dstDir,
		)
	}

	dstName, err := resolveDestName(
		c.raw,
		dstDir,
		name,
	)

	if err != nil {
		return "", err
	}

	dst := path.Join(
		dstDir,
		dstName,
	)

	if err := c.raw.Rename(
		src,
		dst,
	); err != nil {

		log.Printf(
			"[sftp] MOVE FAILED src=%q dst=%q err=%v",
			src,
			dst,
			err,
		)

		return "", fmt.Errorf(
			"move %s -> %s: %w",
			src,
			dst,
			err,
		)
	}

	log.Printf(
		"[sftp] MOVE SUCCESS src=%q dst=%q",
		src,
		dst,
	)

	return dst, nil
}

func resolveDestName(
	client *sftp.Client,
	dstDir string,
	name string,
) (string, error) {

	originalPath := path.Join(
		dstDir,
		name,
	)

	_, err := client.Stat(originalPath)

	if err != nil {
		if os.IsNotExist(err) {
			return name, nil
		}

		return "", fmt.Errorf(
			"cannot check destination %s: %w",
			originalPath,
			err,
		)
	}

	ext := path.Ext(name)

	base := strings.TrimSuffix(
		name,
		ext,
	)

	ts := time.Now().
		UTC().
		Format("20060102150405")

	candidate := fmt.Sprintf(
		"%s-%s%s",
		base,
		ts,
		ext,
	)

	_, err = client.Stat(
		path.Join(dstDir, candidate),
	)

	if err != nil {
		if os.IsNotExist(err) {
			return candidate, nil
		}

		return "", err
	}

	for i := 1; i < 1000; i++ {

		alternative := fmt.Sprintf(
			"%s-%s-%d%s",
			base,
			ts,
			i,
			ext,
		)

		_, err := client.Stat(
			path.Join(dstDir, alternative),
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

func getenv(
	key string,
	def string,
) string {

	if value := os.Getenv(key); value != "" {
		return value
	}

	return def
}

func maskPassword(password string) string {

	if len(password) <= 4 {
		return strings.Repeat(
			"*",
			len(password),
		)
	}

	return password[:2] +
		strings.Repeat(
			"*",
			len(password)-4,
		) +
		password[len(password)-2:]
}
