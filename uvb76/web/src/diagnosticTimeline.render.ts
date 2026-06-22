// Diagnostic Timeline Render - Render summary cards, filter controls, table rows, and expanded evidence panels
import type { TimelineEvent, ProbeKindSummary, TimelineState } from './diagnosticTimeline.model';
import type { TimelineFilters } from './diagnosticTimeline.filters';
import { formatSpikeTime, formatLatencyMs } from './format';
import { formatLocalDateTime, parseApiInstant, formatRemainingCooldown } from './time';

// ---------------------------------------------------------------------------
// XSS Protection
// ---------------------------------------------------------------------------

/** HTML escape helper for XSS protection */
function escapeText(s: string | null | undefined): string {
  if (!s) return '';
  const div = document.createElement('div');
  div.textContent = s;
  return div.innerHTML;
}

// ---------------------------------------------------------------------------
// CSS Class Helpers
// ---------------------------------------------------------------------------

/** Get CSS class for probe kind */
function getProbeKindClass(probeKind: string): string {
  return probeKind === 'http' ? 'probe-http' : 'probe-icmp';
}

/** Get CSS class for severity badge */
function getSeverityClass(severity: string): string {
  return severity === 'critical' ? 'severity-critical' : 'severity-warn';
}

/** Get CSS class for capture status badge */
function getCaptureStatusClass(status: string): string {
  switch (status) {
    case 'captured': return 'capture-captured';
    case 'suppressed': return 'capture-suppressed';
    case 'failed': return 'capture-failed';
    case 'not_attempted': return 'capture-muted';
    default: return 'capture-muted';
  }
}

/** Get label for capture status */
function getCaptureStatusLabel(status: string): string {
  switch (status) {
    case 'captured': return 'captured';
    case 'suppressed': return 'suppressed';
    case 'failed': return 'failed';
    case 'not_attempted': return 'not attempted';
    default: return status;
  }
}

// ---------------------------------------------------------------------------
// Summary Card Rendering
// ---------------------------------------------------------------------------

/** Render a single probe kind summary card */
export function renderSummaryCard(summary: ProbeKindSummary): string {
  const { probeKind, totalEvents, capturedCount, suppressedCount, failedCount, criticalCount, warningCount } = summary;
  
  const label = probeKind === 'http' ? 'HTTP' : 'ICMP';
  const className = getProbeKindClass(probeKind);
  
  return `
    <div class="timeline-summary-card ${className}">
      <div class="timeline-summary-header">
        <span class="timeline-summary-label">${label}</span>
        <span class="timeline-summary-total">${totalEvents} events</span>
      </div>
      <div class="timeline-summary-stats">
        <div class="timeline-summary-stat">
          <span class="stat-captured">${capturedCount}</span>
          <span class="stat-label">captured</span>
        </div>
        <div class="timeline-summary-stat">
          <span class="stat-suppressed">${suppressedCount}</span>
          <span class="stat-label">suppressed</span>
        </div>
        <div class="timeline-summary-stat">
          <span class="stat-failed">${failedCount}</span>
          <span class="stat-label">failed</span>
        </div>
        <div class="timeline-summary-stat">
          <span class="stat-critical">${criticalCount}</span>
          <span class="stat-label">critical</span>
        </div>
        <div class="timeline-summary-stat">
          <span class="stat-warning">${warningCount}</span>
          <span class="stat-label">warning</span>
        </div>
      </div>
    </div>
  `;
}

/** Render both summary cards */
export function renderSummaryCards(httpSummary: ProbeKindSummary, icmpSummary: ProbeKindSummary): string {
  return `
    <div class="timeline-summary-container">
      ${renderSummaryCard(httpSummary)}
      ${renderSummaryCard(icmpSummary)}
    </div>
  `;
}

// ---------------------------------------------------------------------------
// Filter Controls Rendering
// ---------------------------------------------------------------------------

