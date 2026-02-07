# Fundament Console frontend

## Development server

Run the cluster with `just dev` or `just dev-hotreload`, open your browser, and navigate to `http://console.fundament.localhost:8080/`. When using `just dev-hotreload`, the application will automatically reload whenever you modify any of the source files.

Credentials:

- Un: `admin@example.com`
- Pw: `password`

## Linting and formatting

To lint or format the code, use:

```bash
bun lint
bun format
```

## Building

To build the project (automatically done with `just dev`), run:

```bash
ng build
```

This will compile your project and store the build artifacts in the `dist/` directory. By default, the production build optimizes your application for performance and speed.

## Running unit tests

To execute unit tests with the [Vitest](https://vitest.dev/) test runner, use the following command:

```bash
ng test
```
