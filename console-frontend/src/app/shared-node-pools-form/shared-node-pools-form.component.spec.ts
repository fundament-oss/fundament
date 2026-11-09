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

  it('shows the pools filled in earlier when the create wizard comes back to them', () => {
    const form = build();
    form.draftData = [
      { name: 'workers', machineType: 'n1-standard-2', autoscaleMin: 2, autoscaleMax: 4 },
      { name: 'batch', machineType: 'n1-standard-4', autoscaleMin: 1, autoscaleMax: 2 },
    ];
    expect(form.nodePoolsForm.value.nodePools).toEqual([
      { name: 'workers', machineType: 'n1-standard-2', autoscaleMin: 2, autoscaleMax: 4 },
      { name: 'batch', machineType: 'n1-standard-4', autoscaleMin: 1, autoscaleMax: 2 },
    ]);
  });

  it('keeps its one pool to fill in when there is no draft yet', () => {
    const form = build();
    form.draftData = null;
    expect(form.nodePools.length).toBe(1);
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

  it('counts a draft pool as new, and leaves its maximum as it was chosen', () => {
    const form = build();
    form.draftData = [
      { name: 'workers', machineType: 'local-medium', autoscaleMin: 1, autoscaleMax: 3 },
    ];
    form.nodeQuotas = new Map([['local-medium', { quota: 10, inUse: 8 }]]);
    // Not shrunk to the room as an untouched pool would be, and not counted as
    // nodes already in use: 3 is over the 2 left, so the form says so.
    expect(form.nodePools.at(0).get('autoscaleMax')?.value).toBe(3);
    expect(form.nodePools.at(0).get('autoscaleMax')?.hasError('quota')).toBe(true);
  });

  it('starts a new pool on a machine type that has room', () => {
    const form = build();
    form.nodeQuotas = quotas;
    expect(form.nodePools.at(0).get('machineType')?.value).toBe('local-medium');
  });

  it('starts a new pool on no more than the room its machine type has left', () => {
    const form = build();
    form.nodeQuotas = new Map([['local-medium', { quota: 10, inUse: 8 }]]);
    expect(form.nodePools.at(0).get('autoscaleMax')?.value).toBe(2);
    expect(form.nodePoolsForm.valid).toBe(true);

    form.nodePools.at(0).get('autoscaleMax')?.setValue(1);
    form.addNodePool();
    expect(form.nodePools.at(1).get('machineType')?.value).toBe('local-medium');
    expect(form.nodePools.at(1).get('autoscaleMax')?.value).toBe(1);
    expect(form.nodePoolsForm.valid).toBe(true);
  });

  it('leaves a maximum the user set when the quotas come in', () => {
    const form = build();
    form.setAutoscaleMax(0, 5);
    form.nodeQuotas = new Map([['local-medium', { quota: 10, inUse: 8 }]]);
    expect(form.nodePools.at(0).get('autoscaleMax')?.value).toBe(5);
  });

  it('starts a new pool off a first machine type that is full', () => {
    const form = build();
    form.nodeQuotas = new Map([
      ['local-small', { quota: 2, inUse: 2 }],
      ['local-medium', { quota: 10, inUse: 6 }],
    ]);
    expect(form.nodePools.at(0).get('machineType')?.value).toBe('local-medium');
    expect(form.machineTypeNote(0, 'local-small')).toBe('No room left in this organization');
    expect(form.machineTypeNote(0, 'local-medium')).toBeNull();
    expect(form.nodePoolsForm.valid).toBe(true);
  });

  it('says at the machine type when there is no quota for it', () => {
    const form = build();
    form.nodeQuotas = quotas;
    form.selectMachineType(0, 'local-small');
    expect(form.hasNoQuota('local-small')).toBe(true);
    expect(form.machineTypeNote(0, 'local-small')).toBe('No quota in this organization');
    expect(form.getMachineTypeQuotaError(0)).toContain('no quota for local-small');
    expect(form.nodePoolsForm.invalid).toBe(true);
  });

  it('refuses a maximum above the room', () => {
    const form = build();
    form.nodeQuotas = quotas;
    form.nodePools.at(0).get('autoscaleMax')?.setValue(4);
    expect(form.nodePoolsForm.valid).toBe(true);

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

    form.nodePools.at(0).get('autoscaleMax')?.setValue(9);
    expect(form.nodePoolsForm.invalid).toBe(true);
    expect(form.getAutoscaleMaxError(0)).toContain(
      'This pool can stay at 8 or shrink, but not grow',
    );
  });

  it('bounds nothing while the quotas are not in', () => {
    const form = build();
    form.nodePools.at(0).get('autoscaleMax')?.setValue(500);
    expect(form.nodePoolsForm.valid).toBe(true);
    expect(form.machineTypeNote(0, 'local-small')).toBeNull();
  });
});

// Updating a pool changes only its autoscale bounds, so the form does not offer
// a machine type it could not save: a pick would be dropped without a word.
describe('shared node pools form with existing pools', () => {
  const machineTypeOptions = [
    { value: 'local-small', label: 'local-small (2 lCPU, 4 GiB RAM)' },
    { value: 'local-medium', label: 'local-medium (4 lCPU, 8 GiB RAM)' },
  ];
  const build = () => {
    const form = TestBed.createComponent(SharedNodePoolsFormComponent).componentInstance;
    form.initialData = [
      { name: 'workers', machineType: 'local-medium', autoscaleMin: 1, autoscaleMax: 3 },
    ];
    return form;
  };

  it('keeps the machine type of an existing pool', () => {
    const form = build();
    form.machineTypeOptions = machineTypeOptions;
    form.selectMachineType(0, 'local-small');
    expect(form.isExistingPool(0)).toBe(true);
    expect(form.nodePools.at(0).get('machineType')?.value).toBe('local-medium');
    expect(form.machineTypeLabel('local-medium')).toBe('local-medium (4 lCPU, 8 GiB RAM)');
  });

  it('keeps a machine type the region does not offer', () => {
    const form = build();
    form.machineTypeOptions = [machineTypeOptions[0]];
    expect(form.nodePools.at(0).get('machineType')?.value).toBe('local-medium');
    expect(form.machineTypeLabel('local-medium')).toBe('local-medium');
  });

  it('lets a pool added next to it choose', () => {
    const form = build();
    form.machineTypeOptions = machineTypeOptions;
    form.addNodePool();
    form.selectMachineType(1, 'local-small');
    expect(form.isExistingPool(1)).toBe(false);
    expect(form.nodePools.at(1).get('machineType')?.value).toBe('local-small');
  });

  it('tells an existing pool without a quota that it can only shrink', () => {
    const form = build();
    form.machineTypeOptions = machineTypeOptions;
    form.nodeQuotas = new Map();
    expect(form.nodePoolsForm.valid).toBe(true);

    form.nodePools.at(0).get('autoscaleMax')?.setValue(4);
    expect(form.getMachineTypeQuotaError(0)).toContain('can keep or lower its maximum');
  });
});
