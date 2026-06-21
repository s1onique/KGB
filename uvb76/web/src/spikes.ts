// Spike diagnostics rendering module
import { api, type SpikeEventWithCaptures, type DiagCapture, type TcpSocketDiagData, type NetworkDiagData, type SpikeRetentionStats, type CaptureCooldownInfo, type TcpAbsenceEvent, type AnchorCaptureResponse } from './api';
import { formatSpikeTime, formatLatencyMs } from './format';
import { formatLocalDateTime, parseApiInstant, formatRemainingCooldown } from './time';

// HTML escape helper for XSS protection
function escapeText(s: string | null | undefined): string {
  if (!s) return '';
  const div = document.createElement('div');
  div.textContent = s;
  return div.innerHTML;
}

// Capture status types for display - explicit statuses matching backend
type CaptureStatusDisplay = 'captured' | 'skipped_cooldown' | 'failed' | 'disabled' | 'not_configured' | 'not_attempted' | 'in_progress' | 'missing' | 'none';

// Format capture status for display - uses explicit capture_status field from backend
function formatCaptureStatus(capture: DiagCapture): CaptureStatusDisplay {
  // First check explicit capture_status from backend
  if (capture.capture_status) {
    return capture.capture_status as CaptureStatusDisplay;
  }
  // Fallback to legacy suppressed_by_cooldown flag
  if (capture.suppressed_by_cooldown) {
    return 'skipped_cooldown';
  }
  switch (capture.status) {
    case 'ok':
      return 'captured';
    case 'error':
      return 'failed';
    case 'timeout':
      return 'failed';
    case 'disabled':
      return 'disabled';
    default:
      return 'none';
  }
}

// Get CSS class for capture status
function getCaptureStatusClass(status: CaptureStatusDisplay): string {
  switch (status) {
    case 'captured':
      return 'status-ok';
    case 'in_progress':
      return 'status-warn';
    case 'failed':
      return 'status-error';
    case 'skipped_cooldown':
      return 'status-suppressed';
    case 'disabled':
    case 'not_configured':
    case 'not_attempted':
    case 'missing':
    case 'none':
    default:
      return 'status-muted';
  }
}

// Get human-readable capture status label
function getCaptureStatusLabel(status: CaptureStatusDisplay): string {
  switch (status) {
    case 'captured':
      return 'captured';
    case 'in_progress':
      return 'in progress';
    case 'failed':
      return 'failed';
    case 'skipped_cooldown':
      return 'skipped: cooldown';
    case 'disabled':
      return 'disabled';
    case 'not_configured':
      return 'not configured';
    case 'not_attempted':
      return 'not attempted';
    case 'missing':
      return 'missing';
    case 'none':
    default:
      return 'none';
  }
}

// Format safe error text (compact, no sensitive details)
function formatError(error: string | null | undefined): string {
  if (!error) return '';
  const text = escapeText(error);
  if (text.length > 80) {
    return text.substring(0, 80) + '…';
  }
  return text;
}

// Format thresholds for display
function formatThresholds(reasons: string[]): string {
  if (!reasons || reasons.length === 0) return '';
  const thresholdParts: string[] = [];
  for (const reason of reasons) {
    if (reason.includes('critical')) {
      thresholdParts.push('critical');
    } else if (reason.includes('warning')) {
      thresholdParts.push('warning');
    }
    if (reason.includes('relative') || reason.includes('10x')) {
      thresholdParts.push('relative 10x median');
    }
  }
  const unique = [...new Set(thresholdParts)];
  return unique.join(', ') || '';
}

// Format target name (short, safe)
function formatTargetName(targetId: string): string {
  if (targetId.length > 20) {
    return escapeText(targetId.substring(0, 17)) + '…';
  }
  return escapeText(targetId);
}

// Format timestamp for display using local timezone (consistent with table).
// Uses parseApiInstant to validate explicit timezone, then formats as local time.
function formatAnchorTime(timestamp: string | undefined): string {
  // Use parseApiInstant which rejects timezone-less formats
  const date = parseApiInstant(timestamp ?? null);
  if (!date) return '—';

  // Format using local timezone (same as table timestamps)
  return formatLocalDateTime(timestamp);
}

