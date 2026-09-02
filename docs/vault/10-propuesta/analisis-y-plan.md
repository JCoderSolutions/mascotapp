# MascotApp — Análisis crítico y plan maestro de desarrollo

> Documento fuente: `propuesta.txt`
> Fecha: 2026-08-28
> Idioma: español (documento de proyecto). **Todo artefacto técnico — código, identificadores, columnas, endpoints, mensajes de commit, tests — en inglés.**

---

## Context

La propuesta plantea una plataforma para que refugios de animales publiquen perros en adopción, gestionen formularios dinámicos de adoptantes, muestren su identidad institucional (misión/visión/donaciones) y den transparencia total al adoptante. El requisito duro es **costo cero de despliegue** en la primera etapa.

El problema real que hay que resolver no es "un CRUD de perritos". Es:

1. **Multi-tenancy con aislamiento verificable** — varios refugios en una sola app, sin que uno pueda ver datos de otro, ni por error de código.
2. **Datos personales sensibles** — las solicitudes de adopción contienen domicilio, documento de identidad, referencias. Esto es PII regulada, no texto libre.
3. **Confianza** — publicar refugios sin verificar convierte la plataforma en vector de estafas de "cuota de adopción".
4. **Continuidad entre sesiones de agentes** — el trabajo lo ejecutan agentes que pierden contexto. Sin estado persistente y machine-readable, cada sesión reempieza.

Este documento entrega: análisis de los 3 roles solicitados, decisiones de arquitectura, modelo de datos, y un plan de fases/tareas con estado persistente que cualquier agente pueda retomar en frío.

---

## 1. Análisis crítico — tres roles

### 1.1 Rol: Líder Técnico / Arquitecto

**Fortalezas de la propuesta**
- Dominio acotado y bien delimitado. No es un marketplace genérico.
- La transparencia como principio de producto es una decisión de diseño correcta y define el modelo de datos.
- Elegir Go como norte para el backend es acertado: binario único, arranque en frío bajo, huella de memoria mínima. Encaja exactamente con hosting gratuito escalado a cero.

**Debilidades y riesgos — lo que hay que corregir antes de escribir código**

| # | Riesgo | Impacto | Mitigación exigida |
|---|--------|---------|--------------------|
| LT-1 | **"Gratis para siempre" es falso.** Dominio, correo transaccional, crecimiento de imágenes y generación de PDF tienen costo. | Migración de emergencia a mitad de camino. | Definir techo de costo explícito (~USD 20–40/año, dominio incluido) y un *plan de degradación* documentado por servicio. Nada de arquitectura acoplada a un proveedor. |
| LT-2 | **No hay verificación de refugios.** Cualquiera se registra, publica, y pide "cuota de adopción". | Riesgo reputacional y legal grave. La plataforma se vuelve vehículo de estafa. | Estado `pending_verification` obligatorio. Un refugio **no puede publicar** hasta ser verificado manualmente. Es requisito de MVP, no de fase 2. |
| LT-3 | **Donaciones.** Si el dinero pasa por una cuenta tuya, te conviertes en intermediario financiero. | Exposición legal y fiscal personal. | La plataforma **nunca procesa pagos**. Solo enlaza a enlaces de pago propios del refugio, verificados durante el onboarding. Cero alcance PCI, cero custodia de fondos. |
| LT-4 | **La adopción no es un formulario, es una máquina de estados.** La propuesta trata la solicitud como un envío único. | El producto queda como un buzón de formularios; los refugios lo abandonan. | Modelar `adoption_applications` con estados explícitos, historial auditable y asignación de responsable. |
| LT-5 | **Falta el ciclo de vida del animal.** ¿Qué pasa cuando se adopta, se devuelve, o fallece? | Datos corruptos, listados fantasma. | Estados de `pets` + `pet_status_history` inmutable desde el día 1. |
| LT-6 | Sin observabilidad, un despliegue gratuito es una caja negra. | Fallos silenciosos en producción. | Logs estructurados (`log/slog` JSON), `/healthz`, `/readyz`, y Sentry free tier desde Fase 0. |

**Qué agregar**
- ADRs (`Architecture Decision Records`) desde el primer día — decisiones fechadas e inmutables.
- Contrato OpenAPI como **fuente única de verdad**, generando tipos de servidor (Go) y cliente (TS). Un solo documento, dos lados tipados.
- Presupuesto de rendimiento explícito: LCP < 2.5 s en 3G simulada, respuesta p95 de API < 300 ms.

---

### 1.2 Rol: Especialista en desarrollo de aplicaciones (producto / frontend / UX)

**Fortalezas**
- La ficha rica del animal (condiciones, edad, fotos) es exactamente lo que convierte adopciones.
- Formularios dinámicos con control de layout es un requisito real: cada refugio pregunta cosas distintas.

**Debilidades y riesgos**

| # | Riesgo | Impacto | Mitigación exigida |
|---|--------|---------|--------------------|
| AD-1 | **Mobile-first no es opcional.** Los voluntarios cargan fotos desde el celular, en la calle, con mala conexión. | La herramienta no se usa. | PWA. Compresión de imagen en cliente **antes** de subir. Cola de subida tolerante a fallos con reintento. Esto es arquitectura, no maquillaje final. |
| AD-2 | **Las imágenes serán el 90% del costo y del rendimiento.** | Se revienta el free tier en meses. | Pipeline estricto: redimensionado en cliente → subida directa con URL prefirmada (no pasa por la API) → derivación server-side de 3 variantes (`thumb`/`card`/`full`) en AVIF con fallback WebP → servido por CDN. Límite duro por refugio. |
| AD-3 | **Búsqueda y filtrado es LA experiencia del adoptante**, y la propuesta no la menciona. | Catálogo inusable con más de 50 animales. | Atributos filtrables **como columnas tipadas**, no texto libre: tamaño, edad, nivel de energía, se lleva con niños/perros/gatos, necesidades especiales, ciudad. Definidos en el modelo desde Fase 1. |
| AD-4 | **Accesibilidad.** Parte del público adoptante es gente mayor. | Exclusión real de usuarios. | WCAG 2.2 AA como criterio de aceptación. Gestión de foco en formularios dinámicos (campos que aparecen/desaparecen rompen lectores de pantalla si no se anuncian). |
| AD-5 | **i18n retroactivo es un infierno.** | Refactor doloroso al primer refugio de otro país. | Estructura de claves de traducción desde Fase 0, aunque solo exista `es-MX`. |
| AD-6 | **EXIF en las fotos.** Las fotos de celular llevan GPS. Publicar la ubicación exacta de un refugio o del domicilio de un adoptante es una fuga real. | Fuga de privacidad. | Stripping de metadatos obligatorio en el pipeline de imagen. |

**Qué agregar**
- Vista previa del formulario en el constructor (WYSIWYG), o los refugios publicarán formularios rotos.
- Estados vacíos, de carga y de error diseñados explícitamente — no como afterthought.
- SEO real en el catálogo público (SSR o prerender + `JSON-LD`). Un perro que no aparece en Google no se adopta.
- Compartir en redes con Open Graph por animal. Es el canal #1 de adopción real.

---

### 1.3 Rol: Especialista en IA / Orquestación de agentes

**Fortalezas**
- Usar Engram como memoria organizacional es la decisión correcta para trabajo distribuido entre sesiones.
- Pedir curaduría ("solo lo importante") demuestra entender el problema real: la memoria sin filtro se vuelve ruido y degrada el recall.

**Debilidades y riesgos**

