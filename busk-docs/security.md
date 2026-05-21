# Seguridad y Control de Acceso

Dado que Busk Seguros procesa archivos que contienen información sensible sobre pólizas de asegurados e información financiera y de deudores (como reportes de MAPFRE y Bolívar), todos los accesos programáticos están resguardados por un esquema de seguridad perimetral.

## Modelo de Autenticación (JWT Bearer)

El sistema de la API REST implementa un modelo de seguridad basado en tokens temporales, utilizando el estándar **JSON Web Token (JWT)**, alineado con las especificaciones técnicas requeridas.

### 1. Interceptor / Auth Middleware
Toda petición entrante que busque consultar el "stock" o disparar comandos de control (`/process/scan`) debe pasar por un Middleware escrito en Go, el cual se asegura de:
- Verificar que el Token posea una firma válida emitida por el backend.
- Validar que el Token no esté expirado.
- (Opcional) Confirmar que el rol del usuario contenga los permisos (Scopes) necesarios para dicha operación.

### 2. Tabla de Credenciales y Roles (`APP_USER`)
El almacenamiento interno de usuarios y contraseñas (hasheadas) se realiza mediante la entidad de base de datos dedicada.

| Tabla | Campo | Tipo | Notas / Negocio |
|-------|-------|------|-----------------|
| **APP_USER** | `id` | UUID (PK) | Identificador interno |
| | `email` | STRING (UK) | Credencial de acceso (Ej. operaciones@busk.com) |
| | `password_hash` | STRING | Criptografía irreversible (Bcrypt / Scrypt) |
| | `role` | STRING | `ADMIN`, `VIEWER`, `SYSTEM` |
| | `is_active` | BOOLEAN | Corta acceso instantáneamente sin borrar registro |
| | `last_login_at` | TIMESTAMP | Registro de auditoría. |

---

## Flujo de Autorización

### 1. Intercambio de Credenciales (Login)
El integrador de software o portal web frontal (Frontend) consume el endpoint de inicio de sesión documentado en las [Firmas de API](api.md).

```http
POST /api/v1/auth/login
Content-Type: application/json

{
  "email": "asesor@busk.com",
  "password": "SuperSecretPassword123"
}
```

Si es correcto, la API devuelve el `access_token` temporal.

### 2. Consumo de API (Autorización)
El cliente debe proveer dicho `access_token` inyectándolo en las cabeceras estándar de Authorización (`Authorization`) en cada petición subsecuente.

```http
GET /api/v1/stock/MAPFRE_VIDA_XYZ/policies HTTP/1.1
Authorization: Bearer eyJhbGciOiJIUzI...
```

### Respuestas de Seguridad (RFC 7807)
Alineado con Zalando Guidelines, un error de autenticación se presentará mediante `application/problem+json`:

```json
{
  "type": "https://docs.buskseguros.com/errors/unauthorized",
  "title": "Unauthorized Request",
  "status": 401,
  "detail": "El token JWT provisto ha expirado o es inválido."
}
```