// Render cooldown anchor explanation for a capture
function renderCooldownAnchorExplanation(capture: DiagCapture): string {
  const cooldownInfo = capture.cooldown_info;
  const isSkippedCooldown = formatCaptureStatus(capture) === 'skipped_cooldown';
  
  if (!isSkippedCooldown) return '';
  
  // Case C: Missing cooldown metadata - render warning
  if (!cooldownInfo) {
    return '<div class="cooldown-warning">' +
      '<span class="cooldown-warning-icon">⚠</span>' +
      '<span class="cooldown-warning-text">Cooldown metadata missing</span>' +
      '<div class="cooldown-warning-detail">Skipped by cooldown, but no prior capture anchor was provided</div>' +
      '</div>';
  }
  
  // Check if anchor is hidden (outside current view)
  if (!cooldownInfo.anchor_visible) {
    const reason = cooldownInfo.anchor_visibility_reason || 'unknown';
    let reasonText = 'outside current view';
    if (reason === 'outside_filter_window') {
      reasonText = 'outside current view';
    } else if (reason === 'evicted_from_retention') {
      reasonText = 'anchor evicted from retention';
    } else if (reason === 'suppressed_cooldown') {
      reasonText = 'anchor also suppressed by cooldown';
    }
    
    const anchorTime = formatAnchorTime(cooldownInfo.last_successful_capture_at);
    const nextEligible = formatAnchorTime(cooldownInfo.next_capture_eligible_at);
    const remainingMs = cooldownInfo.remaining_cooldown_ms;
    const cooldownKey = escapeText(cooldownInfo.cooldown_key || '');
    const scope = escapeText(cooldownInfo.scope || '');
    const anchorProbeKind = escapeText(cooldownInfo.anchor_probe_kind || '');
    const suppressedProbeKind = escapeText(cooldownInfo.suppressed_probe_kind || '');
    const anchorCaptureId = escapeText(cooldownInfo.anchor_capture_id || '');
    const isCrossProbe = cooldownInfo.is_cross_probe_suppression;
    
    let html = '<div class="cooldown-anchor-explanation hidden-anchor">';
    
    // Cross-probe suppression message: explain what happened clearly
    if (isCrossProbe && anchorProbeKind && suppressedProbeKind) {
      html += '<div class="cooldown-explanation-summary">';
      html += '<span class="cross-probe-suppression">' + suppressedProbeKind.toUpperCase() + ' spike suppressed by prior ' + anchorProbeKind.toUpperCase() + ' diagnostic capture due to source-level cooldown</span>';
      html += '</div>';
    } else {
      html += '<div class="cooldown-explanation-summary">Prior diagnostic capture is outside the current view</div>';
    }
    
    html += '<div class="cooldown-anchor-details">';
    
    // Show anchor probe kind and suppressed probe kind for cross-probe cases
    if (isCrossProbe) {
      html += '<div class="cooldown-detail-row"><span class="cooldown-detail-label">Suppressed probe:</span> <span class="cooldown-detail-value">' + suppressedProbeKind.toUpperCase() + '</span></div>';
      html += '<div class="cooldown-detail-row"><span class="cooldown-detail-label">Anchor probe:</span> <span class="cooldown-detail-value">' + anchorProbeKind.toUpperCase() + '</span></div>';
    }
    
    if (anchorTime !== '—') {
      html += '<div class="cooldown-detail-row"><span class="cooldown-detail-label">Anchor:</span> <span class="cooldown-detail-value">prior successful capture at ' + anchorTime + '</span></div>';
    }
    
    if (anchorCaptureId) {
      html += '<div class="cooldown-detail-row"><span class="cooldown-detail-label">Capture ID:</span> <span class="cooldown-detail-value capture-id">' + anchorCaptureId + '</span></div>';
    }
    
    if (scope) {
      html += '<div class="cooldown-detail-row"><span class="cooldown-detail-label">Scope:</span> <span class="cooldown-detail-value">' + scope + '</span></div>';
    }
    
    if (cooldownKey) {
      html += '<div class="cooldown-detail-row"><span class="cooldown-detail-label">Cooldown key:</span> <span class="cooldown-detail-value">' + cooldownKey + '</span></div>';
    }
    
    if (nextEligible !== '—') {
      html += '<div class="cooldown-detail-row"><span class="cooldown-detail-label">Next eligible:</span> <span class="cooldown-detail-value">' + nextEligible + '</span></div>';
    }
    
    if (remainingMs !== undefined && remainingMs > 0) {
      // Use formatRemainingCooldown for adaptive formatting (ms vs seconds)
      // Label clarifies this is the value "at decision" time, not live
      const formattedRemaining = formatRemainingCooldown(remainingMs);
      html += '<div class="cooldown-detail-row"><span class="cooldown-detail-label">Remaining cooldown:</span> <span class="cooldown-detail-value">' + formattedRemaining + ' (at decision)</span></div>';
    }
    
    html += '<div class="cooldown-detail-row"><span class="cooldown-detail-label">Reason:</span> <span class="cooldown-detail-value">' + reasonText + '</span></div>';
    
    // Show degraded message when anchor spike event is outside retention or view
    // evicted_from_retention: spike event was purged from retention entirely
    if (reason === 'evicted_from_retention') {
      html += '<div class="cooldown-detail-row anchor-degraded-message">Anchor capture artifact is available; original ' + anchorProbeKind.toUpperCase() + ' spike event is outside retention</div>';
    } else if (reason === 'outside_filter_window') {
      // outside_filter_window: spike event exists but is outside the current view/filter window
      html += '<div class="cooldown-detail-row anchor-degraded-message">Anchor capture artifact is available; original ' + anchorProbeKind.toUpperCase() + ' spike event is outside the current view/filter window</div>';
    }
    
    html += '</div></div>';
    return html;
  }
  
  // Case B: Visible anchor - render less alarming explanation
  if (cooldownInfo.anchor_visible && cooldownInfo.anchor_visibility_reason === 'retained_visible') {
    const anchorTime = formatAnchorTime(cooldownInfo.last_successful_capture_at);
    const anchorProbeKind = escapeText(cooldownInfo.anchor_probe_kind || '');
    const suppressedProbeKind = escapeText(cooldownInfo.suppressed_probe_kind || '');
    const isCrossProbe = cooldownInfo.is_cross_probe_suppression;
    
    let html = '<div class="cooldown-anchor-explanation visible-anchor">';
    
    // Cross-probe suppression message
    if (isCrossProbe && anchorProbeKind && suppressedProbeKind) {
      html += '<div class="cooldown-explanation-summary">';
      html += '<span class="cross-probe-suppression">' + suppressedProbeKind.toUpperCase() + ' spike suppressed by prior ' + anchorProbeKind.toUpperCase() + ' diagnostic capture</span>';
      html += '</div>';
    } else {
      html += '<div class="cooldown-explanation-summary">Skipped because a recent diagnostic capture is already retained</div>';
    }
    
    if (anchorTime !== '—') {
      html += '<div class="cooldown-anchor-time">Prior capture at ' + anchorTime + '</div>';
    }
    html += '</div>';
    return html;
  }
  
  return '';
}

