---
type: convention
score: 5
topic_key: mascotapp/convention/queries-never-filter-by-shelter-id
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-c94143b56078801e
task: T-01-033
rationale: "Parece una regla de estilo y es de seguridad: una query que filtra por shelter_id sigue devolviendo lo correcto aunque la policy desaparezca, y eso vuelve invisible la perdida de la capa que sostiene todo el modelo multi-tenant."
---

# Una query no filtra por `shelter_id`. La policy lo hace

Regla dura para todo `internal/db/query/*.sql`: **ninguna lectura lleva `shelter_id` en un
`WHERE`, en un `AND` ni en una condicion de `JOIN`.**

**Parece estilo y es seguridad.** El argumento entero a favor de RLS (§3) es que *"un `WHERE
shelter_id = ?` olvidado en una query es cuestion de tiempo"* — y que con RLS ese olvido devuelve
**cero filas** en vez de datos de otro refugio.

Una query que filtra **ella misma** invierte esa propiedad. Devuelve exactamente las mismas filas
exista o no la policy. Asi que el dia que una policy se dropee —por un `Down` mal escrito, por una
migracion que la reemplaza, por un rol mal aprovisionado— **todas esas queries siguen dando la
respuesta correcta**, y la perdida solo aparece a traves de alguna otra query que se olvido de
filtrar. La capa de seguridad se cae y el sintoma llega por el camino equivocado, tarde.

**Belt-and-braces es el instinto equivocado aca.** El cinturon tiene que ser la capa que **no se
puede olvidar**, y los tiradores no pueden **tapar su ausencia**. Duplicar una garantia en un
lugar que se olvida no la refuerza: la vuelve inobservable.

**Los INSERT estan exentos, y tienen que estarlo.** `shelter_id` es `NOT NULL` en toda tabla de
tenant, asi que una escritura lo **provee** — y `WITH CHECK` es lo que verifica el valor. Proveer
una columna no es filtrar por ella. El test distingue las dos cosas por la forma: `shelter_id`
despues de `where`, `and` u `on`, no dentro de una lista de columnas.

**El corolario que hace la regla usable:** bajo RLS, `SELECT * FROM shelters LIMIT 1` **es** "mi
refugio", y `WHERE key = $1` sobre una tabla con `UNIQUE (shelter_id, key)` resuelve a exactamente
una fila sin nombrar el refugio. Las queries salen mas cortas, no mas largas.
