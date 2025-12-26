import { inject } from '@angular/core';
import { Router, CanActivateFn } from '@angular/router';
import { ApiService } from './api.service';

export const authGuard: CanActivateFn = async (route, state) => {
  const apiService = inject(ApiService);
  const router = inject(Router);

  // If we already have user info in state, allow access
  if (apiService.isAuthenticated()) {
    return true;
  }

  // Otherwise, try to fetch user info from server
  try {
    await apiService.getUserInfo();
    return true;
  } catch {
    // Not authenticated, store return URL and redirect to login
    localStorage.setItem('returnUrl', state.url);

    router.navigate(['/login']);
    return false;
  }
};
