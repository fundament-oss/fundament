# Contributing to Fundament

Welcome, fellow developer! Happy to see you're interested to join the fun.

Please follow these guidelines in your contributions.

## Project Structure

```
fundament/
├── build/                      # Build artifacts
│   ├── docker/                 # Production Dockerfiles
│   └── hotreload/              # Development hot-reload configs (Air)
│       ├── appstore-api/
│       ├── authn-api/
│       └── organization-api/
├── cmd/                        # Application entrypoints
│   ├── appstore-api/
│   ├── authn-api/
│   └── organization-api/
├── db/                         # Database schema and migrations
├── deployments/                # Deployment configurations
│   ├── charts/fundament/       # Helm chart
│   └── k3d/                    # Local k3d cluster config
├── gen/                        # Generated code (do not edit)
│   └── proto/                  # Generated protobuf/connect code
├── internal/                   # Private application code
│   ├── appstore/
│   ├── authn/                  # Authentication service
│   │   ├── authn/              # Core auth logic
│   │   ├── db/                 # Database queries (sqlc)
│   │   └── http/               # HTTP handlers (oapi-codegen)
│   └── organization/           # Organization service
│       ├── db/
│       └── organization/
├── pkg/                        # Shared library code
│   └── psqldb/                 # PostgreSQL utilities
├── proto/                      # Protocol buffer definitions
│   ├── authn/v1/
│   └── organization/v1/
└── web/                        # Frontend applications
    ├── console/                # Angular admin console
    └── login/                  # React login frontend
```

## NeRDS

This project follows the [Nederlandse Richtlijn Digitale Systemen (NeRDS)](https://minbzk.github.io/NeRDS/production/richtlijnen/) as a baseline for quality and consistency.

In practice, there may be situations where we take a pragmatic approach and deviate from these guidelines. Such deviations are acceptable when they serve the project’s needs.

## Workflow

We accept contributions from developers and users through GitHub PRs.

## Technologies

Do not introduce a new programming language or tech-stack dependency without prior discussion and approval.

## Styling

### Markdown

- Use [Github Flavored Markdown](https://github.github.com/gfm/).
- Use a single line per paragraph. If you prefer text to be wrapped, use soft-wrapping in your text editor.
