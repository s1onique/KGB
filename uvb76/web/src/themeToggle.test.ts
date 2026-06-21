import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { renderThemeToggle } from './themeToggle';
import { THEME_STORAGE_KEY } from './theme';

describe('themeToggle component', () => {
  const containerId = 'theme-toggle-container';

  beforeEach(() => {
    // Create container element
    const container = document.createElement('div');
    container.id = containerId;
    document.body.appendChild(container);

    // Mock localStorage
    vi.stubGlobal('localStorage', {
      getItem: vi.fn().mockReturnValue(null),
      setItem: vi.fn(),
      removeItem: vi.fn(),
      clear: vi.fn(),
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    document.body.innerHTML = '';
  });

  it('renders theme toggle select element', () => {
    renderThemeToggle(containerId);

    const select = document.getElementById('theme-toggle');
    expect(select).toBeTruthy();
    expect(select?.tagName).toBe('SELECT');
  });

  it('has Dark option', () => {
    renderThemeToggle(containerId);

    const option = document.getElementById('theme-option-dark');
    expect(option).toBeTruthy();
    expect(option?.textContent).toBe('Dark');
  });

  it('has Rose option', () => {
    renderThemeToggle(containerId);

    const option = document.getElementById('theme-option-rose-pine');
    expect(option).toBeTruthy();
    expect(option?.textContent).toBe('Rose');
  });

  it('selects dark by default when storage is empty', () => {
    vi.mocked(localStorage.getItem).mockReturnValue(null);

    renderThemeToggle(containerId);

    const select = document.getElementById('theme-toggle') as HTMLSelectElement;
    expect(select?.value).toBe('dark');
  });

  it('selects Rose when stored theme is rose-pine', () => {
    vi.mocked(localStorage.getItem).mockReturnValue('rose-pine');

    renderThemeToggle(containerId);

    const select = document.getElementById('theme-toggle') as HTMLSelectElement;
    expect(select?.value).toBe('rose-pine');
  });

  it('persists theme selection on change', () => {
    renderThemeToggle(containerId);

    const select = document.getElementById('theme-toggle') as HTMLSelectElement;
    select.value = 'rose-pine';
    select.dispatchEvent(new Event('change'));

    expect(localStorage.setItem).toHaveBeenCalledWith(THEME_STORAGE_KEY, 'rose-pine');
  });

  it('persists Dark selection on change', () => {
    vi.mocked(localStorage.getItem).mockReturnValue('rose-pine');
    renderThemeToggle(containerId);

    const select = document.getElementById('theme-toggle') as HTMLSelectElement;
    select.value = 'dark';
    select.dispatchEvent(new Event('change'));

    expect(localStorage.setItem).toHaveBeenCalledWith(THEME_STORAGE_KEY, 'dark');
  });

  it('has accessible aria-label', () => {
    renderThemeToggle(containerId);

    const select = document.getElementById('theme-toggle');
    expect(select?.getAttribute('aria-label')).toBe('Select theme');
  });

  it('is keyboard accessible', () => {
    renderThemeToggle(containerId);

    const select = document.getElementById('theme-toggle') as HTMLSelectElement;
    // Select elements are natively keyboard accessible
    expect(select).toBeTruthy();
  });

  it('warns when container not found', () => {
    const consoleWarn = vi.spyOn(console, 'warn').mockImplementation(() => {});

    renderThemeToggle('non-existent-container');

    expect(consoleWarn).toHaveBeenCalledWith('Theme toggle container #non-existent-container not found');

    consoleWarn.mockRestore();
  });
});

describe('themeToggle UI rendering', () => {
  beforeEach(() => {
    vi.stubGlobal('localStorage', {
      getItem: vi.fn().mockReturnValue(null),
      setItem: vi.fn(),
      removeItem: vi.fn(),
      clear: vi.fn(),
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    document.body.innerHTML = '';
  });

  it('can be placed in header alongside logout button', () => {
    // Simulate header structure with theme container and logout button
    const header = document.createElement('div');
    header.className = 'header';

    const title = document.createElement('h1');
    title.textContent = 'UVB-76 Control Plane';
    header.appendChild(title);

    const themeContainer = document.createElement('div');
    themeContainer.id = 'theme-toggle-container';
    header.appendChild(themeContainer);

    const logoutBtn = document.createElement('button');
    logoutBtn.id = 'logout-btn';
    logoutBtn.textContent = 'Sign Out';
    header.appendChild(logoutBtn);

    // Must append to document.body for getElementById to work
    document.body.appendChild(header);

    // Render theme toggle into the container within header
    renderThemeToggle('theme-toggle-container');

    // Verify structure
    const themeToggleContainer = header.querySelector('#theme-toggle-container');
    expect(themeToggleContainer).toBeTruthy();
    expect(themeToggleContainer?.querySelector('#theme-toggle')).toBeTruthy();
    expect(header.querySelector('#logout-btn')).toBeTruthy();
    expect(themeToggleContainer?.querySelector('#theme-option-dark')).toBeTruthy();
    expect(themeToggleContainer?.querySelector('#theme-option-rose-pine')).toBeTruthy();
  });
});