/** Render filter controls */
export function renderFilterControls(filters: TimelineFilters, totalCount: number, filteredCount: number): string {
  const hasActiveFilters = filters.probeKind !== 'all' || filters.captureStatus !== 'all' || filters.severity !== 'all';
  
  return `
    <div class="timeline-filters">
      <div class="filter-group">
        <label class="filter-label">Probe:</label>
        <select class="filter-select" data-filter="probeKind">
          <option value="all" ${filters.probeKind === 'all' ? 'selected' : ''}>All</option>
          <option value="http" ${filters.probeKind === 'http' ? 'selected' : ''}>HTTP</option>
          <option value="icmp" ${filters.probeKind === 'icmp' ? 'selected' : ''}>ICMP</option>
        </select>
      </div>
      <div class="filter-group">
        <label class="filter-label">Status:</label>
        <select class="filter-select" data-filter="captureStatus">
          <option value="all" ${filters.captureStatus === 'all' ? 'selected' : ''}>All</option>
          <option value="captured" ${filters.captureStatus === 'captured' ? 'selected' : ''}>Captured</option>
          <option value="suppressed" ${filters.captureStatus === 'suppressed' ? 'selected' : ''}>Suppressed</option>
          <option value="failed" ${filters.captureStatus === 'failed' ? 'selected' : ''}>Failed</option>
        </select>
      </div>
      <div class="filter-group">
        <label class="filter-label">Severity:</label>
        <select class="filter-select" data-filter="severity">
          <option value="all" ${filters.severity === 'all' ? 'selected' : ''}>All</option>
          <option value="warning" ${filters.severity === 'warning' ? 'selected' : ''}>Warning</option>
          <option value="critical" ${filters.severity === 'critical' ? 'selected' : ''}>Critical</option>
        </select>
      </div>
      <div class="filter-count">
        Showing ${filteredCount} of ${totalCount}
      </div>
      ${hasActiveFilters ? '<button class="filter-reset-btn" data-action="reset-filters">Clear filters</button>' : ''}
    </div>
  `;
}

// ---------------------------------------------------------------------------
// Timestamp Formatting
// ---------------------------------------------------------------------------

/** Format timestamp for display using local timezone */
function formatTimelineTime(timestamp: string | undefined): string {
  const date = parseApiInstant(timestamp ?? null);
  if (!date) return '—';
  return formatLocalDateTime(timestamp);
}

// ---------------------------------------------------------------------------
// Cross-Probe Suppression Wording
// ---------------------------------------------------------------------------

/** Render cross-probe suppression explanation */
function renderCrossProbeSuppression(event: TimelineEvent): string {
  const capture = event.primaryCapture;
  if (!capture?.cooldown_info) return '';
  
  const info = capture.cooldown_info;
  if (!info.is_cross_probe_suppression) return '';
  
  const anchorKind = (info.anchor_probe_kind || 'unknown').toUpperCase();
  const suppressedKind = (info.suppressed_probe_kind || event.probeKind).toUpperCase();
  
  return `
    <div class="cross-probe-explanation">
      <span class="cross-probe-text">${suppressedKind} spike suppressed by prior ${anchorKind} diagnostic capture due to source-level cooldown.</span>
    </div>
  `;
}

// ---------------------------------------------------------------------------
// Cooldown Details
// ---------------------------------------------------------------------------

