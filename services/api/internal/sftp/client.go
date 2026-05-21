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
	Host      string
	Port      string
	User      string
	Password  string
	RemoteDir string
}

type Client struct {
	raw     *sftp.Client
	conn    *ssh.Client
	baseDir string
}

func NewFromEnv() (Config, error) {
	cfg := Config{
		Host:      getenv("SFTP_HOST", "192.168.46.101"),
		Port:      getenv("SFTP_PORT", "2222"),
		User:      getenv("SFTP_USER", "usuario_ftp"),
		Password:  getenv("SFTP_PASSWORD", "BuskS3g26*"),
		RemoteDir: getenv("SFTP_REMOTE_DIR", "."),
	}
	cfg.Host = strings.TrimPrefix(strings.TrimPrefix(cfg.Host, "sftp://"), "http://")
	if cfg.Password == "" {
		return Config{}, fmt.Errorf("missing SFTP_PASSWORD")
	}
	return cfg, nil
}

func Connect(cfg Config) (*Client, error) {
	log.Printf("[sftp] iniciando conexión host=%s port=%s user=%s remote_dir=%s", cfg.Host, cfg.Port, cfg.User, cfg.RemoteDir)
	sshCfg := &ssh.ClientConfig{
		User: cfg.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(cfg.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}
	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	conn, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		log.Printf("[sftp] error en ssh dial addr=%s err=%v", addr, err)
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}
	log.Printf("[sftp] conexión SSH establecida addr=%s", addr)
	raw, err := sftp.NewClient(conn)
	if err != nil {
		_ = conn.Close()
		log.Printf("[sftp] error creando cliente SFTP addr=%s err=%v", addr, err)
		return nil, fmt.Errorf("sftp new client: %w", err)
	}
	log.Printf("[sftp] cliente SFTP listo addr=%s", addr)
	return &Client{raw: raw, conn: conn, baseDir: cfg.RemoteDir}, nil
}

func (c *Client) Close() {
	_ = c.raw.Close()
	_ = c.conn.Close()
}

func (c *Client) ListRootFiles() ([]os.FileInfo, error) {
	return c.raw.ReadDir(c.baseDir)
}

func (c *Client) OpenRelative(name string) (io.ReadCloser, error) {
	return c.raw.Open(path.Join(c.baseDir, name))
}

func (c *Client) MoveToFolder(name, folder string) (string, error) {
	src := path.Join(c.baseDir, name)
	dstDir := path.Join(c.baseDir, folder)
	if err := c.raw.MkdirAll(dstDir); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dstDir, err)
	}
	dst := path.Join(dstDir, name)
	if err := c.raw.Rename(src, dst); err != nil {
		return "", fmt.Errorf("rename %s -> %s: %w", src, dst, err)
	}
	return dst, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
