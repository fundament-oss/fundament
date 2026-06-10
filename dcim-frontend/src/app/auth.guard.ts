import { inject } from '@angular/core';
import { Router, CanActivateFn } from '@angular/router';
import AuthService from './auth.service';

const authGuard: CanActivateFn = async (_, state) => {
  const authService = inject(AuthService);
  const router = inject(Router);

  if (authService.isAuthenticated()) {
    return true;
  }

  await authService.initializeAuth();

  if (authService.isAuthenticated()) {
    return true;
  }

  try {
    await authService.refreshToken();
    await authService.getUserInfo();
    return true;
  } catch {
    localStorage.setItem('returnUrl', state.url);
    router.navigate(['/login']);
    return false;
  }
};

export default authGuard;
