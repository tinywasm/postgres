# Implementation Plan: Postgres Adapter API Update (fmt.Model)

## Goal
Actualizar el adaptador `postgres` para que sea compatible con los últimos cambios arquitectónicos introducidos en `tinywasm/orm`, donde se reemplazó la interfaz de modelo y la validación a `tinywasm/fmt`.

## Proposed Changes

### [Component] Core API Update
- **Target Files:** Archivos que implementen `Compiler` y `Executor` (usualmente `adapter.go`, `compiler.go` o similares).
- **Acciones:**
  - Cambiar el uso de `orm.Model` a la nueva interfaz unificada `fmt.Model`.
  - Reemplazar cualquier dependencia directa hacia la versión antigua de validadores si los hubiera.
  - Asegurar que la construcción de esquemas de tablas consuma `ModelName()` en lugar del anticuado (y eliminado) `TableName()`.

### [Component] Tests & Mocks
- **Target Files:** Archivos de validación interna (`tests/`, etc.).
- **Acciones:**
  - Actualizar todos los mocks que simulaban entidades y compiladores del ORM para que apliquen la nueva interfaz.

## Verification Plan
- Correr el comando `gotest` dentro del directorio `postgres/` para diagnosticar que no existan incompatibilidades durante la compilación ni inconsistencias de interfaz.
