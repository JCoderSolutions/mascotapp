---
type: convention
score: 4
topic_key: mascotapp/convention/structural-vs-behavioural-absence-tests
task: T-02-022
status: aprobado
observation_id: obs-fb93b875590f191e
aprobado: 2026-09-05
relacionado: obs-9e00e3a6413dc7d2
rationale: "Media docena de requisitos de este proyecto son ausencias: el stub no llama a la red, los logs no llevan PII, el rol público no lee borradores, las variantes de imagen no conservan EXIF. La forma refleja de testear una ausencia es ejercitar el camino y verificar que no pasó nada — y esa forma solo cubre el camino que se te ocurrió. Dentro de tres meses, cuando alguien agregue el adaptador de Resend o el pipeline de imágenes, la pregunta '¿esto lo prueba, o solo lo observó?' decide si el test sirve. La distinción es barata de aplicar y cara de descubrir tarde."
---

# Probar una ausencia por comportamiento solo prueba el camino que tomaste

(T-02-022, MascotApp Fase 02. Relacionado con `obs-9e00e3a6413dc7d2`.)

## El requisito

El spec `email-delivery` dice que el adaptador stub **no debe hacer ninguna llamada de red
saliente**. Un requisito con forma de ausencia.

## La prueba refleja, y por qué prueba menos de lo que parece

```go
http.DefaultTransport = failingTransport{t: t}   // falla el test si alguien lo usa
sendMagicLink(ctx, email.NewLogSender(logger), "adopter@example.test")
```

Verde. Y lo que quedó probado es: **esta** llamada no salió por **ese** cliente.

Pasan por al lado, sin despeinarse:

- un `&http.Client{Transport: ...}` construido a mano;
- `net.Dial` directo;
- `exec.Command("sendmail", ...)`;
- cualquier código que corra en un camino que el test no ejercitó.

El test no es inútil — cubre el caso más común y es el que atrapa un `http.Get` distraído. Pero
es una **observación sobre una corrida**, y se lee como una garantía.

## La prueba estructural

```go
// lee los .go que NO son _test.go del propio paquete y falla ante:
//   net · net/http · net/smtp · os/exec
```

Esta no prueba que no salió. Prueba que **no tiene por dónde salir**.

| | Qué afirma | Qué la rompe |
|---|---|---|
| Comportamental | esta llamada no salió por ese cliente | otro cliente, otro camino |
| Estructural | el paquete no puede alcanzar la red | agregar el import — y ahí falla |

## La regla

> **Una ausencia probada por comportamiento cubre el camino que se te ocurrió. Una ausencia
> probada por estructura cubre los que no.**
>
> Cuando el requisito es "esto NO puede pasar", preguntá si existe una propiedad estática —
> imports, grants, tipos, esquema— que lo haga imposible. Si existe, ese es el test. El
> comportamental se queda igual: son complementarios, no sustitutos.

## El segundo efecto, que es el que más vale

Un test estructural convierte el cambio futuro en una **decisión explícita**. Cuando llegue el
adaptador de Resend, alguien va a tener que darle su propio paquete o **borrar este test a
propósito** — y ese es exactamente el momento en que la decisión de diseño merece volver a
discutirse. Un test comportamental no ofrece ese momento: sigue en verde mientras el adaptador
nuevo hace lo que quiere por un camino que él no mira.

## La trampa que este test tenía adentro

`parser.ParseFile` sobre un directorio vacío no encuentra nada prohibido, así que **pasa**. Un
verde sobre cero archivos.

Por eso el test cuenta lo que parseó y falla con menos de dos archivos. Es el mismo patrón de
`obs-9e00e3a6413dc7d2` — *la capa que contesta primero vacía el test de abajo*—, acá con el
directorio en el papel de la capa: un test que se auto-vacía es peor que no tenerlo, porque
además tranquiliza.

## Dónde más aplica en este proyecto

- `internal/email` no puede alcanzar la red *(hecho, T-02-022)*
- las variantes de imagen no conservan EXIF/GPS *(F04, AD-6)*
- el rol `app_public` no puede leer borradores — acá la propiedad estática es el **grant**, no el
  import, y `00015` ya usa ese patrón
- `application_events` y `audit_log` son append-only — otra vez grants, no disciplina de código