| # | Riesgo | Impacto | Mitigación exigida |
|---|--------|---------|--------------------|
| IA-1 | **"Guardar solo lo importante" sin rúbrica es subjetivo** — cada agente decide distinto. | Memoria inconsistente e inútil a los 2 meses. | Rúbrica de puntuación 0–5 explícita + cola de aprobación manual (detalle en §5). |
| IA-2 | **Estado de avance en prosa = pérdida de contexto.** Un agente nuevo no puede parsear "vamos por la mitad de auth". | Retrabajo, tareas duplicadas. | `PROJECT_STATE.md` con frontmatter machine-readable + IDs de tarea estables. |
| IA-3 | **Sin invariante de "una tarea en progreso"**, dos agentes tocan lo mismo. | Conflictos, trabajo perdido. | Invariante duro: como máximo **un** `[~]` en todo el tablero. |
| IA-4 | **Engram y Obsidian pueden divergir.** | Dos fuentes de verdad contradictorias. | Jerarquía clara: el repo (`docs/vault/`) es la verdad operativa; Engram es índice semántico de decisiones. Cada observación de Engram lleva el `topic_key` y el ID de tarea que la originó. |
| IA-5 | Riesgo de sobre-ingeniería de IA en el producto. | Se quema tiempo de MVP en features que nadie pidió. | La IA de producto va **post-v1**, en Fase 12, y nunca decide sobre personas. |
| IA-6 | **Modelos gratuitos sin criterio objetivo de aptitud.** "Tarea sencilla" es subjetivo, y los endpoints `:free` desaparecen sin aviso. | Retrabajo — exactamente lo que se quiere evitar. | Regla de verificabilidad automática + lista blanca/negra + piloto medido con criterio de muerte (§7.2). |

**Qué agregar — IA en el producto (Fase 11, no antes)**
- Borrador automático de descripción del animal a partir de fotos + atributos. El refugio edita y aprueba. Ahorra el trabajo que más odian los voluntarios.
- Detección de publicaciones duplicadas entre refugios.
- Priorización de solicitudes: **solo ordena y explica; jamás rechaza automáticamente.** Regla dura, escrita en el spec.

---

## 2. Stack recomendado

La propuesta delega explícitamente la elección ("*tecnologías de back front y demás serán propuestas por ti*"). Decisión tomada:

### Backend
| Componente | Elección | Razón |
|---|---|---|
| Lenguaje | **Go 1.23+** | Binario único, arranque en frío ~300 ms, ~20 MB RAM en reposo. Es lo que hace viable el escalado a cero gratuito. |
| Router | `chi` v5 | `net/http` estándar, sin magia, middleware componible. |
| Acceso a datos | `sqlc` + `pgx/v5` | SQL escrito a mano, structs de Go generadas. Sin ORM, sin sorpresas en el plan de ejecución. |
| Migraciones | `goose` | Migraciones SQL versionadas, up/down, embebibles en el binario. |
| Contrato API | OpenAPI 3.1 + `oapi-codegen` | Fuente única de verdad. Genera interfaces de servidor. |
| PDF | `maroto/v2` | Go puro, determinista, sin navegador headless (crítico en free tier). |
| Auth | JWT propio (`golang-jwt/v5`) + Argon2id | Sin dependencia de proveedor externo; evita el lock-in de Supabase Auth. |
| Logs | `log/slog` en JSON | Estándar de la librería, sin dependencia. |

### Frontend
| Componente | Elección | Razón |
|---|---|---|
| Base | **React 19 + Vite + TypeScript (strict)** | Alineado con tu expertise. Build rápido, bundle controlable. |
| Routing/datos | TanStack Router + TanStack Query | Rutas tipadas, caché de servidor resuelta. |
| Formularios | React Hook Form + Zod | El schema Zod se **genera en runtime** desde la definición del formulario dinámico. |
| Estilos | Tailwind v4 + shadcn/ui | Sin CSS-in-JS en runtime. |
| Arquitectura | Atomic Design + Container/Presentational + Screaming Architecture | Coherente con tus convenciones. Carpetas por dominio, no por tipo de archivo. |
| SEO | Prerender del catálogo público en build + revalidación | Necesario para que los animales aparezcan en Google. |

### Infraestructura (free tier, verificado agosto 2026)
| Servicio | Proveedor | Límite libre | Plan de degradación |
|---|---|---|---|
| API Go | **Google Cloud Run** | 2M req/mes, 180k vCPU-s, escala a cero | → Koyeb o Render si se agota |
| Postgres | **Neon** | 0.5 GB storage, 100 CU-h/mes, escala a cero | → Supabase (500 MB) |
| Objetos/Imágenes | **Cloudflare R2** | ~10 GB + **egreso cero** | Insustituible; el egreso cero es la razón |
| Frontend | **Cloudflare Pages** | Builds y ancho de banda sin costo | → Vercel Hobby |
| Correo | **Resend** | ~3.000 correos/mes | → Brevo |
| Errores | **Sentry** | Free tier | Opcional |

> ⚠️ **Advertencia consciente:** Neon y Cloud Run escalan a cero. La **primera** petición tras inactividad tardará ~1–2 s. Es un intercambio aceptable a cambio de costo cero y hay que decírselo a los refugios, no ocultarlo. Mitigación opcional: un cron gratuito de GitHub Actions que golpee `/healthz` cada 10 min en horario hábil.
>
> ⚠️ Los límites de free tier cambian. **Verificarlos al ejecutar la Fase 0**, no asumirlos.

---

## 3. Multi-tenancy — metodología

**Decisión: base compartida, esquema compartido, discriminador `shelter_id` + Row Level Security de PostgreSQL.**

Descartadas: esquema-por-refugio (dolor de migraciones, no escala a cientos) y base-por-refugio (imposible en free tier).

**Defensa en profundidad — tres capas, ninguna suficiente por sí sola:**

1. **Middleware** — resuelve el tenant desde el claim `shelter_id` del JWT. Nunca desde un parámetro de URL o header controlable por el cliente.
2. **Transacción** — cada transacción abre fijando la GUC del tenant con `SELECT set_config('app.shelter_id', $1, true)`. La app se conecta con un rol **no-superusuario** (RLS no aplica a superusuarios — este es el error clásico que anula toda la protección).

   > **Corrección (2026-08-29).** Este punto decía `SET LOCAL app.shelter_id = '<uuid>'`. `SET LOCAL` no acepta parámetros de vínculo, así que esa forma obliga a interpolar el UUID como string dentro del SQL. `set_config(nombre, valor, true)` es una función y sí los acepta, con la misma semántica de alcance transaccional. Detalle en `openspec/changes/phase-01-domain-and-data/exploration.md`.
3. **Base de datos** — política RLS por tabla:

```sql
ALTER TABLE pets ENABLE ROW LEVEL SECURITY;
ALTER TABLE pets FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pets
  USING (shelter_id = current_setting('app.shelter_id', true)::uuid);
```

El catálogo público usa una **conexión y rol separados, de solo lectura**, con su propia política:

```sql
CREATE POLICY public_catalog ON pets FOR SELECT TO app_public
  USING (status = 'available' AND published_at IS NOT NULL AND deleted_at IS NULL);
```

**Por qué así:** un `WHERE shelter_id = ?` olvidado en una query es cuestión de tiempo. Con RLS, ese olvido devuelve cero filas en lugar de filtrar datos de otro refugio. La base es la última línea de defensa y no depende de la disciplina del que escribe el SQL.

**Requisito de test no negociable:** por cada tabla con `shelter_id`, un test de integración que pruebe *"el tenant A no puede leer/escribir/borrar filas del tenant B"*. Sin ese test, la tabla no se considera terminada.

---

## 4. Modelo de datos

Nomenclatura: `snake_case`, tablas en plural, PK `uuid` v7 (ordenable en el tiempo), `created_at`/`updated_at` en `timestamptz`, borrado lógico vía `deleted_at` donde aplique.

### 4.1 Tenancy e identidad
```
shelters(id, slug UNIQUE, legal_name, display_name, country, state, city,
         status[pending_verification|verified|suspended|archived], verified_at, verified_by,
         mission, vision, about, logo_media_id, cover_media_id,
         contact_email, contact_phone, website, socials JSONB,
         donation_links JSONB,      -- [{provider,label,url,verified_at}]
         storage_bytes_used, storage_quota_bytes,
         created_at, updated_at)

users(id, email CITEXT UNIQUE, password_hash NULL, full_name, phone,
      status, email_verified_at, totp_secret_enc NULL, last_login_at,
      created_at, updated_at)

memberships(id, user_id, shelter_id, role[owner|admin|staff|volunteer],
            status[invited|active|revoked], invited_by, accepted_at,
            UNIQUE(user_id, shelter_id))

refresh_tokens(id, user_id, token_hash, family_id, expires_at,
               revoked_at, replaced_by, user_agent, ip_hash)
```
`password_hash` nulo = usuario solo de enlace mágico (adoptantes). No se almacenan contraseñas que no hacen falta.

