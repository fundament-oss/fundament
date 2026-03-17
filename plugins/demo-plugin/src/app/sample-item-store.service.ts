import { Injectable, signal } from '@angular/core';

export interface SampleItem {
  name: string;
  namespace: string;
  replicas: number;
  image: string;
  status: 'Ready' | 'Pending' | 'Error';
}

const INITIAL_ITEMS: SampleItem[] = [
  { name: 'web-frontend', namespace: 'default', replicas: 3, image: 'nginx:1.25', status: 'Ready' },
  { name: 'api-service', namespace: 'default', replicas: 2, image: 'node:20-alpine', status: 'Ready' },
  { name: 'worker', namespace: 'jobs', replicas: 1, image: 'python:3.12-slim', status: 'Pending' },
  { name: 'legacy-app', namespace: 'legacy', replicas: 1, image: 'ubuntu:20.04', status: 'Error' },
];

/**
 * Shared in-memory store for SampleItem demo data.
 * Provided in root so all demo components share the same state.
 */
@Injectable({ providedIn: 'root' })
export class SampleItemStoreService {
  items = signal<SampleItem[]>(INITIAL_ITEMS);

  add(item: SampleItem): void {
    this.items.update((current) => [...current, item]);
  }
}