// Check if any capture has hidden anchor (for summary context)
function hasHiddenAnchor(cooldownInfo: CaptureCooldownInfo | undefined): boolean {
  return cooldownInfo !== undefined && !cooldownInfo.anchor_visible;
}

// Default retention stats for backward compatibility
const defaultRetentionStats: SpikeRetentionStats = {
  retained_spike_count: 0,
  visible_spike_count: 0,
  protected_capture_count: 0,
  purge_eligible_count: 0,
  max_uncaptured_spikes: 200,
};

// Render retention summary
function renderRetentionSummary(retention: SpikeRetentionStats): string {
  const {
    visible_spike_count,
    retained_spike_count,
    protected_capture_count,
    purge_eligible_count,
    max_uncaptured_spikes
  } = retention;

  return '<div class="retention-summary">' +
    '<span class="retention-label">Spike diagnostics:</span> ' +
    '<span class="retention-value">showing ' + visible_spike_count + ' newest of ' + retained_spike_count + ' retained</span> ' +
    '<span class="retention-separator">—</span> ' +
    '<span class="retention-value">' + protected_capture_count + ' protected by captures</span>' +
    '<span class="retention-separator">,</span> ' +
    '<span class="retention-value">' + purge_eligible_count + ' purge-eligible</span>' +
    '<span class="retention-separator">,</span> ' +
    '<span class="retention-value">max uncaptured retained: ' + max_uncaptured_spikes + '</span>' +
    '</div>';
}

// Format TCP socket details for expanded view
function formatTcpDetails(sockets: TcpSocketDiagData[]): string {
  if (!sockets || sockets.length === 0) return '<span class="diag-muted">No TCP sockets captured</span>';

  return sockets.map(sock => {
    const name = escapeText(sock.name) || 'socket';
    const state = escapeText(sock.state) || 'UNKNOWN';
    const fields: string[] = [];
    fields.push('<span class="tcp-field"><span class="tcp-label">socket:</span> ' + name + '</span>');
    fields.push('<span class="tcp-field"><span class="tcp-label">state:</span> ' + state + '</span>');
    if (sock.rtt_ms !== undefined) {
      fields.push('<span class="tcp-field"><span class="tcp-label">RTT:</span> ' + sock.rtt_ms.toFixed(1) + ' ms</span>');
    }
    if (sock.rto_ms !== undefined) {
      fields.push('<span class="tcp-field"><span class="tcp-label">RTO:</span> ' + sock.rto_ms + ' ms</span>');
    }
    if (sock.retransmits !== undefined) {
      fields.push('<span class="tcp-field"><span class="tcp-label">retransmits:</span> ' + sock.retransmits + '</span>');
    }
    if (sock.unacked !== undefined) {
      fields.push('<span class="tcp-field"><span class="tcp-label">unacked:</span> ' + sock.unacked + '</span>');
    }
    if (sock.cwnd !== undefined) {
      fields.push('<span class="tcp-field"><span class="tcp-label">cwnd:</span> ' + sock.cwnd + '</span>');
    }
    return '<div class="tcp-socket-row">' + fields.join('') + '</div>';
  }).join('');
}

// Format TCP absence reason for display
function formatTcpAbsenceReason(reasonCode: string): string {
  const reasons: Record<string, string> = {
    'no_matching_socket': 'no matching socket found',
    'socket_closed_before_capture': 'socket closed before capture',
    'command_failed': 'diagnostic command failed',
    'not_configured': 'TCP diagnostics are disabled for this peer',
    'permission_denied': 'permission denied for diagnostic commands',
    'target_not_tcp': 'probe path is not TCP',
    'target_mapping_missing': 'target peer mapping not found',
    'parse_failed': 'failed to parse diagnostic output',
    'unsupported_platform': 'platform does not support TCP diagnostics',
    'underlay_tcp_disabled': 'underlay TCP diagnostics disabled by config',
  };
  return reasons[reasonCode] || reasonCode;
}

