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

// The quota is the organization's, over every cluster, so the form takes its
// bounds from it instead of a number of its own. The API stays the authority:
// these are what the form tells you before you save.
describe('shared node pools form under a node quota', () => {
  const machineTypeOptions = [
    { value: 'local-small', label: 'local-small' },
    { value: 'local-medium', label: 'local-medium' },
  ];
  const build = () => {
    const form = TestBed.createComponent(SharedNodePoolsFormComponent).componentInstance;
    form.machineTypeOptions = machineTypeOptions;
    return form;
  };
  const quotas = new Map([['local-medium', { quota: 10, inUse: 6 }]]);

  it('starts a new pool on a machine type that has room', () => {
    const form = build();
    form.nodeQuotas = quotas;
    expect(form.nodePools.at(0).get('machineType')?.value).toBe('local-medium');
  });

  it('says at the machine type when there is no quota for it', () => {
    const form = build();
    form.nodeQuotas = quotas;
    form.selectMachineType(0, 'local-small');
    expect(form.hasNoQuota('local-small')).toBe(true);
    expect(form.getMachineTypeQuotaError(0)).toContain('no quota for local-small');
    expect(form.nodePoolsForm.invalid).toBe(true);
  });

  it('shows the room under the maximum and refuses a value above it', () => {
    const form = build();
    form.nodeQuotas = quotas;
    expect(form.getAutoscaleMaxHint(0)).toContain('Up to 4 for this pool');

    form.nodePools.at(0).get('autoscaleMax')?.setValue(5);
    expect(form.nodePools.at(0).get('autoscaleMax')?.hasError('quota')).toBe(true);
    expect(form.getAutoscaleMaxError(0)).toContain('At most 4');
  });

  it('lets a second pool of the same type have only what the first leaves', () => {
    const form = build();
    form.nodeQuotas = quotas;
    form.addNodePool();
    form.selectMachineType(1, 'local-medium');
    form.nodePools.at(1).get('autoscaleMax')?.setValue(1);
    expect(form.nodePoolsForm.valid).toBe(true);

    form.nodePools.at(0).get('autoscaleMax')?.setValue(4);
    expect(form.nodePools.at(0).get('autoscaleMax')?.hasError('quota')).toBe(true);
  });

  it('lets an existing pool shrink under a quota the organization is over', () => {
    const form = build();
    form.initialData = [
      { name: 'workers', machineType: 'local-medium', autoscaleMin: 1, autoscaleMax: 8 },
    ];
    // Lowered to 4 since, while the others take 6: the pool's 8 no longer fits.
    form.nodeQuotas = new Map([['local-medium', { quota: 4, inUse: 14 }]]);
    form.nodePools.at(0).get('autoscaleMax')?.setValue(6);
    expect(form.nodePoolsForm.valid).toBe(true);
    expect(form.getAutoscaleMaxHint(0)).toContain('can shrink but not grow');

    form.nodePools.at(0).get('autoscaleMax')?.setValue(9);
    expect(form.nodePoolsForm.invalid).toBe(true);
  });

  it('bounds nothing while the quotas are not in', () => {
    const form = build();
    form.nodePools.at(0).get('autoscaleMax')?.setValue(500);
    expect(form.nodePoolsForm.valid).toBe(true);
    expect(form.getAutoscaleMaxHint(0)).toBe('');
  });
});
