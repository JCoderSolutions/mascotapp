---
fecha: 2026-09-05
tarea: T-02-022
tipo: convention
engram: obs-fb93b875590f191e
relacionado: obs-9e00e3a6413dc7d2
---

# Probar una ausencia por comportamiento solo prueba el camino que tomaste

## El requisito

El spec `email-delivery` dice que el adaptador stub **no debe hacer ninguna
llamada de red saliente**. Un requisito con forma de ausencia.

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

El test no es inútil — cubre el caso más común y atrapa un `http.Get` distraído.
Pero es una **observación sobre una corrida**, y se lee como una garantía.

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

## El segundo efecto, que es el que más vale

Un test estructural convierte el cambio futuro en una **decisión explícita**.
Cuando llegue el adaptador de Resend, alguien va a tener que darle su propio
paquete o **borrar este test a propósito** — y ese es exactamente el momento en
que la decisión de diseño merece volver a discutirse. Un test comportamental no
ofrece ese momento: sigue en verde mientras el adaptador nuevo hace lo que
quiere por un camino que él no mira.

## Cómo se agarró la trampa que el test tenía adentro

`parser.ParseFile` sobre un directorio vacío no encuentra nada prohibido, así
que **pasa**. Un verde sobre cero archivos.

Por eso el test cuenta lo que parseó y falla con menos de dos archivos. Es el
mismo patrón de `obs-9e00e3a6413dc7d2` — *la capa que contesta primero vacía el
test de abajo* —, acá con el directorio en el papel de la capa: un test que se
auto-vacía es peor que no tenerlo, porque además tranquiliza.

## Dónde más aplica

- `internal/email` no puede alcanzar la red *(hecho, T-02-022)*
- las variantes de imagen no conservan EXIF/GPS *(F04, AD-6)*
- el rol `app_public` no puede leer borradores — acá la propiedad estática es el
  **grant**, no el import, y `00015` ya usa ese patrón
- `application_events` y `audit_log` son append-only — otra vez grants, no
  disciplina de código