// Render TCP absence explanation
function renderTcpAbsenceExplanation(absenceEvents: TcpAbsenceEvent[]): string {
  if (!absenceEvents || absenceEvents.length === 0) return '';
  
  const html = absenceEvents.map(event => {
    const reason = formatTcpAbsenceReason(event.reason_code);
    const source = escapeText(event.source);
    
    // Build structured rows for operator-friendly display
    const rows: string[] = [];
    
    // First row: the main reason (prominent)
    rows.push('<div class="tcp-absence-reason-row"><span class="tcp-absence-reason">' + reason + '</span></div>');
    
    // Expected peer row (if present)
    if (event.expected_peer) {
      rows.push('<div class="tcp-absence-detail-row"><span class="tcp-absence-label">Expected peer:</span> <span class="tcp-absence-value">' + escapeText(event.expected_peer) + '</span></div>');
    }
    
    // Expected port row (if present)
    if (event.expected_port !== undefined) {
      rows.push('<div class="tcp-absence-detail-row"><span class="tcp-absence-label">Expected port:</span> <span class="tcp-absence-value">' + event.expected_port + '</span></div>');
    }
    
    // Probe kind row (if present)
    if (event.probe_kind) {
      rows.push('<div class="tcp-absence-detail-row"><span class="tcp-absence-label">Probe:</span> <span class="tcp-absence-value">' + escapeText(event.probe_kind) + '</span></div>');
    }
    
    // Namespace row (if present)
    if (event.namespace) {
      rows.push('<div class="tcp-absence-detail-row"><span class="tcp-absence-label">Namespace:</span> <span class="tcp-absence-value">' + escapeText(event.namespace) + '</span></div>');
    }
    
    // Tool/command row (if present)
    if (event.command_tool) {
      rows.push('<div class="tcp-absence-detail-row"><span class="tcp-absence-label">Tool:</span> <span class="tcp-absence-value">' + escapeText(event.command_tool) + '</span></div>');
    }
    
    // Raw match count row (if present)
    if (event.raw_match_count !== undefined) {
      rows.push('<div class="tcp-absence-detail-row"><span class="tcp-absence-label">Raw matches:</span> <span class="tcp-absence-value">' + event.raw_match_count + '</span></div>');
    }
    
    // Detail row (if present and not already covered)
    if (event.detail) {
      rows.push('<div class="tcp-absence-detail-row"><span class="tcp-absence-label">Detail:</span> <span class="tcp-absence-value">' + escapeText(event.detail) + '</span></div>');
    }
    
    return '<div class="tcp-absence-event"><span class="tcp-absence-source">[' + source + ']</span><div class="tcp-absence-rows">' + rows.join('') + '</div></div>';
  }).join('');
  
  return '<div class="tcp-absence-explanation">' + html + '</div>';
}

// Render network diagnostics details
function renderNetworkDiagDetails(diag: NetworkDiagData, tcpAbsenceEvents?: TcpAbsenceEvent[]): string {
  const diagStatus = escapeText(diag.status) || 'unknown';
  const statusClass = diag.status === 'ok' ? 'status-ok' : 'status-warn';
  let html = '<div class="detail-section"><div class="detail-label">Network diagnostics:</div><div class="detail-value"><span class="' + statusClass + '">' + diagStatus + '</span></div></div>';
  if (diag.underlay_tcp && diag.underlay_tcp.length > 0) {
    html += '<div class="detail-section"><div class="detail-label">Underlay TCP:</div><div class="detail-tcp">' + formatTcpDetails(diag.underlay_tcp) + '</div></div>';
  } else {
    // Show absence explanation if available, otherwise generic message
    if (tcpAbsenceEvents && tcpAbsenceEvents.length > 0) {
      html += '<div class="detail-section"><div class="detail-label">Underlay TCP:</div><div class="detail-value"><span class="diag-muted">No TCP diagnostics captured</span></div></div>';
      html += renderTcpAbsenceExplanation(tcpAbsenceEvents);
    } else {
      html += '<div class="detail-section"><div class="detail-label">Underlay TCP:</div><div class="detail-value"><span class="diag-muted">No TCP diagnostics captured</span></div></div>';
    }
  }
  return html;
}

// Render capture details
function renderCaptureDetails(capture: DiagCapture): string {
  const status = formatCaptureStatus(capture);
  const statusText = getCaptureStatusLabel(status);
  const statusClass = getCaptureStatusClass(status);
  let html = '<div class="detail-section"><div class="detail-label">Source:</div><div class="detail-value">' + escapeText(capture.source) + '</div></div>';
  html += '<div class="detail-section"><div class="detail-label">Status:</div><div class="detail-value"><span class="' + statusClass + '">' + statusText + '</span></div></div>';
  if (capture.duration_ms !== undefined) {
    html += '<div class="detail-section"><div class="detail-label">Duration:</div><div class="detail-value">' + capture.duration_ms + ' ms</div></div>';
  }
  html += '<div class="detail-section"><div class="detail-label">Started:</div><div class="detail-value">' + escapeText(capture.capture_started_at) + '</div></div>';
  if (capture.capture_finished_at) {
    html += '<div class="detail-section"><div class="detail-label">Finished:</div><div class="detail-value">' + escapeText(capture.capture_finished_at) + '</div></div>';
  }
  html += '<div class="detail-section"><div class="detail-label">Suppressed by cooldown:</div><div class="detail-value">' + (capture.suppressed_by_cooldown ? 'yes' : 'no') + '</div></div>';
  if (capture.error) {
    html += '<div class="detail-section"><div class="detail-label">Error:</div><div class="detail-value capture-error">' + formatError(capture.error) + '</div></div>';
  }
  if (capture.network_diag && !capture.suppressed_by_cooldown) {
    html += renderNetworkDiagDetails(capture.network_diag, capture.tcp_absence_events);
  }
  return html;
}

// Generate safe filename from string
function sanitizeFilename(str: string): string {
  return str.replace(/[^a-zA-Z0-9\-_]/g, '-').replace(/-+/g, '-').replace(/^-|-$/g, '').substring(0, 64) || 'unnamed';
}

