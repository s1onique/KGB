// Diagnostic Timeline - Orchestration entrypoint
import type { TimelineState, TimelineEvent } from './diagnosticTimeline.model';
import { fetchTimelineResponses, buildTimelineState, buildEmptyTimelineState, buildLoadingTimelineState, buildErrorTimelineState } from './diagnosticTimeline.model';
import type { TimelineFilters } from './diagnosticTimeline.filters';
import { defaultFilters, applyFilters } from './diagnosticTimeline.filters';
import { renderTimeline } from './diagnosticTimeline.render';

// ---------------------------------------------------------------------------
// Timeline Controller
// ---------------------------------------------------------------------------

/** Timeline controller managing state, filters, and rendering */
export class DiagnosticTimelineController {
  private container: HTMLElement | null = null;
  private targetId: string = '';
  private state: TimelineState;
  private filters: TimelineFilters;
  // Track expanded event IDs for state preservation across refreshes
  private expandedEventIds: Set<string> = new Set();
  
  constructor(targetId: string) {
    this.targetId = targetId;
    this.state = buildEmptyTimelineState();
    this.filters = { ...defaultFilters };
  }

  /** Get the target ID (useful for testing) */
  getTargetId(): string {
    return this.targetId;
  }
  
  /** Mount the timeline to a container element */
  mount(containerId: string): void {
    const container = document.getElementById(containerId);
    if (!container) {
      console.error(`Timeline container #${containerId} not found`);
      return;
    }
    this.container = container;
    this.setupEventListeners();
    this.load();
  }
  
  /** Get the filtered events */
  private getFilteredEvents(): TimelineEvent[] {
    return applyFilters(this.state.mergedEvents, this.filters);
  }
  
  /** Render the current state */
  private render(): void {
    if (!this.container) return;
    renderTimeline(this.container, this.state, this.filters, this.getFilteredEvents());
  }
  
  /** Setup event listeners for filter controls and row interactions */
  private setupEventListeners(): void {
    if (!this.container) return;
    
    // Filter select changes
    this.container.addEventListener('change', (e) => {
      const target = e.target as HTMLSelectElement;
      if (target.classList.contains('filter-select')) {
        const filterName = target.dataset.filter as keyof TimelineFilters;
        const value = target.value;
        
        // Update filter based on type
        switch (filterName) {
          case 'probeKind':
            this.filters.probeKind = value === 'all' ? 'all' : (value as 'http' | 'icmp');
            break;
          case 'captureStatus':
            this.filters.captureStatus = value === 'all' ? 'all' : (value as 'captured' | 'suppressed' | 'failed');
            break;
          case 'severity':
            this.filters.severity = value === 'all' ? 'all' : (value as 'warning' | 'critical');
            break;
        }
        
        this.render();
      }
    });
    
    // Reset filters button
    this.container.addEventListener('click', (e) => {
      const target = e.target as HTMLElement;
      if (target.dataset.action === 'reset-filters') {
        this.filters = { ...defaultFilters };
        this.render();
      }
    });
    
    // Row details toggle - tracks expanded state for preservation across refresh
    this.container.addEventListener('click', (e) => {
      const target = e.target as HTMLButtonElement;
      if (target.classList.contains('timeline-details-btn')) {
        const detailsId = target.dataset.detailsId;
        const eventId = target.dataset.eventId;
        if (!detailsId) return;
        
        const detailsEl = this.container?.querySelector('#' + detailsId);
        if (detailsEl) {
          const isHidden = detailsEl.style.display === 'none' || detailsEl.style.display === '';
          detailsEl.style.display = isHidden ? 'block' : 'none';
          target.textContent = isHidden ? 'Hide' : 'Details';
          
          // Track expanded state by eventId
          if (eventId) {
            if (isHidden) {
              // Was hidden, now expanding - add to expanded set
              this.expandedEventIds.add(eventId);
            } else {
              // Was visible, now collapsing - remove from expanded set
              this.expandedEventIds.delete(eventId);
            }
          }
        }
      }
      
      // Download button
      if (target.classList.contains('download-btn')) {
        const eventId = target.dataset.eventId;
        if (!eventId) return;
        
        const event = this.state.mergedEvents.find(e => e.eventId === eventId);
        if (event) {
          this.downloadEventJson(event);
        }
      }
      
      // Copy button
      if (target.classList.contains('copy-btn')) {
        const eventId = target.dataset.eventId;
        if (!eventId) return;
        
        const event = this.state.mergedEvents.find(e => e.eventId === eventId);
        if (event) {
          this.copyEventJson(event);
        }
      }
    });
  }

