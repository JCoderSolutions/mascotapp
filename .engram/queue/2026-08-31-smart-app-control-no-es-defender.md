---
type: constraint
score: 5
topic_key: mascotapp/ops/windows-appcontrol-go-tests
approved: 2026-08-31 por el usuario
observation_id: obs-09d0eb05887b634f
task: T-01-020
supersedes:
  - 2026-08-29-windows-appcontrol-blocks-go-test-binaries.md
  - 2026-08-30-defender-no-application-control.md
rationale: "Este proyecto registro TRES causas y las tres eran falsas, y la cura que recomendo -- una exclusion de Defender -- no habria hecho absolutamente nada. Sin esto, la cuarta sesion vuelve a perder una tarde en el mismo lugar."
---

# No es Defender. Es **Windows Smart App Control**, y no tiene lista de exclusiones

El sintoma nunca cambio:

```
fork/exec .../.gotmp/go-build.../b001/db.test.exe:
    An Application Control policy has blocked this file.
```

Lo que cambio es que en T-01-020 dejo de ser intermitente y bloqueo **todas** las corridas.
Eso hizo barato lo que en tres tareas nadie hizo: preguntarle al sistema en vez de teorizar.

```
Get-MpComputerStatus
  AMRunningMode              : Passive Mode
  RealTimeProtectionEnabled  : False

HKLM:\SYSTEM\CurrentControlSet\Control\CI\Policy
  VerifiedAndReputablePolicyState : 1      # 0 apagado, 1 enforcement, 2 evaluacion
```

**Defender ni siquiera esta activo.** Esta en modo pasivo con la proteccion en tiempo real
apagada. La exclusion de Defender que la entrada anterior recomendaba como "lo que arregla"
no habria tenido efecto alguno — y peor, se habria leido como que el problema quedaba
resuelto.

## Por que este bloqueo no se puede sortear desde el repo

Smart App Control bloquea ejecutables **sin firma y sin reputacion**. A diferencia de
Defender, **no tiene lista de exclusiones**: esta prendido o apagado, y apagarlo **no se
puede deshacer sin reinstalar Windows**. Cada `go test` linkea un binario nuevo sin firma,
asi que no existe una ubicacion, un flag ni un target que lo evite.

Y explica el detalle que sostenia los diagnosticos falsos: `golangci-lint`, `govulncheck` y
`go vet` siguen funcionando porque ya tienen reputacion establecida. Solo se bloquea lo
recien linkeado. Por eso el lint nunca mostro el sintoma, y por eso parecia intermitente.

## El arreglo real

`make test-api-container`: la misma suite, sobre Linux, donde Smart App Control no existe.
Dos montajes son estructurales y ninguno es obvio — el socket de Docker, porque
testcontainers arranca Postgres como contenedor **hermano** (de ahi
`TESTCONTAINERS_HOST_OVERRIDE=host.docker.internal`), y el `GOMODCACHE` del host con
`GOPROXY=off`, porque la red intercepta TLS y las descargas de modulos mueren con
`certificate signed by unknown authority`.

## La leccion de proceso, corregida

La entrada anterior decia: "cuando el sintoma no nombra un componente, consegui un mensaje
que si lo nombre". Buen consejo, mal ejecutado — porque el mensaje de cuarentena que
aparecio despues **tambien** era ambiguo, y se lo tomo como confirmacion.

La version que aguanta: **un mensaje de error no es una fuente de diagnostico, es una
pista.** La fuente es consultarle el estado al componente. Tres teorias encadenadas costaron
tres tareas de workarounds porque ninguna se verifico contra el sistema, solo contra si el
sintoma se iba esa vez.

Relacionado: [[2026-08-30-defender-no-application-control]],
[[2026-08-29-windows-appcontrol-blocks-go-test-binaries]].
