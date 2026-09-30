// Secretos por process.env a propósito: este archivo está versionado, así que nunca debe
// llevar credenciales reales dentro. Exportarlas antes de arrancar PM2, p.ej.
//
//   export MYSQL_DSN='root:TU_CONTRASEÑA@tcp(localhost:3306)/busk?parseTime=true&multiStatements=true'
//   export SFTP_PASSWORD='...'
//   export SENDGRID_API_KEY='SG....'
//   pm2 restart busk-api --update-env
//
// Ojo: la app carga el .env con godotenv.Load, que NO pisa variables ya presentes en el
// entorno. Por eso PM2 gana sobre el .env: si aquí se define una variable con un valor
// equivocado (o un placeholder), ese valor manda y el .env no lo corrige.
module.exports = {
  apps: [
    {
      name: 'busk-api',
      script: './services/api/busk-api',
      instances: 1,
      exec_mode: 'fork',
      restart_delay: 3000,
      max_memory_restart: '500M',
      env: {
        MYSQL_DSN: process.env.MYSQL_DSN || '',
        PROCESSOR_WORKERS: '2',
        PROCESSOR_READ_FULL_FILE_ON_ROW_ERRORS: 'false',
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
      },
      error_file: './logs/error.log',
      out_file: './logs/out.log',
      log_file: './logs/combined.log',
      time_format: 'YYYY-MM-DD HH:mm:ss Z',
    },
  ],
};
