// Package app is the application assembly layer: it composes the Container,
// instantiates Context modules, drives the four-stage lifecycle, mounts the
// router, merges OpenAPI specs, and hosts the HTTP server bootstrap ([Run]).
//
// Skeleton (P0): the package exists so that future PRs can land container.go,
// modules.go, bootstrap.go, router.go, migrations.go and openapi.go without
// further structural churn (see aranea/docs/0 main design.md §6).
package app
