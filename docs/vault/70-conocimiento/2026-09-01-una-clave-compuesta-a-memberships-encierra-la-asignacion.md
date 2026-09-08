---
type: architecture
score: 5
topic_key: mascotapp/arch/assignment-bounded-by-membership-key
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-cb3fdedda1fa5e49
task: T-01-027
rationale: "Ninguna policy puede cachar un valor que el tenant escribe en su propia fila. La clave compuesta a memberships convierte una regla de handler en una que la base hace cumplir, y la clave referenciada ya existia."
---

# Una clave compuesta a `memberships` encierra la asignacion dentro del refugio

`adoption_applications.assigned_to_user_id` **no** es una referencia a `users`. Es una clave
compuesta a **`memberships (user_id, shelter_id)`**.

**Por que ninguna policy alcanza:** el tenant escribe ese valor en **SU PROPIA fila**. La policy
de aislamiento mira `shelter_id`, ese valor es correcto, y `WITH CHECK` pasa. La regla *"el
asignado tiene que ser de este refugio"* es sobre OTRA columna, y RLS no la ve. Con
`REFERENCES users (id)` a secas, un refugio asigna un caso a cualquier persona del sistema — y
el nombre de esa persona renderiza en la cola de ese refugio.

**Por que la clave existe sin haberla pedido:** `memberships` ya lleva `UNIQUE (user_id,
shelter_id)` por su cuenta, para que nadie tenga dos membresias en el mismo refugio. Resulta ser
**exactamente** la clave referenciada que D5 pide. No hubo que agregar nada.

**Nullable a proposito.** `assigned_to_user_id` es NULL en una aplicacion sin asignar, que es el
estado normal de una nueva. `MATCH SIMPLE` — el default de PostgreSQL — **saltea el chequeo
entero** cuando cualquier columna de la clave es NULL, asi que la restriccion no estorba.

**La forma general, que vale para toda columna de referencia que un tenant escribe:** preguntar
si existe una tabla que ya relacione ese valor con el shelter. Si existe y tiene la clave unica,
la referencia compuesta es gratis y convierte una regla de aplicacion en una invariante de base.
Si no existe, la regla queda en el dominio y hay que decirlo, no suponerlo.

Ver [[2026-09-01-la-conformidad-de-d5-se-pregunta-desde-el-hijo]].
