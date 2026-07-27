package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nxdir-s/gopher/internal/adapters"
	"github.com/nxdir-s/gopher/internal/config"
	"github.com/nxdir-s/gopher/internal/core/domain"
	"github.com/nxdir-s/gopher/internal/logs"
	"github.com/nxdir-s/gopher/templates"
)

// Version is set at build time with -ldflags
var Version string = "dev"

func main() {
	os.Exit(run())
}

// run wires the dependencies and dispatches, returning the exit code
func run() int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger := logs.NewLogger(os.Stderr, len(os.Getenv(logs.DebugEnv)) > 0)

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve the working directory: %s\n", err.Error())

		return adapters.ExitError
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())

		return adapters.ExitError
	}

	storeOpts := make([]adapters.StoreOpt, 0, 3)
	for _, dir := range cfg.TemplateDirs() {
		storeOpts = append(storeOpts, adapters.WithTemplateDir(dir))
	}

	store := adapters.NewStoreAdapter(templates.FS, templates.Root, storeOpts...)
	writer := adapters.NewFsAdapter()
	registry := domain.NewRegistry()

	generator, err := domain.NewGenerator(logger,
		domain.WithRegistry(registry),
		domain.WithTemplateSource(store),
		domain.WithRenderer(adapters.NewTemplateAdapter()),
		domain.WithFormatter(adapters.NewFormatAdapter()),
		domain.WithFileWriter(writer),
		domain.WithMerger(adapters.NewGoSourceAdapter()),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create generator: %s\n", err.Error())

		return adapters.ExitError
	}

	catalog, err := domain.NewCatalog(logger,
		domain.WithTemplateCatalog(store),
		domain.WithCatalogWriter(writer),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create catalog: %s\n", err.Error())

		return adapters.ExitError
	}

	cli := adapters.NewCliAdapter(generator, catalog, registry, cfg, Version)

	return cli.Run(ctx, os.Args[1:])
}
