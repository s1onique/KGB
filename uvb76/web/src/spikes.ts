// Spike diagnostics rendering module
import { api, type SpikeEventWithCaptures, type DiagCapture, type TcpSocketDiagData, type NetworkDiagData, type SpikeRetentionStats } from './api';
import { formatSpikeTime, formatLatencyMs } from './format';

// HTML escape helper for XSS protection
function escapeText(s: string | null | undefined): string {
  if (!s) return '';
  const div = document.createElement('div');
  div.textContent = s;
  return div.innerHTML;
}

// Capture status types for display
type CaptureStatusDisplay = 'ready' | 'in_progress' | 'timeout' | 'error' | 'suppressed_by_cooldown' | 'missing' | 'none';

// Format capture status for display
function formatCaptureStatus(capture: DiagCapture): CaptureStatusDisplay {
  if (capture.suppressed_by_cooldown) {
    return 'suppressed_by_cooldown';
  }
  switch (capture.status) {
    case 'ok':
      return 'ready';
    case 'error':
      return 'error';
    case 'timeout':
      return 'timeout';
    default:
      return 'none';
  }
}

// Get CSS class for capture status
function getCaptureStatusClass(status: CaptureStatusDisplay): string {
  switch (status) {
    case 'ready':
      return 'status-ok';
    case 'in_progress':
      return 'status-warn';
    case 'timeout':
    case 'error':
      return 'status-error';
    case 'suppressed_by_cooldown':
      return 'status-suppressed';
    default:
      return 'status-muted';
  }
}

// Get human-readable capture status label
function getCaptureStatusLabel(status: CaptureStatusDisplay): string {
  switch (status) {
    case 'ready':
      return 'ready';
    case 'in_progress':
      return 'in progress';
    case 'timeout':
      return 'timeout';
    case 'error':
      return 'error';
    case 'suppressed_by_cooldown':
      return 'suppressed by cooldown';
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

// Render network diagnostics details
function renderNetworkDiagDetails(diag: NetworkDiagData): string {
  const diagStatus = escapeText(diag.status) || 'unknown';
  const statusClass = diag.status === 'ok' ? 'status-ok' : 'status-warn';
  let html = '<div class="detail-section"><div class="detail-label">Network diagnostics:</div><div class="detail-value"><span class="' + statusClass + '">' + diagStatus + '</span></div></div>';
  if (diag.underlay_tcp && diag.underlay_tcp.length > 0) {
    html += '<div class="detail-section"><div class="detail-label">Underlay TCP:</div><div class="detail-tcp">' + formatTcpDetails(diag.underlay_tcp) + '</div></div>';
  } else {
    html += '<div class="detail-section"><div class="detail-label">Underlay TCP:</div><div class="detail-value"><span class="diag-muted">No TCP diagnostics captured</span></div></div>';
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
    html += renderNetworkDiagDetails(capture.network_diag);
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
        if (captureStatus === 'suppressed_by_cooldown') {
          captureBadge = '<span class="capture-badge ' + statusClass + '">suppressed</span>';
        } else {
          captureBadge = '<span class="capture-badge ' + statusClass + '">' + statusLabel + '</span>';
        }
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
          expandedCaptures += '<div class="capture-row">' +
            '<div class="capture-header">' +
            '<span class="capture-source">' + sourceName + duration + '</span>' +
            '<span class="capture-status ' + statusClass + '">Capture: ' + statusText + '</span>' +
            '</div>' +
            (errorText ? '<div class="capture-error">' + errorText + '</div>' : '') +
            (networkDiagStatus ? '<div class="diag-row">' + networkDiagStatus + '</div>' : '') +
            '<div class="capture-actions">' +
            '<button type="button" class="view-details-btn" data-details-id="' + detailsId + '" data-spike-index="' + spikeIndex + '" data-capture-index="' + captureIndex + '" data-target-id="' + targetId + '">View details</button>' +
            '<button type="button" class="download-capture-btn" data-spike-index="' + spikeIndex + '" data-capture-index="' + captureIndex + '" data-target-id="' + targetId + '">Download capture JSON</button>' +
            '<button type="button" class="download-spike-btn" data-spike-index="' + spikeIndex + '">Download spike bundle</button>' +
            '</div>' +
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
  } catch (e) {
    console.error('Failed to load spike diagnostics for', targetId, e);
    container.innerHTML = '<div class="spike-diag-error">Spike diagnostics unavailable</div>';
  }
}
