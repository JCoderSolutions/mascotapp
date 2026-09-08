---
type: bug
score: 4
topic_key: mascotapp/convention/enumerated-guard-must-quantify
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-535e166e505a8722
task: T-01-027
rationale: "El guard enumerado se escribio en T-01-025 justamente para cerrar una lista a mano, y a la tarea siguiente dejo pasar un mutante: preguntaba si EXISTE una referencia compuesta, no si TODAS lo son."
---

# Un guard enumerado tambien puede preguntar de menos: cuantifica

Enumerar en vez de listar a mano fue la leccion de T-01-022, T-01-023 y T-01-025. **No alcanza.**
El guard enumerado tiene su propio modo de falla, y aparecio una tarea despues de escribirlo.

`TestTenantChildren_ReferenceTheirParentCompositely` recorria `Schema.TenantChildren` y
preguntaba:

> ¿tiene esta tabla **UNA** foreign key compuesta que incluya `shelter_id`?

Un mutante que redujo `form_submissions.application_id` a una sola columna **sobrevivio igual**:
la tabla seguia teniendo su clave compuesta a `form_template_versions`, asi que el `EXISTS`
contestaba que si. La enumeracion era correcta —recorria la declaracion, no una lista— y **el
cuantificador era el equivocado**.

La forma correcta es universal, no existencial:

> **TODA** foreign key de esta tabla que referencie una tabla de tenant tiene que incluir
> `shelter_id`.

Con una sola excepcion, y hay que nombrarla o el guard se muerde la cola: el `shelter_id
REFERENCES shelters (id)` de la propia tabla, cuya clave es exactamente `shelter_id` y que es lo
que le da sentido a la columna en primer lugar.

**La regla general:** cuando un guard recorre una declaracion, el `EXISTS` es casi siempre el
cuantificador equivocado. `EXISTS` dice *"algo cumple"*, que es lo que uno escribe sin pensar y
es lo que deja pasar la segunda referencia, la segunda policy, la segunda columna. Lo que se
quiere afirmar casi siempre es `NOT EXISTS (algo que incumple)` — que ademas puede **nombrar** lo
que fallo, mientras que un `EXISTS` en falso solo puede decir que no encontro nada.

**Y el guard sigue necesitando su propia anti-vacuidad**: con la version universal, una tabla que
perdiera *todas* sus claves pasaria el bucle por no tener nada que recorrer. Las dos aserciones
van juntas.

Ver [[2026-09-01-la-conformidad-de-d5-se-pregunta-desde-el-hijo]] e
[[2026-08-31-una-lista-a-mano-en-un-inventario-excluye-en-silencio]].
