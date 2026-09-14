import { TestBed } from '@angular/core/testing';
import { SharedNodePoolsFormComponent } from './shared-node-pools-form.component';

// The same form serves two pages that mean different things by "no pools".
// The create wizard has nothing yet and wants one to fill in; the sheet that
// edits an existing cluster's pools is reporting what is there, and inventing a
// pool would contradict the page behind it and create one on save.
describe('shared node pools form', () => {
  const build = () => TestBed.createComponent(SharedNodePoolsFormComponent).componentInstance;

  it('starts the create wizard off with one pool to fill in', () => {
    expect(build().nodePools.length).toBe(1);
  });

  it('shows no pools when the cluster has none', () => {
    const form = build();
    form.initialData = [];
    expect(form.nodePools.length).toBe(0);
  });

  it('shows the pools the cluster has', () => {
    const form = build();
    form.initialData = [
      { name: 'workers', machineType: 'n1-standard-2', autoscaleMin: 2, autoscaleMax: 4 },
    ];
    expect(form.nodePools.length).toBe(1);
    expect(form.nodePools.at(0).get('name')?.value).toBe('workers');
  });
});