### 4.2 Medios
```
media(id, shelter_id, kind[image|document], storage_key, mime, bytes,
      width, height, checksum_sha256, variants JSONB,  -- {thumb,card,full}
      alt_text, uploaded_by, created_at, deleted_at)
```

### 4.3 Dominio — animales
```
species(id, code, name)            -- dog, cat, other (no pintarse a una esquina)
breeds(id, species_id, name)

pets(id, shelter_id, public_code, name, species_id, breed_id NULL, breed_note,
     sex[male|female|unknown], size[xs|s|m|l|xl],
     birth_date_estimate, age_precision[exact|month|year|estimated], weight_kg,
     status[draft|available|reserved|in_process|adopted|unavailable|deceased],
     status_changed_at,
     sterilized, vaccinated, dewormed, microchip_id,
     energy_level[low|medium|high],
     good_with_kids[yes|no|unknown], good_with_dogs[...], good_with_cats[...],
     special_needs, special_needs_note,
     story, description,
     intake_date, intake_reason,
     adoption_fee_cents, adoption_fee_currency,
     published_at, created_by, created_at, updated_at, deleted_at)

pet_media(pet_id, media_id, position, is_primary)
pet_health_records(id, pet_id, type[vaccine|deworming|surgery|treatment|checkup],
                   occurred_on, description, vet_name, document_media_id)
pet_status_history(id, pet_id, from_status, to_status, reason, actor_user_id, occurred_at)
```
Los atributos de filtrado (`size`, `energy_level`, `good_with_*`) son **columnas tipadas**, no JSON. Son el eje de la búsqueda del adoptante (AD-3).

### 4.4 Formularios dinámicos
```
form_templates(id, shelter_id, key, name,
               purpose[adoption_application|home_visit|followup|custom],
               is_active, UNIQUE(shelter_id, key))

form_template_versions(id, template_id, version, definition JSONB,
                       published_at, created_by, UNIQUE(template_id, version))

form_submissions(id, shelter_id, template_version_id, application_id NULL,
                 submitted_by_user_id NULL, answers JSONB,
                 answers_encrypted BYTEA NULL, submitted_at, ip_hash)
```

**Reglas de diseño — esto es lo que casi todos hacen mal:**

1. **Las versiones publicadas son inmutables.** Editar un formulario crea `version + 1`. Una respuesta enviada siempre se renderiza contra la versión con la que se llenó. Sin esto, los datos históricos se vuelven ilegibles.
2. **`field.id` es inmutable de por vida.** Borrar un campo en v2 no borra respuestas de v1.
3. **Los tipos de campo son una unión cerrada**, no JSON Schema arbitrario: `text | textarea | number | date | select | multiselect | radio | checkbox | email | phone | file | address | signature`. Validable en Go y en TS desde una definición única.
4. **Layout por rejilla de 12 columnas.** Cada campo tiene `span: 1..12`. Dos inputs en un renglón = dos campos con `span: 6`. Resuelve el requisito exacto de la propuesta.
5. **Lógica condicional declarativa**, nunca código ejecutable del usuario.

```jsonc
{
  "schemaVersion": 1,
  "sections": [{
    "id": "personal", "title": "Datos personales",
    "rows": [{
      "id": "r1",
      "fields": [
        { "id": "full_name", "type": "text",  "label": "Nombre completo", "span": 6,
          "required": true, "validation": { "minLength": 3, "maxLength": 120 } },
        { "id": "email",     "type": "email", "label": "Correo",          "span": 6,
          "required": true }
      ]
    }]
  }],
  "logic": [
    { "if": { "field": "has_other_pets", "eq": true }, "then": { "show": ["other_pets_detail"] } }
  ]
}
```

**Veredicto de factibilidad: sí, es perfectamente viable** — siempre que se respete el versionado inmutable y la unión cerrada de tipos. Sin esas dos reglas se convierte en deuda técnica irrecuperable.

### 4.5 Flujo de adopción y documentos
```
adoption_applications(id, shelter_id, pet_id, applicant_user_id,
    status[draft|submitted|in_review|interview_scheduled|home_visit_scheduled
          |approved|rejected|withdrawn|contract_signed|delivered|returned],
    status_changed_at, assigned_to_user_id, priority_score, decision_note,
    created_at, updated_at)

application_events(id, application_id, type, payload JSONB, actor_user_id, occurred_at)
application_notes(id, application_id, author_user_id, body,
                  visibility[internal|shared], created_at)

documents(id, shelter_id, application_id NULL,
          type[adoption_contract|receipt|health_certificate|custom],
          media_id, generated_from JSONB, signed_at, signature JSONB, created_at)

audit_log(id BIGSERIAL, shelter_id, actor_user_id, action, entity_type, entity_id,
          before JSONB, after JSONB, ip_hash, user_agent, occurred_at)
```

Las transiciones de estado se validan **en el dominio** (Go), no en el handler HTTP. Cada transición emite un `application_event`. `application_events` y `audit_log` son *append-only*: sin UPDATE, sin DELETE.

### 4.6 Índices clave
```sql
CREATE INDEX ON pets (shelter_id, status, published_at DESC);
CREATE INDEX ON pets (species_id, size, energy_level) WHERE status = 'available';
CREATE INDEX ON adoption_applications (shelter_id, status, created_at DESC);
CREATE INDEX ON form_submissions USING GIN (answers);
CREATE INDEX ON audit_log (shelter_id, occurred_at DESC);
```

---

## 5. Seguridad

### 5.1 Conexiones y transporte
- TLS obligatorio extremo a extremo. HSTS con `preload`. Sin puertos en claro.
- Postgres con `sslmode=verify-full`. Pool de conexiones acotado (Neon free: pool pequeño, `pgxpool` con `MaxConns` bajo).
- **Dos roles de base de datos**: `app_tenant` (RLS activo, lectura/escritura) y `app_public` (solo lectura, catálogo público). Nunca el rol `owner` desde la aplicación.
- CORS con lista blanca explícita de orígenes. Sin comodines.
- CSP estricta, `X-Content-Type-Options`, `Referrer-Policy`, `Permissions-Policy`.

### 5.2 Autenticación y sesión
- Personal de refugio: contraseña **Argon2id** + TOTP obligatorio para roles `owner`/`admin`.
- Adoptantes: **enlace mágico por correo**. Sin contraseña = sin base de contraseñas que filtrar.
- Access token JWT de 15 min; refresh token rotatorio en cookie `HttpOnly; Secure; SameSite=Strict`.
- Detección de reutilización de refresh token → revocación de toda la familia (`family_id`).
- Rate limiting por IP y por cuenta en login, enlace mágico y envío de formularios.

### 5.3 Autorización
- RBAC por `membership.role`, evaluado en el dominio con una matriz de permisos explícita y testeada.
- El `shelter_id` **solo** sale del claim del token. Jamás de la URL, jamás de un header.

### 5.4 Datos personales
- Las respuestas de formulario con PII (documento, domicilio, referencias) se cifran a nivel de aplicación con AES-256-GCM; clave de datos envuelta por una KEK en secretos del entorno.
- Política de retención: solicitudes rechazadas se purgan a los N meses (configurable por refugio). Endpoint de eliminación a petición del titular.
- Los logs nunca contienen PII. Las IP se guardan como hash con sal.

### 5.5 Carga de archivos
- Subida directa a R2 con **URL prefirmada**, TTL corto. Los bytes nunca atraviesan la API.
- Validación por *magic bytes*, no por extensión ni por `Content-Type` declarado.
- Re-encodificación server-side obligatoria → **elimina EXIF/GPS** y neutraliza payloads embebidos (AD-6).
- Cuota de almacenamiento por refugio, aplicada antes de emitir la URL prefirmada.

### 5.6 Pagos
- **La plataforma no procesa pagos ni custodia fondos.** Solo enlaza a enlaces de pago propiedad del refugio, verificados en el onboarding. Alcance PCI: cero (LT-3).

