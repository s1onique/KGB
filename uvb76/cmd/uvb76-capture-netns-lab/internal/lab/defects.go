package lab

import (
	"context"
	"fmt"
	"log"
	"strings"
)

// injectLabProbeDefect creates the lab probe failure file to make /lab/probe return 503.
func (o *Orchestrator) injectLabProbeDefect(ctx context.Context) error {
	failureFile := o.labDir + "/tovarisch-lab-probe-failing"

	// Create the failure file
	res := o.runNsCommand(ctx, o.Config.TovarischNS.Name, "touch", failureFile)
	if !res.OK() {
		return fmt.Errorf("create failure file: %w", res.Err)
	}

	log.Printf("Lab probe defect injected: %s", failureFile)
	return nil
}

// injectDefect injects tc netem 100% loss on the tovarisch namespace interface.
// This is the lab contract defect: Phase 2 must produce skipped_cooldown.
func (o *Orchestrator) injectDefect(ctx context.Context) error {
	res := o.Netns.TC(ctx, o.Config.TovarischNS.Name,
		"qdisc", "add", "dev", "tovarisch-veth", "root", "netem", "loss", "100%")
	if !res.OK() {
		// Try replace if add fails (qdisc already exists)
		res = o.Netns.TC(ctx, o.Config.TovarischNS.Name,
			"qdisc", "replace", "dev", "tovarisch-veth", "root", "netem", "loss", "100%")
		if !res.OK() {
			return fmt.Errorf("inject tc netem loss: %w", res.Err)
		}
	}
	log.Printf("Defect injected: tc netem 100%% loss on tovarisch-veth")
	return nil
}

// clearDefect clears the defect injection.
func (o *Orchestrator) clearDefect(ctx context.Context) error {
	// Clear lab probe defect (remove failure file)
	failureFile := o.labDir + "/tovarisch-lab-probe-failing"
	o.runNsCommand(ctx, o.Config.TovarischNS.Name, "rm", "-f", failureFile)

	// Remove the netem qdisc from tovarisch namespace
	res := o.Netns.TC(ctx, o.Config.TovarischNS.Name,
		"qdisc", "del", "dev", "tovarisch-veth", "root")
	// Tolerate "No such file or directory" - qdisc may not exist
	if !res.OK() && res.ExitCode != 2 && !strings.Contains(res.Stderr, "No such file or directory") {
		log.Printf("Warning: clear defect: %v", res.Err)
	}
	log.Printf("Defect cleared: netem qdisc removed")
	return nil
}