// Download JSON blob helper
function downloadJson(data: object, filename: string): void {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

// Download capture as JSON
function downloadCaptureJson(capture: DiagCapture, spikeEventId: string, targetId: string): void {
  const exportData = {
    export_kind: 'uvb76_diagnostic_capture',
    exported_at: new Date().toISOString(),
    target_id: targetId,
    spike_event_id: spikeEventId,
    capture: capture,
  };
  const filename = 'uvb76-capture-' + sanitizeFilename(targetId) + '-' + sanitizeFilename(spikeEventId) + '-' + sanitizeFilename(capture.source) + '.json';
  downloadJson(exportData, filename);
}

// Download spike bundle as JSON
function downloadSpikeBundle(spike: SpikeEventWithCaptures): void {
  const exportData = {
    export_kind: 'uvb76_spike_diagnostics_bundle',
    exported_at: new Date().toISOString(),
    target_id: spike.target_id,
    spike: spike,
    captures: spike.captures || [],
  };
  const filename = 'uvb76-spike-' + sanitizeFilename(spike.target_id) + '-' + sanitizeFilename(spike.event_id) + '.json';
  downloadJson(exportData, filename);
}

// Helper to create a styled div with text content safely
function createDivWithStyle(className: string, style: string, textContent?: string): HTMLDivElement {
  const div = document.createElement('div');
  div.className = className;
  div.style.cssText = style;
  if (textContent !== undefined) {
    div.textContent = textContent;
  }
  return div;
}

// Helper to create a key-value row with safe text content
function createDetailRow(label: string, value: string): HTMLDivElement {
  const row = document.createElement('div');
  row.style.cssText = 'margin-bottom:8px;';

  const labelSpan = document.createElement('strong');
  labelSpan.textContent = label;
  row.appendChild(labelSpan);

  const valueSpan = document.createElement('span');
  valueSpan.textContent = value;
  row.appendChild(valueSpan);

  return row;
}

// Display anchor capture details in a modal dialog using DOM APIs for XSS safety
function displayAnchorCaptureModal(anchorResponse: AnchorCaptureResponse, targetId: string): void {
  // Create modal overlay
  const overlay = document.createElement('div');
  overlay.className = 'anchor-modal-overlay';
  overlay.style.cssText = 'position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,0.5);z-index:1000;display:flex;align-items:center;justify-content:center;';

  // Create modal content
  const modal = document.createElement('div');
  modal.className = 'anchor-modal-content';
  modal.style.cssText = 'background:white;border-radius:8px;max-width:600px;max-height:80vh;overflow:auto;padding:24px;';

  // === Header Section ===
  const headerDiv = document.createElement('div');
  headerDiv.className = 'anchor-modal-header';

  const title = document.createElement('h3');
  title.style.cssText = 'margin:0 0 16px 0;';
  title.textContent = 'Anchor Capture Details';
  headerDiv.appendChild(title);

  // Show status badge
  if (anchorResponse.degraded) {
    const statusBadge = document.createElement('div');
    statusBadge.className = 'anchor-modal-status degraded';
    statusBadge.style.cssText = 'background:#fff3cd;color:#856404;padding:8px 12px;border-radius:4px;margin-bottom:16px;';

    const warnText = document.createElement('strong');
    warnText.textContent = '⚠ Degraded: ';
    statusBadge.appendChild(warnText);

    const msgText = document.createElement('span');
    msgText.textContent = anchorResponse.message || 'Anchor metadata retained but capture artifact is missing';
    statusBadge.appendChild(msgText);

    headerDiv.appendChild(statusBadge);
  } else if (anchorResponse.status === 'available') {
    const statusBadge = document.createElement('div');
    statusBadge.className = 'anchor-modal-status available';
    statusBadge.style.cssText = 'background:#d4edda;color:#155724;padding:8px 12px;border-radius:4px;margin-bottom:16px;';

    const okText = document.createElement('strong');
    okText.textContent = '✓ Available: ';
    statusBadge.appendChild(okText);

    const msgText = document.createElement('span');
    msgText.textContent = 'Anchor capture artifact is available';
    statusBadge.appendChild(msgText);

    headerDiv.appendChild(statusBadge);
  }
  modal.appendChild(headerDiv);

  // === Anchor Metadata Section ===
  if (anchorResponse.anchor) {
    const anchor = anchorResponse.anchor;

    const metadataDiv = document.createElement('div');
    metadataDiv.className = 'anchor-metadata';
    metadataDiv.style.cssText = 'background:#f8f9fa;padding:16px;border-radius:4px;margin-bottom:16px;';

    const metadataTitle = document.createElement('h4');
    metadataTitle.style.cssText = 'margin:0 0 12px 0;';
    metadataTitle.textContent = 'Cooldown Anchor Provenance';
    metadataDiv.appendChild(metadataTitle);

    // Build metadata rows with safe textContent
    if (anchor.anchor_capture_id) {
      metadataDiv.appendChild(createDetailRow('Capture ID: ', anchor.anchor_capture_id));
    }
    if (anchor.anchor_source) {
      metadataDiv.appendChild(createDetailRow('Source: ', anchor.anchor_source));
    }
    if (anchor.anchor_target_id) {
      metadataDiv.appendChild(createDetailRow('Target: ', anchor.anchor_target_id));
    }
    if (anchor.anchor_probe_kind) {
      metadataDiv.appendChild(createDetailRow('Probe: ', anchor.anchor_probe_kind));
    }
    if (anchor.anchor_created_at) {
      metadataDiv.appendChild(createDetailRow('Started: ', formatAnchorTime(anchor.anchor_created_at)));
    }
    if (anchor.created_from) {
      metadataDiv.appendChild(createDetailRow('Created from: ', anchor.created_from));
    }
    if (anchor.is_warmup_anchor) {
      const warmupNote = document.createElement('div');
      warmupNote.style.cssText = 'margin-bottom:8px;color:#6c757d;';
      const warmupEm = document.createElement('em');
      warmupEm.textContent = 'Warmup capture (startup diagnostic)';
      warmupNote.appendChild(warmupEm);
      metadataDiv.appendChild(warmupNote);
    }

    modal.appendChild(metadataDiv);
  }

  // === Capture Details Section ===
  if (anchorResponse.capture) {
    const capture = anchorResponse.capture;

    const captureDiv = document.createElement('div');
    captureDiv.className = 'anchor-capture-details';
    captureDiv.style.cssText = 'margin-bottom:16px;';

    const captureTitle = document.createElement('h4');
    captureTitle.style.cssText = 'margin:0 0 12px 0;';
    captureTitle.textContent = 'Capture Artifact';
    captureDiv.appendChild(captureTitle);

    const captureContent = document.createElement('div');
    captureContent.style.cssText = 'background:#e9ecef;padding:16px;border-radius:4px;';

    captureContent.appendChild(createDetailRow('Status: ', capture.capture_status || capture.status));
    captureContent.appendChild(createDetailRow('Source: ', capture.source));
    captureContent.appendChild(createDetailRow('Started: ', capture.capture_started_at));

    if (capture.capture_finished_at) {
      captureContent.appendChild(createDetailRow('Finished: ', capture.capture_finished_at));
    }
    if (capture.duration_ms !== undefined) {
      captureContent.appendChild(createDetailRow('Duration: ', capture.duration_ms.toString() + ' ms'));
    }
    if (capture.network_diag) {
      captureContent.appendChild(createDetailRow('Network diag: ', capture.network_diag.status));
    }
    if (capture.error) {
      captureContent.appendChild(createDetailRow('Error: ', formatError(capture.error)));
    }

    captureDiv.appendChild(captureContent);
    modal.appendChild(captureDiv);
  }

  // === Footer Section ===
  const footerDiv = document.createElement('div');
  footerDiv.className = 'anchor-modal-footer';
  footerDiv.style.cssText = 'display:flex;gap:12px;justify-content:flex-end;';

  // Download button (if capture available)
  let downloadBtn: HTMLButtonElement | null = null;
  if (anchorResponse.capture) {
    downloadBtn = document.createElement('button');
    downloadBtn.type = 'button';
    downloadBtn.className = 'download-anchor-btn';
    downloadBtn.style.cssText = 'padding:8px 16px;background:#007bff;color:white;border:none;border-radius:4px;cursor:pointer;';
    downloadBtn.textContent = 'Download anchor JSON';
    footerDiv.appendChild(downloadBtn);
  }

  // Close button
  const closeBtn = document.createElement('button');
  closeBtn.type = 'button';
  closeBtn.className = 'close-anchor-modal-btn';
  closeBtn.style.cssText = 'padding:8px 16px;background:#6c757d;color:white;border:none;border-radius:4px;cursor:pointer;';
  closeBtn.textContent = 'Close';
  footerDiv.appendChild(closeBtn);

  modal.appendChild(footerDiv);
  overlay.appendChild(modal);
  document.body.appendChild(overlay);

  // === Event Handlers ===
  closeBtn.addEventListener('click', () => {
    document.body.removeChild(overlay);
  });
  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) {
      document.body.removeChild(overlay);
    }
  });

  // Download handler
  if (downloadBtn && anchorResponse.capture) {
    downloadBtn.addEventListener('click', () => {
      const exportData = {
        export_kind: 'uvb76_anchor_capture',
        exported_at: new Date().toISOString(),
        target_id: targetId,
        anchor: anchorResponse.anchor,
        capture: anchorResponse.capture,
        degraded: anchorResponse.degraded,
      };
      const filename = 'uvb76-anchor-' + sanitizeFilename(targetId) + '-' + (anchorResponse.anchor?.anchor_capture_id || 'unknown') + '.json';
      downloadJson(exportData, filename);
    });
  }
}

