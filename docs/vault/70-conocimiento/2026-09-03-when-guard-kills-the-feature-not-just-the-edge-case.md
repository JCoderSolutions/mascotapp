---
type: bug
score: 4
topic_key: mascotapp/domain/trigger-when-guard-covers-the-common-case-not-the-edge
task: T-02-007
status: guardado
observation_id: obs-011d82c25ec9d1d5
rationale: "El comentario que justifica una guarda describe el alcance que su autor creyó, y ese alcance se usa después para decidir si la guarda se puede simplificar. Acá el autor —yo— escribió que la guarda protegía un caso de borde (desasignar) cuando en realidad protege el camino MÁS COMÚN (crear). Quien lea el comentario equivocado va a concluir que sacarla cuesta poco. La mutación es lo que midió la diferencia, y la regla general es que el alcance de una guarda se mide, no se razona."
---

# La guarda `WHEN` de un trigger cubría el caso común, no el de borde — y el comentario decía lo contrario

`00016_assignee_active_membership` instala un trigger que exige que el asignado de una
solicitud de adopción tenga una membresía `active`:

```sql
CREATE TRIGGER adoption_applications_assignee_active
    BEFORE INSERT OR UPDATE OF assigned_to_user_id ON adoption_applications
    FOR EACH ROW WHEN (NEW.assigned_to_user_id IS NOT NULL)
    EXECUTE FUNCTION assignee_must_be_active_member();
```

Escribí el comentario de la guarda `WHEN` así: *"desasignar un caso es cómo vuelve a la cola, y
`assigned_to_user_id = NULL` no matchea ninguna membresía, así que sin la guarda la función
levantaría en cada desasignación"*. Razonable, y **subestimado**.

**La mutación lo midió: sacar la guarda mata CUATRO de los cinco casos, no uno.** Y la mayoría
mueren en su propio *setup*.

El motivo: **toda solicitud de adopción se crea SIN ASIGNAR.** El trigger corre en `INSERT`
también, así que sin la guarda el `NULL` de cada alta levanta `23514` y **no se puede crear
ninguna solicitud en absoluto**. La guarda no protege un camino de borde; protege el camino
más transitado del dominio.

## Por qué importa que el comentario estuviera mal

Nadie iba a romper esto hoy. El problema es después: un comentario que dice *"esto protege la
desasignación"* invita a que alguien concluya que sacar la guarda cuesta una feature menor, y a
que lo haga en una refactorización. El comentario ahora dice lo que la mutación mostró.

## La regla transferible

**El alcance de una guarda se mide, no se razona.** Un `WHEN`, un `if` de salida temprana, una
cláusula de exclusión: cada uno tiene un conjunto de casos que deja pasar, y ese conjunto casi
nunca es el que uno nombra primero, porque uno nombra el caso que tenía en mente al escribirla
— no el que atraviesa el sistema mil veces por día.

Sacarla y contar qué muere es una operación de tres minutos y responde exactamente esa
pregunta.

## Los otros dos mutantes, y por qué la forma importa

- Sacar `AND m.status = 'active'` mató los tres rechazos y dejó verdes los dos de
  anti-vacuidad. Esa es la forma **correcta**: la propiedad de seguridad muere, la feature no.
- Sacar `INSERT OR` del evento mató **exactamente un caso** y ninguno más. Aislamiento
  perfecto: ese caso guarda ese brazo y nada más.

Un mutante que mata todo no distingue nada; un mutante que mata exactamente lo que su caso
afirma es lo que prueba que la cobertura está donde dice estar.

Relacionado: [[isolation-probe-must-use-a-writable-column]] — la otra cara, donde una sonda
medía una capa distinta de la que creía.
