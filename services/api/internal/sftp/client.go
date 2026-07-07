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
	raw           *sftp.Client
	conn          *ssh.Client
	baseDir       string
	keepaliveStop chan struct{}
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

	// Dial TCP nosotros para activar keepalive del OS. Sin esto, si el peer muere silenciosamente
	// (cable, corte, servidor cae), lecturas SFTP posteriores bloquean por horas (~2h default de
	// keepalive del kernel Linux) y el worker queda colgado sin panicar.
	netConn, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		log.Printf("[sftp] error en tcp dial addr=%s err=%v", addr, err)
		return nil, fmt.Errorf("tcp dial %s: %w", addr, err)
	}
	if tcp, ok := netConn.(*net.TCPConn); ok {
		_ = tcp.SetKeepAlive(true)
		_ = tcp.SetKeepAlivePeriod(30 * time.Second)
	}

	sshClientConn, chans, reqs, err := ssh.NewClientConn(netConn, addr, sshCfg)
	if err != nil {
		_ = netConn.Close()
		log.Printf("[sftp] error en ssh handshake addr=%s err=%v", addr, err)
		return nil, fmt.Errorf("ssh handshake %s: %w", addr, err)
	}
	conn := ssh.NewClient(sshClientConn, chans, reqs)
	log.Printf("[sftp] conexión SSH establecida addr=%s", addr)

	raw, err := sftp.NewClient(conn)
	if err != nil {
		_ = conn.Close()
		log.Printf("[sftp] error creando cliente SFTP addr=%s err=%v", addr, err)
		return nil, fmt.Errorf("sftp new client: %w", err)
	}
	log.Printf("[sftp] cliente SFTP listo addr=%s", addr)

	// Keepalive SSH periódico: si el peer no responde en ~30 s dos veces seguidas, cerramos la
	// conexión para que cualquier I/O bloqueado (io.Copy, Open, ReadDir) devuelva error en vez
	// de esperar indefinido.
	stop := make(chan struct{})
	go sshKeepalive(conn, addr, stop)

	return &Client{raw: raw, conn: conn, baseDir: cfg.RemoteDir, keepaliveStop: stop}, nil
}

// sshKeepalive envía requests keepalive@openssh.com cada 30 s. Si dos consecutivos fallan,
// cierra la conexión para desbloquear cualquier operación SFTP colgada. Termina cuando el
// consumidor cierra el canal stop (típicamente desde Close).
func sshKeepalive(conn *ssh.Client, addr string, stop <-chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	misses := 0
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			_, _, err := conn.SendRequest("keepalive@openssh.com", true, nil)
			if err == nil {
				misses = 0
				continue
			}
			misses++
			log.Printf("[sftp] keepalive fallo addr=%s misses=%d err=%v", addr, misses, err)
			if misses >= 2 {
				log.Printf("[sftp] keepalive: cerrando conexión colgada addr=%s", addr)
				_ = conn.Close()
				return
			}
		}
	}
}

func (c *Client) Close() {
	if c.keepaliveStop != nil {
		select {
		case <-c.keepaliveStop:
			// ya cerrado
		default:
			close(c.keepaliveStop)
		}
	}
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
