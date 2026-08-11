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
}

type Client struct {
	raw           *sftp.Client
	conn          *ssh.Client
	baseDir       string
	keepaliveStop chan struct{}
}

func NewFromEnv() (Config, error) {
	cfg := Config{
		Host:           getenv("SFTP_HOST", "192.168.46.101"),
		Port:           getenv("SFTP_PORT", "2222"),
		User:           getenv("SFTP_USER", "usuario_ftp"),
		Password:       getenv("SFTP_PASSWORD", "BuskS3g26*"),
		RemoteDir:      getenv("SFTP_REMOTE_DIR", "."),
		PrivateKeyPath: getenv("SFTP_PRIVATE_KEY_PATH", ""),
	}
	cfg.Host = strings.TrimPrefix(strings.TrimPrefix(cfg.Host, "sftp://"), "http://")
	if cfg.Password == "" && cfg.PrivateKeyPath == "" {
		return Config{}, fmt.Errorf("missing SFTP_PASSWORD or SFTP_PRIVATE_KEY_PATH")
	}
	return cfg, nil
}

func Connect(cfg Config) (*Client, error) {
	log.Printf("[sftp] iniciando conexión host=%s port=%s user=%s remote_dir=%s pubkey=%v pass_len=%d pass_hint=%q", cfg.Host, cfg.Port, cfg.User, cfg.RemoteDir, cfg.PrivateKeyPath != "", len(cfg.Password), maskPassword(cfg.Password))
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

// buildAuthMethods arma la lista de métodos SSH según cfg. Si hay una llave privada,
// va primero; el password queda siempre como fallback si está definido. No se usa
// keyboard-interactive porque cuando el server la rechaza sin mandar challenge, x/crypto/ssh
// aborta el handshake con "unexpected message type 51" en vez de reportar auth failure limpio.
func buildAuthMethods(cfg Config) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if cfg.PrivateKeyPath != "" {
		keyBytes, err := os.ReadFile(cfg.PrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("leer llave privada %s: %w", cfg.PrivateKeyPath, err)
		}
		signer, err := ssh.ParsePrivateKey(keyBytes)
		if err != nil {
			return nil, fmt.Errorf("parsear llave privada %s: %w", cfg.PrivateKeyPath, err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if cfg.Password != "" {
		methods = append(methods, ssh.Password(cfg.Password))
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("sin métodos de auth SFTP configurados")
	}
	return methods, nil
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

// DeleteFile borra el archivo del baseDir del SFTP. Se usa para archivos duplicados
// por SHA-256 que ya fueron manejados: no hay motivo para dejar copias residuales.
func (c *Client) DeleteFile(name string) error {
	return c.raw.Remove(path.Join(c.baseDir, name))
}

// RenameInPlace renombra el archivo dentro del mismo baseDir agregando un prefijo con
// timestamp UTC (formato "<prefix>-YYYYMMDDHHMMSS-<name>"). Fallback del MOVE a subcarpeta
// cuando los permisos del usuario SFTP no alcanzan para crear/escribir en ERROR|PROCESSED/
// pero sí permiten renombrar dentro del mismo directorio. Devuelve el nombre nuevo (sin path).
func (c *Client) RenameInPlace(name, prefix string) (string, error) {
	ts := time.Now().UTC().Format("20060102150405")
	newName := fmt.Sprintf("%s-%s-%s", prefix, ts, name)
	src := path.Join(c.baseDir, name)
	dst := path.Join(c.baseDir, newName)
	if err := c.raw.Rename(src, dst); err != nil {
		return "", fmt.Errorf("rename in place %s -> %s: %w", src, dst, err)
	}
	return newName, nil
}

func (c *Client) MoveToFolder(name, folder string) (string, error) {
	src := path.Join(c.baseDir, name)
	dstDir := path.Join(c.baseDir, folder)
	if err := c.raw.MkdirAll(dstDir); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dstDir, err)
	}
	// SSH_FXP_RENAME de SFTPv3 falla si el destino existe, aunque el usuario tenga permisos.
	// Si ya hay un archivo con el mismo nombre en la carpeta destino, insertamos un sufijo
	// timestamp antes de la extensión para preservar ambos y garantizar que el move nunca falle
	// por colisión.
	dst := path.Join(dstDir, resolveDestName(c.raw, dstDir, name))
	if err := c.raw.Rename(src, dst); err != nil {
		return "", fmt.Errorf("rename %s -> %s: %w", src, dst, err)
	}
	return dst, nil
}

// resolveDestName elige un nombre libre dentro de dstDir. Si name no colisiona lo devuelve tal
// cual; si colisiona, agrega -YYYYMMDDHHMMSS antes de la extensión y, si aún colisiona (mismo
// segundo, poco probable), agrega -N incremental.
func resolveDestName(c *sftp.Client, dstDir, name string) string {
	if _, err := c.Stat(path.Join(dstDir, name)); err != nil {
		return name
	}
	ext := path.Ext(name)
	base := strings.TrimSuffix(name, ext)
	ts := time.Now().UTC().Format("20060102150405")
	candidate := fmt.Sprintf("%s-%s%s", base, ts, ext)
	if _, err := c.Stat(path.Join(dstDir, candidate)); err != nil {
		return candidate
	}
	for i := 1; i < 1000; i++ {
		alt := fmt.Sprintf("%s-%s-%d%s", base, ts, i, ext)
		if _, err := c.Stat(path.Join(dstDir, alt)); err != nil {
			return alt
		}
	}
	return fmt.Sprintf("%s-%s-%d%s", base, ts, time.Now().UnixNano(), ext)
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// maskPassword devuelve una versión enmascarada para logs: primeros 2 y últimos 2 caracteres
// visibles, el resto como asteriscos. Solo para diagnóstico — nunca loguear el password real.
func maskPassword(p string) string {
	if len(p) <= 4 {
		return strings.Repeat("*", len(p))
	}
	return p[:2] + strings.Repeat("*", len(p)-4) + p[len(p)-2:]
}