/** Render cooldown details section */
function renderCooldownDetails(event: TimelineEvent): string {
  const capture = event.primaryCapture;
  if (!capture?.cooldown_info) return '';
  
  const info = capture.cooldown_info;
  const anchorKind = escapeText(info.anchor_probe_kind || '');
  const suppressedKind = escapeText(info.suppressed_probe_kind || '');
  const isCrossProbe = info.is_cross_probe_suppression === true;
  const anchorCaptureId = escapeText(info.anchor_capture_id || '');
  const scope = escapeText(info.scope || '');
  const cooldownKey = escapeText(info.cooldown_key || '');
  
  let html = '<div class="cooldown-details">';
  html += '<div class="cooldown-section-title">Cooldown Information</div>';
  
  if (isCrossProbe) {
    html += `<div class="cooldown-row"><span class="cooldown-label">Anchor probe:</span><span class="cooldown-value">${anchorKind}</span></div>`;
    html += `<div class="cooldown-row"><span class="cooldown-label">Suppressed probe:</span><span class="cooldown-value">${suppressedKind}</span></div>`;
  }
  
  if (info.last_successful_capture_at) {
    html += `<div class="cooldown-row"><span class="cooldown-label">Anchor capture:</span><span class="cooldown-value">${formatTimelineTime(info.last_successful_capture_at)}</span></div>`;
  }
  
  if (anchorCaptureId) {
    html += `<div class="cooldown-row"><span class="cooldown-label">Anchor capture ID:</span><span class="cooldown-value capture-id">${anchorCaptureId}</span></div>`;
  }
  
  if (scope) {
    html += `<div class="cooldown-row"><span class="cooldown-label">Scope:</span><span class="cooldown-value">${scope}</span></div>`;
  }
  
  if (cooldownKey) {
    html += `<div class="cooldown-row"><span class="cooldown-label">Cooldown key:</span><span class="cooldown-value">${cooldownKey}</span></div>`;
  }
  
  if (info.next_capture_eligible_at) {
    html += `<div class="cooldown-row"><span class="cooldown-label">Next eligible:</span><span class="cooldown-value">${formatTimelineTime(info.next_capture_eligible_at)}</span></div>`;
  }
  
  if (info.remaining_cooldown_ms !== undefined && info.remaining_cooldown_ms > 0) {
    const formatted = formatRemainingCooldown(info.remaining_cooldown_ms);
    html += `<div class="cooldown-row"><span class="cooldown-label">Remaining cooldown:</span><span class="cooldown-value">${formatted}</span></div>`;
  }
  
  if (!info.anchor_visible && info.anchor_visibility_reason) {
    let reasonText = 'outside current view';
    if (info.anchor_visibility_reason === 'evicted_from_retention') {
      reasonText = 'anchor evicted from retention';
    } else if (info.anchor_visibility_reason === 'suppressed_cooldown') {
      reasonText = 'anchor also suppressed by cooldown';
    }
    html += `<div class="cooldown-row"><span class="cooldown-label">Anchor:</span><span class="cooldown-value anchor-hidden">${reasonText}</span></div>`;
  }
  
  html += '</div>';
  return html;
}

// ---------------------------------------------------------------------------
// Capture Details
// ---------------------------------------------------------------------------

/** Render capture details section */
function renderCaptureDetailsSection(event: TimelineEvent): string {
  const capture = event.primaryCapture;
  if (!capture) return '';
  
  const source = escapeText(capture.source);
  const duration = capture.duration_ms !== undefined ? ` (${capture.duration_ms}ms)` : '';
  const statusClass = getCaptureStatusClass(event.captureStatus);
  const statusLabel = getCaptureStatusLabel(event.captureStatus);
  
  let html = '<div class="capture-details-section">';
  html += '<div class="capture-section-title">Capture Details</div>';
  html += `<div class="capture-row-header"><span class="capture-source-label">${source}${duration}</span><span class="capture-status-badge ${statusClass}">${statusLabel}</span></div>`;
  
  html += `<div class="detail-row"><span class="detail-label">Started:</span><span class="detail-value">${formatTimelineTime(capture.capture_started_at)}</span></div>`;
  
  if (capture.capture_finished_at) {
    html += `<div class="detail-row"><span class="detail-label">Finished:</span><span class="detail-value">${formatTimelineTime(capture.capture_finished_at)}</span></div>`;
  }
  
  if (capture.duration_ms !== undefined) {
    html += `<div class="detail-row"><span class="detail-label">Duration:</span><span class="detail-value">${capture.duration_ms} ms</span></div>`;
  }
  
  if (capture.error) {
    const errorText = escapeText(capture.error);
    const truncated = errorText.length > 80 ? errorText.substring(0, 80) + '…' : errorText;
    html += `<div class="detail-row error-row"><span class="detail-label">Error:</span><span class="detail-value error-text">${truncated}</span></div>`;
  }
  
  html += '</div>';
  return html;
}

// ---------------------------------------------------------------------------
// Network Diagnostics
// ---------------------------------------------------------------------------

