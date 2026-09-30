module.exports = {
  apps: [
    {
      name: 'busk-api',
      script: './services/api/busk-api',
      instances: 1,
      exec_mode: 'fork',
      restart_delay: 3000,
      // El host tiene 16 GB y el pico real del procesador ronda 700 MB con archivos de
      // ~30 MB. Con 500M PM2 mataba el proceso a mitad del mapeo, el archivo se quedaba en
      // la raíz del SFTP y el auto-esaneo de 5 min lo volvía a encolar en bucle infinito.
      // 2G deja margen de sobra para 2 workers concurrentes conservando la red de seguridad.
      max_memory_restart: '2G',
      env: {
        MYSQL_DSN: 'root:TU_CONTRASEÑA_AQUI@tcp(localhost:3306)/busk?parseTime=true&multiStatements=true',
        PROCESSOR_WORKERS: '2',
        // Seguir leyendo tras una fila inválida en vez de abortar el archivo entero.
        PROCESSOR_READ_FULL_FILE_ON_ROW_ERRORS: 'true',
        // Gate de archivo: con true se cargan las filas válidas y las problemáticas quedan
        // en MANUAL_REVIEW (documentadas en el informe) en vez de descartar el archivo
        // completo. Poner en false para volver al comportamiento histórico "todo o nada".
        PROCESSOR_IMPORT_WITH_REVIEW_ROWS: 'true',
        FILES_ARCHIVE_DIR: './services/api/data/files-archive',
        REPORTS_ARCHIVE_DIR: './services/api/data/reports-archive',
        SFTP_HOST: '192.168.46.101',
        SFTP_PORT: '2222',
        SFTP_USER: 'usuario_ftp',
        SFTP_PASSWORD: process.env.SFTP_PASSWORD || '',
        SFTP_REMOTE_DIR: '/',
        SENDGRID_API_KEY: process.env.SENDGRID_API_KEY || '',
        SENDGRID_FROM_EMAIL: 'alertas@buskseguros.com',
        SENDGRID_ERROR_TO_EMAILS: 'desarrollador@buskseguros.com,joaquin.anaya@buskseguros.com,sara.rubiano@buskseguros.com',
        API_PUBLIC_BASE_URL:'http://62.146.228.79'
      },
      error_file: './logs/error.log',
      out_file: './logs/out.log',
      log_file: './logs/combined.log',
      time_format: 'YYYY-MM-DD HH:mm:ss Z',
    },
  ],
};
