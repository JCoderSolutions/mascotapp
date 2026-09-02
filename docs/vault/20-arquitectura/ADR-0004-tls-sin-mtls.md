# ADR-0004 — TLS 1.3 sin mTLS

- **Fecha:** 2026-08-28
- **Estado:** aceptado
- **Fase:** 00

## Contexto

Se evaluó si hacía falta mTLS, por experiencia previa en integraciones con VISA donde
era obligatorio.

## Decisión

**No se implementa mTLS en v1. TLS 1.3 es suficiente.**

## Análisis canal por canal

| Canal | Veredicto |
|---|---|
| Navegador a API | **Inviable.** Cliente público; no hay forma operativa de distribuir y rotar certificados a adoptantes anónimos |
| API a Neon Postgres | **No ofrecido.** Neon autentica con SCRAM sobre TLS; no expone auth por certificado de cliente |
| API a Cloudflare R2 | **No aplica.** Firma HMAC SigV4 sobre TLS |
| API a Resend y Sentry | **No aplica.** Bearer token sobre TLS |

## Por qué en VISA sí y aquí no

En redes de tarjetas, mTLS es **mandato contractual del adquirente bajo PCI-DSS**: canal
B2B, endpoints fijos, ambas partes controlan su extremo, y el dato en tránsito es el PAN.
Acá **no se procesan pagos**, así que el proyecto queda **fuera de alcance PCI**.
El driver que forzaba mTLS no existe.

## Costo que lo descarta por sí solo

Cloud Run **no soporta certificados de cliente** en su URL `*.run.app`. mTLS en GCP exige
un Global External Application Load Balancer más Certificate Manager: regla de reenvío con
costo fijo mensual (~USD 18–25) más USD 0.45 por millón de conexiones mTLS. Destruye
ADR-0003 para proteger canales que ya están protegidos.

## Lo que sí se implementa

- TLS 1.3 mínimo; TLS 1.2 solo con suites AEAD.
- HSTS con `preload`.
- `sslmode=verify-full` a Postgres con CA pinneada. **Esto neutraliza el MITM** que mTLS
  también neutralizaría.
- Access tokens de 15 min más rotación de refresh con detección de reutilización.

## Consecuencia que se evita

mTLS tiene un modo de fallo que casi nadie contabiliza: **un certificado vencido es caída
total del servicio**, no degradación. Más rotación, distribución y CRL/OCSP que mantener.

## Condiciones de reapertura

1. Integrar un procesador de pagos o banco que lo exija por contrato.
2. Exponer una API administrativa a un conjunto fijo y pequeño de máquinas propias.
3. Introducir un segundo servicio propio sobre una red no controlada.
4. Un refugio institucional lo exija para intercambio de datos B2B.

Si alguna se cumple, el alcance es **ese canal**, nunca el tráfico de navegador.
