# itw-crud

Backend CRUD service for the ITW stack — owns PostgreSQL schema + access, exposed via REST/JSON.

Spec: [intelligent-tech-watch/docs/superpowers/specs/2026-05-13-itw-crud-design.md](https://github.com/AlexandreGuil/intelligent-tech-watch/blob/main/docs/superpowers/specs/2026-05-13-itw-crud-design.md)

## Dev

```bash
mise install
mise run test
mise run build
```

## Stack

Go 1.23+, chi router, pgx/v5, log/slog, OTel SDK, prometheus/client_golang, testcontainers-go, dbmate, distroless Docker.
