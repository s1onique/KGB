// Theme toggle component for UVB-76
// Renders a compact accessible theme selector in the header

import { THEMES, THEME_LABELS, readStoredTheme, setStoredTheme, type Theme } from './theme';

/**
 * Render the theme toggle into a container element.
 * The container should be placed in the header near the logout button.
 */
export function renderThemeToggle(containerId: string): void {
  const container = document.getElementById(containerId);
  if (!container) {
    console.warn(`Theme toggle container #${containerId} not found`);
    return;
  }

  // Build select element
  const select = document.createElement('select');
  select.id = 'theme-toggle';
  select.setAttribute('aria-label', 'Select theme');

  // Add options for each theme
  for (const [key, label] of Object.entries(THEME_LABELS)) {
    const option = document.createElement('option');
    option.value = key;
    option.textContent = label;
    option.id = `theme-option-${key}`;
    select.appendChild(option);
  }

  // Set current selection
  const currentTheme = readStoredTheme();
  select.value = currentTheme;

  // Handle theme change
  select.addEventListener('change', () => {
    const selectedTheme = select.value as Theme;
    setStoredTheme(selectedTheme);
  });

  container.appendChild(select);
}
