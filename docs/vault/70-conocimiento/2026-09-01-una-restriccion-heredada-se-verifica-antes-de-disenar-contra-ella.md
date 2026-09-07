---
type: architecture
score: 5
topic_key: mascotapp/arch/verify-inherited-constraints
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-ed6c4139b505a445
task: T-01-034
rationale: "D3 generalizo desde UN caso verdadero a todos, sobrevivio como regla sin que nadie la probara, y ya habia desviado el esquema de la propuesta antes de que se cayera en un contenedor descartable."
---

# Una restriccion heredada se verifica ANTES de disenar contra ella

**D3** decia: *"no dependemos de extensiones de PostgreSQL, porque el free tier de Neon no las
garantiza"*.

Se formo a partir de **un** caso verdadero: `pg_uuidv7` genuinamente **no** esta en Neon. Y de ahi
salto a **todas**. Bajo esa regla, `users.email` iba a ser `text` con un indice unico sobre
`lower(email)`, desviandose de lo que §4 escribe.

**La premisa se verifico falsa en cinco minutos.** `citext` no es una apuesta: Neon lo documenta
en una pagina dedicada, y `postgres:17-alpine` trae `citext 1.6` de fabrica — confirmado corriendo
`CREATE EXTENSION` en un contenedor descartable. **Los dos entornos objetivo lo tienen.**

**Lo que costo mientras nadie la probaba** no fue tiempo: fue una desviacion del modelo de datos.
Con el indice sobre `lower(email)`, un `WHERE email = $1` **falla en silencio** contra una
direccion escrita distinto, salvo que **cada** call site se acuerde de normalizar al leer y al
escribir. O sea, correccion apoyada en disciplina — que es exactamente el modo de falla que
[[ADR-0002]] existe para eliminar. La regla no verificada estaba a punto de reintroducir el
problema que la arquitectura entera trata de sacar.

**La forma general:** una restriccion heredada —de un ADR anterior, de un README, de "ya sabemos
que X no se puede"— es una **afirmacion sobre el mundo**, y las afirmaciones sobre el mundo se
chequean. Sobre todo estas dos formas:

- **la generalizacion desde un caso**: "esta extension no esta" -> "no usamos extensiones";
  "este proveedor no soporta X" -> "X no se puede".
- **la restriccion con fecha implicita**: "verificado en agosto de 2026" no es "verificado". Vuelve
  a chequearse cuando cambia el proveedor o la version mayor.

**Un "no se puede" sin evidencia cuesta mas que un "probemoslo".** El costo de probar es un
contenedor descartable; el costo de no probar es un diseno torcido que despues nadie sabe por que
esta torcido.

Y el corolario de proceso: cuando una restriccion se angosta, **el alcance viejo se dice en voz
alta**. D3 no se rompio, se **angosto** — `pg_uuidv7` sigue rechazado, UUIDv7 se sigue generando
en Go. Escribir "D3 queda angostado, no roto" es lo que evita que la proxima persona lea la
excepcion como que la regla no existe.