  /** Restore expanded row state after render */
  private restoreExpandedState(): void {
    if (!this.container) return;
    
    for (const eventId of this.expandedEventIds) {
      // Find all rows with this eventId that are still present
      const buttons = this.container.querySelectorAll(
        `.timeline-details-btn[data-event-id="${CSS.escape(eventId)}"]`
      );
      buttons.forEach((btn) => {
        const button = btn as HTMLButtonElement;
        const detailsId = button.dataset.detailsId;
        if (!detailsId) return;
        
        const detailsEl = this.container?.querySelector('#' + detailsId);
        if (detailsEl) {
          detailsEl.style.display = 'block';
          button.textContent = 'Hide';
        }
      });
    }
  }
  
  /** Load timeline data */
  async load(): Promise<void> {
    // Show loading state
    this.state = buildLoadingTimelineState();
    this.render();
    
    try {
      const { http, icmp } = await fetchTimelineResponses(this.targetId);
      this.state = buildTimelineState(http, icmp);
    } catch (e) {
      const errorMessage = e instanceof Error ? e.message : 'Failed to load diagnostic timeline';
      this.state = buildErrorTimelineState(errorMessage);
    }
    
    this.render();
  }
  
  /** Refresh timeline data, preserving filters and expanded row state */
  async refresh(): Promise<void> {
    try {
      const { http, icmp } = await fetchTimelineResponses(this.targetId);
      this.state = buildTimelineState(http, icmp);
      this.render();
      // Restore expanded state for events that still exist
      this.restoreExpandedState();
    } catch (e) {
      console.error('Failed to refresh timeline:', e);
      // Render error state so operator sees the failure
      const errorMessage = e instanceof Error ? e.message : 'Failed to refresh diagnostic timeline';
      this.state = buildErrorTimelineState(errorMessage);
      this.render();
    }
  }
  
  /** Download event as JSON */
  private downloadEventJson(event: TimelineEvent): void {
    const exportData = {
      export_kind: 'uvb76_timeline_event',
      exported_at: new Date().toISOString(),
      target_id: this.targetId,
      event: {
        event_id: event.eventId,
        target_id: event.targetId,
        probe_kind: event.probeKind,
        severity: event.severity,
        latency_ms: event.latencyMs,
        sample_ts: event.sampleTs,
        collected_at: event.collectedAt,
        reasons: event.reasons,
        rolling_median_ms: event.rollingMedianMs,
        thresholds: event.thresholds,
        capture_status: event.captureStatus,
      },
      captures: event.captures,
    };
    
    const filename = `uvb76-timeline-${this.sanitizeFilename(this.targetId)}-${this.sanitizeFilename(event.eventId)}.json`;
    const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  }
  
  /** Copy event JSON to clipboard */
  private async copyEventJson(event: TimelineEvent): Promise<void> {
    const exportData = {
      export_kind: 'uvb76_timeline_event',
      exported_at: new Date().toISOString(),
      target_id: this.targetId,
      event: {
        event_id: event.eventId,
        target_id: event.targetId,
        probe_kind: event.probeKind,
        severity: event.severity,
        latency_ms: event.latencyMs,
        sample_ts: event.sampleTs,
        collected_at: event.collectedAt,
        reasons: event.reasons,
        rolling_median_ms: event.rollingMedianMs,
        thresholds: event.thresholds,
        capture_status: event.captureStatus,
      },
      captures: event.captures,
    };
    
    try {
      await navigator.clipboard.writeText(JSON.stringify(exportData, null, 2));
    } catch (e) {
      console.error('Failed to copy to clipboard:', e);
    }
  }
  
  /** Sanitize filename */
  private sanitizeFilename(str: string): string {
    return str.replace(/[^a-zA-Z0-9\-_]/g, '-').replace(/-+/g, '-').replace(/^-|-$/g, '').substring(0, 64) || 'unnamed';
  }
}

// ---------------------------------------------------------------------------
// Convenience Functions
// ---------------------------------------------------------------------------

/** Create and mount a timeline controller */
export function mountDiagnosticTimeline(targetId: string, containerId: string): DiagnosticTimelineController {
  const controller = new DiagnosticTimelineController(targetId);
  controller.mount(containerId);
  return controller;
}

// ---------------------------------------------------------------------------
// Legacy Compatibility (for gradual migration)
// ---------------------------------------------------------------------------

/** Load diagnostic timeline for a target (legacy interface) */
export async function loadDiagnosticTimeline(targetId: string): Promise<void> {
  const containerId = `timeline-${targetId}`;
  const controller = mountDiagnosticTimeline(targetId, containerId);
  // Controller is now managing the timeline
}
