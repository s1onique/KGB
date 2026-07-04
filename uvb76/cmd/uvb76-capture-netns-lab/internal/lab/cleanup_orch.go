package lab

import (
	"context"
	"log"
)

// deleteNamespaces deletes the network namespaces.
func (o *Orchestrator) deleteNamespaces(ctx context.Context) error {
	if errs := o.Netns.DeleteNamespaces(ctx); len(errs) > 0 {
		for _, err := range errs {
			log.Printf("Namespace cleanup warning: %v", err)
		}
	}
	return nil
}

// writeCleanupLog writes the cleanup log to artifacts.
func (o *Orchestrator) writeCleanupLog() error {
	if len(o.CommandLog) == 0 {
		return nil
	}
	return WriteJSON(o.Artifacts.Root, "cleanup-log.json", o.CommandLog)
}

// CleanupOnError runs cleanup and reports any errors.
func (o *Orchestrator) CleanupOnError(ctx context.Context, originalErr error) error {
	log.Printf("Lab failed: %v", originalErr)
	log.Printf("Running cleanup...")

	if errs := o.Cleanup.Run(ctx); len(errs) > 0 {
		log.Printf("Cleanup errors:")
		for _, err := range errs {
			log.Printf("  - %v", err)
		}
	}

	return originalErr
}