/** Render network diagnostics section */
function renderNetworkDiagSection(event: TimelineEvent): string {
  const capture = event.primaryCapture;
  if (!capture?.network_diag) return '';
  
  // Don't show network diag if suppressed by cooldown
  if (capture.suppressed_by_cooldown) {
    return '<div class="network-diag-section"><span class="diag-muted">Network diagnostics suppressed by cooldown</span></div>';
  }
  
  const diag = capture.network_diag;
  const statusClass = diag.status === 'ok' ? 'status-ok' : 'status-warn';
  
  let html = '<div class="network-diag-section">';
  html += '<div class="network-section-title">Network Diagnostics</div>';
  html += `<div class="detail-row"><span class="detail-label">Status:</span><span class="detail-value"><span class="${statusClass}">${escapeText(diag.status)}</span></span></div>`;
  
  if (diag.underlay_tcp && diag.underlay_tcp.length > 0) {
    html += '<div class="tcp-sockets">';
    for (const sock of diag.underlay_tcp) {
      const name = escapeText(sock.name) || 'socket';
      const state = escapeText(sock.state) || 'UNKNOWN';
      html += `<div class="tcp-socket-row">`;
      html += `<span class="tcp-socket-name">${name}</span>`;
      html += `<span class="tcp-socket-state">${state}</span>`;
      if (sock.rtt_ms !== undefined) {
        html += `<span class="tcp-socket-rtt">RTT: ${sock.rtt_ms.toFixed(1)} ms</span>`;
      }
      if (sock.retransmits !== undefined) {
        html += `<span class="tcp-socket-retrans">retrans: ${sock.retransmits}</span>`;
      }
      html += '</div>';
    }
    html += '</div>';
  } else if (diag.underlay_tcp !== undefined) {
    html += '<span class="diag-muted">No TCP sockets captured</span>';
  }
  
  html += '</div>';
  return html;
}

// ---------------------------------------------------------------------------
// Expanded Row Rendering
// ---------------------------------------------------------------------------

/** Render expanded evidence panel for an event */
export function renderExpandedPanel(event: TimelineEvent, rowIndex: number): string {
  const detailsId = `timeline-details-${rowIndex}`;
  
  let html = `<div class="timeline-expanded-panel" id="${detailsId}">`;
  
  // Event metadata
  html += '<div class="event-metadata">';
  html += `<div class="detail-row"><span class="detail-label">Event ID:</span><span class="detail-value event-id">${escapeText(event.eventId)}</span></div>`;
  html += `<div class="detail-row"><span class="detail-label">Sample time:</span><span class="detail-value">${formatTimelineTime(event.sampleTs)}</span></div>`;
  html += `<div class="detail-row"><span class="detail-label">Collected at:</span><span class="detail-value">${formatTimelineTime(event.collectedAt)}</span></div>`;
  html += `<div class="detail-row"><span class="detail-label">Latency:</span><span class="detail-value latency-value">${formatLatencyMs(event.latencyMs)}</span></div>`;
  html += `<div class="detail-row"><span class="detail-label">Rolling median:</span><span class="detail-value">${event.rollingMedianMs} ms</span></div>`;
  
  if (event.reasons.length > 0) {
    html += `<div class="detail-row"><span class="detail-label">Reasons:</span><span class="detail-value">${escapeText(event.reasons.join(', '))}</span></div>`;
  }
  
  html += '</div>';
  
  // Cross-probe suppression wording
  html += renderCrossProbeSuppression(event);
  
  // Capture details
  html += renderCaptureDetailsSection(event);
  
  // Cooldown details
  html += renderCooldownDetails(event);
  
  // Network diagnostics
  html += renderNetworkDiagSection(event);
  
  // Action buttons
  html += '<div class="timeline-actions">';
  html += `<button class="timeline-action-btn copy-btn" data-event-id="${escapeText(event.eventId)}">Copy JSON</button>`;
  html += `<button class="timeline-action-btn download-btn" data-event-id="${escapeText(event.eventId)}">Download</button>`;
  html += '</div>';
  
  html += '</div>';
  return html;
}

// ---------------------------------------------------------------------------
// Timeline Row Rendering
// ---------------------------------------------------------------------------

/** Render a single timeline row */
export function renderTimelineRow(event: TimelineEvent, rowIndex: number): string {
  const time = formatSpikeTime(event.sampleTs);
  const latency = formatLatencyMs(event.latencyMs);
  const probeKindClass = getProbeKindClass(event.probeKind);
  const severityClass = getSeverityClass(event.severity);
  const captureStatusClass = getCaptureStatusClass(event.captureStatus);
  const captureStatusLabel = getCaptureStatusLabel(event.captureStatus);
  
  // Get error text if failed
  let detailsText = '—';
  if (event.captureStatus === 'failed' && event.primaryCapture?.error) {
    const error = event.primaryCapture.error;
    detailsText = error.length > 60 ? error.substring(0, 57) + '…' : error;
  }
  
  const detailsId = `timeline-details-${rowIndex}`;
  
  return `
    <tr class="timeline-row" data-row-index="${rowIndex}">
      <td class="timeline-cell time-cell">${time}</td>
      <td class="timeline-cell probe-cell"><span class="probe-badge ${probeKindClass}">${event.probeKind.toUpperCase()}</span></td>
      <td class="timeline-cell severity-cell"><span class="severity-badge ${severityClass}">${event.severity.toUpperCase()}</span></td>
      <td class="timeline-cell latency-cell">${latency}</td>
      <td class="timeline-cell capture-cell"><span class="capture-badge ${captureStatusClass}">${captureStatusLabel}</span></td>
      <td class="timeline-cell details-cell">${escapeText(detailsText)}</td>
      <td class="timeline-cell action-cell">
        <button class="timeline-details-btn" data-details-id="${detailsId}" data-row-index="${rowIndex}">Details</button>
      </td>
    </tr>
  `;
}

