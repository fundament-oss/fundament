import { ProgressStep } from '../progress-stepper/progress-stepper.component';

export const ADD_CLUSTER_STEPS: ProgressStep[] = [
  { name: 'Basics', route: '/add-cluster' },
  { name: 'Worker nodes', route: '/add-cluster-nodes' },
  { name: 'Plugins', route: '/add-cluster-plugins' },
  { name: 'Summary', route: '/add-cluster-summary' },
];
