import { Injectable, signal } from '@angular/core';

export interface NewClusterFormState {
  // Basic cluster information (step 1). Names come from the region catalog
  // (ListRegions) and are exactly what the create request sends.
  clusterName?: string;
  region?: string;
  kubernetesVersion?: string;

  // Node pools (step 2)
  nodePools?: {
    name: string;
    machineType: string;
    autoscaleMin: number;
    autoscaleMax: number;
  }[];

  // Track which steps are completed
  completedSteps: Set<number>;
}

@Injectable({
  providedIn: 'root',
})
export class NewNewClusterFormStateService {
  private state = signal<NewClusterFormState>({
    completedSteps: new Set<number>(),
  });

  /** Which step is showing. The form sits in a sheet over whatever page you
   *  were on, so a step cannot be a route of its own: routing would unmount
   *  that page and leave an empty pane behind the sheet. */
  readonly stepIndex = signal(0);

  goToStep(index: number) {
    this.stepIndex.set(index);
  }

  getState() {
    return this.state();
  }

  updateBasicInfo(data: { clusterName?: string; region?: string; kubernetesVersion?: string }) {
    this.state.update((current) => ({
      ...current,
      ...data,
    }));
  }

  updateNodePools(nodePools: NewClusterFormState['nodePools']) {
    this.state.update((current) => ({
      ...current,
      nodePools,
    }));
  }

  markStepCompleted(stepIndex: number) {
    this.state.update((current) => {
      const newCompletedSteps = new Set(current.completedSteps);
      newCompletedSteps.add(stepIndex);
      return {
        ...current,
        completedSteps: newCompletedSteps,
      };
    });
  }

  isStepCompleted(stepIndex: number): boolean {
    return this.state().completedSteps.has(stepIndex);
  }

  isFirstStepCompleted(): boolean {
    const state = this.state();
    return !!(state.clusterName && state.region && state.kubernetesVersion);
  }

  hasState(): boolean {
    const state = this.state();
    return !!(state.clusterName || state.region || state.kubernetesVersion);
  }

  reset() {
    this.state.set({
      completedSteps: new Set<number>(),
    });
    this.stepIndex.set(0);
  }
}