// Render spike diagnostics for a target as a semantic table
export async function loadSpikeDiagnostics(targetId: string): Promise<void> {
  const container = document.getElementById('spike-diag-' + targetId);
  if (!container) return;

  try {
    const response = await api.getLatencySpikesWithCaptures(targetId, 10);
    const responseData = response;
    const retention = response.retention || defaultRetentionStats;

    if (!response.spikes || response.spikes.length === 0) {
      container.innerHTML = '<div class="spike-diag-header"><span class="spike-diag-title">Spike diagnostics</span></div>' +
        renderRetentionSummary(retention) +
        '<div class="spike-diag-empty">No recent spikes</div>';
      return;
    }

    // Build table rows
    const rowsHTML = response.spikes.map((spike: SpikeEventWithCaptures, spikeIndex: number) => {
      const time = formatSpikeTime(spike.sample_ts);
      const kind = escapeText(spike.kind);
      const severity = escapeText(spike.severity);
      const latency = formatLatencyMs(spike.latency_ms);
      const thresholds = formatThresholds(spike.reasons);
      const targetName = formatTargetName(spike.target_id);

      let captureStatus: CaptureStatusDisplay = 'none';
      let captureError = '';
      let captureBadge = '';

      if (spike.captures && spike.captures.length > 0) {
        const capture = spike.captures[spike.captures.length - 1];
        captureStatus = formatCaptureStatus(capture);
        captureError = formatError(capture.error);
        const statusLabel = getCaptureStatusLabel(captureStatus);
        const statusClass = getCaptureStatusClass(captureStatus);
        // Show explicit status label for all capture statuses
        captureBadge = '<span class="capture-badge ' + statusClass + '">' + statusLabel + '</span>';
      }

      const severityClass = severity.toLowerCase() === 'critical' ? 'severity-critical' : 'severity-warn';
      const detailsText = (captureStatus === 'timeout' || captureStatus === 'error') && captureError ? captureError : '—';

      return '<tr class="spike-row">' +
        '<td class="spike-cell time-cell">' + time + '</td>' +
        '<td class="spike-cell probe-cell">' + kind + '</td>' +
        '<td class="spike-cell severity-cell"><span class="severity-badge ' + severityClass + '">' + severity.toUpperCase() + '</span></td>' +
        '<td class="spike-cell latency-cell">' + latency + '</td>' +
        '<td class="spike-cell target-cell">' + targetName + '</td>' +
        '<td class="spike-cell thresholds-cell">' + thresholds + '</td>' +
        '<td class="spike-cell capture-cell">' + captureBadge + '</td>' +
        '<td class="spike-cell details-cell">' + detailsText + '</td>' +
        '</tr>';
    }).join('');

    // Build expanded captures section
    let expandedCaptures = '';
    for (let spikeIndex = 0; spikeIndex < response.spikes.length; spikeIndex++) {
      const spike = response.spikes[spikeIndex];
      if (spike.captures && spike.captures.length > 0) {
        for (let captureIndex = 0; captureIndex < spike.captures.length; captureIndex++) {
          const capture = spike.captures[captureIndex];
          const status = formatCaptureStatus(capture);
          const statusClass = getCaptureStatusClass(status);
          const statusText = getCaptureStatusLabel(status);
          const sourceName = escapeText(capture.source);
          const duration = capture.duration_ms !== undefined ? ' (' + capture.duration_ms + 'ms)' : '';
          const detailsId = 'capture-details-' + spikeIndex + '-' + captureIndex;

          let networkDiagStatus = '';
          if (capture.suppressed_by_cooldown) {
            networkDiagStatus = '<span class="diag-label">Network diag:</span> <span class="status-muted">suppressed</span>';
          } else if (capture.network_diag) {
            const diag = capture.network_diag;
            networkDiagStatus = '<span class="diag-label">Network diag:</span> <span class="' + (diag.status === 'ok' ? 'status-ok' : 'status-warn') + '">' + escapeText(diag.status) + '</span>';
          } else if (capture.status === 'ok') {
            networkDiagStatus = '<span class="diag-label">Network diag:</span> <span class="status-warn">missing</span>';
          }

          const errorText = formatError(capture.error);
          // Render cooldown anchor explanation for skipped cooldown captures
          const cooldownExplanation = renderCooldownAnchorExplanation(capture);
          
          // Enhanced network diag status with explanation when suppressed by cooldown
          let enhancedNetworkDiagStatus = networkDiagStatus;
          if (capture.suppressed_by_cooldown && capture.cooldown_info && !capture.cooldown_info.anchor_visible) {
            enhancedNetworkDiagStatus = '<span class="diag-label">Network diag:</span> <span class="status-muted">suppressed</span> <span class="diag-suppressed-note">by cooldown</span>' +
              '<div class="diag-suppressed-explanation">Prior capture anchor: outside current view</div>';
          }
          
          // Build action buttons using DOM APIs for XSS safety
          const actionsDiv = document.createElement('div');
          actionsDiv.className = 'capture-actions';
          
          // View details button
          const viewDetailsBtn = document.createElement('button');
          viewDetailsBtn.type = 'button';
          viewDetailsBtn.className = 'view-details-btn';
          viewDetailsBtn.dataset.detailsId = detailsId;
          viewDetailsBtn.dataset.spikeIndex = String(spikeIndex);
          viewDetailsBtn.dataset.captureIndex = String(captureIndex);
          viewDetailsBtn.dataset.targetId = targetId;
          viewDetailsBtn.textContent = 'View details';
          actionsDiv.appendChild(viewDetailsBtn);
          
          // Download capture JSON button
          const downloadCaptureBtn = document.createElement('button');
          downloadCaptureBtn.type = 'button';
          downloadCaptureBtn.className = 'download-capture-btn';
          downloadCaptureBtn.dataset.spikeIndex = String(spikeIndex);
          downloadCaptureBtn.dataset.captureIndex = String(captureIndex);
          downloadCaptureBtn.dataset.targetId = targetId;
          downloadCaptureBtn.textContent = 'Download capture JSON';
          actionsDiv.appendChild(downloadCaptureBtn);
          
          // Download spike bundle button
          const downloadSpikeBtn = document.createElement('button');
          downloadSpikeBtn.type = 'button';
          downloadSpikeBtn.className = 'download-spike-btn';
          downloadSpikeBtn.dataset.spikeIndex = String(spikeIndex);
          downloadSpikeBtn.textContent = 'Download spike bundle';
          actionsDiv.appendChild(downloadSpikeBtn);
          
          // Add "View anchor capture" button ONLY if real anchor_capture_id exists
          // Without a durable anchor ID, we cannot look up the anchor
          if (capture.suppressed_by_cooldown && capture.cooldown_info && !capture.cooldown_info.anchor_visible && capture.cooldown_info.anchor_capture_id) {
            const anchorCaptureBtn = document.createElement('button');
            anchorCaptureBtn.type = 'button';
            anchorCaptureBtn.className = 'view-anchor-capture-btn';
            anchorCaptureBtn.dataset.anchorCaptureId = capture.cooldown_info.anchor_capture_id;
            anchorCaptureBtn.dataset.targetId = targetId;
            anchorCaptureBtn.textContent = 'View anchor capture';
            actionsDiv.appendChild(anchorCaptureBtn);
          }
          
          // If anchor timestamp exists but no durable ID, show degraded message instead of button
          if (capture.suppressed_by_cooldown && capture.cooldown_info && !capture.cooldown_info.anchor_visible && 
              capture.cooldown_info.last_successful_capture_at && !capture.cooldown_info.anchor_capture_id) {
            const degradedNote = document.createElement('div');
            degradedNote.className = 'cooldown-degraded-note';
            degradedNote.textContent = 'Cooldown anchor timestamp exists, but durable anchor ID is missing from retention';
            actionsDiv.appendChild(degradedNote);
          }
          
          expandedCaptures += '<div class="capture-row">' +
            '<div class="capture-header">' +
            '<span class="capture-source">' + sourceName + duration + '</span>' +
            '<span class="capture-status ' + statusClass + '">Capture: ' + statusText + '</span>' +
            '</div>' +
            (errorText ? '<div class="capture-error">' + errorText + '</div>' : '') +
            (enhancedNetworkDiagStatus ? '<div class="diag-row">' + enhancedNetworkDiagStatus + '</div>' : '') +
            (cooldownExplanation ? '<div class="cooldown-explanation-row">' + cooldownExplanation + '</div>' : '') +
            '<div class="capture-actions-row">' + actionsDiv.outerHTML + '</div>' +
            '<div class="capture-details" id="' + detailsId + '" style="display: none;">' +
            renderCaptureDetails(capture) +
            '</div>' +
            '</div>';
        }
      }
    }

    container.innerHTML = '<div class="spike-diag-header"><span class="spike-diag-title">Spike diagnostics</span></div>' +
      renderRetentionSummary(retention) +
      '<table class="spike-table" aria-label="Spike diagnostics">' +
      '<caption class="spike-caption">Showing ' + retention.visible_spike_count + ' newest of ' + retention.retained_spike_count + ' retained spikes — ' + retention.protected_capture_count + ' protected by captures, ' + retention.purge_eligible_count + ' purge-eligible</caption>' +
      '<thead><tr>' +
      '<th scope="col">Time</th>' +
      '<th scope="col">Probe</th>' +
      '<th scope="col">Severity</th>' +
      '<th scope="col">Latency</th>' +
      '<th scope="col">Target</th>' +
      '<th scope="col">Thresholds</th>' +
      '<th scope="col">Capture</th>' +
      '<th scope="col">Details</th>' +
      '</tr></thead>' +
      '<tbody>' + rowsHTML + '</tbody></table>' +
      (expandedCaptures ? '<div class="spike-captures-section"><h4 class="captions-section-title">Capture Details</h4>' + expandedCaptures + '</div>' : '');

    // Attach event handlers
    container.querySelectorAll('.view-details-btn').forEach(btn => {
      btn.addEventListener('click', (e) => {
        const target = e.target as HTMLButtonElement;
        const detailsId = target.dataset.detailsId;
        const detailsEl = detailsId ? container.querySelector('#' + detailsId) : null;
        if (detailsEl) {
          const isHidden = detailsEl.style.display === 'none';
          detailsEl.style.display = isHidden ? 'block' : 'none';
          target.textContent = isHidden ? 'Hide details' : 'View details';
        }
      });
    });

    container.querySelectorAll('.download-capture-btn').forEach(btn => {
      btn.addEventListener('click', (e) => {
        const target = e.target as HTMLButtonElement;
        const spikeIndex = parseInt(target.dataset.spikeIndex || '', 10);
        const captureIndex = parseInt(target.dataset.captureIndex || '', 10);
        const tgtId = target.dataset.targetId;
        if (!isNaN(spikeIndex) && !isNaN(captureIndex) && tgtId) {
          const spike = responseData.spikes[spikeIndex];
          if (spike && spike.captures && spike.captures[captureIndex] !== undefined) {
            downloadCaptureJson(spike.captures[captureIndex], spike.event_id, tgtId);
          }
        }
      });
    });

    container.querySelectorAll('.download-spike-btn').forEach(btn => {
      btn.addEventListener('click', (e) => {
        const target = e.target as HTMLButtonElement;
        const index = parseInt(target.dataset.spikeIndex || '0', 10);
        const spike = responseData.spikes[index];
        if (spike) {
          downloadSpikeBundle(spike);
        }
      });
    });

    // Handle "View anchor capture" button for skipped cooldown with hidden anchor
    container.querySelectorAll('.view-anchor-capture-btn').forEach(btn => {
      btn.addEventListener('click', async (e) => {
        const target = e.target as HTMLButtonElement;
        const anchorCaptureId = target.dataset.anchorCaptureId;
        const tgtId = target.dataset.targetId;
        if (!anchorCaptureId || !tgtId) return;

        // Disable button while loading
        target.disabled = true;
        const originalText = target.textContent;
        target.textContent = 'Loading...';

        try {
          const anchorResponse = await api.getAnchorCapture(anchorCaptureId);
          displayAnchorCaptureModal(anchorResponse, tgtId);
        } catch (err) {
          console.error('Failed to fetch anchor capture:', err);
          // Show error in a simple alert (could be improved with a modal)
          alert('Failed to fetch anchor capture details. The anchor may have been purged from retention.');
        } finally {
          target.disabled = false;
          target.textContent = originalText;
        }
      });
    });
  } catch (e) {
    console.error('Failed to load spike diagnostics for', targetId, e);
    container.innerHTML = '<div class="spike-diag-error">Spike diagnostics unavailable</div>';
  }
}
