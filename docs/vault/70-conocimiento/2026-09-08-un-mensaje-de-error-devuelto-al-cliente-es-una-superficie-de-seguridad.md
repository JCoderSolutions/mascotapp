---
type: architecture
score: 5
topic_key: mascotapp/security/error-text-is-an-enumeration-oracle
task: T-02-027
status: guardado
aprobado: 2026-09-08 por el usuario
observation_id: obs-0873734ada88dedb
rationale: "Es un defecto que ninguna revisión encuentra leyendo, porque el código correcto y el incorrecto se ven igual de razonables — devolver `err.Error()` parece útil, no peligroso. Y la clase de test que lo atrapa es la que casi nunca se escribe: todo test de handler afirma lo que se escribió, y el fallo se prueba mirando solo el código de estado. Aplica a cada endpoint que se agregue de acá en adelante, y a los tres que la Fase 02 todavía no escribió: login, magic link y refresh. Además enseña que un comentario que declara una propiedad de seguridad es una afirmación verificable, no una nota."
---

# El texto de un error devuelto al cliente es una superficie de seguridad

(T-02-027, MascotApp Fase 02.)

## La regla

Un handler devuelve un mensaje **fijo** en el camino 5xx. El error real va al
log, con su contexto, y no al cuerpo de la respuesta.

Y la contraparte, que es la parte que se olvida: **hay que testear el cuerpo del
fallo**, no solo su código de estado.

## Por qué

PostgreSQL es explícito cuando rechaza. Un `INSERT` que choca con un `UNIQUE`
devuelve el nombre del constraint, la tabla, la columna, el SQLSTATE **y el
valor que colisionó**:

```
ERROR: duplicate key value violates unique constraint "shelters_slug_key"
DETAIL: Key (slug)=(refugio-choque) already exists. (SQLSTATE 23505)
```

En el endpoint de registro —**abierto a cualquiera, sin autenticar**— devolver
eso convierte un 500 en un oráculo:

- un slug duplicado dice **qué refugios existen**
- un correo duplicado dice **qué direcciones están registradas**

Lo segundo es la mitad de la prueba de indistinguibilidad que `T-02-030` va a
necesitar para el magic link. Un 500 conversador la rompe desde otro endpoint.

## La trampa

`return err.Error()` **parece útil**. Un revisor lo lee como buena ergonomía de
depuración, no como una fuga. No hay nada visualmente sospechoso.

Y la trampa de testing es peor, porque es sistemática:

> **Todo test de handler afirma lo que el handler ESCRIBIÓ.** Ninguno lee el
> cuerpo de un fallo. El camino de error se prueba con `if rec.Code != 500`, y
> ahí se termina.

## Cómo se agarró: sobrevivió a siete tests

El mutante que reemplaza el mensaje fijo por `err.Error()` **sobrevivió** a los
siete tests del archivo. Todos verdes, con la fuga puesta.

Lo notable es que **el comentario del handler ya declaraba la propiedad**: decía
que un error de base nombra columnas y constraints y que ahí eso es un oráculo
de enumeración. Estaba escrito, era correcto, y no había una sola línea que lo
verificara.

Es la tercera vez en este proyecto que un comentario afirma más de lo que su
test sostiene, y las tres veces lo agarró la mutación y no una lectura. Ver
`[[2026-09-04-un-rollback-contesta-antes-que-el-orden-de-tu-guarda]]`.

> **Un comentario que declara una propiedad de seguridad es una afirmación
> verificable.** Si no se le puede nombrar el test, no está probada — está
> escrita.

## La forma del test que lo mata

No alcanza con "no contiene el mensaje completo". Se enumera **cada cosa que el
driver diría** y se verifica que ninguna esté:

```go
for _, leak := range []string{"shelters", "slug", "duplicate", "SQLSTATE", "23505", "refugio-choque"} {
    if strings.Contains(strings.ToLower(rec.Body.String()), strings.ToLower(leak)) {
        t.Errorf("the response body leaks %q", leak)
    }
}
```

Provocar el fallo con una **causa real** —registrar dos veces el mismo slug— y
no con un doble que devuelve un error inventado. Un doble prueba el formateo del
handler; la causa real prueba qué dice la base cuando de verdad rechaza.

## Dónde más aplica

Todo endpoint que este proyecto todavía no escribió:

- login: un error distinto para "no existe" y "contraseña incorrecta" enumera cuentas
- magic link: **la indistinguibilidad es un requisito explícito del spec** (`T-02-030`)
- refresh: el motivo del rechazo le dice al ladrón si el token fue detectado
- cualquier `409` sobre un recurso con `slug` o `email` único
