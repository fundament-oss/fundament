import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { ClusterWizardStateService } from './cluster-wizard-state.service';

export const clusterWizardGuard: CanActivateFn = (route) => {
  const stateService = inject(ClusterWizardStateService);
  const router = inject(Router);
  
  // Allow access to the first step always
  if (route.routeConfig?.path === '') {
    return true;
  }
  
  // For other steps, check if first step is completed
  if (!stateService.isFirstStepCompleted()) {
    // Redirect to first step if first step not completed
    return router.createUrlTree(['/add-cluster']);
  }
  
  return true;
};
