---
type: architecture
score: 5
topic_key: mascotapp/arch/multitenancy
task: T-00-010
status: guardado
observation_id: obs-4ae5918c16167e36
rationale: "Define cómo se escribe TODA consulta del proyecto. Sin esto, cada sesión nueva reinventa el aislamiento."
---

Multi-tenancy de MascotApp: base compartida + esquema compartido + discriminador
`shelter_id` + Row Level Security de PostgreSQL, con defensa en tres capas
(middleware desde el claim JWT, `SET LOCAL app.shelter_id` por transacción, política RLS).

Motivo de elegir RLS por encima del discriminador solo: un `WHERE shelter_id = ?`
olvidado filtra datos de otro refugio, y es cuestión de tiempo. Con RLS ese olvido
devuelve cero filas.

**Trampa crítica:** RLS **no aplica a superusuarios**. La app debe conectarse con el rol
no-superusuario `app_tenant`, o toda la protección se anula en silencio. El catálogo
público usa un rol separado de solo lectura, `app_public`.

Requisito de test no negociable: por cada tabla con `shelter_id`, un test que pruebe que
el tenant A no puede leer/escribir/borrar filas del tenant B.

Ver ADR-0002.