### 5.7 Cadena de suministro
- `govulncheck` y `npm audit` en CI, bloqueantes.
- Dependabot/Renovate. `go.sum` y lockfiles siempre commiteados.
- Secretos solo en el gestor del proveedor. `gitleaks` en pre-commit.

### 5.8 mTLS vs TLS — análisis y veredicto

Pregunta concreta: ¿hace falta mTLS aquí, como en las integraciones con VISA?

**Veredicto: no. TLS 1.3 es suficiente. Implementar mTLS en esta arquitectura sería seguridad de culto al cargo.**

**Canal por canal:**

| Canal | Naturaleza | ¿mTLS? |
|---|---|---|
| Navegador → API | Cliente **público**, miles de dispositivos no controlados | **Inviable.** Distribuir y rotar certificados de cliente a adoptantes anónimos no tiene solución operativa. Un certificado alojado en un navegador no prueba nada que un token de 15 min con rotación no pruebe mejor. |
| API → Neon Postgres | Servicio gestionado | **No ofrecido.** Neon autentica con SCRAM sobre TLS y usa CA pública (ISRG Root X1). No expone autenticación por certificado de cliente. |
| API → Cloudflare R2 | S3-compatible | **No aplica.** El protocolo autentica con firma HMAC (SigV4) sobre TLS. mTLS no forma parte de él. |
| API → Resend / Sentry | SaaS | **No aplica.** Bearer token sobre TLS. |

**El costo lo descarta por sí solo.** Cloud Run **no** soporta certificados de cliente en su URL `*.run.app`. mTLS en GCP exige un *Global External Application Load Balancer* + Certificate Manager: regla de reenvío con costo fijo mensual (~USD 18–25) más USD 0.45 por millón de conexiones mTLS. Eso destruye el requisito de costo cero — para proteger canales que ya están protegidos.

**Por qué en VISA sí era obligatorio y aquí no.** En integraciones con redes de tarjetas, mTLS es un **mandato contractual** del adquirente bajo PCI-DSS. Ese canal es B2B, con endpoints fijos, ambas partes controlan su extremo, y el dato en tránsito es el PAN. Aquí ninguna de esas condiciones se cumple, y sobre todo: **al no procesar pagos (decisión LT-3), el proyecto queda fuera de alcance PCI.** El driver que forzaba mTLS simplemente no existe.

Además, mTLS tiene un costo operativo que casi nadie contabiliza: rotación de certificados, distribución, CRL/OCSP, y un modo de fallo brutal — **un certificado vencido es una caída total del servicio**, no una degradación.

**Lo que sí se implementa** (entrega la propiedad de seguridad que realmente buscarías con mTLS):
- TLS 1.3 como mínimo; TLS 1.2 solo con suites AEAD. Nada por debajo.
- HSTS con `preload`.
- `sslmode=verify-full` a Postgres con CA pinneada. **Esto es lo que neutraliza el MITM** que mTLS también neutralizaría.
- Access tokens de 15 min + rotación de refresh con detección de reutilización.
- Secretos en el gestor del proveedor, rotables sin redespliegue.

**Condiciones que reabrirían la decisión — se documentan en `ADR-0004` para no re-discutirlo cada trimestre:**
1. Integrar un procesador de pagos o banco que lo exija por contrato.
2. Exponer una API administrativa a un conjunto fijo y pequeño de máquinas propias.
3. Introducir un segundo servicio propio (worker, generador de PDF) sobre una red no controlada.
4. Un refugio institucional grande lo exija para intercambio de datos B2B.

Ninguna aplica en v1. Si alguna se cumple, el alcance es **ese canal**, nunca el tráfico de navegador.

---

## 6. Persistencia de trabajo — Obsidian + Engram + tablero de tareas

Este es el mecanismo que permite que **cualquier agente, en una sesión nueva y en frío, retome la última tarea pendiente**.

### 6.1 Estructura en el repositorio

```
mascotapp/
├── PROJECT_STATE.md              ← Fuente única de "dónde vamos". Machine-readable.
├── docs/
│   └── vault/                    ← Vault de Obsidian (markdown puro, sin plugins requeridos)
│       ├── 00-inbox/
│       ├── 10-propuesta/         propuesta original + análisis de este documento
│       ├── 20-arquitectura/      ADR-0001.md, ADR-0002.md, ...
│       ├── 30-fases/             FASE-00.md ... FASE-11.md   ← tablero de tareas
│       ├── 40-bitacora/          2026-08-28.md, ...          ← log diario del agente
│       ├── 50-specs/             SPEC-<área>.md
│       ├── 60-presentacion/      material para presentar la propuesta
│       └── 99-plantillas/        plantillas de tarea, ADR, bitácora
└── .engram/
    └── queue/                    ← cola de candidatos a memoria, pendientes de aprobación
```

Markdown plano y enlaces `[[wiki]]`. Obsidian abre `docs/vault/` directamente; el repositorio sigue siendo la verdad. Sin dependencia de plugins de pago.

### 6.2 `PROJECT_STATE.md` — contrato de continuidad

```markdown
---
project: mascotapp
current_phase: "02"
current_task: "T-02-004"
sdd_change: "phase-02-auth-multitenancy"
rdd_enabled: true               # RDD encendido: fase de alto riesgo
task_status: in_progress        # not_started | in_progress | blocked | done
blocked_by: null
last_updated: 2026-08-28T14:20:00-06:00
last_agent: sdd-apply
next_action: "Escribir test rojo de resolución de tenant desde claim JWT"
---

## Resumen en una línea
Fase 02 (Auth y multi-tenancy) — middleware de tenant en curso.

## Invariantes
- Como máximo UNA tarea en estado [~] en todo el tablero.
- Ninguna tarea pasa a [x] sin test en verde + lint limpio.
```

### 6.3 Formato de tarea (en `30-fases/FASE-NN.md`)

```markdown
- [ ] **T-02-004** · Middleware de resolución de tenant
      - spec: [[SPEC-auth#tenant-resolution]]
      - tests: `internal/http/middleware/tenant_test.go`
      - dod: test rojo→verde · aislamiento A/B probado · lint limpio · cobertura ≥80%
      - engram: <se rellena con el observation_id al cerrar>
```

Estados: `[ ]` pendiente · `[~]` en progreso · `[x]` hecha · `[!]` bloqueada

IDs `T-<fase>-<secuencia>`, estables y nunca reutilizados.

### 6.4 Ritual de inicio de sesión (obligatorio para todo agente)

1. Leer `PROJECT_STATE.md`.
2. `mem_context` + `mem_search` sobre `current_phase`.
3. Abrir `30-fases/FASE-<current_phase>.md`, localizar el primer `[~]`; si no hay, el primer `[ ]`.
4. Leer el spec enlazado y la última entrada de `40-bitacora/`.
5. Confirmar la tarea al usuario en una línea. **Entonces** empezar.

### 6.5 Ritual de cierre de tarea

1. Tests en verde, lint limpio.
2. Marcar `[x]`; mover el `[~]` a la siguiente tarea.
3. Actualizar `PROJECT_STATE.md` (incluido `next_action`).
4. Añadir entrada a `40-bitacora/<fecha>.md`.
5. Evaluar candidatos a memoria contra la rúbrica (§6.6) → escribir a `.engram/queue/`.
6. Commit convencional: `feat(auth): T-02-004 tenant resolution middleware`.

### 6.6 Curaduría de Engram — rúbrica y cola de aprobación

**Rúbrica de puntuación (0–5). Solo entra a Engram lo que puntúe ≥ 3.**

| Puntos | Criterio | Ejemplo |
|---|---|---|
| 5 | Decisión de arquitectura con alternativas descartadas | "RLS sobre discriminador porque un WHERE olvidado filtra datos" |
| 4 | Restricción externa descubierta | "Neon free: pool máximo de N conexiones" |
| 4 | Regla de dominio no evidente en el código | "Una versión de formulario publicada nunca se edita" |
| 3 | Convención establecida del proyecto | "Los eventos de dominio son append-only" |
| 3 | Bug no obvio + causa raíz | "RLS no aplica a superusuario; usar app_tenant" |
| 0–2 | **No se guarda** | avance de tareas, listados de archivos, "corrí los tests", refactors triviales |

