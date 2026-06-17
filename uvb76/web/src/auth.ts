// Auth/session handling module
import { api, type AuthCheckResponse } from './api';

export interface AuthState {
  isAuthenticated: boolean;
  username?: string;
}

type AuthCallback = (state: AuthState) => void;

class AuthManager {
  private listeners: AuthCallback[] = [];
  private currentState: AuthState = { isAuthenticated: false };
  private checkInterval: number | null = null;

  async checkAuth(): Promise<AuthState> {
    try {
      const response: AuthCheckResponse = await api.authCheck();
      this.currentState = {
        isAuthenticated: response.authenticated,
        username: response.username,
      };
    } catch {
      this.currentState = { isAuthenticated: false };
    }
    this.notifyListeners();
    return this.currentState;
  }

  async login(username: string, password: string): Promise<{ success: boolean; error?: string }> {
    try {
      const response = await api.login(username, password);
      if (response.success) {
        this.currentState = { isAuthenticated: true, username };
        this.notifyListeners();
        this.startAutoRefresh();
        return { success: true };
      }
      return { success: false, error: response.error || 'Invalid credentials' };
    } catch {
      return { success: false, error: 'Connection error. Please try again.' };
    }
  }

  async logout(): Promise<void> {
    this.stopAutoRefresh();
    try {
      await api.logout();
    } catch {
      // Ignore logout errors
    }
    this.currentState = { isAuthenticated: false };
    this.notifyListeners();
  }

  subscribe(callback: AuthCallback): () => void {
    this.listeners.push(callback);
    // Immediately notify with current state
    callback(this.currentState);
    return () => {
      this.listeners = this.listeners.filter((l) => l !== callback);
    };
  }

  getState(): AuthState {
    return this.currentState;
  }

  startAutoRefresh(intervalMs: number = 60000): void {
    this.stopAutoRefresh();
    this.checkInterval = window.setInterval(() => {
      this.checkAuth();
    }, intervalMs);
  }

  stopAutoRefresh(): void {
    if (this.checkInterval !== null) {
      clearInterval(this.checkInterval);
      this.checkInterval = null;
    }
  }

  private notifyListeners(): void {
    for (const listener of this.listeners) {
      listener(this.currentState);
    }
  }
}

export const auth = new AuthManager();
