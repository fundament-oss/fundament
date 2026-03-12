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

  // Wait for app-level initialization (deduplicates with App.ngOnInit's initializeAuth call)
  await apiService.initializeAuth();

  if (apiService.isAuthenticated()) {
    return true;
  }

  // Still not authenticated - try refreshing the token
  try {
    await apiService.refreshToken();
    await apiService.getUserInfo();
    return true;
  } catch {
    // Refresh failed - not authenticated, store return URL and redirect to login
    localStorage.setItem('returnUrl', state.url);

    router.navigate(['/login']);
    return false;
  }
};

export default authGuard;