**Flujo de aprobación (responde al requisito de la propuesta):**

1. **Al cerrar cada tarea**, el agente escribe candidatos a `.engram/queue/<fecha>-<slug>.md`, con frontmatter:
   ```yaml
   type: architecture | domain | convention | constraint | bug
   score: 4
   topic_key: mascotapp/arch/multitenancy
   task: T-02-004
   rationale: "Por qué esto importa dentro de 3 meses"
   ```
2. **En pre-commit**, un hook falla si hay elementos en la cola sin revisar. Nada se sube en silencio.
3. **Un pase de curaduría** revisa la cola: descarta `score < 3`, fusiona duplicados, y **presenta los supervivientes al usuario para aprobación explícita**.
4. Solo tras el "sí" se llama `mem_save` con `capture_prompt: false` y el `topic_key` correspondiente.
5. El `observation_id` devuelto se escribe de vuelta en la línea de la tarea → trazabilidad bidireccional.

**Taxonomía de `topic_key`:**
```
mascotapp/arch/*         decisiones de arquitectura
mascotapp/domain/*       reglas de negocio
mascotapp/convention/*   convenciones de código y proceso
mascotapp/ops/*          despliegue, límites de free tier, incidentes
mascotapp/security/*     decisiones y hallazgos de seguridad
```

**Regla dura:** nunca `mem_save` de transcripciones en bruto ni de avance de tareas. El avance vive en `PROJECT_STATE.md`; Engram guarda *por qué*, no *qué se hizo*.

---

## 7. Orquestación: Gentle AI, SDD y modelos gratuitos

### 7.1 Gentle AI en el flujo de trabajo

Gentle AI es el orquestador; el vault y `PROJECT_STATE.md` son la capa de continuidad humana. No compiten: SDD gobierna **la ejecución de una fase**, el vault gobierna **el recorrido completo del proyecto**.

**Configuración en Fase 00**

| Paso | Comando | Cuándo |
|---|---|---|
| Bootstrap SDD | `/sdd-init` (backend híbrido: engram + openspec) | T-00-003 |
| Índice de código | `gentle-ai codegraph init --cwd <root>` | T-00-013, al final de F00 (necesita código) |
| Registro de skills | `skill-registry` | T-00-014 |

**Ciclo SDD por fase** — cada fase del §8 es exactamente un *change* de SDD:

```
/sdd-new <fase>   → explore + propose
/sdd-ff           → spec · design · tasks   (fast-forward de planificación)
/sdd-apply        → implementación TDD
/sdd-verify       → validación contra spec
/sdd-archive      → cierre, specs fusionadas
```

**Modo de persistencia: `hybrid` (openspec + engram).** Verificado en la práctica: el
dispatcher nativo (`gentle-ai sdd-status`, `sdd-continue`, `sdd-attempt`) resuelve
`store: openspec` y `planning_home: <repo>/openspec` **de forma incondicional**. En modo
`engram` puro, `openspec/` no se crea y todo el enrutamiento nativo de SDD queda
**bloqueado de forma permanente**. `hybrid` no es preferencia; es requisito de funcionamiento.

**Mapeo SDD ↔ vault** — no son dos fuentes de verdad, son dos granularidades:

| Artefacto | Granularidad | Vida |
|---|---|---|
| `openspec/changes/<change>/` | La **fase activa**: proposal, spec, design, tasks | Efímero — se archiva al cerrar |
| `docs/vault/30-fases/FASE-NN.md` | El **roadmap** de las 13 fases | Permanente |
| `PROJECT_STATE.md` | Dónde estamos **ahora** | Permanente, se actualiza cada tarea |
| Engram | **Por qué** se decidió cada cosa | Permanente, entre proyectos |

- `FASE-NN.md` **enlaza** al change SDD activo; no duplica su contenido.
- `PROJECT_STATE.md` lleva `sdd_change: <nombre>` junto a `current_task`.
- Al ejecutar `/sdd-archive`, el reporte se copia a `40-bitacora/` y los ADRs a `20-arquitectura/`.

**Excepción para Fase 00:** las fundaciones se ejecutan por ruta **delegada directa**, sin
ciclo SDD. Son scaffolding sin ambigüedad de diseño, y arrancar un change SDD a mitad de
una fase ya empezada genera artefactos que no describen lo que realmente pasó.
**El primer change SDD real es `fase-01-dominio-y-datos`.**

**Receipt-Driven Development (RDD)** — es **opt-in y está apagado por defecto**. No se activa por vos.

> **Recomendación:** encenderlo con `gentle-ai review mode enable --scope clone` únicamente para las fases de alto riesgo — **01 (RLS), 02 (auth), 04 (medios), 07 (formularios)** — y apagarlo en las fases de UI y catálogo, donde el costo de revisión no compensa. Usar siempre `--scope clone`: sin ese flag el default es `global` y afectaría todos tus repos.

**Judgment Day** — revisión ciega dual. Reservada para los merges de los tres puntos donde un error no se recupera: políticas RLS, rotación de tokens, y cifrado de PII. No para todo.

**Delegación a sub-agentes — habilitada.** Es la metodología de trabajo de Gentle AI, no una excepción. El orquestador coordina y sintetiza; los sub-agentes ejecutan. Regla de topología:

| Situación | Ruta |
|---|---|
| Leer 1–3 archivos para decidir o verificar | **inline** |
| Entender algo que abarca 4+ archivos | **un explorador** (`Explore` / `sdd-explore`) |
| Escribir 1 archivo mecánico ya entendido | **inline** |
| Escribir 2+ archivos no triviales | **un escritor** (`sdd-apply`) |
| Leer como preparación para escribir | **delegado junto con la escritura** |
| Tests, builds, instalaciones | **worker fresco por acción**, sin cambiar la ruta |

**Un solo escritor por vez.** Sin escritores en paralelo salvo worktrees aislados aprobados.

**Mapeo de agentes por fase SDD:**

| Fase | Agente | Modelo sugerido |
|---|---|---|
| Exploración | `sdd-explore` | rápido |
| Propuesta | `sdd-propose` | razonador |
| Spec | `sdd-spec` | razonador |
| Diseño | `sdd-design` | razonador |
| Tareas | `sdd-tasks` | rápido |
| Implementación | `sdd-apply` | razonador |
| Verificación | `sdd-verify` | razonador |
| Archivo | `sdd-archive` | rápido |
| Revisión dual | `jd-judge-a` + `jd-judge-b` (ciegos) → `jd-fix-agent` | razonador |
| Lentes de review | `review-risk` · `review-readability` · `review-reliability` · `review-resilience` | los elige RAR, no el orquestador |

**Regla anti-inflación:** delegar existe para no inflar el contexto del orquestador, no para parecer ocupado. Si la tarea cabe inline sin ensuciar el hilo, va inline.

### 7.2 OpenCode con modelos gratuitos — evaluación crítica

Pediste ser crítico porque no querés retrabajo. Lo soy: **la premisa "usar modelos gratis para tareas sencillas" es demasiado vaga y, tal como está enunciada, garantiza el retrabajo que querés evitar.** Hay que endurecerla antes de probar nada.

**Tres hallazgos duros de la investigación (agosto 2026):**

1. **La disponibilidad es volátil.** El endpoint `:free` de Qwen3 Coder desapareció a mediados de 2026; DeepSeek, Mistral y Gemini ya no tienen modelos a $0 en OpenRouter. **Nunca atar el flujo a un modelo concreto.** Los que hoy sostienen trabajo agéntico son `laguna-m.1:free` (262K) y `north-mini-code:free` (256K), pero eso puede cambiar el mes que viene.
2. **Los límites de tasa rompen el trabajo agéntico.** OpenRouter free: ~20 req/min y **200 req/día**. Una tarea agéntica no trivial son 30–80 llamadas. Eso da 3–5 tareas por día — y si se corta a mitad de una, quedás con estado sucio en el repo. Groq da más volumen (30 RPM, 14.4k RPD) pero con TPM bajo, lo que castiga justamente los contextos grandes.
3. **El costo real no es la inferencia, es la revisión.** Código plausible-pero-incorrecto cuesta más revisarlo que escribirlo. Ese es *exactamente* el retrabajo del que hablás.

