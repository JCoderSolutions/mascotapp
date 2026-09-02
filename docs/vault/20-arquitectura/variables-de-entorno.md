# Variables de entorno del API

> **Por qué existe este archivo.** `.env.example` es la plantilla versionada y
> canónica, pero la regla `Read(./.env.*)` de `.claude/settings.json` la vuelve
> **ilegible para los agentes**. Un agente en sesión fría no puede descubrir qué
> variables existen. Este documento es el espejo legible.
>
> Si agregás una variable, tocá **los dos** archivos.

Alcance: lo que lee `apps/api/internal/config/config.go`. Los servicios del
compose local (Postgres, MinIO) tienen sus propias variables en
`docker-compose.yml`.

| Variable | Requerida | Por defecto | Valores |
|---|---|---|---|
| `PORT` | no | `8080` | entero entre 1 y 65535 |
| `ENV` | no | `development` | `development` · `staging` · `production` |
| `LOG_LEVEL` | no | `info` | `debug` · `info` · `warn` · `error` (un valor desconocido cae a `info`) |
| `DATABASE_URL` | sí en despliegue | vacío | cadena de conexión de Postgres, con `sslmode=verify-full` |
| `TRUSTED_PROXIES` | **sí en despliegue** | vacío | lista separada por comas de CIDR o IPs |

Una variable presente pero inválida es un **error de arranque**, no un
_warning_. El proceso no levanta con una configuración a medias.

## `TRUSTED_PROXIES` — la que hay que entender antes de desplegar

Define qué direcciones son infraestructura propia, y con eso decide si el header
`X-Forwarded-For` de una petición puede creerse. Sin ella, la resolución de IP
de cliente (`internal/httpapi/clientip.go`, tarea T-00-022) devuelve siempre el
peer TCP.

```
# un rango
TRUSTED_PROXIES=10.0.0.0/8

# varios
TRUSTED_PROXIES=10.0.0.0/8,192.168.0.0/16

# una IP suelta también vale: se toma como /32 (o /128 en IPv6)
TRUSTED_PROXIES=10.0.0.1
```

**Dejarla vacía es seguro, pero no es correcto en producción.** Vacía significa
"no confiar en ningún proxy", así que toda petición resuelve al peer TCP —que
detrás de Cloud Run es el front-end de Google, no el usuario. Nunca devuelve una
dirección falsificada, pero:

- el rate limiting por IP de Fase 02 mete a **todos** los usuarios en el mismo
  bucket, y
- el `ip_hash` del audit log guarda siempre la misma dirección.

Falla cerrada, no abierta. Pero queda inútil.

Fijar los rangos reales es un arrastre explícito en [[FASE-11]]. El valor debe
determinarse **contra la documentación vigente de Google al momento de
desplegar** —no de memoria, no de este documento— y verificarse contra tráfico
real: una petición desde una IP conocida tiene que resolver a esa IP.

Si el despliegue termina detrás de un Global External ALB, los rangos son los
del balanceador, no los del front-end de Cloud Run. El algoritmo soporta ambas
topologías sin tocar código; solo cambia esta variable.
