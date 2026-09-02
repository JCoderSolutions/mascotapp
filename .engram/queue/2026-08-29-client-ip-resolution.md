---
type: architecture
score: 5
topic_key: mascotapp/security/client-ip-resolution
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-878517463d368981
task: T-00-022
rationale: "Cualquiera que toque rate limiting, audit log o el despliegue va a preguntarse de dónde sale la IP del cliente y por qué no se usa la librería estándar. Sin esto se reintroduce middleware.RealIP."
---

La IP de cliente en MascotApp se resuelve con **la primera entrada no confiable
de `X-Forwarded-For`, escaneando de derecha a izquierda**, en
`internal/httpapi/clientip.go`. `middleware.RealIP` de chi está prohibido: es
spoofeable (GHSA-3fxj-6jh8-hvhx) porque reescribe `r.RemoteAddr` desde la
entrada **más a la izquierda** sin verificar quién es el peer.

Por qué derecha a izquierda es seguro y izquierda a derecha no: cada proxy de
reenvío **agrega** la dirección que observó al final de la cadena. Lo que un
cliente falsifica queda entonces a la izquierda de lo que agregó la
infraestructura propia, y el escaneo desde la derecha se detiene en el valor
agregado antes de llegar a la parte falsificada. Es una propiedad del protocolo,
no una heurística.

Reglas que acompañan al algoritmo y que no son obvias leyendo el código:

- Si el peer TCP **no** es un proxy confiable, el header se ignora entero. No
  llegó por nuestra infraestructura, así que es input del cliente sin validar.
- Una entrada malformada **detiene** el escaneo; nunca se saltea. Saltearla
  permitiría a quien ya está dentro de un rango confiable usar basura como
  muralla para que el escaneo siga hasta una dirección que él mismo falsificó.
- Se leen **todos** los headers `X-Forwarded-For` repetidos, no solo el primero.
  Usar `Header.Get` permitiría partir la cadena en varias líneas para evadir el
  escaneo.
- `X-Real-IP` y `True-Client-IP` se ignoran a propósito: nada en el destino de
  despliegue los escribe.
- `TRUSTED_PROXIES` por defecto está **vacío**, lo que hace que toda petición
  resuelva al peer TCP. Es fallo cerrado deliberado: un despliegue sin
  configurar obtiene una dirección menos precisa, nunca una controlada por el
  atacante. Fijar los rangos reales es tarea de despliegue (arrastre en FASE-11).
