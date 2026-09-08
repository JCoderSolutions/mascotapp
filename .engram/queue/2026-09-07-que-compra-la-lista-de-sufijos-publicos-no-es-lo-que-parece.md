---
type: architecture
score: 4
topic_key: mascotapp/arch/public-suffix-list-buys-false-positives
task: T-02-026
status: pendiente-de-aprobacion
rationale: "La justificación de una dependencia es lo que se cita cuando alguien propone sacarla, y una justificación equivocada pierde esa discusión aunque la dependencia sea correcta. Acá la razón intuitiva —'sin esto se nos escapa un riesgo de seguridad'— es falsa, y la mutación lo midió: el modo de falla es el opuesto. Aplica cada vez que se compare la propiedad de dos hosts, que es un patrón que reaparece en cookies, CORS, redirecciones y verificación de webhooks. Y el método vale más que el caso: la mutación no solo mide si un test alcanza, mide si el COMENTARIO dice la verdad."
---

# Qué compra la lista de sufijos públicos no es lo que parece

(T-02-026, MascotApp Fase 02.)

## La regla

Comparar la propiedad de dos hosts —"¿son del mismo dueño?"— necesita la lista de
sufijos públicos (`golang.org/x/net/publicsuffix`). La alternativa obvia, comparar
las dos últimas etiquetas, **no falla como uno espera**.

## La trampa

La justificación intuitiva es *"sin la lista se nos escapa un parentesco real"*.
Es falsa, y sostenerla pierde la discusión el día que alguien proponga sacar la
dependencia.

Medido por mutación: reemplazar `EffectiveTLDPlusOne` por las dos últimas
etiquetas mata **exactamente dos** casos, y los dos son la **misma** dirección.

La comparación ingenua **sobre-agrupa**. Lee `pages.dev` y `com.mx` como dominios
registrables, así que todo lo que cuelga de un sufijo le parece del mismo dueño:

| Par | Realidad | Comparación ingenua |
|---|---|---|
| `mascotapp.pages.dev` vs `mascotapp-api.pages.dev` | dueños distintos | "mismo dueño" ❌ |
| `mascotapp.com.mx` vs `mascotapp-api.com.mx` | dueños distintos | "mismo dueño" ❌ |
| `app.mascotapp.org` vs `api.mascotapp.org` | mismo dueño | "mismo dueño" ✅ |

**No puede fallar en el otro sentido.** Si dos hosts comparten dueño de verdad,
comparten también sus dos últimas etiquetas. Nunca se le escapa un parentesco
real; solo inventa parentescos falsos.

## Lo que eso cambia

La lista no compra "tapar un agujero de seguridad": compra **no rechazar una
configuración correcta**. El costo de equivocarse es un deploy que no levanta, no
una falla silenciosa.

Ese es un argumento más débil que el intuitivo — y es el verdadero. Acá alcanza
igual, porque `pages.dev` y `.com.mx` son las formas de despliegue reales del
proyecto: Cloudflare Pages, y la región mexicana que supone el plan. Un falso
rechazo no sería teórico; sería el primer intento de arranque en producción.

## La segunda lección, sobre el método

**La mutación no midió si el test alcanzaba: midió si el comentario decía la
verdad.** El test ya pasaba. Lo que estaba mal era la explicación escrita al
lado, que afirmaba dos direcciones de falla donde hay una.

Es la misma familia que `[[2026-09-04-un-rollback-contesta-antes-que-el-orden-de-tu-guarda]]`
— ahí un comentario le atribuía a un test una garantía que el test no daba, y
también lo agarró la mutación y no una lectura. Dos veces seguidas, el mismo
defecto y el mismo detector.

> Cuando un comentario explica **por qué** un test cubre algo, esa explicación es
> una afirmación verificable. Correr el mutante que el comentario describe es
> barato y contesta exactamente esa pregunta.

## Dónde más aplica

Toda comparación de "mismo sitio" o "mismo dueño" tiene esta forma:

- el atributo `Domain` de una cookie, y si un subdominio puede pisarla
- decidir si una redirección sale del sitio
- verificar el origen de un webhook contra una lista de dominios permitidos
- agrupar orígenes en una allowlist de CORS por dueño en vez de por host
