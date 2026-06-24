// Diagnostic Timeline Controller - Wires DOM events, timers, and effects to the update loop

import type { TimelineModel } from './model';
import { createInitialModel } from './model';
import type { TimelineMsg } from './msg';
import { update } from './update';
import { executeEffect } from './effects';
import { renderTimeline, renderShell } from './view';
import { loadRowsPerPage, saveRowsPerPage, type SafePageSize } from './storage';

// ---------------------------------------------------------------------------
// Controller
// ---------------------------------------------------------------------------

/** Diagnostic timeline controller - wires events to update loop */
export class DiagnosticTimelineController {
  private container: HTMLElement | null = null;
  private targetId: string = '';
  private model: TimelineModel;
  private dispatch: (msg: TimelineMsg) => void;
  
  constructor(targetId: string) {
    this.targetId = targetId;
    
    // Create initial model and restore page size from localStorage
    this.model = createInitialModel();
    this.applyStoredPageSize();
    
    // Dispatch function: update model, render, then execute effects
    this.dispatch = (msg: TimelineMsg) => {
      const result = update(this.model, msg);
      this.model = result.model;
      
      // Handle storage side effects for page size changes
      if (msg.type === 'PageSizeChanged') {
        // Save to localStorage when page size changes
        saveRowsPerPage(msg.pageSize as SafePageSize);
      }
      
      // Render based on state
      if (this.container) {
        renderTimeline(this.container, this.model);
        this.restoreExpandedState();
      }
      
      // Execute side effects
      if (msg.type === 'RefreshRequested') {
        executeEffect({ type: 'FetchTimeline', targetId: this.targetId }, this.dispatch);
      }
    };
  }
  
  /** Apply stored page size from localStorage to the model */
  private applyStoredPageSize(): void {
    const storedPageSize = loadRowsPerPage();
    if (storedPageSize !== null) {
      this.model = {
        ...this.model,
        pagination: {
          ...this.model.pagination,
          pageSize: storedPageSize,
        },
      };
    }
  }

  /** Get the target ID (useful for testing) */
  getTargetId(): string {
    return this.targetId;
  }
  
  /** Get current model (for testing) */
  getModel(): TimelineModel {
    return this.model;
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
  
  /** Restore expanded row state after render - no longer needed, view handles this */
  private restoreExpandedState(): void {
    // Expansion state is now rendered directly from model.expandedEventIds in view.ts
    // This method is kept for backwards compatibility but does nothing
  }
  
  /** Setup event listeners for filter, pagination, and row interactions */
  private setupEventListeners(): void {
    if (!this.container) return;
    
    // Change events (selects)
    this.container.addEventListener('change', (e) => {
      const target = e.target as HTMLSelectElement;
      
      // Page size change
      if (target.classList.contains('page-size-select')) {
        const newPageSize = parseInt(target.value, 10);
        if (!isNaN(newPageSize)) {
          this.dispatch({ type: 'PageSizeChanged', pageSize: newPageSize });
        }
        return;
      }
      
      // Filter select changes
      if (target.classList.contains('filter-select')) {
        const filterName = target.dataset.filter;
        
        switch (filterName) {
          case 'probeKind':
            this.dispatch({
              type: 'FilterChanged',
              filters: { probeKind: target.value as 'all' | 'http' | 'icmp' }
            });
            break;
          case 'captureStatus':
            this.dispatch({
              type: 'FilterChanged',
              filters: { captureStatus: target.value as 'all' | 'captured' | 'suppressed' | 'failed' }
            });
            break;
          case 'severity':
            this.dispatch({
              type: 'FilterChanged',
              filters: { severity: target.value as 'all' | 'warning' | 'critical' }
            });
            break;
        }
      }
    });
    
    // Click events
    this.container.addEventListener('click', (e) => {
      const target = e.target as HTMLElement;
      
      // Reset filters button
      if (target.dataset.action === 'reset-filters') {
        this.dispatch({ type: 'FiltersReset' });
        return;
      }
      
      // Pagination navigation buttons
      if (target.dataset.action === 'page-first') {
        this.dispatch({ type: 'PageChanged', page: 0 });
        return;
      }
      if (target.dataset.action === 'page-prev') {
        this.dispatch({ type: 'PageChanged', page: this.model.pagination.pageIndex - 1 });
        return;
      }
      if (target.dataset.action === 'page-next') {
        this.dispatch({ type: 'PageChanged', page: this.model.pagination.pageIndex + 1 });
        return;
      }
      if (target.dataset.action === 'page-last') {
        // Use the same filtering logic as view.ts
        const filtered = this.model.mergedEvents.filter(e => {
          if (this.model.filters.probeKind !== 'all' && e.probeKind !== this.model.filters.probeKind) return false;
          if (this.model.filters.captureStatus !== 'all' && e.captureStatus !== this.model.filters.captureStatus) return false;
          if (this.model.filters.severity !== 'all' && e.severity !== this.model.filters.severity) return false;
          return true;
        });
        const totalPages = Math.max(1, Math.ceil(filtered.length / this.model.pagination.pageSize));
        this.dispatch({ type: 'PageChanged', page: totalPages - 1 });
        return;
      }
      
      // Page number buttons
      if (target.dataset.pageNumber !== undefined) {
        const pageNum = parseInt(target.dataset.pageNumber, 10);
        if (!isNaN(pageNum)) {
          this.dispatch({ type: 'PageChanged', page: pageNum });
        }
        return;
      }
      
      // Row details toggle
      const buttonTarget = e.target as HTMLButtonElement;
      if (buttonTarget.classList.contains('timeline-details-btn')) {
        const detailsId = buttonTarget.dataset.detailsId;
        const eventId = buttonTarget.dataset.eventId;
        if (!detailsId) return;
        
        const detailsEl = this.container?.querySelector('#' + detailsId);
        if (detailsEl) {
          const isHidden = detailsEl.style.display === 'none';
          detailsEl.style.display = isHidden ? '' : 'none';
          buttonTarget.textContent = isHidden ? 'Hide' : 'Details';
          
          // Track expanded state by eventId
          if (eventId) {
            this.dispatch({ type: 'RowToggled', eventId });
          }
        }
        return;
      }
      
      // Download button
      if (buttonTarget.classList.contains('download-btn')) {
        const eventId = buttonTarget.dataset.eventId;
        if (!eventId) return;
        
        const event = this.model.mergedEvents.find(ev => ev.eventId === eventId);
        if (event) {
          this.downloadEventJson(event);
        }
        return;
      }
      
      // Copy button
      if (buttonTarget.classList.contains('copy-btn')) {
        const eventId = buttonTarget.dataset.eventId;
        if (!eventId) return;
        
        const event = this.model.mergedEvents.find(ev => ev.eventId === eventId);
        if (event) {
          this.copyEventJson(event);
        }
      }
    });
  }
  
  /** Load timeline data */
  private load(): void {
    // Show loading state
    this.dispatch({ type: 'LoadStarted' });
    
    // Trigger fetch
    this.dispatch({ type: 'RefreshRequested' });
  }
  
  /** Refresh timeline data, preserving filters and expanded row state */
  async refresh(): Promise<void> {
    this.dispatch({ type: 'RefreshRequested' });
  }
  
  /** Download event as JSON */
  private downloadEventJson(event: import('../diagnosticTimeline.model').TimelineEvent): void {
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
  private async copyEventJson(event: import('../diagnosticTimeline.model').TimelineEvent): Promise<void> {
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
