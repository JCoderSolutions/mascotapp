# Rúbrica de curaduría de memoria (Engram)

Regla central: **Engram guarda *por qué*, nunca *qué se hizo*.** El avance vive en
`PROJECT_STATE.md`. Si una observación se vuelve obsoleta cuando la tarea termina,
no pertenece a la memoria.

## Puntuación — solo entra lo que puntúe ≥ 3

| Puntos | Criterio | Ejemplo |
|---|---|---|
| 5 | Decisión de arquitectura, con las alternativas descartadas y el motivo | "RLS por encima del discriminador: un `WHERE shelter_id` olvidado filtra datos de otro refugio" |
| 4 | Restricción externa descubierta en la práctica | "Neon free limita el pool de conexiones; `pgxpool.MaxConns` bajo" |
| 4 | Regla de dominio que no es evidente leyendo el código | "Una versión publicada de formulario nunca se edita; se crea `version + 1`" |
| 3 | Convención establecida del proyecto | "`application_events` y `audit_log` son append-only" |
| 3 | Bug no obvio junto a su causa raíz | "RLS no aplica a superusuario; la app debe conectarse con el rol `app_tenant`" |
| 0–2 | **No se guarda** | Avance de tareas, listados de archivos, "corrí los tests", refactors triviales, resúmenes de sesión |

## Flujo de aprobación

1. **Al cerrar cada tarea**, el agente escribe candidatos a `.engram/queue/<fecha>-<slug>.md`.
2. **En pre-commit**, el hook falla si quedan elementos sin revisar en la cola. Nada se sube en silencio.
3. **El pase de curaduría** descarta `score < 3`, fusiona duplicados y **presenta los supervivientes al usuario para aprobación explícita**.
4. Solo tras el "sí" se llama `mem_save` con `capture_prompt: false` y su `topic_key`.
5. El `observation_id` devuelto se escribe de vuelta en la línea de la tarea, en el tablero de la fase, y en el frontmatter del candidato.
6. **El archivo se mueve de `.engram/queue/` a `docs/vault/70-conocimiento/`.** Esta carpeta es una sala de espera, no un archivo: lo aprobado vive dentro del vault, donde Obsidian lo indexa y sus enlaces `[[wiki]]` resuelven. Un archivo que sigue acá es un candidato que sigue esperando.

## Formato del candidato

```yaml
---
type: architecture | domain | convention | constraint | bug
score: 4
topic_key: mascotapp/arch/multitenancy
task: T-02-004
rationale: "Por qué esto importa dentro de tres meses"
---

Cuerpo de la observación. Concreto y autocontenido: quien lo lea en otra
sesión no tiene el contexto de hoy.
```

## Taxonomía de `topic_key`

| Prefijo | Contenido |
|---|---|
| `mascotapp/arch/*` | Decisiones de arquitectura |
| `mascotapp/domain/*` | Reglas de negocio |
| `mascotapp/convention/*` | Convenciones de código y de proceso |
| `mascotapp/ops/*` | Despliegue, límites de free tier, incidentes |
| `mascotapp/security/*` | Decisiones y hallazgos de seguridad |

## Regla dura

Nunca `mem_save` de transcripciones en bruto ni de avance de tareas.
