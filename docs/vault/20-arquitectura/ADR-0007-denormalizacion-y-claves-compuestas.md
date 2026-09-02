# ADR-0007 — Denormalización de `shelter_id` en tablas hijas, sostenida por claves compuestas

- **Fecha:** 2026-09-01
- **Estado:** aceptado
- **Fase:** 01

## Contexto

[[ADR-0002]] pone la última línea de defensa en la base: cada tabla con `shelter_id` lleva una
política RLS, y un `WHERE` olvidado devuelve cero filas en vez de datos de otro refugio.

Eso deja una pregunta abierta para las tablas **hijas**. Una foto pertenece a un animal, y el
animal a un refugio. ¿La foto lleva su propio `shelter_id`, o se deriva del animal con un
`EXISTS`?

Y hay un hecho de PostgreSQL que decide la respuesta, verificado y no supuesto:

> **Los chequeos de integridad referencial SIEMPRE pasan por encima de row security.** Corren
> como el dueño de la constraint, no como el rol que consulta.

Así que una clave `pet_id REFERENCES pets (id)` **resuelve el animal de otro refugio en nombre de
este tenant**. El tenant no puede *leer* esa fila y la está referenciando igual. Y ninguna
política lo ve: la fila que inserta lleva **su propio** `shelter_id`, así que `WITH CHECK` está
satisfecho y se corre al costado.

## Decisión

**Donde una fila pertenece a exactamente un refugio, se denormaliza `shelter_id` y se sostiene la
verdad de esa columna con una clave foránea COMPUESTA a `padre (id, shelter_id)`.** Cada padre
declara un `UNIQUE (id, shelter_id)` para que la clave tenga a qué apuntar.

```sql
CONSTRAINT pet_media_pet_fkey
    FOREIGN KEY (pet_id, shelter_id) REFERENCES pets (id, shelter_id)
```

El par `(pet_id, shelter_id)` es **irresoluble** si el animal es de otro refugio: no existe una
fila de `pets` con ese id y ese shelter. La clave contesta, no la política.

**Donde una fila pertenece a varios refugios**, la denormalización es imposible y se usa un
`EXISTS` a través de una tabla puente. `users` es la única excepción sancionada del esquema: una
persona puede ser miembro de N refugios y adoptante en otros.

**Rechazar no es la propiedad. Rechazar IGUAL sí lo es.** Un padre ajeno y uno inexistente tienen
que devolver el **mismo** SQLSTATE **y el mismo nombre de constraint. Dos rechazos distinguibles
siguen siendo un oráculo: el tenant B enumera los animales de A leyendo la diferencia en el error.

## Alternativas descartadas

| Alternativa | Por qué no |
|---|---|
| Política con `EXISTS` sobre el padre en cada hija | Funciona para leer y **no cierra la escritura**: el `EXISTS` se evalúa bajo RLS, pero la foreign key no, así que el hijo sigue pudiendo apuntar al padre ajeno. Además cada lectura paga un subquery. |
| Clave de una sola columna al padre, confiando en la política | Es exactamente el agujero de arriba. Verificado: una mutación que reduce la clave compuesta a una sola columna **sobrevivió la suite entera** en T-01-025 y en T-01-027. Nada que lea puede notarlo. |
| `shelter_id` derivado por un trigger en vez de por una clave | Un trigger es código; una clave es una invariante. Y un trigger `BEFORE INSERT` no impide que un `UPDATE` posterior mueva la fila. |

## Consecuencias

**A favor**

- Un hijo cruzado es **imposible**, no improbable. La base lo rechaza sin consultar a nadie.
- La política de cada hija es la misma plantilla directa que la del padre. Sin subqueries, sin
  casos especiales, sin costo de lectura.
- El `UNIQUE (id, shelter_id)` del padre es barato y ya paga por sí mismo.

**En contra, y se acepta a sabiendas**

- **`shelter_id` está duplicado en cada hija.** Es denormalización, con su costo de espacio y la
  posibilidad teórica de divergir — que es justamente lo que la clave compuesta impide.
- **Cada padre carga un `UNIQUE (id, shelter_id)` que se lee redundante** al lado de la primary
  key, porque `id` ya es único. Es el blanco perfecto de un "esto sobra" en una limpieza
  posterior. Por eso hay un test que lo fija por tabla.
- **La conformidad hay que preguntarla desde el HIJO, y por enumeración.** Un inventario que mira
  a los padres no ve una hija que perdió su clave. Y el cuantificador correcto es **universal**:
  *toda* referencia a una tabla de tenant incluye `shelter_id`. Preguntar si existe **una**
  referencia compuesta deja pasar la segunda — pasó con `form_submissions.application_id`.

**Una consecuencia que no es de seguridad y decide diseño igual:** la dirección del `ON DELETE`
no la fija la costumbre, la decide **qué es el hijo**. Si el hijo es la **evidencia** de lo que
pasó, va `RESTRICT` — con `CASCADE`, borrar el padre borra el rastro por una puerta que nadie
mira. Si el hijo es el **dato personal** cuyo motivo de existir era el padre, va `CASCADE` — con
`RESTRICT`, una purga de retención deja la PII atrás. Y si es las dos cosas, está mal modelado y
hay que partirlo.

## Condiciones de reapertura

1. Que PostgreSQL cambie la semántica de los chequeos referenciales bajo RLS. La premisa entera
   de este ADR es que **no** los aplican.
2. Que aparezca una entidad hija que legítimamente pertenezca a varios refugios. Ahí la
   denormalización deja de ser posible y hay que usar la carve-out de `EXISTS`, con la regla que
   la acompaña: **quien puede ESCRIBIR el puente puede LEER lo que el puente hace visible.**
3. Que la duplicación de `shelter_id` se vuelva un costo de almacenamiento medible en el free
   tier. Hoy son 16 bytes por fila.
