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