**Por eso el criterio no es "tarea sencilla" — eso es subjetivo. El criterio es:**

> **Una tarea es apta para modelo gratuito solo si su corrección puede verificarse por completo, de forma automática y objetiva, con una comprobación que ya existe antes de empezar.**
>
> Si hay que *leer* la salida para saber si está bien, **no es apta**.

**Lista blanca — cada tarea con su verificador obligatorio:**

| Tarea | Verificador automático |
|---|---|
| Stubs de claves i18n desde strings existentes | script de paridad de claves entre locales |
| Casos table-driven para una función pura ya especificada | `go test -race` contra la implementación existente |
| Historias de Storybook desde props existentes | `tsc --noEmit` + build de Storybook |
| Datos semilla y fixtures | constraints + RLS de la base los rechazan si están mal |
| Docs markdown derivados de `openapi.yaml` | diff contra regeneración determinista |
| Mensajes de commit convencionales desde el diff | `commitlint` |
| Entradas de bitácora desde `git log` | validación de frontmatter |
| Borradores de `alt_text` para imágenes | revisión humana de 5 s, sin costo de retrabajo |
| Renombrados mecánicos | `gopls` / `tsc` + suite de tests existente |

**Lista negra dura — nunca modelo gratuito, sin excepciones:**
- Migraciones y políticas RLS
- Auth, criptografía, manejo de tokens
- Máquinas de estado del dominio
- Motor de formularios dinámicos
- ADRs y decisiones de arquitectura
- **Cualquier archivo sin test previo que lo cubra**
- Cualquier código que toque PII

**Piloto controlado** (responde a "probar de manera controlada"):

| Parámetro | Valor |
|---|---|
| Fase | **06 — Catálogo público.** Alto volumen mecánico, bajo riesgo, buena cobertura de tests |
| Aislamiento | Rama `experiment/opencode-free`. **Nunca directo a `main`** |
| Muestra | 10 tareas de la lista blanca |
| Registro | `docs/vault/40-bitacora/opencode-pilot.md` |
| Métricas | `first_pass_rate` (% que pasa el verificador sin intervención) · `rework_minutes` (mediana de corrección por tarea) · `wall_clock` vs. estimación directa |
| **Criterio de continuación** | `first_pass_rate ≥ 70%` **y** mediana de `rework_minutes ≤ 5` |
| **Criterio de muerte** | Dos tareas consecutivas que cuesten más corregir que ejecutar → se aborta, se registra en `ADR-0005` y no se vuelve a discutir esta iteración |

**Configuración `opencode.json`:**
- **Mínimo dos proveedores con fallback** (OpenRouter + Groq). Los modelos gratuitos desaparecen; el flujo no puede depender de uno.
- Claves solo en variables de entorno, nunca en el repo. `gitleaks` ya lo cubre.
- Permisos de escritura acotados por ruta a la lista blanca. El agente no puede tocar `internal/domain/`, `migrations/` ni `internal/auth/`.

**Regla de integración innegociable:** OpenCode **no escribe** en `PROJECT_STATE.md`, ni en `.engram/queue/`, ni en `docs/vault/20-arquitectura/`. Produce código; Claude + Gentle AI mantienen estado, memoria y decisiones. **Una sola mano en el timón**, o volvés a tener dos fuentes de verdad (IA-4) con el agravante de que una de ellas es un modelo pequeño.

### 7.3 Supervisión del modo bypass — qué se puede hacer cumplir de verdad

Se verificó contra la documentación oficial qué sobrevive realmente en `bypassPermissions`, en lugar de asumirlo.

#### Estado actual — tres hallazgos

Tu `~/.claude/settings.json` tiene hoy:

```json
"permissions": { "defaultMode": "bypassPermissions" },
"skipDangerousModePermissionPrompt": true
```

1. **Bypass está activo globalmente, en todos tus proyectos, sobre el host Windows y sin contenedor.** La documentación es explícita: *"Only use this mode in isolated environments like containers, VMs, or dev containers without internet access, where Claude Code cannot damage your host system."*

2. **Tu lista `deny` es más estrecha de lo que parece.** Las reglas Bash sin `*` final son **coincidencia exacta**. `Bash(rm -rf /)` bloquea *literalmente* esa cadena y nada más. Hoy pasan sin freno: `rm -rf ./apps`, `git reset --hard`, `git clean -fdx`, `git push --force`, `sudo <cualquier-otra-cosa>`, `curl … | sh`, y cualquier `DROP TABLE`.

3. **En una sesión con bypass disponible, el modo plan no se hace cumplir.** La documentación lo dice textualmente: *"In sessions with bypass permissions available, Claude Code also doesn't enforce plan mode's blocks."* Es decir: en tu configuración actual, el modo plan es **advertencia, no barrera** — se sostiene porque el modelo obedece la instrucción, no porque el harness lo impida. Todo tu flujo de revisión-antes-de-ejecutar descansa sobre eso.

#### Qué sobrevive en bypass (verificado)

| Mecanismo | ¿Funciona en bypass? | Fuente |
|---|---|---|
| Reglas **`deny`** | ✅ **Sí, en todos los modos** | *"Deny rules block in every mode, including `bypassPermissions`"* |
| Reglas **`ask`** | ✅ **Sí — fuerzan prompt** | Listado en *"actions no mode auto-approves"* |
| Reglas **`allow`** | ❌ **No tienen ningún efecto** | *"Allow rules have no effect in `bypassPermissions`"* |
| Cortacircuitos de *critical path* | ✅ Pregunta siempre | Ningún `allow` ni hook `"allow"` lo aprueba |
| Rutas protegidas (`.git`, `.claude`) | ❌ Escritura permitida | Tabla de *Protected paths* |
| Bloqueos del modo plan | ❌ No se aplican | Ver hallazgo 3 |
| Hooks `PreToolUse` | ⚠️ **No documentado** | Se **verifica empíricamente** en T-00-018 |

> Consecuencia de diseño: **`deny` y `ask` son los únicos controles con garantía documentada.** Toda la supervisión se construye sobre ellos; el hook es capa complementaria, nunca la principal.

#### Recomendación directa

**Para este proyecto, sobre tu host Windows sin contenedor, `bypassPermissions` es el modo equivocado. Usá `auto`.**

`auto` te da prácticamente el mismo caudal sin prompts, pero pasa cada acción por un clasificador que **ya bloquea por defecto** exactamente lo que te preocupa: force-push, borrar recursos con estado que el agente no creó, repuntar URLs de API o registries, comentar o forzar tests que protegen seguridad, imprimir credenciales al transcript, y lanzar loops autónomos sin sandbox. `bypassPermissions` no tiene **nada** de eso.

Bypass tiene un lugar legítimo en este plan: **dentro del devcontainer del piloto de OpenCode (§7.2)**, donde el contenedor es la barrera real. En el host, no.

#### Capa 0 — Aislamiento (la única garantía dura)

Las reglas comparan **la cadena del comando, no su efecto.** Un `make clean` que internamente ejecuta `rm -rf` **no se bloquea**. Eso convierte a las capas 1–3 en barandas contra error del modelo, **no en un sandbox**. El aislamiento es lo único que acota el daño de verdad.

