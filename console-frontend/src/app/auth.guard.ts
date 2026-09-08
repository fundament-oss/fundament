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
    // Refresh failed - not authenticated, store return URL and redirect to
    // login. The bare root names no page in particular, so there is nothing to
    // come back to: logging in from there lands on the default page instead.
    if (state.url === '/') {
      localStorage.removeItem('returnUrl');
    } else {
      localStorage.setItem('returnUrl', state.url);
    }

    router.navigate(['/login']);
    return false;
  }
};

export default authGuard;
