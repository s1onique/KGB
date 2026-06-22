// Diagnostic Timeline Filters Tests - Unit tests for filter logic

import { describe, it, expect } from 'vitest';
import {
  defaultFilters,
  createFilters,
  matchesProbeKind,
  matchesCaptureStatus,
  matchesSeverity,
  matchesAllFilters,
  applyFilters,
  hasActiveFilters,
  getActiveFilterCount,
  getProbeKindFilterLabel,
  getCaptureStatusFilterLabel,
  getSeverityFilterLabel,
} from './diagnosticTimeline.filters';
import { createHttpTimelineEvent, createIcmpTimelineEvent, createCriticalTimelineEvent } from './diagnosticTimeline.fixtures';

describe('diagnosticTimeline.filters', () => {
  describe('defaultFilters', () => {
    it('has correct default values', () => {
      expect(defaultFilters.probeKind).toBe('all');
      expect(defaultFilters.captureStatus).toBe('all');
      expect(defaultFilters.severity).toBe('all');
    });
  });

  describe('createFilters', () => {
    it('creates filters with defaults', () => {
      const filters = createFilters();
      expect(filters).toEqual(defaultFilters);
    });

    it('creates filters with overrides', () => {
      const filters = createFilters({ probeKind: 'http' });
      expect(filters.probeKind).toBe('http');
      expect(filters.captureStatus).toBe('all');
      expect(filters.severity).toBe('all');
    });

    it('allows multiple overrides', () => {
      const filters = createFilters({
        probeKind: 'icmp',
        captureStatus: 'captured',
        severity: 'critical',
      });
      expect(filters.probeKind).toBe('icmp');
      expect(filters.captureStatus).toBe('captured');
      expect(filters.severity).toBe('critical');
    });
  });

  describe('matchesProbeKind', () => {
    const httpEvent = createHttpTimelineEvent();
    const icmpEvent = createIcmpTimelineEvent();

    it('returns true for all filter', () => {
      expect(matchesProbeKind(httpEvent, 'all')).toBe(true);
      expect(matchesProbeKind(icmpEvent, 'all')).toBe(true);
    });

    it('matches http filter', () => {
      expect(matchesProbeKind(httpEvent, 'http')).toBe(true);
      expect(matchesProbeKind(icmpEvent, 'http')).toBe(false);
    });

    it('matches icmp filter', () => {
      expect(matchesProbeKind(icmpEvent, 'icmp')).toBe(true);
      expect(matchesProbeKind(httpEvent, 'icmp')).toBe(false);
    });
  });

  describe('matchesCaptureStatus', () => {
    const capturedEvent = createHttpTimelineEvent({ captureStatus: 'captured' });
    const suppressedEvent = createHttpTimelineEvent({ captureStatus: 'suppressed' });
    const failedEvent = createHttpTimelineEvent({ captureStatus: 'failed' });

    it('returns true for all filter', () => {
      expect(matchesCaptureStatus(capturedEvent, 'all')).toBe(true);
      expect(matchesCaptureStatus(suppressedEvent, 'all')).toBe(true);
    });

    it('matches captured filter', () => {
      expect(matchesCaptureStatus(capturedEvent, 'captured')).toBe(true);
      expect(matchesCaptureStatus(suppressedEvent, 'captured')).toBe(false);
    });

    it('matches suppressed filter', () => {
      expect(matchesCaptureStatus(suppressedEvent, 'suppressed')).toBe(true);
      expect(matchesCaptureStatus(capturedEvent, 'suppressed')).toBe(false);
    });

    it('matches failed filter', () => {
      expect(matchesCaptureStatus(failedEvent, 'failed')).toBe(true);
      expect(matchesCaptureStatus(capturedEvent, 'failed')).toBe(false);
    });
  });

  describe('matchesSeverity', () => {
    const warningEvent = createHttpTimelineEvent({ severity: 'warning' });
    const criticalEvent = createCriticalTimelineEvent();

    it('returns true for all filter', () => {
      expect(matchesSeverity(warningEvent, 'all')).toBe(true);
      expect(matchesSeverity(criticalEvent, 'all')).toBe(true);
    });

    it('matches warning filter', () => {
      expect(matchesSeverity(warningEvent, 'warning')).toBe(true);
      expect(matchesSeverity(criticalEvent, 'warning')).toBe(false);
    });

    it('matches critical filter', () => {
      expect(matchesSeverity(criticalEvent, 'critical')).toBe(true);
      expect(matchesSeverity(warningEvent, 'critical')).toBe(false);
    });
  });

  describe('matchesAllFilters', () => {
    it('returns true when all filters match', () => {
      const event = createHttpTimelineEvent({
        severity: 'warning',
        captureStatus: 'captured',
      });
      const filters = {
        probeKind: 'http',
        captureStatus: 'captured',
        severity: 'warning',
      };
      expect(matchesAllFilters(event, filters)).toBe(true);
    });

    it('returns false when any filter does not match', () => {
      const event = createHttpTimelineEvent({
        severity: 'warning',
        captureStatus: 'captured',
      });
      const filters = {
        probeKind: 'icmp', // doesn't match
        captureStatus: 'captured',
        severity: 'warning',
      };
      expect(matchesAllFilters(event, filters)).toBe(false);
    });

    it('respects all filter in any position', () => {
      const event = createHttpTimelineEvent({
        severity: 'warning',
        captureStatus: 'captured',
      });
      const filters = {
        probeKind: 'all',
        captureStatus: 'captured',
        severity: 'warning',
      };
      expect(matchesAllFilters(event, filters)).toBe(true);
    });
  });

  describe('applyFilters', () => {
    it('returns all events when no filters active', () => {
      const events = [
        createHttpTimelineEvent(),
        createIcmpTimelineEvent(),
        createCriticalTimelineEvent(),
      ];
      const filtered = applyFilters(events, defaultFilters);
      expect(filtered.length).toBe(3);
    });

    it('filters by probe kind', () => {
      const events = [
        createHttpTimelineEvent({ eventId: 'http-1' }),
        createIcmpTimelineEvent({ eventId: 'icmp-1' }),
        createHttpTimelineEvent({ eventId: 'http-2' }),
      ];
      const filters = createFilters({ probeKind: 'http' });
      const filtered = applyFilters(events, filters);
      expect(filtered.length).toBe(2);
      expect(filtered.every(e => e.probeKind === 'http')).toBe(true);
    });

    it('filters by capture status', () => {
      const events = [
        createHttpTimelineEvent({ eventId: 'cap-1', captureStatus: 'captured' }),
        createHttpTimelineEvent({ eventId: 'cap-2', captureStatus: 'suppressed' }),
        createHttpTimelineEvent({ eventId: 'cap-3', captureStatus: 'captured' }),
      ];
      const filters = createFilters({ captureStatus: 'captured' });
      const filtered = applyFilters(events, filters);
      expect(filtered.length).toBe(2);
      expect(filtered.every(e => e.captureStatus === 'captured')).toBe(true);
    });

    it('filters by severity', () => {
      const events = [
        createHttpTimelineEvent({ eventId: 'warn-1', severity: 'warning' }),
        createCriticalTimelineEvent({ eventId: 'crit-1' }),
        createHttpTimelineEvent({ eventId: 'warn-2', severity: 'warning' }),
      ];
      const filters = createFilters({ severity: 'critical' });
      const filtered = applyFilters(events, filters);
      expect(filtered.length).toBe(1);
      expect(filtered[0].severity).toBe('critical');
    });

    it('combines multiple filters', () => {
      const events = [
        createHttpTimelineEvent({ eventId: '1', captureStatus: 'captured', severity: 'warning' }),
        createIcmpTimelineEvent({ eventId: '2', captureStatus: 'captured', severity: 'warning' }),
        createHttpTimelineEvent({ eventId: '3', captureStatus: 'captured', severity: 'critical' }),
        createHttpTimelineEvent({ eventId: '4', captureStatus: 'suppressed', severity: 'warning' }),
      ];
      const filters = createFilters({
        probeKind: 'http',
        captureStatus: 'captured',
      });
      const filtered = applyFilters(events, filters);
      expect(filtered.length).toBe(2); // events 1 and 3 match (HTTP + captured)
      expect(filtered[0].eventId).toBe('1'); // sorted newest-first, 1 is earlier
    });
  });

  describe('hasActiveFilters', () => {
    it('returns false for default filters', () => {
      expect(hasActiveFilters(defaultFilters)).toBe(false);
    });

    it('returns true when probeKind is not all', () => {
      expect(hasActiveFilters(createFilters({ probeKind: 'http' }))).toBe(true);
    });

    it('returns true when captureStatus is not all', () => {
      expect(hasActiveFilters(createFilters({ captureStatus: 'captured' }))).toBe(true);
    });

    it('returns true when severity is not all', () => {
      expect(hasActiveFilters(createFilters({ severity: 'critical' }))).toBe(true);
    });
  });

  describe('getActiveFilterCount', () => {
    it('returns 0 for default filters', () => {
      expect(getActiveFilterCount(defaultFilters)).toBe(0);
    });

    it('counts active filters', () => {
      expect(getActiveFilterCount(createFilters({ probeKind: 'http' }))).toBe(1);
      expect(getActiveFilterCount(createFilters({
        probeKind: 'http',
        captureStatus: 'captured',
      }))).toBe(2);
      expect(getActiveFilterCount(createFilters({
        probeKind: 'http',
        captureStatus: 'captured',
        severity: 'critical',
      }))).toBe(3);
    });
  });

  describe('filter label helpers', () => {
    it('returns correct probe kind labels', () => {
      expect(getProbeKindFilterLabel('http')).toBe('HTTP');
      expect(getProbeKindFilterLabel('icmp')).toBe('ICMP');
      expect(getProbeKindFilterLabel('all')).toBe('All probes');
    });

    it('returns correct capture status labels', () => {
      expect(getCaptureStatusFilterLabel('captured')).toBe('Captured');
      expect(getCaptureStatusFilterLabel('suppressed')).toBe('Suppressed');
      expect(getCaptureStatusFilterLabel('failed')).toBe('Failed');
      expect(getCaptureStatusFilterLabel('all')).toBe('All statuses');
    });

    it('returns correct severity labels', () => {
      expect(getSeverityFilterLabel('warning')).toBe('Warning');
      expect(getSeverityFilterLabel('critical')).toBe('Critical');
      expect(getSeverityFilterLabel('all')).toBe('All severities');
    });
  });
});
