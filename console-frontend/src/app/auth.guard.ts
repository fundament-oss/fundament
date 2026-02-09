import { inject } from '@angular/core';
import { Router, CanActivateFn } from '@angular/router';
import AuthnApiService from './authn-api.service';

const authGuard: CanActivateFn = async (route, state) => {
  const apiService = inject(AuthnApiService);
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
    // Access token might be expired, try to refresh it
    try {
      await apiService.refreshToken();
      // Retry getting user info after successful refresh
      await apiService.getUserInfo();
      return true;
    } catch {
      // Refresh failed - not authenticated, store return URL and redirect to login
      localStorage.setItem('returnUrl', state.url);

      router.navigate(['/login']);
      return false;
    }
  }
};

export default authGuard;
