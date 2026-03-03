import { Injectable, signal } from '@angular/core';

export interface ClusterWizardState {
  // Basic cluster information (step 1)
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

  // Plugins (step 3)
  preset?: string;
  plugins?: string[];

  // Track which steps are completed
  completedSteps: Set<number>;
}

@Injectable({
  providedIn: 'root',
})
export class ClusterWizardStateService {
  private state = signal<ClusterWizardState>({
    completedSteps: new Set<number>(),
  });

  getState() {
    return this.state();
  }

  updateBasicInfo(data: { clusterName?: string; region?: string; kubernetesVersion?: string }) {
    this.state.update((current) => ({
      ...current,
      ...data,
    }));
  }

  updateNodePools(nodePools: ClusterWizardState['nodePools']) {
    this.state.update((current) => ({
      ...current,
      nodePools,
    }));
  }

  updatePlugins(data: { preset?: string; plugins?: string[] }) {
    this.state.update((current) => ({
      ...current,
      preset: data.preset,
      plugins: data.plugins,
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
  }
}
