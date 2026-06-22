// Diagnostic Timeline Render Escape Tests - Regression tests for escaping and HTML validity

import { describe, it, expect } from 'vitest';
import { renderTimelineRow } from './diagnosticTimeline.render';
import { createHttpTimelineEvent } from './diagnosticTimeline.fixtures';

// Helper to create a minimal test event
function createTestEvent(overrides: Partial<{
  eventId: string;
}> = {}): ReturnType<typeof createHttpTimelineEvent> {
  return createHttpTimelineEvent({
    eventId: 'test-event-1',
    ...overrides,
  });
}

describe('escapeAttr regression tests', () => {
  it('escapes double quotes in event IDs as "', () => {
    const event = createTestEvent({ eventId: 'test"event' });
    const html = renderTimelineRow(event, 0);
    
    // Double quote must be escaped as " not left as-is
    expect(html).toContain('"');
    // Must NOT have unescaped double quotes that break the attribute
    expect(html).not.toContain('data-event-id="test"event"');
  });

  it('escapes single quotes in event IDs as &#39;', () => {
    const event = createTestEvent({ eventId: "test'event" });
    const html = renderTimelineRow(event, 0);
    
    // Single quote must be escaped as &#39;
    expect(html).toContain('&#39;');
    // Must NOT have unescaped single quotes inside double-quoted attribute
    expect(html).not.toContain("data-event-id=\"test'event\"");
  });

  it('escapes both quotes in event IDs', () => {
    const event = createTestEvent({ eventId: 'test"both\'quotes' });
    const html = renderTimelineRow(event, 0);
    
    expect(html).toContain('"');
    expect(html).toContain('&#39;');
  });

  it('escapes XSS payloads in event IDs', () => {
    const event = createTestEvent({ eventId: '"><script>alert(1)</script>' });
    const html = renderTimelineRow(event, 0);
    
    // The quotes should be escaped to prevent HTML breakout
    expect(html).toContain('"');
    expect(html).not.toContain('"><script>');
  });
});

describe('data-event-id attribute validity', () => {
  it('renders valid data-event-id attribute without stray characters', () => {
    const event = createTestEvent({ eventId: 'test-event-123' });
    const html = renderTimelineRow(event, 0);
    
    // Must have proper data-event-id attribute
    expect(html).toMatch(/data-event-id="[^"]+"/);
    // Must NOT have stray closing paren that was in the broken version
    expect(html).not.toContain('>)"');
    // Must NOT have malformed patterns like ")">
    expect(html).not.toMatch(/\)"[^>]*>/);
  });

  it('renders valid data-event-id for event ID with special characters', () => {
    const event = createTestEvent({ eventId: 'evt_123-abc_def' });
    const html = renderTimelineRow(event, 0);
    
    // Must have proper attribute
    expect(html).toMatch(/data-event-id="[^"]+"/);
    expect(html).not.toContain('>)"');
  });

  it('renders valid data-event-id for event ID with quotes (XSS prevention)', () => {
    const event = createTestEvent({ eventId: 'test"quote"event' });
    const html = renderTimelineRow(event, 0);
    
    // The attribute must be properly escaped and closed
    expect(html).toMatch(/data-event-id="[^"]+"/);
    // Check no HTML breakage - the button tag must be well-formed
    expect(html).not.toContain('data-event-id="test"');
    expect(html).not.toContain('>)"');
  });

  it('renders valid data-event-id for event ID containing double quote', () => {
    // Event ID: test + " + event
    const event = createTestEvent({ eventId: 'test"event' });
    const html = renderTimelineRow(event, 0);
    
    // The escaped version should contain "
    expect(html).toContain('"');
    // But should NOT break the attribute - no unescaped quotes inside
    expect(html).not.toContain('data-event-id="test"event"');
    expect(html).toMatch(/data-event-id="test&quot;event"/);
  });
});

describe('renderTimelineRow HTML structure', () => {
  it('produces well-formed HTML table row', () => {
    const event = createTestEvent();
    const html = renderTimelineRow(event, 0);
    
    // Should start with <tr and end with </tr>
    expect(html.trim()).toMatch(/^<tr /);
    expect(html).toContain('</tr>');
    
    // Should have exactly one closing </tr>
    expect(html.split('</tr>').length - 1).toBe(1);
  });

  it('has all required table cells', () => {
    const event = createTestEvent();
    const html = renderTimelineRow(event, 0);
    
    expect(html).toContain('<td class="timeline-cell time-cell">');
    expect(html).toContain('<td class="timeline-cell probe-cell">');
    expect(html).toContain('<td class="timeline-cell severity-cell">');
    expect(html).toContain('<td class="timeline-cell latency-cell">');
    expect(html).toContain('<td class="timeline-cell capture-cell">');
    expect(html).toContain('<td class="timeline-cell details-cell">');
    expect(html).toContain('<td class="timeline-cell action-cell">');
  });

  it('Details button is properly closed without stray characters', () => {
    const event = createTestEvent({ eventId: 'test-event-1' });
    const html = renderTimelineRow(event, 0);
    
    // The button tag must be properly closed
    expect(html).toContain('<button class="timeline-details-btn"');
    expect(html).toContain('</button>');
    
    // Must NOT have stray )" that was in the broken version
    expect(html).not.toContain('>)"');
    
    // Verify button has correct attributes
    expect(html).toContain('data-details-id="timeline-details-0"');
    expect(html).toContain('data-row-index="0"');
    expect(html).toContain('data-event-id="test-event-1"');
  });

  it('attribute escaping does not break button text', () => {
    const event = createTestEvent({ eventId: 'test"quote"end' });
    const html = renderTimelineRow(event, 0);
    
    // The button should still have "Details" text
    expect(html).toContain('>Details</button>');
  });
});