- Devcontainer con **solo el repositorio montado**. `C:\Users\Jose\` nunca se monta.
- El agente nunca corre sobre la única copia: rama por fase, commits frecuentes.

#### Capa 1 — `deny` en `<repo>/.claude/settings.json`

```json
{
  "permissions": {
    "deny": [
      "Bash(rm *)", "Bash(rmdir *)", "Bash(del *)", "Bash(Remove-Item *)",
      "Bash(sudo *)", "Bash(shutdown *)", "Bash(reboot *)",
      "Bash(mkfs*)", "Bash(dd *)", "Bash(chmod -R 777*)",
      "Bash(git push --force*)", "Bash(git push -f*)",
      "Bash(git reset --hard*)", "Bash(git clean -fd*)",
      "Bash(git checkout -- *)", "Bash(git branch -D*)",
      "Bash(git remote set-url*)", "Bash(git remote add*)",
      "Bash(gh repo delete*)", "Bash(gh pr merge*)",
      "Bash(docker system prune*)", "Bash(docker volume rm*)",
      "Bash(*DROP TABLE*)", "Bash(*DROP DATABASE*)", "Bash(*TRUNCATE*)",
      "Bash(goose down*)", "Bash(goose reset*)",
      "Bash(gcloud * delete*)", "Bash(wrangler * delete*)", "Bash(neonctl * delete*)",
      "Bash(npm publish*)",
      "Bash(curl * | sh*)", "Bash(curl * | bash*)", "Bash(wget * | sh*)",
      "Read(./.env)", "Read(./.env.*)", "Read(./secrets/**)",
      "Edit(./.env)", "Edit(./.claude/settings.json)"
    ]
  }
}
```

**`Bash(rm *)` prohibido por completo, a propósito.** Todo borrado legítimo pasa por un target de `Makefile` (`make clean`) cuyo contenido está bajo revisión de código. El agente no borra a mano; ejecuta un borrado que un humano ya revisó. Es la diferencia entre confiar en el juicio del modelo y confiar en un artefacto versionado.

Y arreglá también las reglas globales: hoy `Bash(rm -rf /)` sin `*` no cubre casi nada.

#### Capa 2 — `ask`: supervisión sin perder velocidad

Esto es exactamente lo que pediste — bypass para el 95% del trabajo, humano obligatorio en lo irreversible:

```json
"ask": [
  "Bash(git push*)", "Bash(gh pr create*)", "Bash(gh *)",
  "Bash(gcloud *)", "Bash(wrangler *)", "Bash(neonctl *)",
  "Bash(goose up*)", "Bash(psql*)",
  "Bash(npm i*)", "Bash(npm install*)", "Bash(go get*)",
  "Bash(git commit*)"
]
```

Criterio: **sale de la máquina** (push, PR, deploy), **toca datos reales** (migraciones, psql), o **incorpora código de terceros** (cadena de suministro).

#### Capa 3 — Cortacircuitos nativo

`rm`/`rmdir` contra raíz del sistema, directorios de primer nivel, tu `home`, raíces de unidad (`C:\`, `C:\Windows`), **y el directorio de trabajo y sus padres** → pregunta **incluso en bypass**, y ningún `allow` ni hook `"allow"` puede aprobarlo. Es gratis y ya está activo.

**Su límite, para que no te confíes:** solo cubre `rm`/`rmdir` sobre *esas* rutas. `rm -rf ./apps/api` **no** es critical path. Por eso existe la Capa 1.

#### Capa 4 — Hook `PreToolUse` (complementaria)

Cubre lo que un patrón no puede expresar: escrituras fuera de la raíz del repo, `mem_save` sin entrada previa en `.engram/queue/`, o modificación de archivos de la lista negra de §7.2 por parte de OpenCode.

Bloquea con `exit 2` (única forma de bloquear solo por código de salida) o con `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny"}}`. **Ojo:** `exit 1` **no bloquea** — Claude Code lo trata como error no bloqueante y continúa.

**T-00-018 debe probar empíricamente si el hook dispara en bypass.** Si no dispara, la capa se descarta y no se cuenta como protección. Una barrera que no se probó no es una barrera.

#### Capa 5 — El interruptor

```json
// ~/.claude/settings.json
"permissions": { "disableBypassPermissionsMode": "disable" }
```

Funciona desde cualquier ámbito de configuración: la documentación confirma que un usuario puede ponerlo en sus propios settings para bloquearse a sí mismo. Es la opción más segura si decidís no usar bypass fuera del contenedor.

---

## 8. Plan de fases y tareas

Modo **TDD estricto**: cada tarea de código empieza con un test que falla. Sin excepción.

**Definition of Done común a toda tarea:**
test rojo→verde · lint limpio (`golangci-lint`, `eslint`, `tsc --noEmit`) · sin bajar cobertura · spec actualizado · `PROJECT_STATE.md` actualizado · commit convencional.

> **Línea de corte del MVP: fin de Fase 08.** Las fases 09–11 son post-lanzamiento. Si hay que recortar alcance, se recorta desde el final, nunca desde el medio.

Columna **RDD**: fases donde se recomienda encender `gentle-ai review mode enable --scope clone`. Es tu decisión — el plan solo la señala, nunca la activa por vos.

| Fase | Nombre | Entregable verificable | RDD | Dep. |
|---|---|---|---|---|
| **00** | Fundaciones | Monorepo, CI, vault Obsidian, `PROJECT_STATE.md`, cola de Engram, **SDD + CodeGraph inicializados**, ADR-0001..0005, límites de free tier **verificados** | — | — |
| **01** | Dominio y datos | Migraciones `goose`, políticas RLS, `sqlc` generado, seeds, **tests A/B de aislamiento por tabla** | ✅ | 00 |
| **02** | Auth y multi-tenancy | Registro/login, Argon2id, TOTP, enlace mágico, rotación de refresh, middleware de tenant, matriz RBAC testeada | ✅ | 01 |
| **03** | Núcleo de refugio | CRUD de refugio, invitaciones y roles, perfil institucional (misión/visión/fotos) | — | 02 |
| **04** | Medios | URLs prefirmadas a R2, validación por magic bytes, stripping EXIF, 3 variantes AVIF/WebP, cuota por refugio, compresión en cliente | ✅ | 03 |
| **05** | Animales | CRUD de `pets`, galería, registros de salud, máquina de estados + `pet_status_history` | — | 04 |
| **06** | Catálogo público | Listado y ficha, filtros tipados, paginación, SEO/JSON-LD/OG, rol `app_public` de solo lectura · **⚑ piloto OpenCode (§7.2)** | — | 05 |
| **07** | Formularios dinámicos | Constructor con rejilla de 12 col + vista previa, versionado inmutable, renderizador con RHF+Zod, lógica condicional, envío y almacenamiento cifrado | ✅ | 03 |
| **08** | Flujo de adopción | Máquina de estados de solicitudes, asignación, notas internas, timeline auditable, notificaciones por correo | — | 06, 07 |
| — | **◆ CORTE MVP ◆** | Despliegue en producción con 1 refugio piloto real | | |
| **09** | Documentos y reportes | Generación de contrato PDF (`maroto`), reportes operativos y export CSV | — | 08 |
| **10** | Confianza y admin | Verificación de refugios, moderación, verificación de enlaces de donación, panel de superadmin | ✅ | 08 |
| **11** | Endurecimiento y lanzamiento | Auditoría WCAG 2.2 AA, i18n, presupuesto de rendimiento, pentest, runbooks, E2E Playwright completos | ✅ | 10 |
| **12** | Asistentes de IA *(post-v1)* | Borrador de descripción desde fotos, detección de duplicados, priorización de solicitudes **(solo ordena, nunca rechaza)** | — | 11 |

**Judgment Day** (revisión ciega dual) se ejecuta antes del merge de exactamente tres entregables, donde un error no se recupera: políticas RLS (F01), rotación de tokens (F02) y cifrado de PII (F07).

### Desglose de Fase 00 (ejemplo del nivel de granularidad esperado)

