import './styles.css';
import { auth } from './auth';
import { api } from './api';
import { initTargets, setupGraphControls } from './targets';
import { loadLatencyForTarget } from './latency';
import { mountDiagnosticTimeline } from './diagnosticTimeline';
import { formatStartTime } from './format';
import { renderThemeToggle } from './themeToggle';

// DOM Elements
const authForm = document.getElementById('auth-form');
const loadingDiv = document.getElementById('loading');
const dashboardDiv = document.getElementById('dashboard');
const loginBtn = document.getElementById('login-btn') as HTMLButtonElement;
const usernameInput = document.getElementById('username') as HTMLInputElement;
const passwordInput = document.getElementById('password') as HTMLInputElement;
const authError = document.getElementById('auth-error');
const logoutBtn = document.getElementById('logout-btn');

let targetsInstance: ReturnType<typeof initTargets> | null = null;
let refreshInterval: number | null = null;

// UI State management
function showLoginForm(): void {
  stopRefreshInterval();
  authForm?.classList.remove('hidden');
  loadingDiv?.classList.add('hidden');
  dashboardDiv?.classList.add('hidden');
}

function showLoading(): void {
  authForm?.classList.add('hidden');
  loadingDiv?.classList.remove('hidden');
  dashboardDiv?.classList.add('hidden');
}

function showDashboard(): void {
  authForm?.classList.add('hidden');
  loadingDiv?.classList.add('hidden');
  dashboardDiv?.classList.remove('hidden');
}

function showError(msg: string): void {
  if (authError) {
    authError.textContent = msg;
    authError.classList.remove('hidden');
  }
}

function hideError(): void {
  authError?.classList.add('hidden');
}

function stopRefreshInterval(): void {
  if (refreshInterval !== null) {
    clearInterval(refreshInterval);
    refreshInterval = null;
  }
}

async function loadDashboard(): Promise<void> {
  if (!targetsInstance) {
    targetsInstance = initTargets('targets');
  }

  // Load server status for start time display
  loadStartTime();

  await targetsInstance.loadTargets();

  // Load latency and diagnostic timeline for all targets
  try {
    const targets = await api.getTargets();
    for (const t of targets) {
      await loadLatencyForTarget(t.id);
      // Mount unified diagnostic timeline for this target
      mountDiagnosticTimeline(t.id, `timeline-${t.id}`);
    }

    // Set up auto-refresh every 30 seconds
    stopRefreshInterval();
    refreshInterval = window.setInterval(async () => {
      try {
        const currentTargets = await api.getTargets();
        for (const t of currentTargets) {
          await loadLatencyForTarget(t.id);
        }
      } catch (e) {
        console.error('Auto-refresh failed:', e);
      }
    }, 30000);
  } catch (e) {
    console.error('Failed to load dashboard:', e);
  }
}

// Render server start time in header
async function loadStartTime(): Promise<void> {
  const startTimeEl = document.getElementById('start-time');
  if (!startTimeEl) return;

  try {
    const status = await api.getStatus();
    const formatted = formatStartTime(status.started_at);
    startTimeEl.textContent = `started ${formatted}`;
  } catch (e) {
    console.error('Failed to load server status:', e);
    startTimeEl.textContent = '';
  }
}

async function handleLogin(): Promise<void> {
  const user = usernameInput?.value || '';
  const pass = passwordInput?.value || '';

  if (!user || !pass) {
    showError('Please enter username and password');
    return;
  }

  // Disable button and show loading state
  if (loginBtn) {
    loginBtn.disabled = true;
    loginBtn.textContent = 'Signing in...';
  }
  hideError();

  try {
    const result = await auth.login(user, pass);
    if (result.success) {
      usernameInput.value = '';
      passwordInput.value = '';
      showDashboard();
      await loadDashboard();
    } else {
      showError(result.error || 'Invalid credentials');
    }
  } catch {
    showError('Connection error. Please try again.');
  } finally {
    if (loginBtn) {
      loginBtn.disabled = false;
      loginBtn.textContent = 'Sign In';
    }
  }
}

async function handleLogout(): Promise<void> {
  stopRefreshInterval();
  await auth.logout();
  showLoginForm();
}

// Initialize
function init(): void {
  // Render theme toggle in header
  renderThemeToggle('theme-toggle-container');

  // Setup graph controls event listeners
  setupGraphControls();

  // Subscribe to auth state changes
  auth.subscribe((state) => {
    if (state.isAuthenticated) {
      showDashboard();
      loadDashboard();
    } else {
      showLoginForm();
    }
  });

  // Login button handler
  loginBtn?.addEventListener('click', handleLogin);

  // Password enter key handler
  passwordInput?.addEventListener('keypress', (e) => {
    if (e.key === 'Enter') {
      handleLogin();
    }
  });

  // Username enter key handler - focus password
  usernameInput?.addEventListener('keypress', (e) => {
    if (e.key === 'Enter') {
      passwordInput?.focus();
    }
  });

  // Logout button handler
  logoutBtn?.addEventListener('click', handleLogout);

  // Check initial auth state
  auth.checkAuth();
}

// Start app when DOM is ready
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', init);
} else {
  init();
}