// ---------------------------------------------------------------------------
// Timeline Table Rendering
// ---------------------------------------------------------------------------

/** Render the complete timeline table */
export function renderTimelineTable(events: TimelineEvent[]): string {
  if (events.length === 0) {
    return '<div class="timeline-empty">No diagnostic events in the selected range.</div>';
  }
  
  const rows = events.map((event, index) => renderTimelineRow(event, index)).join('');
  
  // Expanded panels
  const expandedPanels = events.map((event, index) => renderExpandedPanel(event, index)).join('');
  
  return `
    <table class="timeline-table" aria-label="Diagnostic timeline">
      <thead>
        <tr>
          <th scope="col">Time</th>
          <th scope="col">Probe</th>
          <th scope="col">Severity</th>
          <th scope="col">Latency</th>
          <th scope="col">Capture</th>
          <th scope="col">Details</th>
          <th scope="col"></th>
        </tr>
      </thead>
      <tbody>
        ${rows}
      </tbody>
    </table>
    <div class="timeline-expanded-panels">
      ${expandedPanels}
    </div>
  `;
}

// ---------------------------------------------------------------------------
// Loading and Error States
// ---------------------------------------------------------------------------

/** Render loading state */
export function renderLoadingState(): string {
  return `
    <div class="timeline-loading">
      <div class="spinner"></div>
      <span>Loading diagnostic timeline...</span>
    </div>
  `;
}

/** Render error state */
export function renderErrorState(error: string): string {
  return `
    <div class="timeline-error">
      <span class="error-icon">⚠</span>
      <span class="error-text">${escapeText(error)}</span>
    </div>
  `;
}

/** Render empty state */
export function renderEmptyState(): string {
  return `
    <div class="timeline-empty-state">
      <div class="empty-icon">📭</div>
      <div class="empty-title">No Diagnostic Events</div>
      <div class="empty-text">No HTTP or ICMP spike diagnostics have been recorded for this target.</div>
    </div>
  `;
}

// ---------------------------------------------------------------------------
// Complete Timeline Render
// ---------------------------------------------------------------------------

/** Render the complete timeline component */
export function renderTimeline(
  container: HTMLElement,
  state: TimelineState,
  filters: TimelineFilters,
  filteredEvents: TimelineEvent[]
): void {
  const { isLoading, error, httpSummary, icmpSummary } = state;
  
  // Loading state
  if (isLoading) {
    container.innerHTML = `
      <div class="timeline-header">
        <span class="timeline-title">Diagnostic Timeline</span>
      </div>
      ${renderLoadingState()}
    `;
    return;
  }
  
  // Error state
  if (error) {
    container.innerHTML = `
      <div class="timeline-header">
        <span class="timeline-title">Diagnostic Timeline</span>
      </div>
      ${renderErrorState(error)}
    `;
    return;
  }
  
  // Summary cards
  const summaryCards = renderSummaryCards(httpSummary, icmpSummary);
  
  // Filter controls
  const filterControls = renderFilterControls(filters, state.mergedEvents.length, filteredEvents.length);
  
  // Timeline table or empty state
  const timelineContent = filteredEvents.length === 0 && state.mergedEvents.length === 0
    ? renderEmptyState()
    : renderTimelineTable(filteredEvents);
  
  container.innerHTML = `
    <div class="timeline-header">
      <span class="timeline-title">Diagnostic Timeline</span>
    </div>
    ${summaryCards}
    ${filterControls}
    ${timelineContent}
  `;
}