```markdown
- [ ] T-00-001 · Estructura del monorepo (apps/api, apps/web, docs/vault)
- [ ] T-00-002 · Vault de Obsidian + plantillas (tarea, ADR, bitácora)
- [ ] T-00-003 · PROJECT_STATE.md + contrato de continuidad · `/sdd-init` (backend híbrido)
- [ ] T-00-004 · .engram/queue + .engram/RUBRIC.md + hook pre-commit de cola
- [ ] T-00-005 · Scaffold de Go: chi, slog, /healthz, /readyz, config por entorno
- [ ] T-00-006 · Scaffold de web: Vite + React 19 + TS strict + Tailwind + shadcn
- [ ] T-00-007 · Docker Compose local (Postgres + MinIO como sustituto de R2)
- [ ] T-00-008 · CI: golangci-lint, go test -race, govulncheck, eslint, tsc, vitest, gitleaks
- [ ] T-00-009 · Esqueleto de OpenAPI 3.1 + oapi-codegen + openapi-typescript
- [ ] T-00-010 · ADR-0001 stack · ADR-0002 multi-tenancy · ADR-0003 costo cero
- [ ] T-00-011 · ADR-0004 TLS sin mTLS + condiciones de reapertura (§5.8)
- [ ] T-00-012 · VERIFICAR límites vigentes de free tier y registrarlos en 20-arquitectura/
- [ ] T-00-013 · Andamiaje de i18n (es-MX) y claves de traducción
- [ ] T-00-014 · `gentle-ai codegraph init` + verificar `.codegraph/` (requiere código: va al final)
- [ ] T-00-015 · `skill-registry` + documentar el ciclo SDD por fase en 20-arquitectura/
- [ ] T-00-016 · `opencode.json` con doble proveedor + permisos por ruta · ADR-0005 (piloto, criterios de muerte)
- [ ] T-00-017 · `.claude/settings.json` del repo: listas `deny` + `ask` de §7.3 · `Makefile` con target `clean`
- [ ] T-00-018 · **Probar empíricamente** si el hook PreToolUse dispara en bypass; registrar resultado en 20-arquitectura/
- [ ] T-00-019 · Devcontainer aislado (solo el repo montado) · ADR-0006 modo de permisos y supervisión
```

Cada fase se expande a este mismo nivel **al inicio de la fase**, no antes. Expandir las 12 fases hoy produce tareas obsoletas.

> `T-00-016` deja la configuración lista y auditada, pero **el piloto de OpenCode no se ejecuta hasta la Fase 06.** No se mezcla un experimento de modelos con el trabajo de fundaciones.

---

## 9. Archivos críticos a crear

| Ruta | Propósito |
|---|---|
| `PROJECT_STATE.md` | Contrato de continuidad entre sesiones |
| `docs/vault/30-fases/FASE-*.md` | Tablero de tareas con checkboxes |
| `docs/vault/20-arquitectura/ADR-*.md` | Decisiones fechadas e inmutables |
| `docs/vault/50-specs/SPEC-*.md` | Especificaciones por área |
| `.engram/queue/` + `.engram/RUBRIC.md` | Cola y rúbrica de curaduría de memoria |
| `opencode.json` | Doble proveedor con fallback + permisos de escritura por ruta (§7.2) |
| `openspec/` | Artefactos SDD por fase (proposal · spec · design · tasks) |
| `api/openapi.yaml` | Contrato único de API |
| `apps/api/internal/domain/` | Dominio puro en Go, sin dependencias de I/O |
| `apps/api/internal/db/migrations/` | Migraciones `goose`, RLS incluida |
| `apps/api/internal/http/middleware/tenant.go` | Resolución de tenant + `SET LOCAL` |
| `apps/web/src/features/` | Screaming architecture por dominio |
| `packages/form-schema/` | Unión cerrada de tipos de campo, compartida Go↔TS |
| `.github/workflows/ci.yml` | Puertas de calidad |

---

## 10. Verificación

**Por tarea** — `go test -race ./...`, `vitest run`, lint y `tsc` limpios.

**Por fase**
- **F01:** para cada tabla con `shelter_id`, test que confirma que el tenant A no lee ni escribe filas del tenant B. Bloqueante.
- **F02:** intento de forjar `shelter_id` en URL/header → 403. Reutilización de refresh token → familia revocada.
- **F04:** subir un JPEG con GPS en EXIF → verificar que las variantes derivadas no contienen metadatos. Subir un `.php` renombrado a `.jpg` → rechazado por magic bytes.
- **F06:** el rol `app_public` no puede leer animales en `draft` ni de refugios no verificados.
- **F07:** publicar v1, enviar respuesta, publicar v2 con un campo eliminado → la respuesta de v1 sigue renderizando correctamente.
- **F08:** transición inválida de estado → rechazada por el dominio y ausente de `application_events`.

**End-to-end (Playwright)** — recorrido crítico completo:
registro de refugio → verificación → alta de animal con fotos → publicación → adoptante encuentra por filtro → envía solicitud → refugio revisa y aprueba → contrato generado → animal marcado como adoptado.

**Producción** — `/healthz` y `/readyz` verdes, Sentry sin errores nuevos, LCP < 2.5 s en 3G simulada, p95 de API < 300 ms (excluida la petición de arranque en frío).

**Verificación de TLS (§5.8)** — `testssl.sh` contra el dominio de producción: TLS 1.3 negociado, TLS 1.0/1.1 rechazados, HSTS presente con `preload`. Test de integración que confirma que la conexión a Postgres **falla** si se degrada `sslmode` por debajo de `verify-full`.

**Verificación del piloto OpenCode (§7.2)** — al cerrar la Fase 06, `opencode-pilot.md` debe contener las 10 tareas con sus tres métricas y una decisión explícita: *continuar* o *abortar*, con el ADR correspondiente. **Un piloto sin números registrados se considera fallido**, no inconcluso.

**Verificación de la supervisión de agentes (§7.3)** — batería ejecutada en T-00-018 y repetida al cerrar cada fase, con resultados en `20-arquitectura/supervision-checks.md`:

| Comando de prueba | Resultado esperado |
|---|---|
| `rm -rf ./apps` | **Bloqueado** por regla `deny` |
| `git reset --hard HEAD~1` | **Bloqueado** por regla `deny` |
| `git push origin main` | **Prompt** al humano (regla `ask`) |
| `goose up` | **Prompt** al humano (regla `ask`) |
| `make clean` | Permitido — el borrado vive en un target versionado y revisado |
| Escritura fuera de la raíz del repo | Bloqueada por el hook, **si T-00-018 confirma que dispara en bypass** |

Cada fila se ejecuta de verdad y se registra el resultado observado. **Una regla que no se probó no cuenta como protección** — y si el hook no dispara, se tacha la última fila y se documenta, en lugar de asumir que protege.

---

## 11. Supuestos declarados

Decisiones tomadas por delegación explícita de la propuesta. Corregibles antes de ejecutar Fase 00:

1. **Región inicial: México / LATAM.** Español `es-MX` como idioma base; enlaces de donación agnósticos de proveedor (Mercado Pago, PayPal, transferencia).
2. **Los adoptantes necesitan cuenta solo para enviar una solicitud** (enlace mágico). Navegar el catálogo es anónimo.
3. **Alcance v1: perros**, pero el modelo soporta `species` desde el día 1 para no repagar el costo después.
4. **La verificación de refugios es manual**, ejecutada por ti. No hay flujo automatizado en v1.
5. **La firma de contratos es de trazabilidad, no legalmente vinculante** en v1 (sin proveedor de firma electrónica certificada).
6. **Sin mTLS en v1** (§5.8). TLS 1.3 + `verify-full` + tokens cortos. Condiciones de reapertura documentadas en `ADR-0004`.
7. **RDD se enciende solo en las fases marcadas** en §8, siempre con `--scope clone`. Nunca se activa sin tu instrucción explícita.
8. **OpenCode entra solo como piloto medido en Fase 06**, en rama aislada, con criterio de muerte escrito por adelantado.
9. **El proyecto se trabaja en modo `auto`, no en `bypassPermissions`** (§7.3). Bypass queda confinado al devcontainer del piloto de OpenCode. Si preferís conservar bypass en el host, el plan funciona igual — pero las capas 1 y 2 pasan de recomendadas a **obligatorias** antes de la primera tarea de código.

---

## 12. Decisiones que quedan en tus manos

Nada de esto bloquea el arranque; se resuelve en Fase 00.

1. **Modo de permisos** — recomendación: cambiar `defaultMode` a `"auto"` en `~/.claude/settings.json`. Es tu llamada; el plan no toca tu configuración global sin que lo pidas.
2. **RDD** — encenderlo con `--scope clone` en las fases marcadas ✅ del §8. También es tuya: no se activa solo.
3. **Región y proveedor de donaciones** — el supuesto es México/LATAM; cambia solo los enlaces, no la arquitectura.
