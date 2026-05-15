# CLAUDE.md — cerebron

Project-local conventions for all agents working in this repository.
See also `AGENTS.md` for architecture rules.

## Go conventions

**Adding a dependency**
Always run `go mod tidy` immediately after `go get` to keep `go.sum` clean.
```
go get <module>@latest
go mod tidy
```

**Changing a public function signature**
Before implementing the change, grep all call sites:
```
grep -rn "FuncName" .
```
Update test call sites in the same pass to avoid compilation failures during `go test`.

This applies to handler constructors too — adding a parameter to `NewRouter`, `NewMCPHandler`,
or any handler constructor cascades into every test file that calls it:
```
grep -rn "NewRouter\|NewMCPHandler\|NewIncidentHandler\|NewSimilarIncidentsHandler" .
```

## HTTP endpoint conventions

**New endpoints**
For any new HTTP endpoint, write or run a targeted test that validates the route returns
the expected content — not just that it compiles. This catches issues like a handler serving
from the wrong registry or reading from the wrong source before review.

Every new handler must have a `*_handler_test.go` file with at minimum:
- one success case (200 OK)
- one failure case (500 on usecase error)

Use the existing `*UseCaseStub` pattern from `test_helpers_test.go`.

## Testing conventions

**Domain constants in tests**
Before writing tests that reference domain constants (`SignalType*`, `SignalSeverity*`, `DeploymentStatus*`, etc.), grep `internal/domain/` to confirm the exact names:
```
grep -rn "= \"" internal/domain/
```
Using a non-existent constant causes a compilation failure that is only caught at `go test` time.

## Interface conventions

**No duplicate interfaces**
Do not define a new interface in a package if one with the same method signature already exists
there — reuse the existing one. This applies in particular to `handler/http` where multiple
handlers may share the same usecase contract.
